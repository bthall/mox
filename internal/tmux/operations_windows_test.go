package tmux

import "testing"

func TestParseWindowLine_TabInName(t *testing.T) {
	// The name is emitted last precisely because it may contain tabs; id and
	// layout never do.
	w, ok := parseWindowLine("@1\tbb62,208x62,0,0,1\tlogs\tprod")
	if !ok {
		t.Fatal("line should parse")
	}
	if w.ID != "@1" {
		t.Errorf("id = %q", w.ID)
	}
	if w.Layout != "bb62,208x62,0,0,1" {
		t.Errorf("layout = %q — corrupted by the tab in the window name", w.Layout)
	}
	if w.Name != "logs\tprod" {
		t.Errorf("name = %q, want the tab preserved", w.Name)
	}
}

func TestParseWindowLine_Plain(t *testing.T) {
	w, ok := parseWindowLine("@3\td67e,80x24,0,0,0\tmain")
	if !ok {
		t.Fatal("line should parse")
	}
	if w.ID != "@3" || w.Name != "main" || w.Layout != "d67e,80x24,0,0,0" {
		t.Errorf("parsed = %+v", w)
	}
}
