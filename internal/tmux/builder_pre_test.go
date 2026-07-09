package tmux_test

import (
	"context"
	"testing"

	"github.com/bthall/mox/internal/config"
	"github.com/bthall/mox/internal/tmux"
	"github.com/bthall/mox/internal/tmux/tmuxtest"
)

// TestBuilder_PreCommands verifies session- and window-level pre commands
// reach every pane, in order, ahead of the pane's own commands.
func TestBuilder_PreCommands(t *testing.T) {
	cfg := &config.Config{
		Sessions: map[string]*config.Session{
			"x": {
				Pre: []string{"export ENV=prod"},
				Windows: []*config.Window{
					{
						Name: "w1",
						Pre:  []string{"cd /srv"},
						Panes: []*config.Pane{
							{Split: config.SplitRoot, Commands: []string{"htop"}},
							{Split: config.SplitHorizontal},
						},
					},
				},
			},
		},
	}
	tx := tmuxtest.NewFake()
	b := tmux.NewBuilder(tx, cfg, nil)
	if err := b.BuildSession(context.Background(), "x", cfg.Sessions["x"]); err != nil {
		t.Fatalf("BuildSession() error = %v", err)
	}

	// Root pane: session pre, window pre, then its own command.
	var rootBatch []string
	for _, batches := range tx.KeysByPane {
		for _, batch := range batches {
			if len(batch) == 3 {
				rootBatch = batch
			}
		}
	}
	if len(rootBatch) != 3 || rootBatch[0] != "export ENV=prod" || rootBatch[1] != "cd /srv" || rootBatch[2] != "htop" {
		t.Errorf("root pane commands = %v, want [export ENV=prod, cd /srv, htop]", rootBatch)
	}

	// The command-less second pane still gets the pre commands.
	count := 0
	for _, batches := range tx.KeysByPane {
		for _, batch := range batches {
			if len(batch) == 2 && batch[0] == "export ENV=prod" && batch[1] == "cd /srv" {
				count++
			}
		}
	}
	if count != 1 {
		t.Errorf("expected exactly one pane with only pre commands, got %d (%v)", count, tx.KeysByPane)
	}
}
