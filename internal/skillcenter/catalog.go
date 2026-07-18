package skillcenter

import (
	"context"
	"encoding/json"
	"fmt"
	"io/fs"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"cortex.local/cortex/internal/hope"
	"gopkg.in/yaml.v3"
)

type Catalog struct {
	store       *hope.Hub
	canonical   string
	hermesShare string
}

type frontmatter struct {
	Name        string   `yaml:"name"`
	Description string   `yaml:"description"`
	Tags        []string `yaml:"tags"`
}

func New(store *hope.Hub, canonical, hermesShare string) *Catalog {
	return &Catalog{store: store, canonical: canonical, hermesShare: hermesShare}
}

func (catalog *Catalog) Sync(ctx context.Context) (int, error) {
	count := 0
	usage := catalog.loadUsage()
	for _, source := range []struct{ path, name string }{{catalog.canonical, "hope"}, {catalog.hermesShare, "hermes"}} {
		entries, err := os.ReadDir(source.path)
		if os.IsNotExist(err) {
			continue
		}
		if err != nil {
			return count, fmt.Errorf("read %s skills: %w", source.name, err)
		}
		for _, entry := range entries {
			if !entry.IsDir() {
				continue
			}
			path := filepath.Join(source.path, entry.Name(), "SKILL.md")
			skill, err := readSkill(path, entry.Name(), source.name)
			if err != nil {
				continue
			}
			if source.name == "hermes" {
				if existing, findErr := catalog.store.Skill(ctx, skill.ID); findErr == nil && existing.Source == "hope" {
					continue
				}
			}
			if existing, findErr := catalog.store.Skill(ctx, skill.ID); findErr == nil {
				skill.UseCount, skill.SuccessCount, skill.FailureCount = existing.UseCount, existing.SuccessCount, existing.FailureCount
			}
			if tracked, ok := usage[skill.ID]; ok {
				skill.UseCount = max(skill.UseCount, tracked.UseCount)
			}
			if err := catalog.store.SaveSkill(ctx, skill); err != nil {
				return count, err
			}
			count++
		}
	}
	return count, nil
}

type usageRecord struct {
	UseCount int `json:"use_count"`
}

func (catalog *Catalog) loadUsage() map[string]usageRecord {
	result := map[string]usageRecord{}
	candidates := []string{
		filepath.Join(catalog.hermesShare, ".usage.json"),
		filepath.Join(filepath.Dir(catalog.hermesShare), "skills", ".usage.json"),
	}
	for _, path := range candidates {
		raw, err := os.ReadFile(path)
		if err != nil {
			continue
		}
		var direct map[string]usageRecord
		if json.Unmarshal(raw, &direct) == nil {
			for id, record := range direct {
				result[safeID(id)] = record
			}
		}
	}
	return result
}

func (catalog *Catalog) Create(ctx context.Context, skill hope.Skill, body string) (hope.Skill, error) {
	skill.ID = safeID(skill.ID)
	if skill.ID == "" {
		skill.ID = safeID(skill.Name)
	}
	if skill.ID == "" || strings.TrimSpace(skill.Name) == "" || strings.TrimSpace(skill.Description) == "" {
		return hope.Skill{}, fmt.Errorf("skill name and description are required")
	}
	directory := filepath.Join(catalog.canonical, skill.ID)
	if err := os.MkdirAll(directory, 0o700); err != nil {
		return hope.Skill{}, fmt.Errorf("create skill folder: %w", err)
	}
	path := filepath.Join(directory, "SKILL.md")
	content := renderSkill(skill, body)
	if err := writeAtomic(path, []byte(content)); err != nil {
		return hope.Skill{}, err
	}
	skill.Path = path
	skill.Source = "hope"
	skill.Enabled = true
	skill.UpdatedAt = time.Now().UTC()
	if err := catalog.store.SaveSkill(ctx, skill); err != nil {
		return hope.Skill{}, err
	}
	return skill, nil
}

func (catalog *Catalog) Read(ctx context.Context, id string) (hope.Skill, string, error) {
	skill, err := catalog.store.Skill(ctx, id)
	if err != nil {
		return hope.Skill{}, "", err
	}
	raw, err := os.ReadFile(skill.Path)
	if err != nil {
		return hope.Skill{}, "", err
	}
	_, body := parseFrontmatter(string(raw))
	return skill, body, nil
}

