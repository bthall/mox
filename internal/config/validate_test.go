package config

import (
	"strings"
	"testing"
)

func TestSessionValidate(t *testing.T) {
	tests := []struct {
		name      string
		key       string
		session   *Session
		wantErr   bool
		errSubstr string
	}{
		{
			name:    "valid simple session",
			key:     "dev",
			session: &Session{Hosts: []string{"host1", "host2"}},
		},
		{
			name: "valid complex session",
			key:  "dev",
			session: &Session{
				Windows: []*Window{{Name: "w1", Hosts: []string{"h1"}}},
			},
		},
		{
			name:      "both hosts and windows",
			key:       "x",
			session:   &Session{Hosts: []string{"h"}, Windows: []*Window{{Name: "w", Hosts: []string{"h"}}}},
			wantErr:   true,
			errSubstr: "both 'hosts' and 'windows'",
		},
		{
			name:    "neither hosts nor windows is now valid (local session)",
			key:     "x",
			session: &Session{},
			wantErr: false,
		},
		{
			name:    "commands without hosts is now valid (local with commands)",
			key:     "x",
			session: &Session{Commands: []string{"echo hi"}},
			wantErr: false,
		},
		{
			name:      "session-level commands in complex mode",
			key:       "x",
			session:   &Session{Commands: []string{"echo hi"}, Windows: []*Window{{Name: "w", Hosts: []string{"h"}}}},
			wantErr:   true,
			errSubstr: "no effect in complex mode",
		},
		{
			name:      "name with colon",
			key:       "bad:name",
			session:   &Session{Hosts: []string{"h"}},
			wantErr:   true,
			errSubstr: "reserved character",
		},
		{
			name:      "name with semicolon",
			key:       "bad;name",
			session:   &Session{Hosts: []string{"h"}},
			wantErr:   true,
			errSubstr: "reserved character",
		},
		{
			name:      "name with dot",
			key:       "bad.name",
			session:   &Session{Hosts: []string{"h"}},
			wantErr:   true,
			errSubstr: "reserved character",
		},
		{
			name:      "name with whitespace",
			key:       "bad name",
			session:   &Session{Hosts: []string{"h"}},
			wantErr:   true,
			errSubstr: "reserved character",
		},
		{
			name:      "host with shell metacharacter",
			key:       "x",
			session:   &Session{Hosts: []string{"good", "evil; rm -rf"}},
			wantErr:   true,
			errSubstr: "unsafe to pass",
		},
		{
			name:      "host starting with hyphen",
			key:       "x",
			session:   &Session{Hosts: []string{"-v"}},
			wantErr:   true,
			errSubstr: "cannot start with '-'",
		},
		{
			name:      "host option injection",
			key:       "x",
			session:   &Session{Hosts: []string{"-Ffoo.conf"}},
			wantErr:   true,
			errSubstr: "cannot start with '-'",
		},
		{
			name:      "host user@host with hyphen host",
			key:       "x",
			session:   &Session{Hosts: []string{"user@-bad"}},
			wantErr:   true,
			errSubstr: "invalid user@host syntax",
		},
		{
			name:      "host user@host with hyphen user",
			key:       "x",
			session:   &Session{Hosts: []string{"-user@bad"}},
			wantErr:   true,
			errSubstr: "cannot start with '-'",
		},
		{
			name:      "host user@host empty user",
			key:       "x",
			session:   &Session{Hosts: []string{"@bad"}},
			wantErr:   true,
			errSubstr: "invalid user@host syntax",
		},
		{
			name:      "host user@host empty host",
			key:       "x",
			session:   &Session{Hosts: []string{"user@"}},
			wantErr:   true,
			errSubstr: "invalid user@host syntax",
		},
		{
			name:      "host user@host multiple at",
			key:       "x",
			session:   &Session{Hosts: []string{"user@host@extra"}},
			wantErr:   true,
			errSubstr: "invalid user@host syntax",
		},
		{
			name:    "valid host with user@host",
			key:     "x",
			session: &Session{Hosts: []string{"deploy@web-1.example.com"}},
		},
		{
			name:    "valid host with percent identifier",
			key:     "x",
			session: &Session{Hosts: []string{"gateway%eth0"}},
		},
		{
			name:      "ssh_user starting with hyphen",
			key:       "x",
			session:   &Session{SSHUser: "-Ffoo.conf", Hosts: []string{"host1"}},
			wantErr:   true,
			errSubstr: "cannot start with '-'",
		},
		{
			// Duplicates are allowed — tmux addresses windows by id, and
			// users commonly have several windows with the same name.
			name:    "duplicate window names allowed",
			key:     "x",
			session: &Session{Windows: []*Window{{Name: "claude", Hosts: []string{"h"}}, {Name: "claude", Hosts: []string{"h"}}}},
		},
		{
			name:      "on_start hook with newline",
			key:       "x",
			session:   &Session{OnStart: []string{"echo 1\necho 2"}},
			wantErr:   true,
			errSubstr: "on_start[0] contains forbidden control characters or newline",
		},
		{
			name:      "on_stop hook with carriage return",
			key:       "x",
			session:   &Session{OnStop: []string{"echo 1\r"}},
			wantErr:   true,
			errSubstr: "on_stop[0] contains forbidden control characters or newline",
		},
		{
			name:      "on_start hook with null byte",
			key:       "x",
			session:   &Session{OnStart: []string{"echo \x00 test"}},
			wantErr:   true,
			errSubstr: "on_start[0] contains forbidden control characters or newline",
		},
		{
			name:      "session pre command with newline",
			key:       "x",
			session:   &Session{Pre: []string{"echo 1\necho 2"}},
			wantErr:   true,
			errSubstr: "pre[0] contains forbidden control characters or newline",
		},
		{
			name:      "session commands with newline",
			key:       "x",
			session:   &Session{Commands: []string{"echo 1\necho 2"}},
			wantErr:   true,
			errSubstr: "commands[0] contains forbidden control characters or newline",
		},
		{
			name:      "session connect template with newline",
			key:       "x",
			session:   &Session{Connect: "ssh {{host}}\nrm -rf /"},
			wantErr:   true,
			errSubstr: "connect contains forbidden control characters or newline",
		},
		{
			name:      "session name with control character",
			key:       "bad\x1bname",
			session:   &Session{Hosts: []string{"h"}},
			wantErr:   true,
			errSubstr: "forbidden control characters",
		},
		{
			name:      "session root with newline",
			key:       "x",
			session:   &Session{Root: "~/dir\n"},
			wantErr:   true,
			errSubstr: "root contains forbidden control characters or newline",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.session.Validate(tt.key)
			if (err != nil) != tt.wantErr {
				t.Fatalf("Validate() error = %v, wantErr = %v", err, tt.wantErr)
			}
			if tt.errSubstr != "" && err != nil && !strings.Contains(err.Error(), tt.errSubstr) {
				t.Errorf("error %q does not contain %q", err.Error(), tt.errSubstr)
			}
		})
	}
}

