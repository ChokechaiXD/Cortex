package httpapi

import (
	"context"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"cortex.local/cortex/internal/controlplane"
	"cortex.local/cortex/internal/cortex"
	"cortex.local/cortex/internal/hermesruntime"
	"cortex.local/cortex/internal/hope"
	"cortex.local/cortex/internal/integrationhub"
)

type passwordlessAuthenticator struct{}

func (passwordlessAuthenticator) Authenticate(string) (string, bool) { return "", false }
func (passwordlessAuthenticator) DashboardAccess() (string, bool)    { return "mika", true }

type fakeHermesRuntime struct{}

func (fakeHermesRuntime) Gateways(context.Context) (map[string]hermesruntime.GatewayStatus, error) {
	return map[string]hermesruntime.GatewayStatus{}, nil
}

func (fakeHermesRuntime) CreateProfile(context.Context, string) error { return nil }

func TestHOPEDashboardCreatesPasswordlessLoopbackSession(t *testing.T) {
	t.Parallel()
	directory := t.TempDir()
	memoryHub, err := cortex.Open(cortex.Config{DatabasePath: filepath.Join(directory, "cortex.db"), AdminAgents: []string{"mika"}})
	if err != nil {
		t.Fatalf("open Cortex: %v", err)
	}
	t.Cleanup(func() { _ = memoryHub.Close() })
	hopeHub, err := hope.Open(filepath.Join(directory, "hope.db"), "")
	if err != nil {
		t.Fatalf("open HOPE: %v", err)
	}
	t.Cleanup(func() { _ = hopeHub.Close() })
	plane := controlplane.New(hopeHub, integrationhub.New(hopeHub), nil, nil, nil, nil, fakeHermesRuntime{})
	handler := NewWithHOPE(memoryHub, passwordlessAuthenticator{}, nil, nil, nil, plane)

	firstRequest := httptest.NewRequest(http.MethodGet, "/", nil)
	firstRequest.RemoteAddr = "127.0.0.1:42000"
	first := httptest.NewRecorder()
	handler.ServeHTTP(first, firstRequest)
	if first.Code != http.StatusSeeOther || first.Header().Get("Location") != "/" {
		t.Fatalf("first response=%d location=%q body=%s", first.Code, first.Header().Get("Location"), first.Body.String())
	}
	cookies := first.Result().Cookies()
	if len(cookies) != 1 {
		t.Fatalf("cookies=%#v", cookies)
	}
	secondRequest := httptest.NewRequest(http.MethodGet, "/", nil)
	secondRequest.RemoteAddr = "127.0.0.1:42000"
	secondRequest.AddCookie(cookies[0])
	second := httptest.NewRecorder()
	handler.ServeHTTP(second, secondRequest)
	if second.Code != http.StatusOK {
		t.Fatalf("dashboard status=%d body=%s", second.Code, second.Body.String())
	}
	for _, expected := range []string{"HOPE", "Mission Control", "READY STATE", "P Choke", "Deputy", "Daily", "Review queue"} {
		if !strings.Contains(second.Body.String(), expected) {
			t.Fatalf("dashboard missing %q: %s", expected, second.Body.String())
		}
	}
}
