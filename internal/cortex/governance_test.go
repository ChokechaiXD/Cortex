package cortex

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"testing"
)

func TestStableMemoryKeyAppendsRevision(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	hub, err := Open(Config{DatabasePath: filepath.Join(t.TempDir(), "cortex.db")})
	if err != nil {
		t.Fatalf("open Cortex: %v", err)
	}
	t.Cleanup(func() { _ = hub.Close() })
	first, err := hub.Remember(ctx, RememberCommand{
		IdempotencyKey: "sola/decision-v1", Kind: KindDecision, Scope: ScopeProject,
		ScopeKey: "novelclaw", MemoryKey: "output.canonical", Title: "Canonical output",
		Content: "Use .translated.json files.", AgentID: "sola",
	})
	if err != nil {
		t.Fatalf("remember first revision: %v", err)
	}
	if _, err := hub.Review(ctx, ReviewCommand{
		IdempotencyKey: "mika/approve-v1", MemoryID: first.ID, ActorID: "mika", Decision: ReviewApprove,
	}); err != nil {
		t.Fatalf("approve first revision: %v", err)
	}
	revised, err := hub.Remember(ctx, RememberCommand{
		IdempotencyKey: "nua/decision-v2", Kind: KindDecision, Scope: ScopeProject,
		ScopeKey: "novelclaw", MemoryKey: "output.canonical", Title: "Canonical output",
		Content: "Use .th.json files.", AgentID: "nua", SourceRef: "project-spec.md",
	})
	if err != nil {
		t.Fatalf("remember revised content: %v", err)
	}
	if revised.ID != first.ID || revised.Revision != 2 || revised.Lifecycle != LifecycleCandidate {
		t.Fatalf("revised memory = %#v, want same id revision 2 candidate", revised)
	}
	if revised.Content != "Use .th.json files." || revised.TruthScore != 0.5 || revised.UtilityScore != 0.5 {
		t.Fatalf("revised content/scores = %#v", revised)
	}
	overview, err := hub.Overview(ctx, "mika", 10)
	if err != nil {
		t.Fatalf("overview: %v", err)
	}
	if len(overview.Memories) != 1 {
		t.Fatalf("stable key created %d memories, want 1", len(overview.Memories))
	}
	history, err := hub.History(ctx, HistoryQuery{MemoryID: first.ID, AgentID: "mika"})
	if err != nil {
		t.Fatalf("history: %v", err)
	}
	if got, want := eventTypes(history), []EventType{EventCreated, EventApproved, EventRevised}; !equalEventTypes(got, want) {
		t.Fatalf("history types = %v, want %v", got, want)
	}
}

func TestFeedbackCannotModifyAnotherAgentsPrivateMemory(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	hub, err := Open(Config{DatabasePath: filepath.Join(t.TempDir(), "cortex.db")})
	if err != nil {
		t.Fatalf("open Cortex: %v", err)
	}
	t.Cleanup(func() { _ = hub.Close() })
	memory, err := hub.Remember(ctx, RememberCommand{
		IdempotencyKey: "sola/private-1", Kind: KindFact, Scope: ScopePrivate,
		MemoryKey: "workflow.private", Title: "Private workflow",
		Content: "Sola private workflow", AgentID: "sola",
	})
	if err != nil {
		t.Fatalf("remember private memory: %v", err)
	}
	if memory.ScopeKey != "sola" {
		t.Fatalf("private scope key = %q, want owner sola", memory.ScopeKey)
	}
	_, err = hub.Feedback(ctx, FeedbackCommand{
		IdempotencyKey: "nua/poison-1", MemoryID: memory.ID, AgentID: "nua", Outcome: FeedbackContradicted,
	})
	if !errors.Is(err, ErrForbidden) {
		t.Fatalf("cross-agent private feedback error = %v, want forbidden", err)
	}
}

