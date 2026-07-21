package cli

import (
	"testing"

	"github.com/bthall/mox/internal/config"
)

func fieldByKey(t *testing.T, fields []fieldDef, key string) fieldDef {
	t.Helper()
	for _, f := range fields {
		if f.key == key {
			return f
		}
	}
	t.Fatalf("no field %q", key)
	return fieldDef{}
}

func TestSessionFieldsSimpleVsComplex(t *testing.T) {
	simple := sessionFields(&config.Session{Hosts: []string{"h1"}})
	keys := map[string]bool{}
	for _, f := range simple {
		keys[f.key] = true
	}
	for _, want := range []string{"hosts", "connect", "ssh_user", "sync", "arrange", "hold", "retry", "root", "pre", "commands", "on_start", "on_stop"} {
		if !keys[want] {
			t.Errorf("simple session missing field %q", want)
		}
	}
	if keys["windows"] {
		t.Error("simple session has a windows row")
	}

	complexSess := sessionFields(&config.Session{Windows: []*config.Window{{Name: "w", Hosts: []string{"h"}}}})
	keys = map[string]bool{}
	for _, f := range complexSess {
		keys[f.key] = true
	}
	if keys["hosts"] || keys["commands"] {
		t.Error("complex session exposes simple-only fields")
	}
	if !keys["windows"] {
		t.Error("complex session missing structure row")
	}
	if fieldByKey(t, complexSess, "windows").kind != fieldStructure {
		t.Error("windows row is not fieldStructure")
	}
	// every field carries help text
	for _, f := range append(simple, complexSess...) {
		if f.help == "" {
			t.Errorf("field %q has no help text", f.key)
		}
	}
}

func TestFieldSettersAndCycles(t *testing.T) {
	s := &config.Session{Hosts: []string{"h1"}}
	fields := sessionFields(s)

	// text set
	if err := fieldByKey(t, fields, "connect").set(s, "ssh -A {{host}}"); err != nil {
		t.Fatal(err)
	}
	if s.Connect != "ssh -A {{host}}" {
		t.Fatalf("connect = %q", s.Connect)
	}

	// number set: valid, invalid, out of range
	retry := fieldByKey(t, fields, "retry")
	if err := retry.set(s, "3"); err != nil || s.Retry != 3 {
		t.Fatalf("retry set 3: err=%v val=%d", err, s.Retry)
	}
	if err := retry.set(s, "abc"); err == nil {
		t.Fatal("retry accepted non-number")
	}
	if err := retry.set(s, "99"); err == nil {
		t.Fatal("retry accepted out-of-range")
	}
	if err := retry.set(s, ""); err != nil || s.Retry != 0 {
		t.Fatalf("retry empty → 0: err=%v val=%d", err, s.Retry)
	}

	// bool cycle
	sync := fieldByKey(t, fields, "sync")
	sync.cycle(s)
	if !s.Sync {
		t.Fatal("sync cycle did not enable")
	}

	// tri-state hold: nil → on → off → nil
	hold := fieldByKey(t, fields, "hold")
	hold.cycle(s)
	if s.Hold == nil || !*s.Hold {
		t.Fatal("hold cycle 1: want on")
	}
	hold.cycle(s)
	if s.Hold == nil || *s.Hold {
		t.Fatal("hold cycle 2: want off")
	}
	hold.cycle(s)
	if s.Hold != nil {
		t.Fatal("hold cycle 3: want inherit (nil)")
	}

	// arrange enum cycles through all layouts then back to none
	arr := fieldByKey(t, fields, "arrange")
	seen := map[string]bool{}
	for i := 0; i < len(arrangeLayouts)+1; i++ {
		arr.cycle(s)
		seen[s.Arrange] = true
	}
	if !seen[""] || len(seen) != len(arrangeLayouts)+1 {
		t.Fatalf("arrange cycle covered %v", seen)
	}

	// list accessor points at the live slice
	hosts := fieldByKey(t, fields, "hosts")
	*hosts.list(s) = append(*hosts.list(s), "h2")
	if len(s.Hosts) != 2 {
		t.Fatal("hosts list accessor not live")
	}
}
