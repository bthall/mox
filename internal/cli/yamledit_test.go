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

	if !strings.Contains(output, "# keep me") {
		t.Fatalf("comment lost after rename: %q", output)
	}
	if !strings.Contains(output, "z:") {
		t.Fatalf("renamed key 'z' not found: %q", output)
	}
}

func TestWriteYAMLNodeAtomic(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yml")
	root, _ := parseDoc(t, "sessions:\n    a:\n        root: /tmp\n")

	if err := writeYAMLNode(path, root, false); err != nil {
		t.Fatalf("writeYAMLNode: %v", err)
	}
	fi, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat: %v", err)
	}
	if fi.Mode().Perm() != 0o600 {
		t.Fatalf("mode = %v, want 0600", fi.Mode().Perm())
	}
	data, _ := os.ReadFile(path)
	if !strings.Contains(string(data), "root: /tmp") {
		t.Fatalf("content lost: %q", data)
	}
	// no temp litter left behind
	entries, _ := os.ReadDir(dir)
	if len(entries) != 1 {
		t.Fatalf("directory has %d entries, want 1 (temp file left behind?)", len(entries))
	}
}
