package cli

import (
	"fmt"
	"strings"

	"github.com/bthall/mox/internal/config"
	"github.com/bthall/mox/internal/tmux"
	"github.com/spf13/cobra"
)

// arrangeLayouts is the static set of tmux layouts that --arrange accepts.
var arrangeLayouts = []string{
	"tiled",
	"even-horizontal",
	"even-vertical",
	"main-horizontal",
	"main-vertical",
}

// completeArrange returns completions for the --arrange flag.
func completeArrange(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
	return arrangeLayouts, cobra.ShellCompDirectiveNoFileComp
}

// completeSessionNamesOnly returns attach candidates: configured session
// names plus any tmux session currently running that's not in the config.
// Descriptions are omitted so bash-completion lays them out in compact
// columns (fits 50+ candidates on a normal screen). Used by the root `-a`
// flag where there are typically many candidates.
func completeSessionNamesOnly(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
	opts := optsFromContext(cmd.Context())

	seen := map[string]bool{}
	var names []string

	if cfg, _ := tryLoadConfig(opts.configPath); cfg != nil {
		for _, n := range cfg.ListSessionNames() {
			if !seen[n] {
				seen[n] = true
				names = append(names, n)
			}
		}
	}

	if client, err := tmux.NewClient(); err == nil {
		if running, err := client.ListSessions(); err == nil {
			for _, n := range running {
				if !seen[n] {
					seen[n] = true
					names = append(names, n)
				}
			}
		}
	}

	return names, cobra.ShellCompDirectiveNoFileComp
}

// completeConfiguredSession returns configured session names with short
// descriptions ("3 hosts", "2 windows", "local"). Used for narrow contexts
// like --from where the type of session matters for the user's decision.
func completeConfiguredSession(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
	opts := optsFromContext(cmd.Context())
	cfg, err := loadConfig(opts.configPath)
	if err != nil {
		return nil, cobra.ShellCompDirectiveNoFileComp
	}
	return sessionsWithDescriptions(cfg), cobra.ShellCompDirectiveNoFileComp
}

// completeHostsOrClusters drives positional-arg completion for 'new'. When
// the partial token starts with the cluster prefix, return cluster
// candidates (configured + clusterssh). Otherwise, return nothing — plain
// hostnames are too unconstrained to usefully suggest.
func completeHostsOrClusters(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
	if !strings.HasPrefix(toComplete, clusterPrefix) {
		if toComplete == "" {
			return []string{clusterPrefix}, cobra.ShellCompDirectiveNoFileComp | cobra.ShellCompDirectiveNoSpace
		}
		return nil, cobra.ShellCompDirectiveNoFileComp
	}
	opts := optsFromContext(cmd.Context())
	cfg, _ := tryLoadConfig(opts.configPath)
	clusters, _ := loadClusterssh()
	return clusterCandidates(cfg, clusters), cobra.ShellCompDirectiveNoFileComp
}

// sessionsWithDescriptions formats configured sessions as "name\tdescription"
// strings. Cobra's completion scripts split on the tab and bash-completion,
// zsh, and fish all render the description column.
func sessionsWithDescriptions(cfg *config.Config) []string {
	names := cfg.ListSessionNames()
	out := make([]string, 0, len(names))
	for _, n := range names {
		s, _ := cfg.GetSession(n)
		out = append(out, n+"\t"+describeSession(s))
	}
	return out
}

func describeSession(s *config.Session) string {
	switch {
	case s.IsSimple():
		return pluralize(len(s.Hosts), "host")
	case len(s.Windows) > 0:
		return pluralize(len(s.Windows), "window")
	default:
		return "local"
	}
}

func pluralize(n int, noun string) string {
	if n == 1 {
		return fmt.Sprintf("%d %s", n, noun)
	}
	return fmt.Sprintf("%d %ss", n, noun)
}
