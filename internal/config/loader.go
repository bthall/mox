package config

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"slices"

	"gopkg.in/yaml.v3"
)

// DefaultConfigPath returns the default configuration file path, honoring
// $XDG_CONFIG_HOME (per the XDG Base Directory Specification). Returns the
// empty string only when neither $XDG_CONFIG_HOME nor $HOME is set.
func DefaultConfigPath() string {
	if base := os.Getenv("XDG_CONFIG_HOME"); base != "" {
		return filepath.Join(base, "mox", "config.yml")
	}
	home, err := os.UserHomeDir()
	if err != nil || home == "" {
		return ""
	}
	return filepath.Join(home, ".config", "mox", "config.yml")
}

// ResolvePath expands a user-supplied path: ~ becomes the home directory,
// and a relative path is left relative to the working directory.
func ResolvePath(path string) string {
	if path == "" {
		return path
	}
	if path == "~" {
		if home, err := os.UserHomeDir(); err == nil {
			return home
		}
		return path
	}
	if len(path) >= 2 && path[:2] == "~/" {
		if home, err := os.UserHomeDir(); err == nil {
			return filepath.Join(home, path[2:])
		}
	}
	return path
}

// Load loads, parses, and validates the configuration at the given path.
// An empty path means "use DefaultConfigPath()". The path is tilde-expanded.
func Load(path string) (*Config, error) {
	if path == "" {
		path = DefaultConfigPath()
	}
	path = ResolvePath(path)

	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read config %s: %w", path, err)
	}

	// Strict decode: unknown YAML keys are an error so that typos like
	// `hots:` instead of `hosts:` fail loudly instead of silently skipping.
	dec := yaml.NewDecoder(bytes.NewReader(data))
	dec.KnownFields(true)
	var cfg Config
	if err := dec.Decode(&cfg); err != nil {
		return nil, fmt.Errorf("parse config %s: %w", path, err)
	}

	if err := cfg.Validate(); err != nil {
		return nil, fmt.Errorf("invalid config %s: %w", path, err)
	}
	return &cfg, nil
}

// Exists reports whether a configuration file is present at path.
// An empty path resolves to DefaultConfigPath(). Symlinks are followed.
func Exists(path string) bool {
	if path == "" {
		path = DefaultConfigPath()
	}
	path = ResolvePath(path)
	_, err := os.Stat(path)
	return err == nil
}

// GetSession retrieves a session by name.
func (c *Config) GetSession(name string) (*Session, bool) {
	session, ok := c.Sessions[name]
	return session, ok
}

// GetLayout retrieves a layout by name.
func (c *Config) GetLayout(name string) (*Layout, bool) {
	layout, ok := c.Layouts[name]
	return layout, ok
}

// ListSessionNames returns all session names sorted alphabetically.
func (c *Config) ListSessionNames() []string {
	names := make([]string, 0, len(c.Sessions))
	for name := range c.Sessions {
		names = append(names, name)
	}
	slices.Sort(names)
	return names
}
