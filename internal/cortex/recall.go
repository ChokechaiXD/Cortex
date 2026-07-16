package cortex

import (
	"context"
	"database/sql"
	"fmt"
	"regexp"
	"strings"
	"time"
)

var searchToken = regexp.MustCompile(`[\p{L}\p{N}_]+`)

func (hub *Hub) Recall(ctx context.Context, query RecallQuery) (RecallResult, error) {
	if err := validateRecall(query); err != nil {
		return RecallResult{}, err
	}
	limit := query.Limit
	if limit == 0 {
		limit = 8
	}
	ftsQuery := buildFTSQuery(query.Text)
	if ftsQuery == "" {
		return RecallResult{Items: []RecallItem{}}, nil
	}
	tx, err := hub.db.BeginTx(ctx, nil)
	if err != nil {
		return RecallResult{}, fmt.Errorf("begin recall: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	if query.IdempotencyKey != "" {
		if recallID, found, err := requestResource(ctx, tx, query.IdempotencyKey, "recall"); err != nil {
			return RecallResult{}, fmt.Errorf("check recall request: %w", err)
		} else if found {
			return loadRecall(ctx, tx, recallID)
		}
	}

	recallID, err := newID("rec")
	if err != nil {
		return RecallResult{}, err
	}
	rows, err := tx.QueryContext(ctx, `
SELECT m.id, bm25(memory_fts) AS rank
FROM memory_fts
JOIN memories m ON m.id = memory_fts.memory_id
WHERE memory_fts MATCH ?
ORDER BY rank
LIMIT 100`, ftsQuery)
	if err != nil {
		return RecallResult{}, fmt.Errorf("search memories: %w", err)
	}
	type rankedID struct {
		id   string
		rank float64
	}
	var ranked []rankedID
	for rows.Next() {
		var item rankedID
		if err := rows.Scan(&item.id, &item.rank); err != nil {
			_ = rows.Close()
			return RecallResult{}, fmt.Errorf("scan search result: %w", err)
		}
		ranked = append(ranked, item)
	}
	if err := rows.Close(); err != nil {
		return RecallResult{}, fmt.Errorf("close search results: %w", err)
	}

	result := RecallResult{ID: recallID, Items: make([]RecallItem, 0, limit)}
	for _, candidate := range ranked {
		memory, err := getMemory(ctx, tx, candidate.id)
		if err != nil {
			return RecallResult{}, err
		}
		if !hub.canRecall(memory, query) {
			continue
		}
		textScore := 1 / (1 + max(0, -candidate.rank))
		score := 0.65*textScore + 0.20*memory.TruthScore + 0.15*memory.UtilityScore
		result.Items = append(result.Items, RecallItem{Memory: memory, Score: score})
		if len(result.Items) == limit {
			break
		}
	}
	if err := hub.persistRecall(ctx, tx, result, query); err != nil {
		return RecallResult{}, err
	}
	if err := tx.Commit(); err != nil {
		return RecallResult{}, fmt.Errorf("commit recall: %w", err)
	}
	return result, nil
}

func (hub *Hub) persistRecall(ctx context.Context, tx *sql.Tx, result RecallResult, query RecallQuery) error {
	now := time.Now().UTC().Format(time.RFC3339Nano)
	if _, err := tx.ExecContext(ctx, `
INSERT INTO recalls(id, query_text, agent_id, session_id, created_at)
VALUES (?, ?, ?, ?, ?)`, result.ID, query.Text, query.AgentID, query.SessionID, now); err != nil {
		return fmt.Errorf("record recall: %w", err)
	}
	for index, item := range result.Items {
		if _, err := tx.ExecContext(ctx, `
INSERT INTO recall_items(recall_id, memory_id, rank, score)
VALUES (?, ?, ?, ?)`, result.ID, item.Memory.ID, index+1, item.Score); err != nil {
			return fmt.Errorf("record recall item: %w", err)
		}
	}
	if err := hub.recordRecallEvents(ctx, tx, result, query, now); err != nil {
		return err
	}
	if query.IdempotencyKey != "" {
		if err := recordRequest(ctx, tx, query.IdempotencyKey, "recall", result.ID, now); err != nil {
			return fmt.Errorf("record recall request: %w", err)
		}
	}
	return nil
}

func loadRecall(ctx context.Context, tx *sql.Tx, recallID string) (RecallResult, error) {
	rows, err := tx.QueryContext(ctx, `
SELECT memory_id, score
FROM recall_items
WHERE recall_id = ?
ORDER BY rank`, recallID)
	if err != nil {
		return RecallResult{}, fmt.Errorf("load recall: %w", err)
	}
	type storedItem struct {
		memoryID string
		score    float64
	}
	var stored []storedItem
	for rows.Next() {
		var item storedItem
		if err := rows.Scan(&item.memoryID, &item.score); err != nil {
			_ = rows.Close()
			return RecallResult{}, fmt.Errorf("scan recall item: %w", err)
		}
		stored = append(stored, item)
	}
	if err := rows.Close(); err != nil {
		return RecallResult{}, fmt.Errorf("close recall items: %w", err)
	}
	result := RecallResult{ID: recallID, Items: make([]RecallItem, 0, len(stored))}
	for _, item := range stored {
		memory, err := getMemory(ctx, tx, item.memoryID)
		if err != nil {
			return RecallResult{}, err
		}
		result.Items = append(result.Items, RecallItem{Memory: memory, Score: item.score})
	}
	return result, nil
}

func (hub *Hub) canRecall(memory Memory, query RecallQuery) bool {
	if memory.Lifecycle != LifecycleActive && memory.Lifecycle != LifecycleCanonical &&
		!(query.IncludeCandidates && memory.Lifecycle == LifecycleCandidate) {
		return false
	}
	if memory.Scope == ScopePrivate && memory.CreatedBy != query.AgentID && !hub.isAdmin(query.AgentID) {
		return false
	}
	switch memory.Scope {
	case ScopeProject:
		return query.Project == "" || memory.ScopeKey == query.Project
	case ScopeDomain:
		return query.Domain == "" || memory.ScopeKey == query.Domain
	default:
		return true
	}
}

func (hub *Hub) recordRecallEvents(ctx context.Context, tx *sql.Tx, result RecallResult, query RecallQuery, now string) error {
	for index, item := range result.Items {
		eventID, err := newID("evt")
		if err != nil {
			return err
		}
		metadata, err := encodeJSON(map[string]any{
			"recall_id": result.ID,
			"rank":      index + 1,
			"score":     item.Score,
		})
		if err != nil {
			return fmt.Errorf("encode recall metadata: %w", err)
		}
		if _, err := tx.ExecContext(ctx, `
INSERT INTO memory_events(id, memory_id, event_type, actor_id, session_id, metadata_json, created_at)
VALUES (?, ?, ?, ?, ?, ?, ?)`, eventID, item.Memory.ID, EventRecalled,
			query.AgentID, query.SessionID, metadata, now); err != nil {
			return fmt.Errorf("record recall event: %w", err)
		}
	}
	return nil
}

func buildFTSQuery(text string) string {
	tokens := searchToken.FindAllString(strings.ToLower(text), -1)
	quoted := make([]string, 0, len(tokens))
	for _, token := range tokens {
		quoted = append(quoted, `"`+token+`"`)
	}
	return strings.Join(quoted, " OR ")
}
