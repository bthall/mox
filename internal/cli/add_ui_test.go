package cli

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/bthall/mox/internal/config"
)

func newTestAddModel() addModel {
	cfg := &config.Config{Sessions: map[string]*config.Session{
		"existing": {Hosts: []string{"a", "b"}},
	}}
	clusters := map[string][]string{"lab": {"lab1", "lab2", "lab3"}}
	return newAddModel(cfg, clusters, "")
}

func sendAdd(t *testing.T, m addModel, keys ...tea.KeyMsg) addModel {
	t.Helper()
	for _, k := range keys {
		next, _ := m.Update(k)
		m = next.(addModel)
	}
	return m
}

// through drives the model with typed text followed by Enter.
func through(t *testing.T, m addModel, text string) addModel {
	t.Helper()
	m = sendAdd(t, m, runes(text)...)
	return sendAdd(t, m, keyEnter)
}

func TestAddWizard_HappyPathBroadcastSession(t *testing.T) {
	m := newTestAddModel()

	m = through(t, m, "dbfarm")    // name
	m = through(t, m, "db1 db2")   // hosts
	m = through(t, m, "root")      // ssh user
	m = sendAdd(t, m, keyEnter)    // sync: keep default (on for >1 host)
	m = sendAdd(t, m, keyEnter)    // arrange: keep default (tiled)
	m = through(t, m, "~/ops")     // root dir
	m = through(t, m, "sudo -i")   // one command
	m = sendAdd(t, m, keyEnter)    // empty command -> confirm step
	m = sendAdd(t, m, keyEnter)    // confirm: save (first choice)

	if m.done.action != addActionSave {
		t.Fatalf("action = %v, want save", m.done.action)
	}
	if m.done.name != "dbfarm" {
		t.Errorf("name = %q", m.done.name)
	}
	s := m.done.sess
	if s == nil {
		t.Fatal("no session built")
	}
	if len(s.Hosts) != 2 || s.Hosts[0] != "db1" {
		t.Errorf("hosts = %v", s.Hosts)
	}
	if s.SSHUser != "root" {
		t.Errorf("ssh_user = %q", s.SSHUser)
	}
	if !s.Sync {
		t.Error("sync should default on for a multi-host session")
	}
	if s.Arrange != "tiled" {
		t.Errorf("arrange = %q, want tiled", s.Arrange)
	}
	if s.Root != "~/ops" {
		t.Errorf("root = %q", s.Root)
	}
	if len(s.Commands) != 1 || s.Commands[0] != "sudo -i" {
		t.Errorf("commands = %v", s.Commands)
	}
	if err := s.Validate(m.done.name); err != nil {
		t.Errorf("built session must validate: %v", err)
	}
}

func TestAddWizard_NameValidation(t *testing.T) {
	m := newTestAddModel()
	m = through(t, m, "bad name")
	if m.step != stepName {
		t.Error("reserved character in name must not advance")
	}
	if m.errMsg == "" {
		t.Error("expected an inline error message")
	}

	// Empty name is also rejected.
	m = newTestAddModel()
	m = sendAdd(t, m, keyEnter)
	if m.step != stepName {
		t.Error("empty name must not advance")
	}
}

func TestAddWizard_CollisionNeedsSecondEnter(t *testing.T) {
	m := newTestAddModel()
	m = through(t, m, "existing")
	if m.step != stepName {
		t.Fatal("collision should hold on the name step first")
	}
	if m.errMsg == "" {
		t.Error("collision should explain itself")
	}
	m = sendAdd(t, m, keyEnter) // confirm overwrite
	if m.step != stepHosts {
		t.Error("second Enter should accept the overwrite and advance")
	}
	if !m.overwrite {
		t.Error("overwrite flag should be set")
	}
}

func TestAddWizard_ClusterExpansionPreview(t *testing.T) {
	m := newTestAddModel()
	m = through(t, m, "labs")
	m = sendAdd(t, m, runes("@lab")...)
	if len(m.expanded) != 3 || m.expanded[0] != "lab1" {
		t.Errorf("expanded = %v, want lab1..lab3", m.expanded)
	}
	view := m.View()
	if !strings.Contains(view, "lab1") {
		t.Error("view should preview the expanded hosts")
	}
}