func (catalog *Catalog) Update(ctx context.Context, skill hope.Skill, body string) (hope.Skill, error) {
	current, err := catalog.store.Skill(ctx, skill.ID)
	if err != nil {
		return hope.Skill{}, err
	}
	if current.Source != "hope" {
		return hope.Skill{}, fmt.Errorf("Hermes-owned skills are read-only in HOPE")
	}
	skill.Path = current.Path
	skill.Source = "hope"
	skill.SourceURL = current.SourceURL
	skill.UseCount, skill.SuccessCount, skill.FailureCount = current.UseCount, current.SuccessCount, current.FailureCount
	if err := writeAtomic(skill.Path, []byte(renderSkill(skill, body))); err != nil {
		return hope.Skill{}, err
	}
	if err := catalog.store.SaveSkill(ctx, skill); err != nil {
		return hope.Skill{}, err
	}
	return skill, nil
}

func (catalog *Catalog) ImportGitHub(ctx context.Context, rawURL string) (hope.Skill, error) {
	repoURL, ref, subdir, err := parseGitHubSkillURL(rawURL)
	if err != nil {
		return hope.Skill{}, err
	}
	temp, err := os.MkdirTemp("", "hope-skill-*")
	if err != nil {
		return hope.Skill{}, err
	}
	defer func() { _ = os.RemoveAll(temp) }()
	args := []string{"clone", "--depth", "1"}
	if ref != "" {
		args = append(args, "--branch", ref)
	}
	args = append(args, repoURL, temp)
	cmd := exec.CommandContext(ctx, "git", args...)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return hope.Skill{}, fmt.Errorf("clone skill repository: %w: %s", err, boundedString(string(output), 500))
	}
	sourceDir := filepath.Join(temp, filepath.FromSlash(subdir))
	skillPath := filepath.Join(sourceDir, "SKILL.md")
	skill, err := readSkill(skillPath, filepath.Base(temp), "hope")
	if err != nil {
		return hope.Skill{}, fmt.Errorf("read imported SKILL.md: %w", err)
	}
	skill.ID = safeID(skill.Name)
	if skill.ID == "" {
		skill.ID = safeID(filepath.Base(strings.TrimSuffix(repoURL, ".git")))
	}
	destination := filepath.Join(catalog.canonical, skill.ID)
	if err := copySkillDirectory(sourceDir, destination); err != nil {
		return hope.Skill{}, err
	}
	skill.Path = filepath.Join(destination, "SKILL.md")
	skill.Source = "hope"
	skill.SourceURL = strings.TrimSpace(rawURL)
	skill.Enabled = true
	if err := catalog.store.SaveSkill(ctx, skill); err != nil {
		return hope.Skill{}, err
	}
	return skill, nil
}

func (catalog *Catalog) Deploy(ctx context.Context, id string) error {
	skill, err := catalog.store.Skill(ctx, id)
	if err != nil {
		return err
	}
	if skill.Source != "hope" {
		return fmt.Errorf("Hermes-owned skills are read-only in HOPE")
	}
	sourceDir := filepath.Dir(skill.Path)
	targetDir := filepath.Join(catalog.hermesShare, skill.ID)
	if err := os.MkdirAll(targetDir, 0o700); err != nil {
		return fmt.Errorf("create Hermes skill folder: %w", err)
	}
	raw, err := os.ReadFile(filepath.Join(sourceDir, "SKILL.md"))
	if err != nil {
		return err
	}
	if err := writeAtomic(filepath.Join(targetDir, "SKILL.md"), raw); err != nil {
		return err
	}
	manifest, _ := json.MarshalIndent(map[string]any{"source": "HOPE", "skill_id": skill.ID, "deployed_at": nowText()}, "", "  ")
	return writeAtomic(filepath.Join(targetDir, ".hope-manifest.json"), manifest)
}

func readSkill(path, fallbackID, source string) (hope.Skill, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return hope.Skill{}, err
	}
	meta, _ := parseFrontmatter(string(raw))
	if meta.Name == "" {
		meta.Name = fallbackID
	}
	info, _ := os.Stat(path)
	updated := time.Time{}
	if info != nil {
		updated = info.ModTime()
	}
	return hope.Skill{
		ID: safeID(fallbackID), Name: meta.Name, Description: meta.Description,
		Path: path, Source: source, Keywords: meta.Tags, Enabled: true, UpdatedAt: updated,
	}, nil
}

