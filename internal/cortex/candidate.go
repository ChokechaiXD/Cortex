package cortex

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"time"
)

type candidateInput struct {
	idempotencyKey string
	operation      string
	kind           MemoryKind
	scope          Scope
	scopeKey       string
	memoryKey      string
	title          string
	content        string
	tags           []string
	agentID        string
	sessionID      string
	sourceRef      string
	truthScore     float64
	utilityScore   float64
	eventType      EventType
	metadata       map[string]any
}

func insertCandidate(ctx context.Context, tx *sql.Tx, input candidateInput) (Memory, bool, error) {
	if memoryID, found, err := requestResource(ctx, tx, input.idempotencyKey, input.operation); err != nil {
		return Memory{}, false, fmt.Errorf("check %s request: %w", input.operation, err)
	} else if found {
		memory, err := getMemory(ctx, tx, memoryID)
		return memory, false, err
	}
	memoryID, err := newID("mem")
	if err != nil {
		return Memory{}, false, err
	}
	eventID, err := newID("evt")
	if err != nil {
		return Memory{}, false, err
	}
	now := time.Now().UTC().Format(time.RFC3339Nano)
	tagsJSON, err := encodeJSON(input.tags)
	if err != nil {
		return Memory{}, false, fmt.Errorf("encode tags: %w", err)
	}
	metadataJSON, err := encodeJSON(input.metadata)
	if err != nil {
		return Memory{}, false, fmt.Errorf("encode import metadata: %w", err)
	}
	_, err = tx.ExecContext(ctx, `
INSERT INTO memories(
    id, kind, scope, scope_key, memory_key, lifecycle,
    truth_score, utility_score, created_by, current_revision, created_at, updated_at
) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, 1, ?, ?)`,
		memoryID, input.kind, input.scope, strings.TrimSpace(input.scopeKey), strings.TrimSpace(input.memoryKey),
		LifecycleCandidate, clamp(input.truthScore), clamp(input.utilityScore), input.agentID, now, now,
	)
	if err != nil {
		return Memory{}, false, fmt.Errorf("insert memory: %w", err)
	}
	_, err = tx.ExecContext(ctx, `
INSERT INTO memory_revisions(
    memory_id, revision, title, content, tags_json, session_id,
    source_ref, created_by, created_at
) VALUES (?, 1, ?, ?, ?, ?, ?, ?, ?)`,
		memoryID, strings.TrimSpace(input.title), strings.TrimSpace(input.content), tagsJSON,
		input.sessionID, input.sourceRef, input.agentID, now,
	)
	if err != nil {
		return Memory{}, false, fmt.Errorf("insert revision: %w", err)
	}
	_, err = tx.ExecContext(ctx,
		"INSERT INTO memory_fts(memory_id, title, content, tags) VALUES (?, ?, ?, ?)",
		memoryID, input.title, input.content, strings.Join(input.tags, " "),
	)
	if err != nil {
		return Memory{}, false, fmt.Errorf("index memory: %w", err)
	}
	_, err = tx.ExecContext(ctx, `
INSERT INTO memory_events(id, memory_id, event_type, actor_id, session_id, metadata_json, created_at)
VALUES (?, ?, ?, ?, ?, ?, ?)`, eventID, memoryID, input.eventType, input.agentID,
		input.sessionID, metadataJSON, now)
	if err != nil {
		return Memory{}, false, fmt.Errorf("record %s event: %w", input.eventType, err)
	}
	if err := recordRequest(ctx, tx, input.idempotencyKey, input.operation, memoryID, now); err != nil {
		return Memory{}, false, fmt.Errorf("record %s request: %w", input.operation, err)
	}
	memory, err := getMemory(ctx, tx, memoryID)
	if err != nil {
		return Memory{}, false, fmt.Errorf("read candidate memory: %w", err)
	}
	return memory, true, nil
}
