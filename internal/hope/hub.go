package hope

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

type Hub struct {
	db *sql.DB
}

func Open(path, defaultProjectRoot string) (*Hub, error) {
	if strings.TrimSpace(path) == "" {
		return nil, fmt.Errorf("HOPE database path is required")
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return nil, fmt.Errorf("create HOPE data directory: %w", err)
	}
	db, err := openDatabase(context.Background(), path)
	if err != nil {
		return nil, err
	}
	hub := &Hub{db: db}
	if err := hub.seed(context.Background(), defaultProjectRoot); err != nil {
		_ = db.Close()
		return nil, err
	}
	return hub, nil
}

func (hub *Hub) Close() error { return hub.db.Close() }

func (hub *Hub) seed(ctx context.Context, root string) error {
	tx, err := hub.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	agents := []Agent{
		{ID: "mika", Name: "MIKA", Role: "Orchestrator", Profile: "default", Enabled: true},
		{ID: "sora", Name: "Sora", Role: "Coding", Profile: "sora", Enabled: true},
		{ID: "nua", Name: "Nua", Role: "Research", Profile: "nua", Enabled: true},
		{ID: "aura", Name: "Aura", Role: "Image", Profile: "aura", Enabled: true},
		{ID: "nari", Name: "Nari", Role: "Assistant", Profile: "nari", Enabled: true},
	}
	for _, agent := range agents {
		if _, err := tx.ExecContext(ctx, `INSERT OR IGNORE INTO agents(id,name,role,profile,enabled) VALUES(?,?,?,?,?)`,
			agent.ID, agent.Name, agent.Role, agent.Profile, boolInt(agent.Enabled)); err != nil {
			return fmt.Errorf("seed agent %s: %w", agent.ID, err)
		}
	}
	modes := []WorkMode{
		{ID: "daily", Name: "Daily", Description: "MIKA พร้อม 9Router สำหรับงานทั่วไป", Integrations: []string{"9router"}, Agents: []string{"mika"}, OpenTelegram: true},
		{ID: "code", Name: "Code", Description: "Sora และเครื่องมือสำหรับงานพัฒนา", Integrations: []string{"9router"}, Agents: []string{"sora"}, OpenTelegram: true},
		{ID: "research", Name: "Research", Description: "Nua พร้อม 9Router สำหรับค้นคว้า", Integrations: []string{"9router"}, Agents: []string{"nua"}, OpenTelegram: true},
	}
	for _, mode := range modes {
		integrations, _ := encodeStrings(mode.Integrations)
		agentIDs, _ := encodeStrings(mode.Agents)
		if _, err := tx.ExecContext(ctx, `INSERT OR IGNORE INTO work_modes(id,name,description,integrations_json,agents_json,open_telegram) VALUES(?,?,?,?,?,?)`,
			mode.ID, mode.Name, mode.Description, integrations, agentIDs, boolInt(mode.OpenTelegram)); err != nil {
			return fmt.Errorf("seed work mode %s: %w", mode.ID, err)
		}
	}
	root = filepath.Clean(strings.TrimSpace(root))
	if root != "." && root != "" {
		if _, err := tx.ExecContext(ctx, `INSERT OR IGNORE INTO project_roots(path) VALUES(?)`, root); err != nil {
			return fmt.Errorf("seed project root: %w", err)
		}
	}
	return tx.Commit()
}

func boolInt(value bool) int {
	if value {
		return 1
	}
	return 0
}

func nowText() string { return time.Now().UTC().Format(time.RFC3339Nano) }
