package cortex

import (
	"context"
	"errors"
	"path/filepath"
	"testing"
)

func TestBrowseCombinesFTSAndStructuredMemoryFilters(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	hub, err := Open(Config{DatabasePath: filepath.Join(t.TempDir(), "cortex.db"), AdminAgents: []string{"mika"}})
	if err != nil {
		t.Fatalf("open Cortex: %v", err)
	}
	t.Cleanup(func() { _ = hub.Close() })
	wanted, err := hub.Remember(ctx, RememberCommand{
		IdempotencyKey: "sola/browse-1", AgentID: "sola", Kind: KindDecision,
		Scope: ScopeProject, ScopeKey: "novelclaw", MemoryKey: "output.canonical",
		Title: "Canonical chapter output", Content: "Write translated chapters to canonical Thai JSON.",
		Tags: []string{"translation", "output"},
	})
	if err != nil {
		t.Fatalf("remember wanted memory: %v", err)
	}
	if _, err := hub.Remember(ctx, RememberCommand{
		IdempotencyKey: "nua/browse-2", AgentID: "nua", Kind: KindFact,
		Scope: ScopeGlobal, MemoryKey: "research.sources", Title: "Research sources",
		Content: "Canonical sources need citations.",
	}); err != nil {
		t.Fatalf("remember other memory: %v", err)
	}

	result, err := hub.Browse(ctx, BrowseQuery{
		AgentID: "mika", Text: "canonical output", Lifecycle: LifecycleCandidate,
		Kind: KindDecision, Scope: ScopeProject, ScopeKey: "novelclaw", CreatedBy: "sola", Limit: 20,
	})
	if err != nil {
		t.Fatalf("browse memories: %v", err)
	}
	if result.Total != 1 || len(result.Memories) != 1 || result.Memories[0].ID != wanted.ID {
		t.Fatalf("browse result = %#v", result)
	}
}

func TestBrowseRequiresGovernorAndValidFilters(t *testing.T) {
	t.Parallel()

	hub, err := Open(Config{DatabasePath: filepath.Join(t.TempDir(), "cortex.db"), AdminAgents: []string{"mika"}})
	if err != nil {
		t.Fatalf("open Cortex: %v", err)
	}
	t.Cleanup(func() { _ = hub.Close() })
	ctx := context.Background()
	if _, err := hub.Browse(ctx, BrowseQuery{AgentID: "nua"}); !errors.Is(err, ErrForbidden) {
		t.Fatalf("non-governor browse error = %v", err)
	}
	if _, err := hub.Browse(ctx, BrowseQuery{AgentID: "mika", Lifecycle: Lifecycle("unknown")}); !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("invalid lifecycle error = %v", err)
	}
}
