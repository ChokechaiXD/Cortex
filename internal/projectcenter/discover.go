package projectcenter

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"cortex.local/cortex/internal/hope"
)

type Catalog struct {
	store *hope.Hub
}

func New(store *hope.Hub) *Catalog { return &Catalog{store: store} }

func (catalog *Catalog) Open(ctx context.Context, id string) error {
	project, err := catalog.store.Project(ctx, id)
	if err != nil {
		return err
	}
	if !project.Available {
		return fmt.Errorf("project folder is unavailable")
	}
	return openFolder(project.Path)
}

func (catalog *Catalog) Discover(ctx context.Context) ([]hope.Project, error) {
	roots, err := catalog.store.ProjectRoots(ctx)
	if err != nil {
		return nil, err
	}
	seen := map[string]bool{}
	var found []hope.Project
	for _, root := range roots {
		if err := walk(root, 0, func(path string) error {
			key := strings.ToLower(filepath.Clean(path))
			if seen[key] {
				return nil
			}
			seen[key] = true
			project := hope.Project{
				ID: slug(filepath.Base(path)), Name: filepath.Base(path), Path: path,
				Kind: projectKind(path), Available: true, Active: true,
			}
			if err := catalog.store.SaveProject(ctx, project); err != nil {
				return err
			}
			found = append(found, project)
			return nil
		}); err != nil {
			return nil, fmt.Errorf("discover projects in %s: %w", root, err)
		}
	}
	return found, nil
}

func walk(path string, depth int, found func(string) error) error {
	if depth > 3 {
		return nil
	}
	entries, err := os.ReadDir(path)
	if err != nil {
		return nil
	}
	if depth > 0 && isProject(path, entries) {
		return found(path)
	}
	for _, entry := range entries {
		if !entry.IsDir() || skipDir(entry.Name()) {
			continue
		}
		if err := walk(filepath.Join(path, entry.Name()), depth+1, found); err != nil {
			return err
		}
	}
	return nil
}

func isProject(path string, entries []os.DirEntry) bool {
	markers := map[string]bool{
		".git": true, "go.mod": true, "package.json": true, "pyproject.toml": true,
		"requirements.txt": true, "Cargo.toml": true, "composer.json": true,
	}
	for _, entry := range entries {
		if markers[entry.Name()] {
			return true
		}
	}
	_, err := os.Stat(filepath.Join(path, ".hermes", "project.json"))
	return err == nil
}

func projectKind(path string) string {
	markers := []struct{ file, kind string }{
		{"go.mod", "Go"}, {"package.json", "Node"}, {"pyproject.toml", "Python"},
		{"requirements.txt", "Python"}, {"Cargo.toml", "Rust"}, {"composer.json", "PHP"},
	}
	for _, marker := range markers {
		if _, err := os.Stat(filepath.Join(path, marker.file)); err == nil {
			return marker.kind
		}
	}
	return "Git"
}

func skipDir(name string) bool {
	if strings.HasPrefix(name, ".") {
		return name != ".config"
	}
	switch strings.ToLower(name) {
	case "node_modules", "vendor", "dist", "build", "bin", "obj", "__pycache__", ".venv", "venv":
		return true
	default:
		return false
	}
}

func slug(value string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	var out strings.Builder
	for _, r := range value {
		if r >= 'a' && r <= 'z' || r >= '0' && r <= '9' {
			out.WriteRune(r)
		} else if out.Len() > 0 && !strings.HasSuffix(out.String(), "-") {
			out.WriteByte('-')
		}
	}
	return strings.Trim(out.String(), "-")
}
