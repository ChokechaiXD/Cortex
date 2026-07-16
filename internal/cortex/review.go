package cortex

import (
	"context"
	"fmt"
	"time"
)

func (hub *Hub) Review(ctx context.Context, cmd ReviewCommand) (Memory, error) {
	if err := validateReview(cmd); err != nil {
		return Memory{}, err
	}
	if !hub.isAdmin(cmd.ActorID) {
		return Memory{}, ErrForbidden
	}
	tx, err := hub.db.BeginTx(ctx, nil)
	if err != nil {
		return Memory{}, fmt.Errorf("begin review: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	requestKey := scopedRequestKey(cmd.ActorID, cmd.IdempotencyKey)
	if memoryID, found, err := requestResource(ctx, tx, requestKey, "review"); err != nil {
		return Memory{}, fmt.Errorf("check review request: %w", err)
	} else if found {
		return getMemory(ctx, tx, memoryID)
	}
	memory, err := getMemory(ctx, tx, cmd.MemoryID)
	if err != nil {
		return Memory{}, err
	}
	next, eventType, err := reviewTransition(memory.Lifecycle, cmd.Decision)
	if err != nil {
		return Memory{}, err
	}
	eventID, err := newID("evt")
	if err != nil {
		return Memory{}, err
	}
	now := time.Now().UTC().Format(time.RFC3339Nano)
	if _, err := tx.ExecContext(ctx,
		"UPDATE memories SET lifecycle = ?, updated_at = ? WHERE id = ?", next, now, cmd.MemoryID,
	); err != nil {
		return Memory{}, fmt.Errorf("update lifecycle: %w", err)
	}
	if _, err := tx.ExecContext(ctx, `
INSERT INTO memory_events(id, memory_id, event_type, actor_id, reason, created_at)
VALUES (?, ?, ?, ?, ?, ?)`, eventID, cmd.MemoryID, eventType, cmd.ActorID, cmd.Reason, now); err != nil {
		return Memory{}, fmt.Errorf("record review event: %w", err)
	}
	if err := recordRequest(ctx, tx, requestKey, "review", cmd.MemoryID, now); err != nil {
		return Memory{}, fmt.Errorf("record review request: %w", err)
	}
	memory, err = getMemory(ctx, tx, cmd.MemoryID)
	if err != nil {
		return Memory{}, err
	}
	if err := tx.Commit(); err != nil {
		return Memory{}, fmt.Errorf("commit review: %w", err)
	}
	return memory, nil
}

func reviewTransition(current Lifecycle, decision ReviewDecision) (Lifecycle, EventType, error) {
	switch decision {
	case ReviewApprove:
		if current != LifecycleCandidate {
			return "", "", fmt.Errorf("%w: only candidates can be approved", ErrConflict)
		}
		return LifecycleActive, EventApproved, nil
	case ReviewPromote:
		if current != LifecycleActive {
			return "", "", fmt.Errorf("%w: only active memories can be promoted", ErrConflict)
		}
		return LifecycleCanonical, EventPromoted, nil
	case ReviewReject:
		if current != LifecycleCandidate && current != LifecycleActive {
			return "", "", fmt.Errorf("%w: memory cannot be rejected from %s", ErrConflict, current)
		}
		return LifecycleRejected, EventRejected, nil
	case ReviewSupersede:
		if current == LifecycleSuperseded || current == LifecycleArchived {
			return "", "", fmt.Errorf("%w: memory cannot be superseded from %s", ErrConflict, current)
		}
		return LifecycleSuperseded, EventSuperseded, nil
	case ReviewArchive:
		if current == LifecycleArchived {
			return "", "", fmt.Errorf("%w: memory is already archived", ErrConflict)
		}
		return LifecycleArchived, EventArchived, nil
	default:
		return "", "", fmt.Errorf("%w: unsupported review decision", ErrInvalidInput)
	}
}
