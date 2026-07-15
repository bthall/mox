package cli

import (
	"testing"

	"github.com/bthall/mox/internal/config"
	"github.com/bthall/mox/internal/tmux"
)

func mustParseLayout(t *testing.T, s string) *tmux.LayoutNode {
	t.Helper()
	n, err := tmux.ParseLayout(s)
	if err != nil {
		t.Fatalf("ParseLayout(%q): %v", s, err)
	}
	return n
}

func TestChainFromLayout_SinglePane(t *testing.T) {
	chain, ok := chainFromLayout(mustParseLayout(t, "bb62,208x62,0,0,1"))
	if !ok {
		t.Fatal("single pane should linearize")
	}
	if len(chain) != 1 {
		t.Fatalf("chain length = %d, want 1", len(chain))
	}
	if chain[0].paneID != "%1" || chain[0].split != config.SplitRoot {
		t.Errorf("chain[0] = %+v, want %%1 root", chain[0])
	}
}

func TestChainFromLayout_SideBySideHalves(t *testing.T) {
	// 104 vs 103 of 208 — an even tmux split; size should be omitted (0).
	chain, ok := chainFromLayout(mustParseLayout(t, "d5d2,208x62,0,0{104x62,0,0,1,103x62,105,0,2}"))
	if !ok {
		t.Fatal("side-by-side should linearize")
	}
	if len(chain) != 2 {
		t.Fatalf("chain length = %d, want 2", len(chain))
	}
	if chain[1].split != config.SplitVertical {
		t.Errorf("second pane split = %q, want vertical (side-by-side)", chain[1].split)
	}
	if chain[1].size != 0 {
		t.Errorf("even split size = %d, want 0 (default)", chain[1].size)
	}
}

func TestChainFromLayout_UnevenStack(t *testing.T) {
	// 70%/30% stack: 62 rows = 43 + separator + 18.
	chain, ok := chainFromLayout(mustParseLayout(t, "9f58,208x62,0,0[208x43,0,0,1,208x18,0,44,2]"))
	if !ok {
		t.Fatal("stack should linearize")
	}
	if chain[1].split != config.SplitHorizontal {
		t.Errorf("second pane split = %q, want horizontal (stacked)", chain[1].split)
	}
	// round(100*18/62) = 29
	if chain[1].size != 29 {
		t.Errorf("size = %d, want 29", chain[1].size)
	}
}

func TestChainFromLayout_NestedRealWorld(t *testing.T) {
	// Captured from a real tmux server: left pane, right side stacked.
	chain, ok := chainFromLayout(mustParseLayout(t, "d67e,80x24,0,0{40x24,0,0,0,39x24,41,0[39x12,41,0,1,39x11,41,13,2]}"))
	if !ok {
		t.Fatal("nested last-child layout should linearize")
	}
	if len(chain) != 3 {
		t.Fatalf("chain length = %d, want 3", len(chain))
	}
	want := []struct {
		id    string
		split config.SplitType
		size  int
	}{
		{"%0", config.SplitRoot, 0},
		{"%1", config.SplitVertical, 49},   // round(100*39/80)
		{"%2", config.SplitHorizontal, 46}, // round(100*11/24)
	}
	for i, w := range want {
		if chain[i].paneID != w.id || chain[i].split != w.split || chain[i].size != w.size {
			t.Errorf("chain[%d] = %+v, want %+v", i, chain[i], w)
		}
	}
}

func TestChainFromLayout_ThreeEvenColumns(t *testing.T) {
	chain, ok := chainFromLayout(mustParseLayout(t, "aaaa,120x24,0,0{40x24,0,0,0,39x24,41,0,1,39x24,81,0,2}"))
	if !ok {
		t.Fatal("three columns should linearize")
	}
	if len(chain) != 3 {
		t.Fatalf("chain length = %d, want 3", len(chain))
	}
	// Second split: remaining 79 of 120 -> 66%. Third: 39 of 79 -> 49... but
	// the point here is direction; sizes just need to be sane (0 or 1-99).
	for i, p := range chain[1:] {
		if p.split != config.SplitVertical {
			t.Errorf("chain[%d].split = %q, want vertical", i+1, p.split)
		}
		if p.size < 0 || p.size > 99 {
			t.Errorf("chain[%d].size = %d out of range", i+1, p.size)
		}
	}
}

func TestChainFromLayout_NonLinearizable(t *testing.T) {
	// First child is a container ({[a,b],c}) — cannot be produced by
	// splitting the previous pane each time.
	_, ok := chainFromLayout(mustParseLayout(t, "aaaa,80x24,0,0{40x24,0,0[40x12,0,0,0,40x11,0,13,1],39x24,41,0,2}"))
	if ok {
		t.Fatal("container in non-last position must not linearize")
	}
}
