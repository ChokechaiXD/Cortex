package workmodes

import (
	"context"
	"path/filepath"
	"sync"
	"testing"

	"cortex.local/cortex/internal/hope"
	"cortex.local/cortex/internal/integrationhub"
)

type recordingAdapter struct {
	id    string
	mu    sync.Mutex
	calls []integrationhub.ActionRequest
}

func (adapter *recordingAdapter) ID() string { return adapter.id }

func (adapter *recordingAdapter) Probe(context.Context, string) integrationhub.Status {
	return integrationhub.Status{ID: adapter.id, Name: adapter.id, State: integrationhub.StateStopped}
}

func (adapter *recordingAdapter) Execute(_ context.Context, request integrationhub.ActionRequest) integrationhub.ActionResult {
	adapter.mu.Lock()
	adapter.calls = append(adapter.calls, request)
	adapter.mu.Unlock()
	return integrationhub.ActionResult{
		Integration: adapter.id, Action: request.Action, Target: request.Target,
		Status: integrationhub.Status{ID: adapter.id, State: integrationhub.StateRunning}, Message: "ok",
	}
}

func TestWorkModeComposesAdaptersAndTelegramReferences(t *testing.T) {
	t.Parallel()
	hub, err := hope.Open(filepath.Join(t.TempDir(), "hope.db"), "")
	if err != nil {
		t.Fatalf("open HOPE: %v", err)
	}
	t.Cleanup(func() { _ = hub.Close() })
	ctx := context.Background()
	if err := hub.SaveAgent(ctx, hope.Agent{ID: "sora", Name: "Sora", Role: "Coding", Profile: "sora", TelegramURL: "https://t.me/sora", Enabled: true}); err != nil {
		t.Fatalf("save agent: %v", err)
	}
	if err := hub.SaveWorkMode(ctx, hope.WorkMode{
		ID: "focus", Name: "Focus", Integrations: []string{"9router"}, Agents: []string{"sora"}, OpenTelegram: true,
	}); err != nil {
		t.Fatalf("save work mode: %v", err)
	}
	router, hermes := &recordingAdapter{id: "9router"}, &recordingAdapter{id: "hermes"}
	manager := New(hub, integrationhub.New(hub, router, hermes))
	started, err := manager.Execute(ctx, "focus", "start")
	if err != nil {
		t.Fatalf("start work mode: %v", err)
	}
	if len(started.Steps) != 2 || len(started.OpenURLs) != 1 || started.OpenURLs[0] != "https://t.me/sora" {
		t.Fatalf("start result=%#v", started)
	}
	if _, err := manager.Execute(ctx, "focus", "stop"); err != nil {
		t.Fatalf("stop work mode: %v", err)
	}
	if len(router.calls) != 2 || router.calls[0].Action != "start" || router.calls[1].Action != "stop" {
		t.Fatalf("router calls=%#v", router.calls)
	}
	if len(hermes.calls) != 2 || hermes.calls[0].Target != "sora" || hermes.calls[1].Action != "stop" {
		t.Fatalf("Hermes calls=%#v", hermes.calls)
	}
}
