package config

import (
	"fmt"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"
)

// configFileMode is intentionally restrictive: the config can list hostnames
// and shell commands that reveal infrastructure layout, so we keep it
// readable only by the owner.
const configFileMode os.FileMode = 0o600

// configDirMode mirrors the standard XDG config directory permissions.
const configDirMode os.FileMode = 0o755

// SchemaURL is the published JSON Schema for the config format. Scaffolded
// configs start with a yaml-language-server modeline pointing here so
// LSP-aware editors offer completion and validation while editing.
const SchemaURL = "https://raw.githubusercontent.com/bthall/mox/main/schema/mox.schema.json"

// exampleConfig is the configuration scaffolded by `mox init`.
func exampleConfig() *Config {
	return &Config{
		Layouts: map[string]*Layout{
			"two-pane": {
				Name: "two-pane",
				Panes: []*Pane{
					{Split: SplitRoot, Commands: []string{"# main pane"}},
					{Split: SplitVertical, Size: 30, Commands: []string{"# side pane"}},
				},
			},
		},
		Sessions: map[string]*Session{
			"example": {
				Root:  "~",
				Hosts: []string{"localhost"},
				Commands: []string{
					"echo 'Welcome to mox!'",
					"echo 'Edit your config to customize.'",
				},
			},
			"dev": {
				Root: "~/projects",
				Windows: []*Window{
					{Name: "editor", Hosts: []string{"api", "web", "worker"}},
					{Name: "logs", Layout: "two-pane"},
				},
			},
		},
	}
}

// Init creates the config directory and writes a default config file.
// Returns an error if the file already exists and force is false.
func Init(force bool) (string, error) {
	configPath := DefaultConfigPath()
	if configPath == "" {
		return "", fmt.Errorf("could not determine config path: $HOME and $XDG_CONFIG_HOME are unset")
	}
	configDir := filepath.Dir(configPath)

	if err := os.MkdirAll(configDir, configDirMode); err != nil {
		return "", fmt.Errorf("create config directory %s: %w", configDir, err)
	}

	if !force && Exists(configPath) {
		return "", fmt.Errorf("config already exists at %s (use --force to overwrite)", configPath)
	}

	data, err := yaml.Marshal(exampleConfig())
	if err != nil {
		return "", fmt.Errorf("marshal default config: %w", err)
	}
	data = append([]byte("# yaml-language-server: $schema="+SchemaURL+"\n\n"), data...)

	if err := os.WriteFile(configPath, data, configFileMode); err != nil {
		return "", fmt.Errorf("write %s: %w", configPath, err)
	}
	return configPath, nil
}
