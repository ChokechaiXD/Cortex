"""Dependency-free HTTP client for the Cortex v1 protocol."""

from __future__ import annotations

import json
import os
import tempfile
import uuid
from pathlib import Path
from typing import Any
from urllib.error import HTTPError, URLError
from urllib.parse import urlparse
from urllib.request import Request, urlopen


class CortexError(RuntimeError):
    pass


class CortexClient:
    def __init__(self, base_url: str, token: str, timeout: float = 2.0):
        self.base_url = base_url.rstrip("/")
        self.token = token.strip()
        self.timeout = timeout
        parsed = urlparse(self.base_url)
        if parsed.scheme not in {"http", "https"} or not parsed.netloc:
            raise ValueError("Cortex URL must be an absolute http(s) URL")
        if not self.token:
            raise ValueError("Cortex token is required")

    def remember(self, payload: dict[str, Any], idempotency_key: str = "") -> dict[str, Any]:
        return self._request("POST", "/v1/memories", payload, idempotency_key or _request_id("remember"))

    def recall(self, payload: dict[str, Any], idempotency_key: str = "") -> dict[str, Any]:
        return self._request("POST", "/v1/recalls", payload, idempotency_key or _request_id("recall"))

    def context_pack(self, payload: dict[str, Any], idempotency_key: str = "") -> dict[str, Any]:
        return self._request(
            "POST",
            "/v1/context-packs",
            payload,
            idempotency_key or _request_id("context-pack"),
        )

    def feedback(self, memory_id: str, payload: dict[str, Any], idempotency_key: str = "") -> dict[str, Any]:
        return self._request(
            "POST",
            f"/v1/memories/{memory_id}/feedback",
            payload,
            idempotency_key or _request_id("feedback"),
        )

    def skill_feedback(
        self,
        context_pack_id: str,
        skill_id: str,
        payload: dict[str, Any],
        idempotency_key: str = "",
    ) -> dict[str, Any]:
        return self._request(
            "POST",
            f"/v1/context-packs/{context_pack_id}/skills/{skill_id}/feedback",
            payload,
            idempotency_key or _request_id("skill-feedback"),
        )

    def review(self, memory_id: str, payload: dict[str, Any], idempotency_key: str = "") -> dict[str, Any]:
        return self._request(
            "POST",
            f"/v1/memories/{memory_id}/review",
            payload,
            idempotency_key or _request_id("review"),
        )

    def history(self, memory_id: str) -> dict[str, Any]:
        return self._request("GET", f"/v1/memories/{memory_id}/history")

    def health(self) -> dict[str, Any]:
        return self._request("GET", "/v1/health", authenticated=False)

    def _request(
        self,
        method: str,
        path: str,
        payload: dict[str, Any] | None = None,
        idempotency_key: str = "",
        authenticated: bool = True,
    ) -> dict[str, Any]:
        data = None if payload is None else json.dumps(payload).encode("utf-8")
        headers = {"Accept": "application/json"}
        if payload is not None:
            headers["Content-Type"] = "application/json"
        if authenticated:
            headers["Authorization"] = f"Bearer {self.token}"
        if idempotency_key:
            headers["Idempotency-Key"] = idempotency_key
        request = Request(self.base_url + path, data=data, headers=headers, method=method)
        try:
            with urlopen(request, timeout=self.timeout) as response:
                raw = response.read()
        except HTTPError as exc:
            raw = exc.read()
            try:
                detail = json.loads(raw).get("error", {}).get("message", "")
            except (ValueError, AttributeError):
                detail = ""
            raise CortexError(f"Cortex returned HTTP {exc.code}: {detail or exc.reason}") from exc
        except URLError as exc:
            raise CortexError(f"Cortex is unavailable: {exc.reason}") from exc
        try:
            decoded = json.loads(raw or b"{}")
        except ValueError as exc:
            raise CortexError("Cortex returned invalid JSON") from exc
        if not isinstance(decoded, dict):
            raise CortexError("Cortex returned a non-object response")
        return decoded


def _request_id(operation: str) -> str:
    return f"hermes/{operation}/{uuid.uuid4().hex}"


def write_private_json(path: Path, value: dict[str, Any]) -> None:
    """Atomically replace a connector config without leaving a partial token file."""
    path.parent.mkdir(parents=True, exist_ok=True)
    descriptor, temporary_name = tempfile.mkstemp(
        prefix=f".{path.name}.", suffix=".tmp", dir=path.parent
    )
    temporary_path = Path(temporary_name)
    try:
        os.chmod(temporary_path, 0o600)
        handle = os.fdopen(descriptor, "w", encoding="utf-8", newline="\n")
        descriptor = -1
        with handle:
            json.dump(value, handle, indent=2, ensure_ascii=False)
            handle.write("\n")
            handle.flush()
            os.fsync(handle.fileno())
        os.replace(temporary_path, path)
        os.chmod(path, 0o600)
    except Exception:
        if descriptor >= 0:
            os.close(descriptor)
        temporary_path.unlink(missing_ok=True)
        raise
