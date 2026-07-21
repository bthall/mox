package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

// parseDoc unmarshals YAML into a document node and returns the top mapping.
func parseDoc(t *testing.T, body string) (*yaml.Node, *yaml.Node) {
	t.Helper()
	var root yaml.Node
	if err := yaml.Unmarshal([]byte(body), &root); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	return &root, root.Content[0]
}

func TestMapKeyHelpers(t *testing.T) {
	_, m := parseDoc(t, "a: 1\nb: 2\n")

	if v := findMapKey(m, "a"); v == nil || v.Value != "1" {
		t.Fatalf("findMapKey(a) = %v, want scalar 1", v)
	}
	if v := findMapKey(m, "missing"); v != nil {
		t.Fatalf("findMapKey(missing) = %v, want nil", v)
	}

	if !renameMapKey(m, "a", "z") {
		t.Fatal("renameMapKey(a→z) = false, want true")
	}
	if findMapKey(m, "z") == nil {
		t.Fatal("key z not found after rename")
	}
	if renameMapKey(m, "nope", "x") {
		t.Fatal("renameMapKey(nope) = true, want false")
	}

	repl := &yaml.Node{Kind: yaml.ScalarNode, Value: "9"}
	if !setMapValue(m, "b", repl) {
		t.Fatal("setMapValue(b) = false, want true")
	}
	if findMapKey(m, "b").Value != "9" {
		t.Fatal("setMapValue did not replace value")
	}
	if setMapValue(m, "nope", repl) {
		t.Fatal("setMapValue(nope) = true, want false")
	}

	if !removeMapKey(m, "z") {
		t.Fatal("removeMapKey(z) = false, want true")
	}
	if findMapKey(m, "z") != nil {
		t.Fatal("key z still present after remove")
	}
	if removeMapKey(m, "z") {
		t.Fatal("second removeMapKey(z) = true, want false")
	}
	// b must survive with its new value
	if v := findMapKey(m, "b"); v == nil || v.Value != "9" {
		t.Fatal("unrelated key damaged by removeMapKey")
	}
}

func TestRenameMapKeyPreservesComments(t *testing.T) {
	root, m := parseDoc(t, "# keep me\na: 1\nb: 2\n")

	if !renameMapKey(m, "a", "z") {
		t.Fatal("renameMapKey(a→z) = false, want true")
	}

	// Re-encode the document and verify comment rode along with the renamed key
	data, err := yaml.Marshal(root)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	output := string(data)

	// Comment must immediately precede the renamed key
	if !strings.Contains(output, "# keep me\nz:") {
		t.Fatalf("comment immediately preceding renamed key not found: %q", output)
	}
}

func TestWriteYAMLNodeAtomic(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yml")
	root, _ := parseDoc(t, "sessions:\n    a:\n        root: /tmp\n")

	// First write: new file should get default 0600 permissions
	if err := writeYAMLNode(path, root, false); err != nil {
		t.Fatalf("first writeYAMLNode: %v", err)
	}
	fi, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat after first write: %v", err)
	}
	if fi.Mode().Perm() != 0o600 {
		t.Fatalf("mode after first write = %v, want 0600", fi.Mode().Perm())
	}
	data, _ := os.ReadFile(path)
	if !strings.Contains(string(data), "root: /tmp") {
		t.Fatalf("content lost after first write: %q", data)
	}
	// no temp litter left behind
	entries, _ := os.ReadDir(dir)
	if len(entries) != 1 {
		t.Fatalf("directory has %d entries after first write, want 1 (temp file left behind?)", len(entries))
	}

	// Second write: replace existing file with changed content, preserving permissions
	root2, _ := parseDoc(t, "sessions:\n    a:\n        root: /new\n")
	if err := writeYAMLNode(path, root2, false); err != nil {
		t.Fatalf("second writeYAMLNode: %v", err)
	}
	fi2, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat after second write: %v", err)
	}
	if fi2.Mode().Perm() != 0o600 {
		t.Fatalf("mode after second write = %v, want 0600", fi2.Mode().Perm())
	}
	data2, _ := os.ReadFile(path)
	if !strings.Contains(string(data2), "root: /new") {
		t.Fatalf("content not updated: %q", data2)
	}
	if strings.Contains(string(data2), "root: /tmp") {
		t.Fatalf("old content still present: %q", data2)
	}
	// still exactly one file
	entries2, _ := os.ReadDir(dir)
	if len(entries2) != 1 {
		t.Fatalf("directory has %d entries after second write, want 1", len(entries2))
	}

	// Symlink case: create real file with 0644, symlink to it, writeYAMLNode through symlink
	realPath := filepath.Join(dir, "real.yml")
	linkPath := filepath.Join(dir, "link.yml")
	if err := os.WriteFile(realPath, []byte("# original\n"), 0o644); err != nil {
		t.Fatalf("write real file: %v", err)
	}
	if err := os.Symlink("real.yml", linkPath); err != nil {
		t.Fatalf("create symlink: %v", err)
	}
	// Verify the symlink was created with the right permissions
	linkStat, err := os.Lstat(linkPath)
	if err != nil {
		t.Fatalf("lstat symlink before write: %v", err)
	}
	if linkStat.Mode()&os.ModeSymlink == 0 {
		t.Fatal("link is not a symlink before write")
	}

	// Write through the symlink with preserved 0644 from the target
	root3, _ := parseDoc(t, "sessions:\n    c:\n        root: /symlink\n")
	if err := writeYAMLNode(linkPath, root3, false); err != nil {
		t.Fatalf("writeYAMLNode through symlink: %v", err)
	}

	// Verify symlink still exists (not replaced with regular file)
	linkStat2, err := os.Lstat(linkPath)
	if err != nil {
		t.Fatalf("lstat symlink after write: %v", err)
	}
	if linkStat2.Mode()&os.ModeSymlink == 0 {
		t.Fatal("symlink was replaced with regular file")
	}

	// Verify content landed in the real file
	realData, _ := os.ReadFile(realPath)
	if !strings.Contains(string(realData), "root: /symlink") {
		t.Fatalf("content not written through symlink to real file: %q", realData)
	}

	// Verify the real file kept its original 0644 permissions
	realStat, err := os.Stat(realPath)
	if err != nil {
		t.Fatalf("stat real file after write: %v", err)
	}
	if realStat.Mode().Perm() != 0o644 {
		t.Fatalf("real file permissions changed to %v, want 0644", realStat.Mode().Perm())
	}
}
