package holographic

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"fmt"
	"net/url"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"cortex.local/cortex/internal/cortex"
	_ "modernc.org/sqlite"
)

type Options struct {
	DatabasePath string
	AgentID      string
	Project      string
}

type Result struct {
	Imported  int      `json:"imported"`
	Replayed  int      `json:"replayed"`
	MemoryIDs []string `json:"memory_ids"`
}

type legacyFact struct {
	ID             int64
	Content        string
	Category       string
	Tags           string
	TrustScore     float64
	RetrievalCount int
	HelpfulCount   int
	CreatedAt      string
	UpdatedAt      string
}

func Import(ctx context.Context, hub *cortex.Hub, options Options) (Result, error) {
	if strings.TrimSpace(options.DatabasePath) == "" || strings.TrimSpace(options.AgentID) == "" {
		return Result{}, fmt.Errorf("database path and agent id are required")
	}
	absPath, err := filepath.Abs(options.DatabasePath)
	if err != nil {
		return Result{}, fmt.Errorf("resolve Holographic database: %w", err)
	}
	database, err := openReadOnly(ctx, absPath)
	if err != nil {
		return Result{}, err
	}
	defer database.Close()
	if err := validateSchema(ctx, database); err != nil {
		return Result{}, err
	}
	facts, err := readFacts(ctx, database)
	if err != nil {
		return Result{}, err
	}
	sourceKey := shortHash(strings.ToLower(filepath.Clean(absPath)))
	result := Result{MemoryIDs: make([]string, 0, len(facts))}
	for _, fact := range facts {
		kind, scope, scopeKey := mapCategory(fact.Category, options.Project)
		contentKey := shortHash(fact.Content + "\x00" + fact.UpdatedAt)
		memory, created, err := hub.ImportCandidate(ctx, cortex.ImportCommand{
			IdempotencyKey: fmt.Sprintf("import/holographic/%s/%d/%s", sourceKey, fact.ID, contentKey),
			Kind:           kind,
			Scope:          scope,
			ScopeKey:       scopeKey,
			MemoryKey:      fmt.Sprintf("holographic.%s.fact.%d", sourceKey, fact.ID),
			Title:          titleFromContent(fact.Content),
			Content:        fact.Content,
			Tags:           importTags(fact.Tags, fact.Category),
			AgentID:        strings.ToLower(strings.TrimSpace(options.AgentID)),
			SourceRef:      absPath + "#fact:" + strconv.FormatInt(fact.ID, 10),
			TruthScore:     clamp(fact.TrustScore),
			UtilityScore:   legacyUtility(fact.RetrievalCount, fact.HelpfulCount),
			Metadata: map[string]any{
				"source":          "holographic",
				"legacy_fact_id":  fact.ID,
				"legacy_category": fact.Category,
				"retrieval_count": fact.RetrievalCount,
				"helpful_count":   fact.HelpfulCount,
				"created_at":      fact.CreatedAt,
				"updated_at":      fact.UpdatedAt,
			},
		})
		if err != nil {
			return result, fmt.Errorf("import fact %d: %w", fact.ID, err)
		}
		result.MemoryIDs = append(result.MemoryIDs, memory.ID)
		if created {
			result.Imported++
		} else {
			result.Replayed++
		}
	}
	return result, nil
}

func openReadOnly(ctx context.Context, path string) (*sql.DB, error) {
	uriPath := filepath.ToSlash(path)
	if filepath.VolumeName(path) != "" {
		uriPath = "/" + uriPath
	}
	uri := url.URL{Scheme: "file", Path: uriPath, RawQuery: "mode=ro"}
	database, err := sql.Open("sqlite", uri.String())
	if err != nil {
		return nil, fmt.Errorf("open Holographic database read-only: %w", err)
	}
	database.SetMaxOpenConns(1)
	if err := database.PingContext(ctx); err != nil {
		_ = database.Close()
		return nil, fmt.Errorf("open Holographic database read-only: %w", err)
	}
	if _, err := database.ExecContext(ctx, "PRAGMA query_only = ON"); err != nil {
		_ = database.Close()
		return nil, fmt.Errorf("protect Holographic database: %w", err)
	}
	return database, nil
}

