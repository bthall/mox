package tmux

import (
	"testing"
)

func TestParseLayout_SinglePane(t *testing.T) {
	node, err := ParseLayout("bb62,208x62,0,0,1")
	if err != nil {
		t.Fatalf("ParseLayout: %v", err)
	}
	if !node.IsLeaf() {
		t.Fatal("single pane should parse to a leaf")
	}
	if node.PaneID != "%1" {
		t.Errorf("PaneID = %q, want %%1", node.PaneID)
	}
	if node.Width != 208 || node.Height != 62 {
		t.Errorf("size = %dx%d, want 208x62", node.Width, node.Height)
	}
}

func TestParseLayout_SideBySide(t *testing.T) {
	// Two panes left-right: {} is tmux's horizontal container.
	node, err := ParseLayout("d5d2,208x62,0,0{104x62,0,0,1,103x62,105,0,2}")
	if err != nil {
		t.Fatalf("ParseLayout: %v", err)
	}
	if node.IsLeaf() || !node.Horizontal {
		t.Fatal("expected a horizontal container")
	}
	if len(node.Children) != 2 {
		t.Fatalf("children = %d, want 2", len(node.Children))
	}
	if node.Children[0].PaneID != "%1" || node.Children[1].PaneID != "%2" {
		t.Errorf("pane ids = %q, %q", node.Children[0].PaneID, node.Children[1].PaneID)
	}
	if node.Children[0].Width != 104 || node.Children[1].Width != 103 {
		t.Errorf("widths = %d, %d, want 104, 103", node.Children[0].Width, node.Children[1].Width)
	}
}

func TestParseLayout_Stacked(t *testing.T) {
	// Two panes top-bottom: [] is tmux's vertical container.
	node, err := ParseLayout("9f58,208x62,0,0[208x31,0,0,1,208x30,0,32,2]")
	if err != nil {
		t.Fatalf("ParseLayout: %v", err)
	}
	if node.IsLeaf() || node.Horizontal {
		t.Fatal("expected a vertical container")
	}
	if len(node.Children) != 2 {
		t.Fatalf("children = %d, want 2", len(node.Children))
	}
	if node.Children[0].Height != 31 || node.Children[1].Height != 30 {
		t.Errorf("heights = %d, %d, want 31, 30", node.Children[0].Height, node.Children[1].Height)
	}
}

func TestParseLayout_Nested(t *testing.T) {
	// Left pane full height; right side split into two stacked panes.
	node, err := ParseLayout("d5d2,208x62,0,0{104x62,0,0,1,103x62,105,0[103x31,105,0,2,103x30,105,32,3]}")
	if err != nil {
		t.Fatalf("ParseLayout: %v", err)
	}
	if node.IsLeaf() || !node.Horizontal {
		t.Fatal("expected top-level horizontal container")
	}
	if len(node.Children) != 2 {
		t.Fatalf("children = %d, want 2", len(node.Children))
	}
	right := node.Children[1]
	if right.IsLeaf() || right.Horizontal {
		t.Fatal("right child should be a vertical container")
	}
	if len(right.Children) != 2 {
		t.Fatalf("right children = %d, want 2", len(right.Children))
	}
	if right.Children[0].PaneID != "%2" || right.Children[1].PaneID != "%3" {
		t.Errorf("nested pane ids = %q, %q", right.Children[0].PaneID, right.Children[1].PaneID)
	}
}

func TestParseLayout_Invalid(t *testing.T) {
	cases := []string{
		"",
		"no-commas",
		"d5d2,208x62,0,0{104x62,0,0,1", // unclosed container
		"d5d2,208x62,0,0{}",            // empty container
		"d5d2,208xBAD,0,0,1",           // bad dimension
		"d5d2,208x62,0,0",              // container/leaf marker missing
	}
	for _, s := range cases {
		if _, err := ParseLayout(s); err == nil {
			t.Errorf("ParseLayout(%q) should fail", s)
		}
	}
}
