package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestInitializeAndAddAgent(t *testing.T) {
	t.Parallel()

	dataDir := t.TempDir()
	created, mikaToken, err := Initialize(dataDir, "mika", "127.0.0.1:7777")
	if err != nil {
		t.Fatalf("initialize config: %v", err)
	}
	if mikaToken == "" || !created.IsAdmin("mika") {
		t.Fatalf("initial config = %#v, token empty=%v", created, mikaToken == "")
	}
	if agentID, ok := created.Authenticate(mikaToken); !ok || agentID != "mika" {
		t.Fatalf("authenticate initial token = %q, %v", agentID, ok)
	}

	raw, err := os.ReadFile(filepath.Join(dataDir, FileName))
	if err != nil {
		t.Fatalf("read config: %v", err)
	}
	if strings.Contains(string(raw), mikaToken) {
		t.Fatal("config persisted the raw bearer token")
	}

	solaToken, err := AddAgent(dataDir, "sola", false)
	if err != nil {
		t.Fatalf("add agent: %v", err)
	}
	loaded, err := Load(dataDir)
	if err != nil {
		t.Fatalf("load config: %v", err)
	}
	if agentID, ok := loaded.Authenticate(solaToken); !ok || agentID != "sola" {
		t.Fatalf("authenticate added token = %q, %v", agentID, ok)
	}
	if _, err := AddAgent(dataDir, "sola", false); err == nil {
		t.Fatal("adding duplicate agent succeeded")
	}
	secondMikaToken, err := IssueToken(dataDir, "mika")
	if err != nil {
		t.Fatalf("issue additional token: %v", err)
	}
	loaded, err = Load(dataDir)
	if err != nil {
		t.Fatalf("reload config: %v", err)
	}
	if agentID, ok := loaded.Authenticate(secondMikaToken); !ok || agentID != "mika" {
		t.Fatalf("additional token authenticated as %q, %v", agentID, ok)
	}
}

func TestInitializeRefusesExistingConfig(t *testing.T) {
	t.Parallel()

	dataDir := t.TempDir()
	if _, _, err := Initialize(dataDir, "mika", "127.0.0.1:7777"); err != nil {
		t.Fatalf("first initialize: %v", err)
	}
	if _, _, err := Initialize(dataDir, "mika", "127.0.0.1:7777"); err == nil {
		t.Fatal("second initialize overwrote existing config")
	}
}
