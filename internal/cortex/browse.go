package cortex

import (
	"context"
	"fmt"
	"strings"
)

func (hub *Hub) Browse(ctx context.Context, query BrowseQuery) (BrowseResult, error) {
	if !hub.isAdmin(query.AgentID) {
		return BrowseResult{}, ErrForbidden
	}
	if query.Lifecycle != "" && !validLifecycle(query.Lifecycle) {
		return BrowseResult{}, fmt.Errorf("%w: unsupported lifecycle", ErrInvalidInput)
	}
	if query.Kind != "" && !validKind(query.Kind) {
		return BrowseResult{}, fmt.Errorf("%w: unsupported memory kind", ErrInvalidInput)
	}
	if query.Scope != "" && !validScope(query.Scope) {
		return BrowseResult{}, fmt.Errorf("%w: unsupported memory scope", ErrInvalidInput)
	}
	if len(query.Text) > 500 || len(query.ScopeKey) > 256 || len(query.CreatedBy) > 128 {
		return BrowseResult{}, fmt.Errorf("%w: memory filter is too long", ErrInvalidInput)
	}
	limit := query.Limit
	if limit <= 0 {
		limit = 100
	}
	if limit > 500 {
		return BrowseResult{}, fmt.Errorf("%w: browse limit cannot exceed 500", ErrInvalidInput)
	}

	from := " FROM memories m"
	clauses := []string{"1 = 1"}
	args := make([]any, 0, 7)
	if text := strings.TrimSpace(query.Text); text != "" {
		ftsQuery := buildFTSQuery(text)
		if ftsQuery == "" {
			return BrowseResult{Memories: []Memory{}}, nil
		}
		from += " JOIN memory_fts ON memory_fts.memory_id = m.id"
		clauses = append(clauses, "memory_fts MATCH ?")
		args = append(args, ftsQuery)
	}
	addFilter := func(column string, value any, enabled bool) {
		if !enabled {
			return
		}
		clauses = append(clauses, column+" = ?")
		args = append(args, value)
	}
	addFilter("m.lifecycle", query.Lifecycle, query.Lifecycle != "")
	addFilter("m.kind", query.Kind, query.Kind != "")
	addFilter("m.scope", query.Scope, query.Scope != "")
	addFilter("m.scope_key", strings.TrimSpace(query.ScopeKey), strings.TrimSpace(query.ScopeKey) != "")
	addFilter("m.created_by", strings.TrimSpace(query.CreatedBy), strings.TrimSpace(query.CreatedBy) != "")
	where := " WHERE " + strings.Join(clauses, " AND ")

	result := BrowseResult{Memories: make([]Memory, 0)}
	if err := hub.db.QueryRowContext(ctx, "SELECT COUNT(*)"+from+where, args...).Scan(&result.Total); err != nil {
		return BrowseResult{}, fmt.Errorf("count filtered memories: %w", err)
	}
	listArgs := append(append([]any{}, args...), limit)
	rows, err := hub.db.QueryContext(ctx,
		"SELECT m.id"+from+where+" ORDER BY m.updated_at DESC, m.rowid DESC LIMIT ?", listArgs...,
	)
	if err != nil {
		return BrowseResult{}, fmt.Errorf("browse memories: %w", err)
	}
	var ids []string
	for rows.Next() {
		var memoryID string
		if err := rows.Scan(&memoryID); err != nil {
			_ = rows.Close()
			return BrowseResult{}, fmt.Errorf("scan browsed memory: %w", err)
		}
		ids = append(ids, memoryID)
	}
	if err := rows.Close(); err != nil {
		return BrowseResult{}, fmt.Errorf("close browsed memories: %w", err)
	}
	for _, memoryID := range ids {
		memory, err := getMemory(ctx, hub.db, memoryID)
		if err != nil {
			return BrowseResult{}, err
		}
		result.Memories = append(result.Memories, memory)
	}
	return result, nil
}

func (hub *Hub) Overview(ctx context.Context, agentID string, limit int) (Overview, error) {
	if limit <= 0 {
		limit = 100
	}
	if limit > 500 {
		return Overview{}, fmt.Errorf("%w: overview limit cannot exceed 500", ErrInvalidInput)
	}
	counts, err := hub.Counts(ctx, agentID)
	if err != nil {
		return Overview{}, err
	}
	result := Overview{Counts: counts, Memories: make([]Memory, 0)}
	rows, err := hub.db.QueryContext(ctx, "SELECT id FROM memories ORDER BY updated_at DESC, rowid DESC LIMIT ?", limit)
	if err != nil {
		return Overview{}, fmt.Errorf("list memories: %w", err)
	}
	ids := make([]string, 0, limit)
	for rows.Next() {
		var memoryID string
		if err := rows.Scan(&memoryID); err != nil {
			_ = rows.Close()
			return Overview{}, fmt.Errorf("scan memory id: %w", err)
		}
		ids = append(ids, memoryID)
	}
	if err := rows.Close(); err != nil {
		return Overview{}, fmt.Errorf("close memory list: %w", err)
	}
	for _, memoryID := range ids {
		memory, err := getMemory(ctx, hub.db, memoryID)
		if err != nil {
			return Overview{}, err
		}
		result.Memories = append(result.Memories, memory)
	}
	return result, nil
}

func (hub *Hub) Counts(ctx context.Context, agentID string) (map[Lifecycle]int, error) {
	if !hub.isAdmin(agentID) {
		return nil, ErrForbidden
	}
	counts := make(map[Lifecycle]int)
	countRows, err := hub.db.QueryContext(ctx, "SELECT lifecycle, COUNT(*) FROM memories GROUP BY lifecycle")
	if err != nil {
		return nil, fmt.Errorf("count memories: %w", err)
	}
	for countRows.Next() {
		var lifecycle string
		var count int
		if err := countRows.Scan(&lifecycle, &count); err != nil {
			_ = countRows.Close()
			return nil, fmt.Errorf("scan memory counts: %w", err)
		}
		counts[Lifecycle(lifecycle)] = count
	}
	if err := countRows.Close(); err != nil {
		return nil, fmt.Errorf("close memory counts: %w", err)
	}
	return counts, nil
}
