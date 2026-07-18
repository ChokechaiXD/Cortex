package skillcenter

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"cortex.local/cortex/internal/hope"
)

func TestHopeSkillCanBeEditedAndDeployedAtomically(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	hub, err := hope.Open(filepath.Join(root, "hope.db"), "")
	if err != nil {
		t.Fatalf("open HOPE: %v", err)
	}
	t.Cleanup(func() { _ = hub.Close() })
	catalog := New(hub, filepath.Join(root, "hope-skills"), filepath.Join(root, "hermes-skills"))
	skill := hope.Skill{ID: "api-design", Name: "API design", Description: "Design stable contracts", Keywords: []string{"api"}}
	if _, err := catalog.Create(context.Background(), skill, "Version one"); err != nil {
		t.Fatalf("create skill: %v", err)
	}
	if _, err := catalog.Create(context.Background(), skill, "Version two"); err != nil {
		t.Fatalf("edit skill: %v", err)
	}
	if err := catalog.Deploy(context.Background(), skill.ID); err != nil {
		t.Fatalf("deploy skill: %v", err)
	}
	if err := catalog.Deploy(context.Background(), skill.ID); err != nil {
		t.Fatalf("redeploy skill: %v", err)
	}
	raw, err := os.ReadFile(filepath.Join(root, "hermes-skills", skill.ID, "SKILL.md"))
	if err != nil {
		t.Fatalf("read deployed skill: %v", err)
	}
	if !strings.Contains(string(raw), "Version two") || strings.Contains(string(raw), "Version one") {
		t.Fatalf("deployed content=%q", raw)
	}
	if _, err := os.Stat(filepath.Join(root, "hermes-skills", skill.ID, ".hope-manifest.json")); err != nil {
		t.Fatalf("manifest missing: %v", err)
	}
	if _, err := catalog.Sync(context.Background()); err != nil {
		t.Fatalf("sync catalog: %v", err)
	}
	stored, err := hub.Skill(context.Background(), skill.ID)
	if err != nil || stored.Source != "hope" {
		t.Fatalf("stored skill=%#v err=%v", stored, err)
	}
}
