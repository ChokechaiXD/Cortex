package httpapi

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"path/filepath"
	"strings"
	"testing"

	"cortex.local/cortex/internal/cortex"
)

func TestDashboardLoginAndReview(t *testing.T) {
	t.Parallel()

	hub, err := cortex.Open(cortex.Config{
		DatabasePath: filepath.Join(t.TempDir(), "cortex.db"),
		AdminAgents:  []string{"mika"},
	})
	if err != nil {
		t.Fatalf("open Cortex: %v", err)
	}
	t.Cleanup(func() { _ = hub.Close() })
	memory, err := hub.Remember(context.Background(), cortex.RememberCommand{
		IdempotencyKey: "dashboard/candidate-1",
		Kind:           cortex.KindDecision,
		Scope:          cortex.ScopeProject,
		ScopeKey:       "novelclaw",
		MemoryKey:      "novelclaw.output-format",
		Title:          "Canonical output uses .th.json",
		Content:        "Translation output must use canonical .th.json files.",
		AgentID:        "sola",
	})
	if err != nil {
		t.Fatalf("create dashboard candidate: %v", err)
	}
	handler := New(hub, StaticAuthenticator{"mika-token": "mika"})

	loginPage := httptest.NewRecorder()
	handler.ServeHTTP(loginPage, httptest.NewRequest(http.MethodGet, "/", nil))
	if loginPage.Code != http.StatusOK || !strings.Contains(loginPage.Body.String(), "Sign in") {
		t.Fatalf("login page status=%d body=%s", loginPage.Code, loginPage.Body.String())
	}

	form := url.Values{"token": {"mika-token"}}
	loginRequest := httptest.NewRequest(http.MethodPost, "/login", strings.NewReader(form.Encode()))
	loginRequest.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	login := httptest.NewRecorder()
	handler.ServeHTTP(login, loginRequest)
	if login.Code != http.StatusSeeOther {
		t.Fatalf("login status=%d body=%s", login.Code, login.Body.String())
	}
	response := login.Result()
	cookies := response.Cookies()
	_ = response.Body.Close()
	if len(cookies) != 1 || !cookies[0].HttpOnly || cookies[0].SameSite != http.SameSiteStrictMode {
		t.Fatalf("login cookies = %#v", cookies)
	}

	dashboardRequest := httptest.NewRequest(http.MethodGet, "/", nil)
	dashboardRequest.AddCookie(cookies[0])
	dashboard := httptest.NewRecorder()
	handler.ServeHTTP(dashboard, dashboardRequest)
	if dashboard.Code != http.StatusOK || !strings.Contains(dashboard.Body.String(), memory.Title) {
		t.Fatalf("dashboard status=%d body=%s", dashboard.Code, dashboard.Body.String())
	}

	reviewForm := url.Values{"decision": {"approve"}, "reason": {"Reviewed in dashboard"}}
	reviewRequest := httptest.NewRequest(
		http.MethodPost,
		"/ui/memories/"+memory.ID+"/review",
		strings.NewReader(reviewForm.Encode()),
	)
	reviewRequest.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	reviewRequest.AddCookie(cookies[0])
	review := httptest.NewRecorder()
	handler.ServeHTTP(review, reviewRequest)
	if review.Code != http.StatusSeeOther {
		body, _ := io.ReadAll(review.Result().Body)
		t.Fatalf("dashboard review status=%d body=%s", review.Code, body)
	}

	recalled, err := hub.Recall(context.Background(), cortex.RecallQuery{
		AgentID: "nua", Text: "canonical output", Project: "novelclaw", Limit: 5,
	})
	if err != nil {
		t.Fatalf("recall approved dashboard memory: %v", err)
	}
	if len(recalled.Items) != 1 || recalled.Items[0].Memory.Lifecycle != cortex.LifecycleActive {
		t.Fatalf("dashboard review did not approve memory: %#v", recalled.Items)
	}
}
