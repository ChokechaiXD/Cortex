package holographic

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"os"
	"path/filepath"
	"testing"

	"cortex.local/cortex/internal/cortex"
	_ "modernc.org/sqlite"
)

func TestImportIsReadOnlyCandidateAndIdempotent(t *testing.T) {
	t.Parallel()

	legacyPath := filepath.Join(t.TempDir(), "memory_store.db")
	createLegacyDatabase(t, legacyPath)
	before := fileHash(t, legacyPath)

	hub, err := cortex.Open(cortex.Config{DatabasePath: filepath.Join(t.TempDir(), "cortex.db")})
	if err != nil {
		t.Fatalf("open Cortex: %v", err)
	}
	t.Cleanup(func() { _ = hub.Close() })

	result, err := Import(context.Background(), hub, Options{
		DatabasePath: legacyPath,
		AgentID:      "sola",
		Project:      "novelclaw",
	})
	if err != nil {
		t.Fatalf("import Holographic: %v", err)
	}
	if result.Imported != 2 || result.Replayed != 0 {
		t.Fatalf("first import result = %#v", result)
	}
	replayed, err := Import(context.Background(), hub, Options{
		DatabasePath: legacyPath,
		AgentID:      "sola",
		Project:      "novelclaw",
	})
	if err != nil {
		t.Fatalf("repeat import: %v", err)
	}
	if replayed.Imported != 0 || replayed.Replayed != 2 {
		t.Fatalf("repeat import result = %#v", replayed)
	}

	recalled, err := hub.Recall(context.Background(), cortex.RecallQuery{
		AgentID:           "mika",
		Text:              "dark mode editor",
		IncludeCandidates: true,
		Limit:             5,
	})
	if err != nil {
		t.Fatalf("recall imported candidate: %v", err)
	}
	if len(recalled.Items) != 1 {
		t.Fatalf("recalled items = %#v", recalled.Items)
	}
	memory := recalled.Items[0].Memory
	if memory.Lifecycle != cortex.LifecycleCandidate || memory.Kind != cortex.KindPreference {
		t.Fatalf("imported memory = %#v", memory)
	}
	if memory.TruthScore != 0.9 || memory.UtilityScore < 0.66 || memory.UtilityScore > 0.67 {
		t.Fatalf("imported scores truth=%v utility=%v", memory.TruthScore, memory.UtilityScore)
	}
	history, err := hub.History(context.Background(), cortex.HistoryQuery{MemoryID: memory.ID, AgentID: "mika"})
	if err != nil {
		t.Fatalf("import history: %v", err)
	}
	if len(history) < 1 || history[0].Type != cortex.EventImported {
		t.Fatalf("import history = %#v", history)
	}

	after := fileHash(t, legacyPath)
	if before != after {
		t.Fatal("legacy Holographic database changed during import")
	}
}

func createLegacyDatabase(t *testing.T, path string) {
	t.Helper()
	db, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatalf("open legacy db: %v", err)
	}
	defer db.Close()
	_, err = db.Exec(`
CREATE TABLE facts (
    fact_id INTEGER PRIMARY KEY AUTOINCREMENT,
    content TEXT NOT NULL UNIQUE,
    category TEXT DEFAULT 'general',
    tags TEXT DEFAULT '',
    trust_score REAL DEFAULT 0.5,
    retrieval_count INTEGER DEFAULT 0,
    helpful_count INTEGER DEFAULT 0,
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    hrr_vector BLOB
);
INSERT INTO facts(content, category, tags, trust_score, retrieval_count, helpful_count)
VALUES
    ('User prefers dark mode in the editor', 'user_pref', 'ui,editor', 0.9, 4, 3),
    ('Force translation once failed without a backup', 'project', 'translation,backup', 0.2, 0, 0);
`)
	if err != nil {
		t.Fatalf("create legacy schema: %v", err)
	}
}

func fileHash(t *testing.T, path string) [sha256.Size]byte {
	t.Helper()
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("hash file: %v", err)
	}
	return sha256.Sum256(raw)
}
