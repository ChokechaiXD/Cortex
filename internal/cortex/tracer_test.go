package cortex

import (
	"context"
	"path/filepath"
	"testing"
)

func TestSharedMemoryTracer(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	hub, err := Open(Config{DatabasePath: filepath.Join(t.TempDir(), "cortex.db")})
	if err != nil {
		t.Fatalf("open Cortex: %v", err)
	}
	t.Cleanup(func() { _ = hub.Close() })

	cmd := RememberCommand{
		IdempotencyKey: "sola/session-42/lesson-1",
		Kind:           KindFailedAttempt,
		Scope:          ScopeProject,
		ScopeKey:       "novelclaw",
		MemoryKey:      "translation.force-overwrite",
		Title:          "Force translation can overwrite canonical output",
		Content:        "Running translation with --force without a backup overwrote canonical chapter files.",
		Tags:           []string{"translation", "force", "backup"},
		AgentID:        "sola",
		SessionID:      "session-42",
		SourceRef:      "translate.py",
	}

	created, err := hub.Remember(ctx, cmd)
	if err != nil {
		t.Fatalf("remember: %v", err)
	}
	if created.Lifecycle != LifecycleCandidate {
		t.Fatalf("new memory lifecycle = %q, want candidate", created.Lifecycle)
	}

	replayed, err := hub.Remember(ctx, cmd)
	if err != nil {
		t.Fatalf("idempotent remember: %v", err)
	}
	if replayed.ID != created.ID {
		t.Fatalf("idempotent remember created %q, want %q", replayed.ID, created.ID)
	}

	hidden, err := hub.Recall(ctx, RecallQuery{
		AgentID: "nua",
		Text:    "force translation backup",
		Project: "novelclaw",
		Limit:   5,
	})
	if err != nil {
		t.Fatalf("recall hidden candidate: %v", err)
	}
	if len(hidden.Items) != 0 {
		t.Fatalf("candidate leaked into normal recall: %#v", hidden.Items)
	}

	approved, err := hub.Review(ctx, ReviewCommand{
		IdempotencyKey: "mika/review/lesson-1",
		MemoryID:       created.ID,
		ActorID:        "mika",
		Decision:       ReviewApprove,
		Reason:         "Failure reproduced from the recorded command.",
	})
	if err != nil {
		t.Fatalf("approve memory: %v", err)
	}
	if approved.Lifecycle != LifecycleActive {
		t.Fatalf("approved lifecycle = %q, want active", approved.Lifecycle)
	}

	recalled, err := hub.Recall(ctx, RecallQuery{
		IdempotencyKey: "nua/research-7/recall-1",
		AgentID:        "nua",
		SessionID:      "research-7",
		Text:           "force translation backup",
		Project:        "novelclaw",
		Limit:          5,
	})
	if err != nil {
		t.Fatalf("recall approved memory: %v", err)
	}
	if len(recalled.Items) != 1 || recalled.Items[0].Memory.ID != created.ID {
		t.Fatalf("recall items = %#v, want memory %q", recalled.Items, created.ID)
	}
	replayedRecall, err := hub.Recall(ctx, RecallQuery{
		IdempotencyKey: "nua/research-7/recall-1",
		AgentID:        "nua",
		SessionID:      "research-7",
		Text:           "force translation backup",
		Project:        "novelclaw",
		Limit:          5,
	})
	if err != nil {
		t.Fatalf("idempotent recall: %v", err)
	}
	if replayedRecall.ID != recalled.ID || len(replayedRecall.Items) != 1 {
		t.Fatalf("idempotent recall = %#v, want recall %q", replayedRecall, recalled.ID)
	}

	before := recalled.Items[0].Memory.UtilityScore
	updated, err := hub.Feedback(ctx, FeedbackCommand{
		IdempotencyKey: "nua/research-7/helpful-1",
		MemoryID:       created.ID,
		AgentID:        "nua",
		SessionID:      "research-7",
		Outcome:        FeedbackHelpful,
	})
	if err != nil {
		t.Fatalf("feedback: %v", err)
	}
	if updated.UtilityScore <= before {
		t.Fatalf("utility score = %v, want greater than %v", updated.UtilityScore, before)
	}

	history, err := hub.History(ctx, HistoryQuery{MemoryID: created.ID, AgentID: "nua"})
	if err != nil {
		t.Fatalf("history: %v", err)
	}
	if got, want := eventTypes(history), []EventType{EventCreated, EventApproved, EventRecalled, EventHelpful}; !equalEventTypes(got, want) {
		t.Fatalf("event types = %v, want %v", got, want)
	}
}

func eventTypes(events []Event) []EventType {
	types := make([]EventType, len(events))
	for i, event := range events {
		types[i] = event.Type
	}
	return types
}

func equalEventTypes(left, right []EventType) bool {
	if len(left) != len(right) {
		return false
	}
	for i := range left {
		if left[i] != right[i] {
			return false
		}
	}
	return true
}