func TestWindowValidate(t *testing.T) {
	tests := []struct {
		name    string
		window  *Window
		wantErr bool
	}{
		{name: "valid hosts", window: &Window{Name: "w", Hosts: []string{"a"}}},
		{name: "valid panes", window: &Window{Name: "w", Panes: []*Pane{{Split: SplitRoot}, {Split: SplitVertical}}}},
		{name: "valid layout", window: &Window{Name: "w", Layout: "two-pane"}},
		{name: "missing name", window: &Window{Hosts: []string{"a"}}, wantErr: true},
		{name: "name with dot", window: &Window{Name: "bad.name", Hosts: []string{"a"}}, wantErr: true},
		{name: "name with semicolon", window: &Window{Name: "bad;name", Hosts: []string{"a"}}, wantErr: true},
		{name: "hosts and panes", window: &Window{Name: "w", Hosts: []string{"a"}, Panes: []*Pane{{Split: SplitRoot}}}, wantErr: true},
		{name: "panes and layout", window: &Window{Name: "w", Panes: []*Pane{{Split: SplitRoot}}, Layout: "x"}, wantErr: true},
		{name: "first pane not root", window: &Window{Name: "w", Panes: []*Pane{{Split: SplitVertical}}}, wantErr: true},
		{name: "empty", window: &Window{Name: "w"}, wantErr: true},
		{name: "window pre with newline", window: &Window{Name: "w", Hosts: []string{"a"}, Pre: []string{"echo\nhi"}}, wantErr: true},
		{name: "window commands with newline", window: &Window{Name: "w", Hosts: []string{"a"}, Commands: []string{"echo\nhi"}}, wantErr: true},
		{name: "window connect with newline", window: &Window{Name: "w", Hosts: []string{"a"}, Connect: "ssh {{host}}\n"}, wantErr: true},
		{name: "window name with control char", window: &Window{Name: "w1\x1b", Hosts: []string{"a"}}, wantErr: true},
		{name: "window root with newline", window: &Window{Name: "w1", Root: "~/dir\n", Hosts: []string{"a"}}, wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.window.Validate()
			if (err != nil) != tt.wantErr {
				t.Errorf("Validate() error = %v, wantErr = %v", err, tt.wantErr)
			}
		})
	}
}

