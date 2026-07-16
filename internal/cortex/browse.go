package cortex

import (
	"context"
	"fmt"
)

func (hub *Hub) Overview(ctx context.Context, agentID string, limit int) (Overview, error) {
	if !hub.isAdmin(agentID) {
		return Overview{}, ErrForbidden
	}
	if limit <= 0 {
		limit = 100
	}
	if limit > 500 {
		return Overview{}, fmt.Errorf("%w: overview limit cannot exceed 500", ErrInvalidInput)
	}
	result := Overview{Counts: make(map[Lifecycle]int), Memories: make([]Memory, 0)}
	countRows, err := hub.db.QueryContext(ctx, "SELECT lifecycle, COUNT(*) FROM memories GROUP BY lifecycle")
	if err != nil {
		return Overview{}, fmt.Errorf("count memories: %w", err)
	}
	for countRows.Next() {
		var lifecycle string
		var count int
		if err := countRows.Scan(&lifecycle, &count); err != nil {
			_ = countRows.Close()
			return Overview{}, fmt.Errorf("scan memory counts: %w", err)
		}
		result.Counts[Lifecycle(lifecycle)] = count
	}
	if err := countRows.Close(); err != nil {
		return Overview{}, fmt.Errorf("close memory counts: %w", err)
	}
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
