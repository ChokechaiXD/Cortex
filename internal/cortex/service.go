package cortex

import (
	"context"
	"database/sql"
	"fmt"
	"slices"
)

type Config struct {
	DatabasePath string
	AdminAgents  []string
}

type Hub struct {
	db          *sql.DB
	adminAgents []string
}

func Open(config Config) (*Hub, error) {
	if config.DatabasePath == "" {
		return nil, fmt.Errorf("%w: database path is required", ErrInvalidInput)
	}
	db, err := openDatabase(context.Background(), config.DatabasePath)
	if err != nil {
		return nil, err
	}
	admins := slices.Clone(config.AdminAgents)
	if len(admins) == 0 {
		admins = []string{"mika"}
	}
	return &Hub{db: db, adminAgents: admins}, nil
}

func (hub *Hub) Close() error {
	return hub.db.Close()
}

func (hub *Hub) isAdmin(agentID string) bool {
	return slices.Contains(hub.adminAgents, agentID)
}

func (hub *Hub) CanGovern(agentID string) bool {
	return hub.isAdmin(agentID)
}