func parseGitHubSkillURL(raw string) (repoURL, ref, subdir string, err error) {
	parsed, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || parsed.Scheme != "https" || parsed.Host != "github.com" {
		return "", "", "", fmt.Errorf("GitHub URL must start with https://github.com/")
	}
	parts := strings.Split(strings.Trim(parsed.Path, "/"), "/")
	if len(parts) < 2 || parts[0] == "" || parts[1] == "" {
		return "", "", "", fmt.Errorf("GitHub URL must include owner and repository")
	}
	repoURL = "https://github.com/" + parts[0] + "/" + parts[1] + ".git"
	if len(parts) >= 4 && parts[2] == "tree" {
		ref = parts[3]
		if len(parts) > 4 {
			subdir = strings.Join(parts[4:], "/")
		}
	}
	return repoURL, ref, subdir, nil
}

func copySkillDirectory(source, destination string) error {
	info, err := os.Stat(filepath.Join(source, "SKILL.md"))
	if err != nil || info.IsDir() || info.Size() > 512*1024 {
		return fmt.Errorf("SKILL.md is missing or too large")
	}
	if err := os.RemoveAll(destination); err != nil {
		return err
	}
	if err := os.MkdirAll(destination, 0o700); err != nil {
		return err
	}
	return filepath.WalkDir(source, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		name := entry.Name()
		if name == ".git" || name == ".github" {
			if entry.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		info, err := entry.Info()
		if err != nil {
			return err
		}
		if info.Mode()&os.ModeSymlink != 0 {
			return fmt.Errorf("skill import rejects symlinks")
		}
		relative, err := filepath.Rel(source, path)
		if err != nil || strings.HasPrefix(relative, "..") {
			return fmt.Errorf("invalid skill path")
		}
		target := filepath.Join(destination, relative)
		if entry.IsDir() {
			return os.MkdirAll(target, 0o700)
		}
		if info.Size() > 512*1024 {
			return fmt.Errorf("skill file is too large: %s", relative)
		}
		raw, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		return writeAtomic(target, raw)
	})
}

func boundedString(value string, limit int) string {
	value = strings.TrimSpace(value)
	if len(value) <= limit {
		return value
	}
	return value[:limit]
}

func parseFrontmatter(content string) (frontmatter, string) {
	if !strings.HasPrefix(content, "---") {
		return frontmatter{}, content
	}
	parts := strings.SplitN(content, "---", 3)
	if len(parts) != 3 {
		return frontmatter{}, content
	}
	var meta frontmatter
	_ = yaml.Unmarshal([]byte(parts[1]), &meta)
	return meta, strings.TrimSpace(parts[2])
}

func renderSkill(skill hope.Skill, body string) string {
	keywords := append([]string(nil), skill.Keywords...)
	sort.Strings(keywords)
	meta := frontmatter{Name: strings.TrimSpace(skill.Name), Description: strings.TrimSpace(skill.Description), Tags: keywords}
	raw, _ := yaml.Marshal(meta)
	return "---\n" + string(raw) + "---\n\n" + strings.TrimSpace(body) + "\n"
}

func writeAtomic(path string, content []byte) error {
	temp, err := os.CreateTemp(filepath.Dir(path), ".hope-*.tmp")
	if err != nil {
		return err
	}
	name := temp.Name()
	defer func() { _ = os.Remove(name) }()
	if err := temp.Chmod(0o600); err != nil {
		_ = temp.Close()
		return err
	}
	if _, err := temp.Write(content); err != nil {
		_ = temp.Close()
		return err
	}
	if err := temp.Close(); err != nil {
		return err
	}
	return os.Rename(name, path)
}

func safeID(value string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	var out strings.Builder
	for _, r := range value {
		if r >= 'a' && r <= 'z' || r >= '0' && r <= '9' || r == '-' || r == '_' {
			out.WriteRune(r)
		} else if out.Len() > 0 {
			out.WriteByte('-')
		}
	}
	return strings.Trim(out.String(), "-")
}

func nowText() string { return time.Now().UTC().Format(time.RFC3339Nano) }