func TestPaneValidate(t *testing.T) {
	tests := []struct {
		name    string
		pane    *Pane
		isFirst bool
		wantErr bool
	}{
		{name: "root as first", pane: &Pane{Split: SplitRoot}, isFirst: true},
		{name: "root not first", pane: &Pane{Split: SplitRoot}, isFirst: false, wantErr: true},
		{name: "vertical not first", pane: &Pane{Split: SplitVertical, Size: 30}, isFirst: false},
		{name: "horizontal not first", pane: &Pane{Split: SplitHorizontal}, isFirst: false},
		{name: "vertical as first", pane: &Pane{Split: SplitVertical}, isFirst: true, wantErr: true},
		{name: "size on root", pane: &Pane{Split: SplitRoot, Size: 30}, isFirst: true, wantErr: true},
		{name: "invalid split", pane: &Pane{Split: "diagonal"}, isFirst: false, wantErr: true},
		{name: "size 100", pane: &Pane{Split: SplitVertical, Size: 100}, isFirst: false, wantErr: true},
		{name: "size negative", pane: &Pane{Split: SplitVertical, Size: -1}, isFirst: false, wantErr: true},
		{name: "pane commands with newline", pane: &Pane{Split: SplitRoot, Commands: []string{"echo\nhi"}}, isFirst: true, wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.pane.Validate(tt.isFirst)
			if (err != nil) != tt.wantErr {
				t.Errorf("Validate(isFirst=%v) error = %v, wantErr = %v", tt.isFirst, err, tt.wantErr)
			}
		})
	}
}

func TestLayoutValidate(t *testing.T) {
	tests := []struct {
		name    string
		key     string
		layout  *Layout
		wantErr bool
	}{
		{
			name:   "valid layout",
			key:    "two-pane",
			layout: &Layout{Panes: []*Pane{{Split: SplitRoot}, {Split: SplitVertical, Size: 30}}},
		},
		{
			name:    "missing name",
			key:     "",
			layout:  &Layout{Panes: []*Pane{{Split: SplitRoot}}},
			wantErr: true,
		},
		{
			name:    "name with control character",
			key:     "layout\x1b",
			layout:  &Layout{Panes: []*Pane{{Split: SplitRoot}}},
			wantErr: true,
		},
		{
			name:    "name with reserved semicolon",
			key:     "layout;bad",
			layout:  &Layout{Panes: []*Pane{{Split: SplitRoot}}},
			wantErr: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.layout.Validate(tt.key)
			if (err != nil) != tt.wantErr {
				t.Errorf("Validate() error = %v, wantErr = %v", err, tt.wantErr)
			}
		})
	}
}

func TestConfigValidateLayoutReferences(t *testing.T) {
	tests := []struct {
		name    string
		config  *Config
		wantErr bool
		errSub  string
	}{
		{
			name: "valid",
			config: &Config{
				Layouts: map[string]*Layout{
					"two-pane": {Panes: []*Pane{{Split: SplitRoot}, {Split: SplitVertical, Size: 30}}},
				},
				Sessions: map[string]*Session{
					"test": {Windows: []*Window{{Name: "w", Layout: "two-pane"}}},
				},
			},
		},
		{
			name: "layout not found",
			config: &Config{
				Sessions: map[string]*Session{
					"test": {Windows: []*Window{{Name: "w", Layout: "nope"}}},
				},
			},
			wantErr: true,
			errSub:  "no layouts: section",
		},
		{
			name:    "no sessions",
			config:  &Config{},
			wantErr: true,
			errSub:  "no sessions defined",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.config.Validate()
			if (err != nil) != tt.wantErr {
				t.Errorf("Validate() error = %v, wantErr = %v", err, tt.wantErr)
			}
			if tt.errSub != "" && err != nil && !strings.Contains(err.Error(), tt.errSub) {
				t.Errorf("error %q does not contain %q", err.Error(), tt.errSub)
			}
		})
	}
}
