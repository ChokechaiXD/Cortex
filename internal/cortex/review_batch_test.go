package cortex

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"testing"
)

func TestReviewBatchAppliesOneDecisionAtomicallyAndIsIdempotent(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	hub, err := Open(Config{DatabasePath: filepath.Join(t.TempDir(), "cortex.db"), AdminAgents: []string{"mika"}})
	if err != nil {
		t.Fatalf("open Cortex: %v", err)
	}
	t.Cleanup(func() { _ = hub.Close() })
	memories := make([]Memory, 0, 3)
	for index := 0; index < 3; index++ {
		memory, err := hub.Remember(ctx, RememberCommand{
			IdempotencyKey: fmt.Sprintf("batch/create/%d", index), Kind: KindFact,
			Scope: ScopeProject, ScopeKey: "cortex", MemoryKey: fmt.Sprintf("batch.%d", index),
			Title: fmt.Sprintf("Candidate %d", index), Content: "Imported candidate", AgentID: "sora",
		})
		if err != nil {
			t.Fatalf("create candidate %d: %v", index, err)
		}
		memories = append(memories, memory)
	}

	command := ReviewBatchCommand{
		IdempotencyKey: "dashboard/batch/one", ActorID: "mika", Decision: ReviewApprove,
		MemoryIDs: []string{memories[0].ID, memories[1].ID}, Reason: "Reviewed together",
	}
	result, err := hub.ReviewBatch(ctx, command)
	if err != nil {
		t.Fatalf("review batch: %v", err)
	}
	if len(result.Memories) != 2 || result.Memories[0].Lifecycle != LifecycleActive ||
		result.Memories[1].Lifecycle != LifecycleActive {
		t.Fatalf("batch result = %#v", result)
	}
	replayed, err := hub.ReviewBatch(ctx, command)
	if err != nil || len(replayed.Memories) != 2 {
		t.Fatalf("replay result=%#v error=%v", replayed, err)
	}
	untouched, err := getMemory(ctx, hub.db, memories[2].ID)
	if err != nil || untouched.Lifecycle != LifecycleCandidate {
		t.Fatalf("unselected memory=%#v error=%v", untouched, err)
	}
}

func TestReviewBatchRollsBackWhenAnyTransitionIsInvalid(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	hub, err := Open(Config{DatabasePath: filepath.Join(t.TempDir(), "cortex.db"), AdminAgents: []string{"mika"}})
	if err != nil {
		t.Fatalf("open Cortex: %v", err)
	}
	t.Cleanup(func() { _ = hub.Close() })
	first, _ := hub.Remember(ctx, RememberCommand{
		IdempotencyKey: "batch/first", Kind: KindFact, Scope: ScopeProject, ScopeKey: "cortex",
		MemoryKey: "batch.first", Title: "First", Content: "First", AgentID: "sora",
	})
	second, _ := hub.Remember(ctx, RememberCommand{
		IdempotencyKey: "batch/second", Kind: KindFact, Scope: ScopeProject, ScopeKey: "cortex",
		MemoryKey: "batch.second", Title: "Second", Content: "Second", AgentID: "sora",
	})
	if _, err := hub.Review(ctx, ReviewCommand{
		IdempotencyKey: "batch/preapprove", MemoryID: second.ID, ActorID: "mika", Decision: ReviewApprove,
	}); err != nil {
		t.Fatalf("preapprove second: %v", err)
	}

	_, err = hub.ReviewBatch(ctx, ReviewBatchCommand{
		IdempotencyKey: "dashboard/batch/invalid", ActorID: "mika", Decision: ReviewApprove,
		MemoryIDs: []string{first.ID, second.ID},
	})
	if !errors.Is(err, ErrConflict) {
		t.Fatalf("batch error=%v, want conflict", err)
	}
	unchanged, err := getMemory(ctx, hub.db, first.ID)
	if err != nil || unchanged.Lifecycle != LifecycleCandidate {
		t.Fatalf("first memory changed despite rollback: %#v error=%v", unchanged, err)
	}
}

func TestReviewBatchRejectsUnsafeShape(t *testing.T) {
	t.Parallel()

	hub, err := Open(Config{DatabasePath: filepath.Join(t.TempDir(), "cortex.db"), AdminAgents: []string{"mika"}})
	if err != nil {
		t.Fatalf("open Cortex: %v", err)
	}
	t.Cleanup(func() { _ = hub.Close() })
	_, err = hub.ReviewBatch(context.Background(), ReviewBatchCommand{
		IdempotencyKey: "dashboard/batch/duplicate", ActorID: "mika", Decision: ReviewPromote,
		MemoryIDs: []string{"same", "same"},
	})
	if !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("unsafe batch error=%v, want invalid input", err)
	}
}
