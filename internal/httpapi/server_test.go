package httpapi

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"

	"cortex.local/cortex/internal/cortex"
)

func TestHTTPMemoryLifecycle(t *testing.T) {
	t.Parallel()

	hub, err := cortex.Open(cortex.Config{
		DatabasePath: filepath.Join(t.TempDir(), "cortex.db"),
		AdminAgents:  []string{"mika"},
	})
	if err != nil {
		t.Fatalf("open Cortex: %v", err)
	}
	t.Cleanup(func() { _ = hub.Close() })

	handler := New(hub, StaticAuthenticator{
		"mika-token": "mika",
		"sola-token": "sola",
		"nua-token":  "nua",
	})

	health := performRequest(t, handler, http.MethodGet, "/v1/health", "", "", nil)
	if health.Code != http.StatusOK {
		t.Fatalf("health status = %d, want 200: %s", health.Code, health.Body.String())
	}

	unauthorized := performRequest(t, handler, http.MethodPost, "/v1/memories", "", "write-unauthorized", map[string]any{})
	if unauthorized.Code != http.StatusUnauthorized {
		t.Fatalf("unauthorized status = %d, want 401", unauthorized.Code)
	}

	rememberBody := map[string]any{
		"kind":       "failed_attempt",
		"scope":      "project",
		"scope_key":  "novelclaw",
		"memory_key": "translation.force-overwrite",
		"title":      "Force translation can overwrite output",
		"content":    "Use a backup before force translation.",
		"tags":       []string{"translation", "backup"},
		"agent_id":   "mika",
	}
	remembered := performRequest(t, handler, http.MethodPost, "/v1/memories", "sola-token", "sola/write-1", rememberBody)
	if remembered.Code != http.StatusCreated {
		t.Fatalf("remember status = %d, want 201: %s", remembered.Code, remembered.Body.String())
	}
	var memory cortex.Memory
	decodeResponse(t, remembered, &memory)
	if memory.CreatedBy != "sola" {
		t.Fatalf("created_by = %q, want authenticated agent sola", memory.CreatedBy)
	}

	replayed := performRequest(t, handler, http.MethodPost, "/v1/memories", "sola-token", "sola/write-1", rememberBody)
	if replayed.Code != http.StatusCreated {
		t.Fatalf("replayed remember status = %d, want 201: %s", replayed.Code, replayed.Body.String())
	}
	var replayedMemory cortex.Memory
	decodeResponse(t, replayed, &replayedMemory)
	if replayedMemory.ID != memory.ID {
		t.Fatalf("replayed memory id = %q, want %q", replayedMemory.ID, memory.ID)
	}

	reviewPath := "/v1/memories/" + memory.ID + "/review"
	forbidden := performRequest(t, handler, http.MethodPost, reviewPath, "sola-token", "sola/review-1", map[string]any{
		"decision": "approve",
	})
	if forbidden.Code != http.StatusForbidden {
		t.Fatalf("non-admin review status = %d, want 403: %s", forbidden.Code, forbidden.Body.String())
	}
	approved := performRequest(t, handler, http.MethodPost, reviewPath, "mika-token", "mika/review-1", map[string]any{
		"decision": "approve",
		"reason":   "Verified from the session evidence.",
	})
	if approved.Code != http.StatusOK {
		t.Fatalf("admin review status = %d, want 200: %s", approved.Code, approved.Body.String())
	}

	recalled := performRequest(t, handler, http.MethodPost, "/v1/recalls", "nua-token", "nua/recall-1", map[string]any{
		"text":    "force translation backup",
		"project": "novelclaw",
		"limit":   5,
	})
	if recalled.Code != http.StatusOK {
		t.Fatalf("recall status = %d, want 200: %s", recalled.Code, recalled.Body.String())
	}
	var recall cortex.RecallResult
	decodeResponse(t, recalled, &recall)
	if len(recall.Items) != 1 || recall.Items[0].Memory.ID != memory.ID {
		t.Fatalf("recall items = %#v, want memory %q", recall.Items, memory.ID)
	}

	history := performRequest(t, handler, http.MethodGet, "/v1/memories/"+memory.ID+"/history", "nua-token", "", nil)
	if history.Code != http.StatusOK {
		t.Fatalf("history status = %d, want 200: %s", history.Code, history.Body.String())
	}
}

func performRequest(t *testing.T, handler http.Handler, method, path, token, idempotencyKey string, body any) *httptest.ResponseRecorder {
	t.Helper()
	var encoded []byte
	if body != nil {
		var err error
		encoded, err = json.Marshal(body)
		if err != nil {
			t.Fatalf("encode request: %v", err)
		}
	}
	req := httptest.NewRequest(method, path, bytes.NewReader(encoded))
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	if idempotencyKey != "" {
		req.Header.Set("Idempotency-Key", idempotencyKey)
	}
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, req)
	return recorder
}

func decodeResponse(t *testing.T, recorder *httptest.ResponseRecorder, target any) {
	t.Helper()
	if err := json.Unmarshal(recorder.Body.Bytes(), target); err != nil {
		t.Fatalf("decode response: %v; body=%s", err, recorder.Body.String())
	}
}