func TestAddWizard_UnknownClusterHolds(t *testing.T) {
	m := newTestAddModel()
	m = through(t, m, "labs")
	m = through(t, m, "@nope")
	if m.step != stepHosts {
		t.Error("unknown cluster must not advance")
	}
}

func TestAddWizard_LocalSessionSkipsBroadcastSteps(t *testing.T) {
	m := newTestAddModel()
	m = through(t, m, "scratch")
	m = sendAdd(t, m, keyEnter) // no hosts
	// ssh user, sync, and arrange make no sense without hosts.
	if m.step != stepRoot {
		t.Errorf("step = %v, want stepRoot (host-only steps skipped)", m.step)
	}
	m = sendAdd(t, m, keyEnter) // root: skip
	m = through(t, m, "htop")   // one command
	m = sendAdd(t, m, keyEnter) // finish commands
	m = sendAdd(t, m, keyEnter) // save

	s := m.done.sess
	if s == nil {
		t.Fatal("no session built")
	}
	if len(s.Hosts) != 0 || s.Sync || s.Arrange != "" || s.SSHUser != "" {
		t.Errorf("local session should have no broadcast fields: %+v", s)
	}
	if err := s.Validate("scratch"); err != nil {
		t.Errorf("built session must validate: %v", err)
	}
}

func TestAddWizard_SyncToggleAndArrangeChoice(t *testing.T) {
	m := newTestAddModel()
	m = through(t, m, "web")
	m = through(t, m, "w1 w2")
	m = sendAdd(t, m, keyEnter) // ssh user: skip
	// Toggle sync off, then advance.
	m = sendAdd(t, m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'n'}})
	m = sendAdd(t, m, keyEnter)
	// Move arrange selection off the default.
	m = sendAdd(t, m, keyDown, keyEnter)
	m = sendAdd(t, m, keyEnter) // root: skip
	m = sendAdd(t, m, keyEnter) // commands: none
	m = sendAdd(t, m, keyEnter) // save

	s := m.done.sess
	if s == nil {
		t.Fatal("no session built")
	}
	if s.Sync {
		t.Error("sync was toggled off")
	}
	if s.Arrange == "tiled" {
		t.Errorf("arrange should have moved off the default, got %q", s.Arrange)
	}
}

func TestAddWizard_EscGoesBackThenCancels(t *testing.T) {
	m := newTestAddModel()
	m = through(t, m, "web")
	if m.step != stepHosts {
		t.Fatal("setup: expected hosts step")
	}
	m = sendAdd(t, m, keyEsc)
	if m.step != stepName {
		t.Error("esc should go back one step")
	}
	m = sendAdd(t, m, keyEsc)
	if m.done.action != addActionCancel {
		t.Error("esc on the first step should cancel")
	}
}

func TestAddWizard_CtrlCCancelsAnywhere(t *testing.T) {
	m := newTestAddModel()
	m = through(t, m, "web")
	m = sendAdd(t, m, keyCtrlC)
	if m.done.action != addActionCancel {
		t.Error("ctrl-c should cancel")
	}
}

func TestAddWizard_ConfirmShowsYAMLAndOffersStart(t *testing.T) {
	m := newTestAddModel()
	m = through(t, m, "dbfarm")
	m = through(t, m, "db1 db2")
	m = sendAdd(t, m, keyEnter, keyEnter, keyEnter, keyEnter, keyEnter) // defaults through to confirm
	if m.step != stepConfirm {
		t.Fatalf("step = %v, want confirm", m.step)
	}
	view := m.View()
	if !strings.Contains(view, "hosts") || !strings.Contains(view, "db1") {
		t.Error("confirm view should show the YAML preview")
	}
	// Second choice is save + start.
	m = sendAdd(t, m, keyDown, keyEnter)
	if m.done.action != addActionSaveStart {
		t.Errorf("action = %v, want save+start", m.done.action)
	}
}

func TestAddWizard_PrefilledName(t *testing.T) {
	cfg := &config.Config{Sessions: map[string]*config.Session{}}
	m := newAddModel(cfg, nil, "prefilled")
	m = sendAdd(t, m, keyEnter)
	if m.step != stepHosts {
		t.Error("prefilled name should be accepted by Enter")
	}
	if m.name != "prefilled" {
		t.Errorf("name = %q", m.name)
	}
}
