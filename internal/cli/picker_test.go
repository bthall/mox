package cli

import (
	"bytes"
	"strings"
	"testing"
	"time"

	"github.com/bthall/mox/internal/history"
	"github.com/bthall/mox/internal/session"
)

func pickerFixtures() []session.SessionInfo {
	now := time.Date(2026, 7, 6, 12, 0, 0, 0, time.UTC)
	return []session.SessionInfo{
		{Name: "batch", Managed: true},
		{Name: "web", Managed: true, Running: true, LastActivity: now.Add(-time.Hour), Hosts: []string{"host1", "host2"}},
		{Name: "dev", Managed: true, Running: true, LastActivity: now.Add(-time.Minute), Hosts: []string{"web1", "web2", "db"}},
		{Name: "old-thing", Managed: true},
	}
}

func TestOrderPickerCandidates(t *testing.T) {
	recent := []history.Entry{
		{Name: "old-thing", Action: history.ActionAttached, Time: time.Date(2026, 7, 6, 11, 0, 0, 0, time.UTC)},
	}
	got := orderPickerCandidates(pickerFixtures(), recent)

	want := []string{"dev", "web", "old-thing", "batch"}
	for i, name := range want {
		if got[i].Name != name {
			t.Fatalf("position %d: got %q, want %q (full: %v)", i, got[i].Name, name, names(got))
		}
	}
}

func names(infos []session.SessionInfo) []string {
	out := make([]string, len(infos))
	for i, s := range infos {
		out[i] = s.Name
	}
	return out
}

func TestResolvePickerChoice(t *testing.T) {
	cands := orderPickerCandidates(pickerFixtures(), nil)

	cases := []struct {
		input   string
		want    string
		wantErr string
	}{
		{input: "1", want: "dev"},
		{input: "4", want: "old-thing"},
		{input: "9", wantErr: "no session numbered"},
		{input: "web", want: "web"},
		{input: "ba", want: "batch"},
		{input: "zzz", wantErr: "no session matches"},
	}
	for _, c := range cases {
		got, err := resolvePickerChoice(c.input, cands)
		if c.wantErr != "" {
			if err == nil || !strings.Contains(err.Error(), c.wantErr) {
				t.Errorf("input %q: want error containing %q, got %v", c.input, c.wantErr, err)
			}
			continue
		}
		if err != nil || got != c.want {
			t.Errorf("input %q: got (%q, %v), want %q", c.input, got, err, c.want)
		}
	}
}

func TestResolvePickerChoice_AmbiguousPrefix(t *testing.T) {
	cands := []session.SessionInfo{{Name: "web1"}, {Name: "web2"}}
	_, err := resolvePickerChoice("web", cands)
	if err == nil || !strings.Contains(err.Error(), "ambiguous") {
		t.Errorf("want ambiguity error, got %v", err)
	}
}

func TestRenderPicker(t *testing.T) {
	var buf bytes.Buffer
	renderPicker(&buf, orderPickerCandidates(pickerFixtures(), nil), time.Date(2026, 7, 6, 12, 0, 0, 0, time.UTC))
	out := buf.String()

	first := lineContaining(t, out, "1.")
	if !strings.Contains(first, "dev") || !strings.Contains(first, "running") {
		t.Errorf("first row should be running dev: %q", first)
	}
	if !strings.Contains(out, "4.") {
		t.Errorf("expected 4 numbered rows:\n%s", out)
	}
}
