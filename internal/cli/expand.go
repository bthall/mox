package cli

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/bthall/mox/internal/config"
)

// clusterPrefix is the sigil used to refer to a named cluster of hosts.
// We use '@' (and not '#') because '#' starts a comment in bash interactive
// mode, which would require quoting on every invocation.
const clusterPrefix = "@"

// expandHosts walks args. For each arg starting with '@', it looks up the
// cluster name in (1) the mox config, then (2) the clusterssh `clusters`
// file, and substitutes the host list. Literal hosts pass through unchanged.
//
// If cfg is nil, only clusterssh is consulted. If clusters is nil,
// clusterssh lookups are skipped.
func expandHosts(args []string, cfg *config.Config, clusters map[string][]string) ([]string, error) {
	out := make([]string, 0, len(args))
	for _, arg := range args {
		if !strings.HasPrefix(arg, clusterPrefix) {
			out = append(out, arg)
			continue
		}
		name := strings.TrimPrefix(arg, clusterPrefix)
		if name == "" {
			return nil, fmt.Errorf("empty cluster name: %q", arg)
		}
		hosts, err := lookupCluster(name, cfg, clusters)
		if err != nil {
			return nil, err
		}
		out = append(out, hosts...)
	}
	return out, nil
}

// lookupCluster resolves a name against the mox config first, then
// clusterssh. For a complex mox session, all simple-mode windows'
// hosts are flattened in window order.
func lookupCluster(name string, cfg *config.Config, clusters map[string][]string) ([]string, error) {
	if cfg != nil {
		if sess, ok := cfg.GetSession(name); ok {
			if sess.IsSimple() {
				return append([]string(nil), sess.Hosts...), nil
			}
			var out []string
			for _, w := range sess.Windows {
				if w.IsSimple() {
					out = append(out, w.Hosts...)
				}
			}
			if len(out) == 0 {
				return nil, fmt.Errorf("@%s: configured session has no hosts to expand", name)
			}
			return out, nil
		}
	}
	if clusters != nil {
		if hosts, ok := clusters[name]; ok {
			return expandClusterssh(name, hosts, clusters, map[string]bool{})
		}
	}
	return nil, fmt.Errorf("@%s: not found in mox config or clusterssh `clusters` file", name)
}

// expandClusterssh recursively flattens nested cluster references.
// clusterssh allows a cluster line to list other cluster names; we follow
// those references and detect cycles.
func expandClusterssh(name string, hosts []string, all map[string][]string, visited map[string]bool) ([]string, error) {
	if visited[name] {
		return nil, fmt.Errorf("cycle detected in clusterssh cluster %q", name)
	}
	visited[name] = true
	defer func() { delete(visited, name) }()

	var out []string
	for _, h := range hosts {
		if sub, ok := all[h]; ok {
			subHosts, err := expandClusterssh(h, sub, all, visited)
			if err != nil {
				return nil, err
			}
			out = append(out, subHosts...)
		} else {
			out = append(out, h)
		}
	}
	return out, nil
}

// loadClusterssh reads a clusterssh `clusters` file and returns
// {cluster_name: [host1, host2, ...]}. Comment lines (#...) and blank
// lines are ignored. Returns (nil, nil) if no file is found at any
// standard location.
func loadClusterssh() (map[string][]string, error) {
	for _, p := range clusterFilePaths() {
		if _, err := os.Stat(p); err != nil {
			continue
		}
		return parseClusterFile(p)
	}
	return nil, nil
}

func clusterFilePaths() []string {
	var paths []string
	if p := os.Getenv("CSSH_CLUSTERS"); p != "" {
		paths = append(paths, p)
	}
	if home, err := os.UserHomeDir(); err == nil {
		paths = append(paths, filepath.Join(home, ".clusterssh", "clusters"))
	}
	paths = append(paths, "/etc/clusters")
	return paths
}

func parseClusterFile(path string) (map[string][]string, error) {
	f, err := os.Open(path) //nolint:gosec // user-configured location
	if err != nil {
		return nil, fmt.Errorf("open %s: %w", path, err)
	}
	defer func() { _ = f.Close() }() // read-only; close error is immaterial

	clusters := make(map[string][]string)
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		fields := strings.Fields(line)
		if len(fields) < 2 {
			continue
		}
		name := fields[0]
		clusters[name] = append([]string(nil), fields[1:]...)
	}
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("read %s: %w", path, err)
	}
	return clusters, nil
}

// clusterCandidates returns the union of cluster names from the mox
// config and the clusterssh file, each prefixed with '@'. Used for tab
// completion. Order: configured first (sorted), then clusterssh (sorted).
func clusterCandidates(cfg *config.Config, clusters map[string][]string) []string {
	seen := map[string]bool{}
	var configured, ssh []string
	if cfg != nil {
		for name := range cfg.Sessions {
			seen[name] = true
			configured = append(configured, clusterPrefix+name)
		}
	}
	for name := range clusters {
		if seen[name] {
			continue
		}
		ssh = append(ssh, clusterPrefix+name)
	}
	slices.Sort(configured)
	slices.Sort(ssh)
	return append(configured, ssh...)
}
