package cli

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/bthall/mox/internal/config"
)

func TestExpandHosts_LiteralPassthrough(t *testing.T) {
	got, err := expandHosts([]string{"a", "b", "c"}, nil, nil)
	if err != nil {
		t.Fatalf("expandHosts() error = %v", err)
	}
	want := []string{"a", "b", "c"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got %v, want %v", got, want)
	}
}

func TestExpandHosts_FromMuxnestConfig(t *testing.T) {
	cfg := &config.Config{
		Sessions: map[string]*config.Session{
			"api-cluster": {Hosts: []string{"api1", "api2", "api3"}},
		},
	}
	got, err := expandHosts([]string{"@api-cluster"}, cfg, nil)
	if err != nil {
		t.Fatalf("expandHosts() error = %v", err)
	}
	want := []string{"api1", "api2", "api3"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got %v, want %v", got, want)
	}
}

func TestExpandHosts_FromComplexSessionFlattens(t *testing.T) {
	cfg := &config.Config{
		Sessions: map[string]*config.Session{
			"pve": {
				Windows: []*config.Window{
					{Name: "mon", Hosts: []string{"pve-mon-1", "pve-mon-2"}},
					{Name: "stor", Hosts: []string{"pve-stor-1", "pve-stor-2"}},
				},
			},
		},
	}
	got, err := expandHosts([]string{"@pve"}, cfg, nil)
	if err != nil {
		t.Fatalf("expandHosts() error = %v", err)
	}
	want := []string{"pve-mon-1", "pve-mon-2", "pve-stor-1", "pve-stor-2"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got %v, want %v", got, want)
	}
}

func TestExpandHosts_FromClusterssh(t *testing.T) {
	clusters := map[string][]string{
		"api-cluster": {"api1.example.com", "api2.example.com"},
	}
	got, err := expandHosts([]string{"@api-cluster"}, nil, clusters)
	if err != nil {
		t.Fatalf("expandHosts() error = %v", err)
	}
	want := []string{"api1.example.com", "api2.example.com"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got %v, want %v", got, want)
	}
}

func TestExpandHosts_ClustersshNestedExpansion(t *testing.T) {
	clusters := map[string][]string{
		"pve-mon":  {"m1", "m2"},
		"pve-virt": {"v1", "v2"},
		"pve":      {"pve-mon", "pve-virt"}, // nested!
	}
	got, err := expandHosts([]string{"@pve"}, nil, clusters)
	if err != nil {
		t.Fatalf("expandHosts() error = %v", err)
	}
	want := []string{"m1", "m2", "v1", "v2"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got %v, want %v", got, want)
	}
}

func TestExpandHosts_ClustersshCycleDetection(t *testing.T) {
	clusters := map[string][]string{
		"a": {"b"},
		"b": {"a"},
	}
	_, err := expandHosts([]string{"@a"}, nil, clusters)
	if err == nil || !strings.Contains(err.Error(), "cycle detected") {
		t.Errorf("expected cycle error, got %v", err)
	}
}

func TestExpandHosts_MuxnestBeatsClusterssh(t *testing.T) {
	cfg := &config.Config{
		Sessions: map[string]*config.Session{
			"foo": {Hosts: []string{"from-mox"}},
		},
	}
	clusters := map[string][]string{
		"foo": {"from-clusterssh"},
	}
	got, err := expandHosts([]string{"@foo"}, cfg, clusters)
	if err != nil {
		t.Fatalf("expandHosts() error = %v", err)
	}
	if !reflect.DeepEqual(got, []string{"from-mox"}) {
		t.Errorf("mox should win, got %v", got)
	}
}

func TestExpandHosts_MixedLiteralAndCluster(t *testing.T) {
	cfg := &config.Config{
		Sessions: map[string]*config.Session{
			"web": {Hosts: []string{"w1", "w2"}},
		},
	}
	got, err := expandHosts([]string{"@web", "manual-host"}, cfg, nil)
	if err != nil {
		t.Fatalf("expandHosts() error = %v", err)
	}
	want := []string{"w1", "w2", "manual-host"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got %v, want %v", got, want)
	}
}

func TestExpandHosts_UnknownClusterErrors(t *testing.T) {
	_, err := expandHosts([]string{"@nope"}, nil, nil)
	if err == nil || !strings.Contains(err.Error(), "not found") {
		t.Errorf("expected not-found error, got %v", err)
	}
}

func TestExpandHosts_ComplexSessionWithoutHosts(t *testing.T) {
	cfg := &config.Config{
		Sessions: map[string]*config.Session{
			"weird": {
				Windows: []*config.Window{
					{Name: "panes-only", Panes: []*config.Pane{{Split: config.SplitRoot}}},
				},
			},
		},
	}
	_, err := expandHosts([]string{"@weird"}, cfg, nil)
	if err == nil || !strings.Contains(err.Error(), "no hosts to expand") {
		t.Errorf("expected 'no hosts to expand' error, got %v", err)
	}
}

func TestParseClusterFile(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "clusters")
	body := `# leading comment
psibu  orion hubble sagan

# blank above
pve-mon pve-mon-1 pve-mon-2 pve-mon-3
pve     pve-mon pve-virt
`
	if err := os.WriteFile(p, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	got, err := parseClusterFile(p)
	if err != nil {
		t.Fatalf("parseClusterFile() error = %v", err)
	}
	want := map[string][]string{
		"psibu":   {"orion", "hubble", "sagan"},
		"pve-mon": {"pve-mon-1", "pve-mon-2", "pve-mon-3"},
		"pve":     {"pve-mon", "pve-virt"},
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got %v, want %v", got, want)
	}
}

func TestClusterCandidates(t *testing.T) {
	cfg := &config.Config{
		Sessions: map[string]*config.Session{
			"dev":  {Hosts: []string{"a"}},
			"prod": {Hosts: []string{"b"}},
		},
	}
	clusters := map[string][]string{
		"prod":   {"override"}, // duplicate; mox entry should win in suggestions
		"rack-1": {"r1"},
	}
	got := clusterCandidates(cfg, clusters)
	want := []string{"@dev", "@prod", "@rack-1"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got %v, want %v", got, want)
	}
}
