package httpapi

import (
	"net/http"
	"path/filepath"
	"testing"

	"cortex.local/cortex/internal/cortex"
	"cortex.local/cortex/internal/hope"
)

func TestContextPackRoutesAndLearnsSkillOutcome(t *testing.T) {
	t.Parallel()
	directory := t.TempDir()
	memoryHub, err := cortex.Open(cortex.Config{DatabasePath: filepath.Join(directory, "cortex.db")})
	if err != nil {
		t.Fatalf("open Cortex: %v", err)
	}
	t.Cleanup(func() { _ = memoryHub.Close() })
	hopeHub, err := hope.Open(filepath.Join(directory, "hope.db"), "")
	if err != nil {
		t.Fatalf("open HOPE: %v", err)
	}
	t.Cleanup(func() { _ = hopeHub.Close() })
	if err := hopeHub.SaveSkill(t.Context(), hope.Skill{
		ID: "api-design", Name: "API design", Description: "Design stable API contracts",
		Keywords: []string{"api", "contract"}, Role: "sora", Project: "cortex", Enabled: true,
	}); err != nil {
		t.Fatalf("save skill: %v", err)
	}
	handler := NewWithSkillMem(memoryHub, StaticAuthenticator{"sora-token": "sora", "nua-token": "nua"}, nil, nil, nil, hopeHub)
	response := performRequest(t, handler, http.MethodPost, "/v1/context-packs", "sora-token", "pack-1", map[string]any{
		"text": "design cortex api contract", "project": "cortex", "skill_limit": 3,
	})
	if response.Code != http.StatusOK {
		t.Fatalf("context pack status=%d body=%s", response.Code, response.Body.String())
	}
	var pack contextPackResponse
	decodeResponse(t, response, &pack)
	if pack.ID == "" || len(pack.Skills) != 1 || pack.Skills[0].ID != "api-design" || pack.Routing.Strategy != "deterministic" {
		t.Fatalf("context pack=%#v", pack)
	}
	feedbackPath := "/v1/context-packs/" + pack.ID + "/skills/api-design/feedback"
	for range 2 {
		feedback := performRequest(t, handler, http.MethodPost, feedbackPath, "sora-token", "feedback-1", map[string]any{"outcome": "success"})
		if feedback.Code != http.StatusOK {
			t.Fatalf("feedback status=%d body=%s", feedback.Code, feedback.Body.String())
		}
	}
	foreign := performRequest(t, handler, http.MethodPost, feedbackPath, "nua-token", "feedback-2", map[string]any{"outcome": "used"})
	if foreign.Code != http.StatusBadRequest {
		t.Fatalf("foreign feedback status=%d body=%s", foreign.Code, foreign.Body.String())
	}
	skill, err := hopeHub.Skill(t.Context(), "api-design")
	if err != nil {
		t.Fatalf("load skill: %v", err)
	}
	if skill.UseCount != 1 || skill.SuccessCount != 1 || skill.FailureCount != 0 {
		t.Fatalf("skill counts=%#v", skill)
	}
}
