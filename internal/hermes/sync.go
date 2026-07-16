package hermes

import (
	"encoding/json"
	"fmt"
	"io/fs"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strings"

	hermesconnector "cortex.local/cortex/connectors/hermes"
	"cortex.local/cortex/internal/config"
	"gopkg.in/yaml.v3"
)

type SyncOptions struct {
	HermesHome string
	DataDir    string
	ServerURL  string
	RootAgent  string
	Activate   bool
}

type SyncedProfile struct {
	AgentID string `json:"agent_id"`
	Home    string `json:"home"`
}

type SyncResult struct {
	Profiles []SyncedProfile `json:"profiles"`
}

type connectorConfig struct {
	URL     string `json:"url"`
	Token   string `json:"token"`
	AgentID string `json:"agent_id"`
}

func Sync(options SyncOptions) (SyncResult, error) {
	if strings.TrimSpace(options.HermesHome) == "" || strings.TrimSpace(options.DataDir) == "" {
		return SyncResult{}, fmt.Errorf("Hermes home and Cortex data directory are required")
	}
	if options.RootAgent == "" {
		options.RootAgent = "mika"
	}
	if err := validateServerURL(options.ServerURL); err != nil {
		return SyncResult{}, err
	}
	cortexConfig, err := config.Load(options.DataDir)
	if err != nil {
		return SyncResult{}, err
	}
	profiles, err := discoverProfiles(options.HermesHome, options.RootAgent)
	if err != nil {
		return SyncResult{}, err
	}
	result := SyncResult{Profiles: make([]SyncedProfile, 0, len(profiles))}
	for _, profile := range profiles {
		token, reusable := existingToken(profile.Home, profile.AgentID, cortexConfig)
		if !reusable {
			if cortexConfig.HasAgent(profile.AgentID) {
				token, err = config.IssueToken(options.DataDir, profile.AgentID)
			} else {
				token, err = config.AddAgent(options.DataDir, profile.AgentID, false)
			}
			if err != nil {
				return SyncResult{}, fmt.Errorf("create token for %s: %w", profile.AgentID, err)
			}
			cortexConfig, err = config.Load(options.DataDir)
			if err != nil {
				return SyncResult{}, err
			}
		}
		if err := installProvider(profile.Home); err != nil {
			return SyncResult{}, fmt.Errorf("install connector for %s: %w", profile.AgentID, err)
		}
		if err := writeConnectorConfig(profile.Home, connectorConfig{
			URL: options.ServerURL, Token: token, AgentID: profile.AgentID,
		}); err != nil {
			return SyncResult{}, fmt.Errorf("configure connector for %s: %w", profile.AgentID, err)
		}
		if options.Activate {
			if err := activateProvider(profile.Home); err != nil {
				return SyncResult{}, fmt.Errorf("activate connector for %s: %w", profile.AgentID, err)
			}
		}
		result.Profiles = append(result.Profiles, profile)
	}
	return result, nil
}

func validateServerURL(rawURL string) error {
	parsed, err := url.Parse(rawURL)
	if err != nil || (parsed.Scheme != "http" && parsed.Scheme != "https") || parsed.Host == "" {
		return fmt.Errorf("Cortex server URL must be an absolute http(s) URL")
	}
	return nil
}

func discoverProfiles(hermesHome, rootAgent string) ([]SyncedProfile, error) {
	profiles := []SyncedProfile{{AgentID: strings.ToLower(rootAgent), Home: hermesHome}}
	entries, err := os.ReadDir(filepath.Join(hermesHome, "profiles"))
	if err != nil && !os.IsNotExist(err) {
		return nil, fmt.Errorf("list Hermes profiles: %w", err)
	}
	for _, entry := range entries {
		if !entry.IsDir() || strings.HasPrefix(entry.Name(), ".") {
			continue
		}
		profiles = append(profiles, SyncedProfile{
			AgentID: strings.ToLower(entry.Name()),
			Home:    filepath.Join(hermesHome, "profiles", entry.Name()),
		})
	}
	sort.Slice(profiles[1:], func(i, j int) bool {
		return profiles[i+1].AgentID < profiles[j+1].AgentID
	})
	return profiles, nil
}

