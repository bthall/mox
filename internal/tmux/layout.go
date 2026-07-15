package tmux

import (
	"fmt"
	"strconv"
	"strings"
)

// LayoutNode is one cell of a parsed tmux window layout: either a leaf pane
// or a container whose children are laid out along one axis. tmux writes the
// layout as `checksum,cell` where a cell is `WxH,X,Y` followed by `,<id>`
// (leaf), `{cells}` (children side-by-side), or `[cells]` (children stacked).
type LayoutNode struct {
	Width, Height int
	X, Y          int

	// PaneID is the tmux pane id ("%N") for a leaf; empty for containers.
	PaneID string

	// Horizontal reports the container axis: true when children sit
	// side-by-side ({...}), false when stacked ([...]).
	Horizontal bool
	Children   []*LayoutNode
}

// IsLeaf reports whether the node is a single pane.
func (n *LayoutNode) IsLeaf() bool { return len(n.Children) == 0 }

// ParseLayout parses the value of tmux's #{window_layout} format.
func ParseLayout(s string) (*LayoutNode, error) {
	// Strip the leading 4-hex-digit checksum.
	i := strings.IndexByte(s, ',')
	if i < 0 {
		return nil, fmt.Errorf("layout %q: missing checksum separator", s)
	}
	p := &layoutParser{s: s, pos: i + 1}
	node, err := p.parseCell()
	if err != nil {
		return nil, fmt.Errorf("layout %q: %w", s, err)
	}
	if p.pos != len(s) {
		return nil, fmt.Errorf("layout %q: trailing garbage at offset %d", s, p.pos)
	}
	return node, nil
}

type layoutParser struct {
	s   string
	pos int
}

func (p *layoutParser) parseCell() (*LayoutNode, error) {
	n := &LayoutNode{}
	var err error
	if n.Width, err = p.parseInt('x'); err != nil {
		return nil, fmt.Errorf("width: %w", err)
	}
	if n.Height, err = p.parseInt(','); err != nil {
		return nil, fmt.Errorf("height: %w", err)
	}
	if n.X, err = p.parseInt(','); err != nil {
		return nil, fmt.Errorf("x: %w", err)
	}
	// Y is followed by ',' (leaf), '{' or '[' (container).
	start := p.pos
	for p.pos < len(p.s) && p.s[p.pos] >= '0' && p.s[p.pos] <= '9' {
		p.pos++
	}
	if n.Y, err = strconv.Atoi(p.s[start:p.pos]); err != nil {
		return nil, fmt.Errorf("y: %w", err)
	}
	if p.pos >= len(p.s) {
		return nil, fmt.Errorf("cell at offset %d: missing pane id or container", start)
	}

	switch p.s[p.pos] {
	case ',':
		p.pos++
		idStart := p.pos
		for p.pos < len(p.s) && p.s[p.pos] >= '0' && p.s[p.pos] <= '9' {
			p.pos++
		}
		if p.pos == idStart {
			return nil, fmt.Errorf("cell at offset %d: empty pane id", idStart)
		}
		n.PaneID = "%" + p.s[idStart:p.pos]
		return n, nil
	case '{':
		n.Horizontal = true
		return p.parseChildren(n, '}')
	case '[':
		return p.parseChildren(n, ']')
	default:
		return nil, fmt.Errorf("cell at offset %d: unexpected %q", p.pos, p.s[p.pos])
	}
}

func (p *layoutParser) parseChildren(n *LayoutNode, closer byte) (*LayoutNode, error) {
	p.pos++ // consume opener
	for {
		child, err := p.parseCell()
		if err != nil {
			return nil, err
		}
		n.Children = append(n.Children, child)
		if p.pos >= len(p.s) {
			return nil, fmt.Errorf("unclosed container (expected %q)", closer)
		}
		switch p.s[p.pos] {
		case ',':
			p.pos++
		case closer:
			p.pos++
			if len(n.Children) < 2 {
				return nil, fmt.Errorf("container with %d child(ren); tmux containers have at least 2", len(n.Children))
			}
			return n, nil
		default:
			return nil, fmt.Errorf("unexpected %q at offset %d", p.s[p.pos], p.pos)
		}
	}
}

func (p *layoutParser) parseInt(sep byte) (int, error) {
	start := p.pos
	for p.pos < len(p.s) && p.s[p.pos] >= '0' && p.s[p.pos] <= '9' {
		p.pos++
	}
	if p.pos == start {
		return 0, fmt.Errorf("expected digits at offset %d", start)
	}
	v, err := strconv.Atoi(p.s[start:p.pos])
	if err != nil {
		return 0, err
	}
	if p.pos >= len(p.s) || p.s[p.pos] != sep {
		return 0, fmt.Errorf("expected %q at offset %d", sep, p.pos)
	}
	p.pos++
	return v, nil
}