func TestRecallFiltersVisibilityBeforeLimit(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	hub, err := Open(Config{DatabasePath: filepath.Join(t.TempDir(), "cortex.db")})
	if err != nil {
		t.Fatalf("open Cortex: %v", err)
	}
	t.Cleanup(func() { _ = hub.Close() })
	for index := 0; index < 105; index++ {
		_, err := hub.Remember(ctx, RememberCommand{
			IdempotencyKey: fmt.Sprintf("sola/hidden-%d", index), Kind: KindFact, Scope: ScopePrivate,
			ScopeKey: "sola", MemoryKey: fmt.Sprintf("hidden.%d", index), Title: "Needle hidden",
			Content: "needle shared lookup", AgentID: "sola",
		})
		if err != nil {
			t.Fatalf("remember hidden candidate %d: %v", index, err)
		}
	}
	visible, err := hub.Remember(ctx, RememberCommand{
		IdempotencyKey: "nua/visible", Kind: KindFact, Scope: ScopeGlobal,
		MemoryKey: "visible.needle", Title: "Needle visible", Content: "needle shared lookup", AgentID: "nua",
	})
	if err != nil {
		t.Fatalf("remember visible memory: %v", err)
	}
	if _, err := hub.Review(ctx, ReviewCommand{
		IdempotencyKey: "mika/approve-visible", MemoryID: visible.ID, ActorID: "mika", Decision: ReviewApprove,
	}); err != nil {
		t.Fatalf("approve visible memory: %v", err)
	}
	result, err := hub.Recall(ctx, RecallQuery{AgentID: "nua", Text: "needle shared lookup", Limit: 5})
	if err != nil {
		t.Fatalf("recall visible memory: %v", err)
	}
	if len(result.Items) != 1 || result.Items[0].Memory.ID != visible.ID {
		t.Fatalf("visible memory starved by hidden candidates: %#v", result.Items)
	}
}

func TestUnscopedRecallReturnsOnlyGlobalMemory(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	hub, err := Open(Config{DatabasePath: filepath.Join(t.TempDir(), "cortex.db")})
	if err != nil {
		t.Fatalf("open Cortex: %v", err)
	}
	t.Cleanup(func() { _ = hub.Close() })
	projectMemory, err := hub.Remember(ctx, RememberCommand{
		IdempotencyKey: "sola/project-scope", Kind: KindFact, Scope: ScopeProject,
		ScopeKey: "novelclaw", MemoryKey: "project.scope", Title: "Scoped needle",
		Content: "scope isolation needle", AgentID: "sola",
	})
	if err != nil {
		t.Fatalf("remember project memory: %v", err)
	}
	if _, err := hub.Review(ctx, ReviewCommand{
		IdempotencyKey: "mika/project-scope", MemoryID: projectMemory.ID, ActorID: "mika", Decision: ReviewApprove,
	}); err != nil {
		t.Fatalf("approve project memory: %v", err)
	}
	unscoped, err := hub.Recall(ctx, RecallQuery{AgentID: "nua", Text: "scope isolation needle", Limit: 5})
	if err != nil {
		t.Fatalf("unscoped recall: %v", err)
	}
	if len(unscoped.Items) != 0 {
		t.Fatalf("unscoped recall leaked project memory: %#v", unscoped.Items)
	}
	scoped, err := hub.Recall(ctx, RecallQuery{
		AgentID: "nua", Text: "scope isolation needle", Project: "novelclaw", Limit: 5,
	})
	if err != nil || len(scoped.Items) != 1 {
		t.Fatalf("scoped recall = %#v, err=%v", scoped.Items, err)
	}
}

func TestGovernorCanSupersedeWithoutDeletingHistory(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	hub, err := Open(Config{DatabasePath: filepath.Join(t.TempDir(), "cortex.db")})
	if err != nil {
		t.Fatalf("open Cortex: %v", err)
	}
	t.Cleanup(func() { _ = hub.Close() })
	memory, err := hub.Remember(ctx, RememberCommand{
		IdempotencyKey: "sola/supersede", Kind: KindDecision, Scope: ScopeGlobal,
		MemoryKey: "system.old-decision", Title: "Old decision", Content: "Legacy behavior", AgentID: "sola",
	})
	if err != nil {
		t.Fatalf("remember memory: %v", err)
	}
	superseded, err := hub.Review(ctx, ReviewCommand{
		IdempotencyKey: "mika/supersede", MemoryID: memory.ID, ActorID: "mika",
		Decision: ReviewSupersede, Reason: "Replaced by a newer decision",
	})
	if err != nil {
		t.Fatalf("supersede memory: %v", err)
	}
	if superseded.Lifecycle != LifecycleSuperseded {
		t.Fatalf("lifecycle = %q, want superseded", superseded.Lifecycle)
	}
	history, err := hub.History(ctx, HistoryQuery{MemoryID: memory.ID, AgentID: "mika"})
	if err != nil {
		t.Fatalf("history: %v", err)
	}
	if got, want := eventTypes(history), []EventType{EventCreated, EventSuperseded}; !equalEventTypes(got, want) {
		t.Fatalf("history types = %v, want %v", got, want)
	}
}
