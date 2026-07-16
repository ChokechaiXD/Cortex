from __future__ import annotations

import json
import sys
import types
import unittest
from pathlib import Path


agent_module = types.ModuleType("agent")
memory_provider_module = types.ModuleType("agent.memory_provider")
memory_provider_module.MemoryProvider = object
tools_module = types.ModuleType("tools")
registry_module = types.ModuleType("tools.registry")
registry_module.tool_error = lambda message: json.dumps({"error": message})
sys.modules.setdefault("agent", agent_module)
sys.modules.setdefault("agent.memory_provider", memory_provider_module)
sys.modules.setdefault("tools", tools_module)
sys.modules.setdefault("tools.registry", registry_module)
sys.path.insert(0, str(Path(__file__).parents[1]))

from provider import CortexMemoryProvider, RECALL_SCHEMA  # noqa: E402


class FakeClient:
    def __init__(self):
        self.payloads: list[dict] = []

    def recall(self, payload: dict) -> dict:
        self.payloads.append(payload)
        return {
            "id": "rec_1",
            "items": [
                {
                    "memory": {
                        "id": "mem_1",
                        "kind": "fact",
                        "content": "Use the canonical output.",
                        "truth_score": 0.9,
                        "utility_score": 0.8,
                    }
                }
            ],
            "token_budget": payload["token_budget"],
            "estimated_tokens": 140,
            "truncated": True,
        }


class CortexProviderBudgetTest(unittest.TestCase):
    def test_prefetch_and_tool_recall_send_bounded_token_budgets(self):
        provider = CortexMemoryProvider(
            {"prefetch_token_budget": 700, "recall_token_budget": 1200}
        )
        client = FakeClient()
        provider._client = client

        rendered = provider.prefetch("canonical output")
        self.assertEqual(client.payloads[0]["token_budget"], 700)
        self.assertIn("140/700 tokens", rendered)
        self.assertIn("trimmed", rendered)

        response = json.loads(
            provider.handle_tool_call(
                "cortex_recall", {"query": "canonical output", "token_budget": 240}
            )
        )
        self.assertEqual(client.payloads[1]["token_budget"], 240)
        self.assertEqual(response["token_budget"], 240)

    def test_recall_schema_exposes_safe_budget_range(self):
        properties = RECALL_SCHEMA["parameters"]["properties"]
        self.assertEqual(properties["token_budget"]["minimum"], 100)
        self.assertEqual(properties["token_budget"]["maximum"], 4000)


if __name__ == "__main__":
    unittest.main()
