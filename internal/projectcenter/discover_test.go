package projectcenter

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"cortex.local/cortex/internal/hope"
)

func TestDiscoverIsBoundedAndHandlesDuplicateFolderNames(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	writeMarker(t, filepath.Join(root, "team-a", "common", "go.mod"))
	writeMarker(t, filepath.Join(root, "team-b", "common", "package.json"))
	writeMarker(t, filepath.Join(root, "node_modules", "ghost", "go.mod"))
	hub, err := hope.Open(filepath.Join(t.TempDir(), "hope.db"), root)
	if err != nil {
		t.Fatalf("open HOPE: %v", err)
	}
	t.Cleanup(func() { _ = hub.Close() })
	if _, err := New(hub).Discover(context.Background()); err != nil {
		t.Fatalf("discover: %v", err)
	}
	projects, err := hub.Projects(context.Background())
	if err != nil {
		t.Fatalf("list projects: %v", err)
	}
	if len(projects) != 2 {
		t.Fatalf("projects=%#v, want two and no node_modules project", projects)
	}
	if projects[0].ID == projects[1].ID {
		t.Fatalf("duplicate project ids: %#v", projects)
	}
	for _, project := range projects {
		if !project.Available || project.Name != "common" {
			t.Fatalf("unexpected project: %#v", project)
		}
	}
}

func writeMarker(t *testing.T, path string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatalf("create marker directory: %v", err)
	}
	if err := os.WriteFile(path, []byte("test"), 0o600); err != nil {
		t.Fatalf("write marker: %v", err)
	}
}
