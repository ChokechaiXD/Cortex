package cortex

import (
	"context"
	"database/sql"
	"fmt"

	_ "modernc.org/sqlite"
)

const connectionPragmas = `
PRAGMA foreign_keys = ON;
PRAGMA journal_mode = WAL;
PRAGMA busy_timeout = 5000;
`

const schemaV1 = `
CREATE TABLE IF NOT EXISTS memories (
    id TEXT PRIMARY KEY,
    kind TEXT NOT NULL,
    scope TEXT NOT NULL,
    scope_key TEXT NOT NULL DEFAULT '',
    memory_key TEXT NOT NULL,
    lifecycle TEXT NOT NULL,
    truth_score REAL NOT NULL,
    utility_score REAL NOT NULL,
    created_by TEXT NOT NULL,
    current_revision INTEGER NOT NULL,
    created_at TEXT NOT NULL,
    updated_at TEXT NOT NULL
);

CREATE INDEX IF NOT EXISTS memories_scope_idx
ON memories(scope, scope_key, lifecycle);

CREATE TABLE IF NOT EXISTS memory_revisions (
    memory_id TEXT NOT NULL REFERENCES memories(id),
    revision INTEGER NOT NULL,
    title TEXT NOT NULL,
    content TEXT NOT NULL,
    tags_json TEXT NOT NULL,
    session_id TEXT NOT NULL DEFAULT '',
    source_ref TEXT NOT NULL DEFAULT '',
    created_by TEXT NOT NULL,
    created_at TEXT NOT NULL,
    PRIMARY KEY(memory_id, revision)
);

CREATE TABLE IF NOT EXISTS memory_events (
    id TEXT PRIMARY KEY,
    memory_id TEXT NOT NULL REFERENCES memories(id),
    event_type TEXT NOT NULL,
    actor_id TEXT NOT NULL,
    session_id TEXT NOT NULL DEFAULT '',
    reason TEXT NOT NULL DEFAULT '',
    metadata_json TEXT NOT NULL DEFAULT '{}',
    created_at TEXT NOT NULL
);

CREATE INDEX IF NOT EXISTS memory_events_history_idx
ON memory_events(memory_id, created_at);

CREATE TABLE IF NOT EXISTS requests (
    idempotency_key TEXT PRIMARY KEY,
    operation TEXT NOT NULL,
    resource_id TEXT NOT NULL,
    created_at TEXT NOT NULL
);

CREATE TABLE IF NOT EXISTS recalls (
    id TEXT PRIMARY KEY,
    query_text TEXT NOT NULL,
    agent_id TEXT NOT NULL,
    session_id TEXT NOT NULL DEFAULT '',
    created_at TEXT NOT NULL
);

CREATE TABLE IF NOT EXISTS recall_items (
    recall_id TEXT NOT NULL REFERENCES recalls(id),
    memory_id TEXT NOT NULL REFERENCES memories(id),
    rank INTEGER NOT NULL,
    score REAL NOT NULL,
    PRIMARY KEY(recall_id, memory_id)
);

CREATE VIRTUAL TABLE IF NOT EXISTS memory_fts USING fts5(
    memory_id UNINDEXED,
    title,
    content,
    tags
);
`

var schemaMigrations = []string{schemaV1}

func openDatabase(ctx context.Context, path string) (*sql.DB, error) {
	db, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, fmt.Errorf("open sqlite: %w", err)
	}
	db.SetMaxOpenConns(1)
	if err := db.PingContext(ctx); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("ping sqlite: %w", err)
	}
	if _, err := db.ExecContext(ctx, connectionPragmas); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("configure sqlite: %w", err)
	}
	if err := applyMigrations(ctx, db); err != nil {
		_ = db.Close()
		return nil, err
	}
	return db, nil
}

func applyMigrations(ctx context.Context, db *sql.DB) error {
	var version int
	if err := db.QueryRowContext(ctx, "PRAGMA user_version").Scan(&version); err != nil {
		return fmt.Errorf("read schema version: %w", err)
	}
	if version > len(schemaMigrations) {
		return fmt.Errorf("database schema version %d is newer than supported version %d", version, len(schemaMigrations))
	}
	for index := version; index < len(schemaMigrations); index++ {
		tx, err := db.BeginTx(ctx, nil)
		if err != nil {
			return fmt.Errorf("begin schema migration %d: %w", index+1, err)
		}
		if _, err := tx.ExecContext(ctx, schemaMigrations[index]); err != nil {
			_ = tx.Rollback()
			return fmt.Errorf("apply schema migration %d: %w", index+1, err)
		}
		if _, err := tx.ExecContext(ctx, fmt.Sprintf("PRAGMA user_version = %d", index+1)); err != nil {
			_ = tx.Rollback()
			return fmt.Errorf("record schema migration %d: %w", index+1, err)
		}
		if err := tx.Commit(); err != nil {
			return fmt.Errorf("commit schema migration %d: %w", index+1, err)
		}
	}
	return nil
}
