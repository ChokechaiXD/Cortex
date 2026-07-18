package cortex

import (
	"context"
	"path/filepath"
	"testing"
)

// TestCurateGlobalCandidateWithSourceApproves verifies the audit fix: a global
// candidate that has a source reference and enough independent supporters is
// auto-approved, instead of being permanently stuck in "protected scope".
func TestCurateGlobalCandidateWithSourceApproves(t *testing.T) {
	hub := openAuditTestHub(t)
	defer hub.Close()

	mem, err := hub.Remember(context.Background(), RememberCommand{
		AgentID:        "mika",
		Kind:           KindFact,
		Scope:          ScopeGlobal,
		MemoryKey:      "audit.test.global_src",
		Title:          "Global fact with source",
		Content:        "evidence-backed content",
		SourceRef:      "file://evidence.txt",
		IdempotencyKey: "seed-src",
	})
	if err != nil {
		t.Fatalf("remember: %v", err)
	}
	observeForTest(t, hub, mem.ID, "sora")
	observeForTest(t, hub, mem.ID, "nua")

	if err := setCuratorSettingsForTest(hub, CuratorSettings{
		Mode: CuratorAutomatic, RunEveryCandidates: 10, BatchLimit: 50, MinAgreement: 2,
	}); err != nil {
		t.Fatalf("set settings: %v", err)
	}

	report, err := hub.Curate(context.Background(), CurateCommand{ActorID: "mika", Trigger: "test", ApplySafe: true})
	if err != nil {
		t.Fatalf("curate: %v", err)
	}
	if report.Applied != 1 {
		t.Fatalf("expected 1 applied, got %d (ready=%d waiting=%d protected=%d)", report.Applied, report.Ready, report.Waiting, report.Protected)
	}
	reloaded := mustGetMemory(t, hub, mem.ID)
	if reloaded.Lifecycle != LifecycleActive {
		t.Fatalf("expected active, got %s", reloaded.Lifecycle)
	}
}

// TestCurateGlobalCandidateWithoutSourceWaits confirms we did NOT weaken
// safety: a global candidate with NO source is still held (waiting), not
// auto-approved.
func TestCurateGlobalCandidateWithoutSourceWaits(t *testing.T) {
	hub := openAuditTestHub(t)
	defer hub.Close()

	mem, err := hub.Remember(context.Background(), RememberCommand{
		AgentID:        "mika",
		Kind:           KindFact,
		Scope:          ScopeGlobal,
		MemoryKey:      "audit.test.global_nosrc",
		Title:          "Global fact no source",
		Content:        "content",
		IdempotencyKey: "seed-nosrc",
	})
	if err != nil {
		t.Fatalf("remember: %v", err)
	}
	observeForTest(t, hub, mem.ID, "sora")
	observeForTest(t, hub, mem.ID, "nua")

	if err := setCuratorSettingsForTest(hub, CuratorSettings{
		Mode: CuratorAutomatic, RunEveryCandidates: 10, BatchLimit: 50, MinAgreement: 2,
	}); err != nil {
		t.Fatalf("set settings: %v", err)
	}
	report, err := hub.Curate(context.Background(), CurateCommand{ActorID: "mika", Trigger: "test", ApplySafe: true})
	if err != nil {
		t.Fatalf("curate: %v", err)
	}
	if report.Applied != 0 {
		t.Fatalf("expected 0 applied for sourceless candidate, got %d", report.Applied)
	}
	if report.Waiting < 1 {
		t.Fatalf("expected the sourceless candidate to wait, got ready=%d waiting=%d protected=%d", report.Ready, report.Waiting, report.Protected)
	}
}

func openAuditTestHub(t *testing.T) *Hub {
	t.Helper()
	hub, err := Open(Config{
		DatabasePath: filepath.Join(t.TempDir(), "cortex.db"),
		AdminAgents:  []string{"mika"},
	})
	if err != nil {
		t.Fatalf("open hub: %v", err)
	}
	return hub
}

func observeForTest(t *testing.T, hub *Hub, memoryID, actor string) {
	t.Helper()
	if _, err := hub.db.ExecContext(context.Background(),
		`INSERT INTO memory_events(id, memory_id, event_type, actor_id, metadata_json, created_at)
		 VALUES (?, ?, 'observed', ?, '{"revision":1}', strftime('%Y-%m-%dT%H:%M:%fZ','now'))`,
		"evt_"+actor+"_"+memoryID, memoryID, actor); err != nil {
		t.Fatalf("observe event: %v", err)
	}
}

func setCuratorSettingsForTest(hub *Hub, s CuratorSettings) error {
	_, err := hub.db.ExecContext(context.Background(), `
UPDATE curator_settings SET mode=?, run_every_candidates=?, batch_limit=?, min_agreement=?, updated_by='test', updated_at=strftime('%Y-%m-%dT%H:%M:%fZ','now')
WHERE id=1`, s.Mode, s.RunEveryCandidates, s.BatchLimit, s.MinAgreement)
	return err
}

func mustGetMemory(t *testing.T, hub *Hub, id string) Memory {
	t.Helper()
	mem, err := getMemory(context.Background(), hub.db, id)
	if err != nil {
		t.Fatalf("get memory: %v", err)
	}
	return mem
}
