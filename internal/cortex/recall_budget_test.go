package cortex

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

func TestRecallTokenBudgetLimitsRecordedUsage(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	hub, err := Open(Config{DatabasePath: filepath.Join(t.TempDir(), "cortex.db")})
	if err != nil {
		t.Fatalf("open Cortex: %v", err)
	}
	t.Cleanup(func() { _ = hub.Close() })

	memories := make([]Memory, 0, 3)
	for index := 0; index < 3; index++ {
		memory, rememberErr := hub.Remember(ctx, RememberCommand{
			IdempotencyKey: fmt.Sprintf("sola/budget-%d", index), Kind: KindFact, Scope: ScopeGlobal,
			MemoryKey: fmt.Sprintf("budget.%d", index), Title: fmt.Sprintf("Budget needle %d", index),
			Content: "budget needle " + strings.Repeat(string(rune('a'+index)), 600), AgentID: "sola",
		})
		if rememberErr != nil {
			t.Fatalf("remember memory %d: %v", index, rememberErr)
		}
		if _, reviewErr := hub.Review(ctx, ReviewCommand{
			IdempotencyKey: fmt.Sprintf("mika/budget-%d", index), MemoryID: memory.ID,
			ActorID: "mika", Decision: ReviewApprove,
		}); reviewErr != nil {
			t.Fatalf("approve memory %d: %v", index, reviewErr)
		}
		memories = append(memories, memory)
	}

	const budget = 700
	result, err := hub.Recall(ctx, RecallQuery{
		AgentID: "nua", Text: "budget needle", Limit: 3, TokenBudget: budget,
	})
	if err != nil {
		t.Fatalf("budgeted recall: %v", err)
	}
	if len(result.Items) == 0 || len(result.Items) >= len(memories) || !result.Truncated ||
		result.TokenBudget != budget || result.EstimatedTokens > budget {
		t.Fatalf("budgeted recall = %#v", result)
	}
	recalled := make([]string, 0, len(result.Items))
	for _, item := range result.Items {
		recalled = append(recalled, item.Memory.ID)
	}
	for _, memory := range memories {
		history, historyErr := hub.History(ctx, HistoryQuery{MemoryID: memory.ID, AgentID: "mika"})
		if historyErr != nil {
			t.Fatalf("history for %s: %v", memory.ID, historyErr)
		}
		hasRecall := slices.Contains(eventTypes(history), EventRecalled)
		if hasRecall != slices.Contains(recalled, memory.ID) {
			t.Fatalf("memory %s recalled event=%v, returned=%v", memory.ID, hasRecall, slices.Contains(recalled, memory.ID))
		}
	}
}

func TestRecallRejectsUnsafeTokenBudget(t *testing.T) {
	t.Parallel()

	err := validateRecall(RecallQuery{AgentID: "nua", Text: "needle", TokenBudget: 99})
	if !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("small token budget error = %v", err)
	}
}
