package cortex

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strings"
	"time"
)

const maxReviewBatch = 100

type ReviewBatchCommand struct {
	IdempotencyKey string
	MemoryIDs      []string
	ActorID        string
	Decision       ReviewDecision
	Reason         string
}

type ReviewBatchResult struct {
	Memories []Memory
}

func (hub *Hub) ReviewBatch(ctx context.Context, command ReviewBatchCommand) (ReviewBatchResult, error) {
	if err := validateReviewBatch(command); err != nil {
		return ReviewBatchResult{}, err
	}
	if !hub.isAdmin(command.ActorID) {
		return ReviewBatchResult{}, ErrForbidden
	}
	tx, err := hub.db.BeginTx(ctx, nil)
	if err != nil {
		return ReviewBatchResult{}, fmt.Errorf("begin batch review: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	requestKey := scopedRequestKey(command.ActorID, command.IdempotencyKey)
	fingerprint := reviewBatchFingerprint(command)
	if resourceID, found, err := requestResource(ctx, tx, requestKey, "review_batch"); err != nil {
		return ReviewBatchResult{}, fmt.Errorf("check batch review request: %w", err)
	} else if found {
		if resourceID != fingerprint {
			return ReviewBatchResult{}, fmt.Errorf("%w: idempotency key was used for another batch", ErrConflict)
		}
		return readBatchMemories(ctx, tx, command.MemoryIDs)
	}

	type transition struct {
		memory Memory
		next   Lifecycle
		event  EventType
	}
	transitions := make([]transition, 0, len(command.MemoryIDs))
	for _, memoryID := range command.MemoryIDs {
		memory, err := getMemory(ctx, tx, memoryID)
		if err != nil {
			return ReviewBatchResult{}, err
		}
		next, eventType, err := reviewTransition(memory.Lifecycle, command.Decision)
		if err != nil {
			return ReviewBatchResult{}, err
		}
		transitions = append(transitions, transition{memory: memory, next: next, event: eventType})
	}

	now := time.Now().UTC().Format(time.RFC3339Nano)
	for _, item := range transitions {
		eventID, err := newID("evt")
		if err != nil {
			return ReviewBatchResult{}, err
		}
		if _, err := tx.ExecContext(ctx,
			"UPDATE memories SET lifecycle = ?, updated_at = ? WHERE id = ?",
			item.next, now, item.memory.ID,
		); err != nil {
			return ReviewBatchResult{}, fmt.Errorf("update batch lifecycle: %w", err)
		}
		if _, err := tx.ExecContext(ctx, `
INSERT INTO memory_events(id, memory_id, event_type, actor_id, reason, created_at)
VALUES (?, ?, ?, ?, ?, ?)`, eventID, item.memory.ID, item.event, command.ActorID,
			strings.TrimSpace(command.Reason), now); err != nil {
			return ReviewBatchResult{}, fmt.Errorf("record batch review event: %w", err)
		}
	}
	if err := recordRequest(ctx, tx, requestKey, "review_batch", fingerprint, now); err != nil {
		return ReviewBatchResult{}, fmt.Errorf("record batch review request: %w", err)
	}
	result, err := readBatchMemories(ctx, tx, command.MemoryIDs)
	if err != nil {
		return ReviewBatchResult{}, err
	}
	if err := tx.Commit(); err != nil {
		return ReviewBatchResult{}, fmt.Errorf("commit batch review: %w", err)
	}
	return result, nil
}

func validateReviewBatch(command ReviewBatchCommand) error {
	if strings.TrimSpace(command.IdempotencyKey) == "" || strings.TrimSpace(command.ActorID) == "" ||
		len(command.MemoryIDs) == 0 || len(command.MemoryIDs) > maxReviewBatch || len(command.Reason) > 500 {
		return fmt.Errorf("%w: batch review requires 1 to %d memories", ErrInvalidInput, maxReviewBatch)
	}
	if command.Decision != ReviewApprove && command.Decision != ReviewReject && command.Decision != ReviewArchive {
		return fmt.Errorf("%w: batch review supports approve, reject, or archive", ErrInvalidInput)
	}
	seen := make(map[string]struct{}, len(command.MemoryIDs))
	for _, memoryID := range command.MemoryIDs {
		memoryID = strings.TrimSpace(memoryID)
		if memoryID == "" {
			return fmt.Errorf("%w: memory id is required", ErrInvalidInput)
		}
		if _, duplicate := seen[memoryID]; duplicate {
			return fmt.Errorf("%w: duplicate memory id", ErrInvalidInput)
		}
		seen[memoryID] = struct{}{}
	}
	return nil
}

func readBatchMemories(ctx context.Context, queryer queryRower, memoryIDs []string) (ReviewBatchResult, error) {
	result := ReviewBatchResult{Memories: make([]Memory, 0, len(memoryIDs))}
	for _, memoryID := range memoryIDs {
		memory, err := getMemory(ctx, queryer, memoryID)
		if err != nil {
			return ReviewBatchResult{}, err
		}
		result.Memories = append(result.Memories, memory)
	}
	return result, nil
}

func reviewBatchFingerprint(command ReviewBatchCommand) string {
	sum := sha256.Sum256([]byte(string(command.Decision) + "\x00" + strings.Join(command.MemoryIDs, "\x00")))
	return "batch_" + hex.EncodeToString(sum[:16])
}
