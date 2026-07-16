package cortex

import (
	"context"
	"fmt"
	"time"
)

func (hub *Hub) Feedback(ctx context.Context, cmd FeedbackCommand) (Memory, error) {
	if err := validateFeedback(cmd); err != nil {
		return Memory{}, err
	}
	tx, err := hub.db.BeginTx(ctx, nil)
	if err != nil {
		return Memory{}, fmt.Errorf("begin feedback: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	if memoryID, found, err := requestResource(ctx, tx, cmd.IdempotencyKey, "feedback"); err != nil {
		return Memory{}, fmt.Errorf("check feedback request: %w", err)
	} else if found {
		return getMemory(ctx, tx, memoryID)
	}
	memory, err := getMemory(ctx, tx, cmd.MemoryID)
	if err != nil {
		return Memory{}, err
	}
	truth, utility, eventType := applyFeedback(memory.TruthScore, memory.UtilityScore, cmd.Outcome)
	eventID, err := newID("evt")
	if err != nil {
		return Memory{}, err
	}
	now := time.Now().UTC().Format(time.RFC3339Nano)
	if _, err := tx.ExecContext(ctx, `
UPDATE memories SET truth_score = ?, utility_score = ?, updated_at = ? WHERE id = ?`,
		truth, utility, now, cmd.MemoryID); err != nil {
		return Memory{}, fmt.Errorf("update scores: %w", err)
	}
	if _, err := tx.ExecContext(ctx, `
INSERT INTO memory_events(id, memory_id, event_type, actor_id, session_id, reason, created_at)
VALUES (?, ?, ?, ?, ?, ?, ?)`, eventID, cmd.MemoryID, eventType,
		cmd.AgentID, cmd.SessionID, cmd.Reason, now); err != nil {
		return Memory{}, fmt.Errorf("record feedback event: %w", err)
	}
	if err := recordRequest(ctx, tx, cmd.IdempotencyKey, "feedback", cmd.MemoryID, now); err != nil {
		return Memory{}, fmt.Errorf("record feedback request: %w", err)
	}
	memory, err = getMemory(ctx, tx, cmd.MemoryID)
	if err != nil {
		return Memory{}, err
	}
	if err := tx.Commit(); err != nil {
		return Memory{}, fmt.Errorf("commit feedback: %w", err)
	}
	return memory, nil
}