func validateSchema(ctx context.Context, database *sql.DB) error {
	rows, err := database.QueryContext(ctx, "PRAGMA table_info(facts)")
	if err != nil {
		return fmt.Errorf("inspect Holographic schema: %w", err)
	}
	defer rows.Close()
	columns := make(map[string]bool)
	for rows.Next() {
		var index int
		var name, dataType string
		var notNull, primaryKey int
		var defaultValue any
		if err := rows.Scan(&index, &name, &dataType, &notNull, &defaultValue, &primaryKey); err != nil {
			return fmt.Errorf("read Holographic schema: %w", err)
		}
		columns[name] = true
	}
	for _, required := range []string{
		"fact_id", "content", "category", "tags", "trust_score",
		"retrieval_count", "helpful_count", "created_at", "updated_at",
	} {
		if !columns[required] {
			return fmt.Errorf("unsupported Holographic schema: facts.%s is missing", required)
		}
	}
	return rows.Err()
}

func readFacts(ctx context.Context, database *sql.DB) ([]legacyFact, error) {
	rows, err := database.QueryContext(ctx, `
SELECT fact_id, content, category, tags, trust_score,
       retrieval_count, helpful_count, created_at, updated_at
FROM facts
ORDER BY fact_id`)
	if err != nil {
		return nil, fmt.Errorf("read Holographic facts: %w", err)
	}
	defer rows.Close()
	facts := make([]legacyFact, 0)
	for rows.Next() {
		var fact legacyFact
		var createdAt, updatedAt any
		if err := rows.Scan(
			&fact.ID, &fact.Content, &fact.Category, &fact.Tags, &fact.TrustScore,
			&fact.RetrievalCount, &fact.HelpfulCount, &createdAt, &updatedAt,
		); err != nil {
			return nil, fmt.Errorf("scan Holographic fact: %w", err)
		}
		fact.CreatedAt = timeString(createdAt)
		fact.UpdatedAt = timeString(updatedAt)
		facts = append(facts, fact)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate Holographic facts: %w", err)
	}
	return facts, nil
}

func mapCategory(category, project string) (cortex.MemoryKind, cortex.Scope, string) {
	switch strings.ToLower(strings.TrimSpace(category)) {
	case "user_pref":
		return cortex.KindPreference, cortex.ScopeGlobal, ""
	case "project":
		if strings.TrimSpace(project) != "" {
			return cortex.KindFact, cortex.ScopeProject, strings.TrimSpace(project)
		}
	}
	return cortex.KindFact, cortex.ScopeGlobal, ""
}

func importTags(rawTags, category string) []string {
	seen := map[string]bool{"holographic": true}
	tags := []string{"holographic"}
	for _, candidate := range append(strings.Split(rawTags, ","), "legacy-category:"+category) {
		tag := strings.TrimSpace(candidate)
		if tag != "" && !seen[tag] {
			seen[tag] = true
			tags = append(tags, tag)
		}
	}
	return tags
}

func legacyUtility(retrievalCount, helpfulCount int) float64 {
	if retrievalCount <= 0 {
		return 0.5
	}
	return clamp(float64(helpfulCount+1) / float64(retrievalCount+2))
}

func titleFromContent(content string) string {
	runes := []rune(strings.TrimSpace(content))
	if len(runes) > 80 {
		return string(runes[:77]) + "..."
	}
	return string(runes)
}

func shortHash(value string) string {
	sum := sha256.Sum256([]byte(value))
	return hex.EncodeToString(sum[:8])
}

func timeString(value any) string {
	switch typed := value.(type) {
	case time.Time:
		return typed.UTC().Format(time.RFC3339Nano)
	case string:
		return typed
	case []byte:
		return string(typed)
	default:
		return fmt.Sprint(value)
	}
}

func clamp(value float64) float64 {
	if value < 0 {
		return 0
	}
	if value > 1 {
		return 1
	}
	return value
}
