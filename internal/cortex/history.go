package cortex

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"time"
)

func (hub *Hub) History(ctx context.Context, query HistoryQuery) ([]Event, error) {
	if query.MemoryID == "" || query.AgentID == "" {
		return nil, fmt.Errorf("%w: memory_id and agent_id are required", ErrInvalidInput)
	}
	memory, err := getMemory(ctx, hub.db, query.MemoryID)
	if err != nil {
		return nil, err
	}
	if !hub.canInspect(memory, query.AgentID) {
		return nil, ErrForbidden
	}
	rows, err := hub.db.QueryContext(ctx, `
SELECT id, memory_id, event_type, actor_id, session_id, reason, metadata_json, created_at
FROM memory_events
WHERE memory_id = ?
ORDER BY created_at, rowid`, query.MemoryID)
	if err != nil {
		return nil, fmt.Errorf("query history: %w", err)
	}
	defer rows.Close()
	events := make([]Event, 0)
	for rows.Next() {
		var event Event
		var eventType, metadataJSON, createdAt string
		if err := rows.Scan(&event.ID, &event.MemoryID, &eventType, &event.ActorID,
			&event.SessionID, &event.Reason, &metadataJSON, &createdAt); err != nil {
			return nil, fmt.Errorf("scan history: %w", err)
		}
		event.Type = EventType(eventType)
		if metadataJSON != "{}" {
			if err := json.Unmarshal([]byte(metadataJSON), &event.Metadata); err != nil {
				return nil, fmt.Errorf("decode event metadata: %w", err)
			}
		}
		parsed, err := time.Parse(time.RFC3339Nano, createdAt)
		if err != nil {
			return nil, fmt.Errorf("decode event time: %w", err)
		}
		event.CreatedAt = parsed
		events = append(events, event)
	}
	if err := rows.Err(); err != nil && err != sql.ErrNoRows {
		return nil, fmt.Errorf("iterate history: %w", err)
	}
	return events, nil
}

func (hub *Hub) canInspect(memory Memory, agentID string) bool {
	if hub.isAdmin(agentID) || memory.CreatedBy == agentID {
		return true
	}
	if memory.Scope == ScopePrivate {
		return false
	}
	return memory.Lifecycle == LifecycleActive || memory.Lifecycle == LifecycleCanonical
}