func existingToken(home, agentID string, cortexConfig config.File) (string, bool) {
	raw, err := os.ReadFile(filepath.Join(home, "cortex.json"))
	if err != nil {
		return "", false
	}
	var existing connectorConfig
	if json.Unmarshal(raw, &existing) != nil || existing.AgentID != agentID {
		return "", false
	}
	authenticated, ok := cortexConfig.Authenticate(existing.Token)
	return existing.Token, ok && authenticated == agentID
}

func installProvider(home string) error {
	destinationRoot := filepath.Join(home, "plugins", "cortex")
	return fs.WalkDir(hermesconnector.ProviderFiles, "provider", func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		relative, err := filepath.Rel("provider", path)
		if err != nil || relative == "." {
			return err
		}
		destination := filepath.Join(destinationRoot, filepath.FromSlash(relative))
		if entry.IsDir() {
			return os.MkdirAll(destination, 0o700)
		}
		content, err := hermesconnector.ProviderFiles.ReadFile(path)
		if err != nil {
			return err
		}
		return writeAtomic(destination, content, 0o600)
	})
}

func writeConnectorConfig(home string, value connectorConfig) error {
	raw, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return err
	}
	raw = append(raw, '\n')
	return writeAtomic(filepath.Join(home, "cortex.json"), raw, 0o600)
}

func activateProvider(home string) error {
	path := filepath.Join(home, "config.yaml")
	raw, err := os.ReadFile(path)
	if err != nil && !os.IsNotExist(err) {
		return err
	}
	if err == nil {
		backup := path + ".cortex.bak"
		if _, backupErr := os.Stat(backup); os.IsNotExist(backupErr) {
			if err := writeAtomic(backup, raw, 0o600); err != nil {
				return fmt.Errorf("backup Hermes config: %w", err)
			}
		}
	}
	document := &yaml.Node{Kind: yaml.DocumentNode}
	if len(raw) > 0 {
		if err := yaml.Unmarshal(raw, document); err != nil {
			return fmt.Errorf("decode Hermes config: %w", err)
		}
	}
	if len(document.Content) == 0 {
		document.Content = []*yaml.Node{{Kind: yaml.MappingNode, Tag: "!!map"}}
	}
	root := document.Content[0]
	if root.Kind != yaml.MappingNode {
		return fmt.Errorf("Hermes config root must be a mapping")
	}
	memory := mappingValue(root, "memory")
	if memory == nil {
		memory = &yaml.Node{Kind: yaml.MappingNode, Tag: "!!map"}
		appendMapping(root, "memory", memory)
	} else if memory.Kind != yaml.MappingNode {
		memory.Kind = yaml.MappingNode
		memory.Tag = "!!map"
		memory.Value = ""
		memory.Content = nil
	}
	setScalar(memory, "provider", "cortex")
	encoded, err := yaml.Marshal(document)
	if err != nil {
		return fmt.Errorf("encode Hermes config: %w", err)
	}
	return writeAtomic(path, encoded, 0o600)
}

func mappingValue(mapping *yaml.Node, key string) *yaml.Node {
	for index := 0; index+1 < len(mapping.Content); index += 2 {
		if mapping.Content[index].Value == key {
			return mapping.Content[index+1]
		}
	}
	return nil
}

func appendMapping(mapping *yaml.Node, key string, value *yaml.Node) {
	mapping.Content = append(mapping.Content,
		&yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: key}, value,
	)
}

func setScalar(mapping *yaml.Node, key, value string) {
	if existing := mappingValue(mapping, key); existing != nil {
		existing.Kind = yaml.ScalarNode
		existing.Tag = "!!str"
		existing.Value = value
		existing.Content = nil
		return
	}
	appendMapping(mapping, key, &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: value})
}

func writeAtomic(path string, content []byte, mode fs.FileMode) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	temporary, err := os.CreateTemp(filepath.Dir(path), ".cortex-*.tmp")
	if err != nil {
		return err
	}
	temporaryPath := temporary.Name()
	defer func() { _ = os.Remove(temporaryPath) }()
	if err := temporary.Chmod(mode); err != nil {
		_ = temporary.Close()
		return err
	}
	if _, err := temporary.Write(content); err != nil {
		_ = temporary.Close()
		return err
	}
	if err := temporary.Close(); err != nil {
		return err
	}
	if err := os.Rename(temporaryPath, path); err != nil {
		return err
	}
	return nil
}
