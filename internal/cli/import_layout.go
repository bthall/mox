package cli

import (
	"math"

	"github.com/bthall/mox/internal/config"
	"github.com/bthall/mox/internal/tmux"
)

// paneGeom is one link of a linearized layout: the pane, how it was split
// from the previous pane, and its size as a percent of the pane it split
// (0 = tmux default, i.e. an even split).
type paneGeom struct {
	paneID string
	split  config.SplitType
	size   int
}

// chainFromLayout converts a parsed tmux layout tree into mox's linear pane
// model, where each pane splits the previous one. Not every tree is
// expressible that way: a chain only ever subdivides the most recently
// created pane, so containers may nest only in the last-child position.
// ok is false for trees that don't linearize.
func chainFromLayout(root *tmux.LayoutNode) ([]paneGeom, bool) {
	var chain []paneGeom
	if !walkLayout(root, config.SplitRoot, 0, &chain) {
		return nil, false
	}
	return chain, true
}

// walkLayout appends node's leaves to chain in creation order. entry/entrySize
// describe the split that carved this node's area out of the previous pane;
// they attach to the node's first leaf.
func walkLayout(node *tmux.LayoutNode, entry config.SplitType, entrySize int, chain *[]paneGeom) bool {
	if node.IsLeaf() {
		*chain = append(*chain, paneGeom{paneID: node.PaneID, split: entry, size: entrySize})
		return true
	}

	// Container axis -> mox split direction for its non-first children.
	// tmux {} = side-by-side = mox vertical; tmux [] = stacked = mox horizontal.
	split := config.SplitHorizontal
	extent := func(n *tmux.LayoutNode) int { return n.Height }
	if node.Horizontal {
		split = config.SplitVertical
		extent = func(n *tmux.LayoutNode) int { return n.Width }
	}

	remaining := extent(node)
	for i, child := range node.Children {
		last := i == len(node.Children)-1
		if !last && !child.IsLeaf() {
			return false
		}
		if i == 0 {
			// The first child is the pane that was split to create this
			// container; it inherits the entry split.
			*chain = append(*chain, paneGeom{paneID: child.PaneID, split: entry, size: entrySize})
			continue
		}

		// This child was split off the previous one. At that moment the
		// previous pane spanned `remaining`; the new pane got everything
		// past the previous child and its 1-cell separator.
		newRemaining := remaining - extent(node.Children[i-1]) - 1
		size := splitPercent(newRemaining, remaining)
		remaining = newRemaining

		if child.IsLeaf() {
			*chain = append(*chain, paneGeom{paneID: child.PaneID, split: split, size: size})
		} else if !walkLayout(child, split, size, chain) {
			return false
		}
	}
	return true
}

// splitPercent returns the new pane's share as a whole percent, clamped to
// the config's 1-99 range, with an even split normalized to 0 (tmux default).
func splitPercent(part, whole int) int {
	if whole <= 0 {
		return 0
	}
	pct := int(math.Round(100 * float64(part) / float64(whole)))
	if pct < 1 {
		pct = 1
	}
	if pct > 99 {
		pct = 99
	}
	if pct == 50 {
		return 0
	}
	return pct
}
