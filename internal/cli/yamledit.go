package cli

// YAML node surgery shared by 'mox import' (append a session) and the config
// editor (replace/rename/remove sessions). Operating on yaml.v3 Nodes keeps
// the rest of the file — ordering, comments on untouched entries, the
// modeline — byte-for-byte intact.

import (
	"bytes"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"

	"github.com/bthall/mox/internal/config"
)

// findOrCreateMapKey returns the value Node for a given key in a MappingNode,
// creating an empty MappingNode if the key doesn't exist yet.
func findOrCreateMapKey(m *yaml.Node, key string) *yaml.Node {
	for i := 0; i+1 < len(m.Content); i += 2 {
		if m.Content[i].Value == key {
			return m.Content[i+1]
		}
	}
	v := &yaml.Node{Kind: yaml.MappingNode}
	m.Content = append(m.Content,
		&yaml.Node{Kind: yaml.ScalarNode, Value: key},
		v,
	)
	return v
}

// findMapKey returns the value Node for key, or nil when absent.
func findMapKey(m *yaml.Node, key string) *yaml.Node {
	for i := 0; i+1 < len(m.Content); i += 2 {
		if m.Content[i].Value == key {
			return m.Content[i+1]
		}
	}
	return nil
}

// setMapValue replaces the value Node for key; reports whether key existed.
func setMapValue(m *yaml.Node, key string, val *yaml.Node) bool {
	for i := 0; i+1 < len(m.Content); i += 2 {
		if m.Content[i].Value == key {
			m.Content[i+1] = val
			return true
		}
	}
	return false
}

// renameMapKey renames key in place — position and attached comments
// survive; reports whether key existed. The caller must ensure newKey is
// not already present: renaming onto an existing key produces a duplicate
// mapping that yaml.v3 refuses to decode.
func renameMapKey(m *yaml.Node, oldKey, newKey string) bool {
	for i := 0; i+1 < len(m.Content); i += 2 {
		if m.Content[i].Value == oldKey {
			m.Content[i].Value = newKey
			return true
		}
	}
	return false
}

// removeMapKey deletes key and its value; reports whether key existed.
func removeMapKey(m *yaml.Node, key string) bool {
	for i := 0; i+1 < len(m.Content); i += 2 {
		if m.Content[i].Value == key {
			m.Content = append(m.Content[:i], m.Content[i+2:]...)
			return true
		}
	}
	return false
}

// writeYAMLNode writes the config document atomically (temp file in the same
// directory, then rename) so an interrupted write can't leave a torn config
// (no fsync — a power loss may still lose the update, an acceptable
// trade-off for a config tool). A symlinked config (dotfiles setups) is
// written through (the symlink is preserved), and an existing file keeps its
// permissions. modeline prepends the yaml-language-server schema comment
// (for brand-new files only — an existing file's header is the user's).
func writeYAMLNode(path string, node *yaml.Node, modeline bool) error {
	// A rename replaces the path itself; a symlinked config (dotfiles setups)
	// must be written through, and an existing file keeps its permissions.
	if resolved, err := filepath.EvalSymlinks(path); err == nil {
		path = resolved
	}
	mode := os.FileMode(0o600)
	if fi, err := os.Stat(path); err == nil {
		mode = fi.Mode().Perm()
	}

	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return err
	}
	var buf bytes.Buffer
	if modeline {
		buf.WriteString("# yaml-language-server: $schema=" + config.SchemaURL + "\n\n")
	}
	enc := yaml.NewEncoder(&buf)
	enc.SetIndent(4)
	if err := enc.Encode(node); err != nil {
		return err
	}
	if err := enc.Close(); err != nil {
		return err
	}

	tmp, err := os.CreateTemp(dir, ".mox-config-*.yml")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	defer os.Remove(tmpName) //nolint:errcheck // no-op after successful rename
	if err := tmp.Chmod(mode); err != nil {
		tmp.Close() //nolint:errcheck,gosec // best-effort cleanup
		return err
	}
	if _, err := tmp.Write(buf.Bytes()); err != nil {
		tmp.Close() //nolint:errcheck,gosec // best-effort cleanup
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Rename(tmpName, path)
}
