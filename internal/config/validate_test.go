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
			// Duplicates are allowed — tmux addresses windows by id, and
			// users commonly have several windows with the same name.
			name:    "duplicate window names allowed",
			key:     "x",
			session: &Session{Windows: []*Window{{Name: "claude", Hosts: []string{"h"}}, {Name: "claude", Hosts: []string{"h"}}}},
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
		{name: "hosts and panes", window: &Window{Name: "w", Hosts: []string{"a"}, Panes: []*Pane{{Split: SplitRoot}}}, wantErr: true},
		{name: "panes and layout", window: &Window{Name: "w", Panes: []*Pane{{Split: SplitRoot}}, Layout: "x"}, wantErr: true},
		{name: "first pane not root", window: &Window{Name: "w", Panes: []*Pane{{Split: SplitVertical}}}, wantErr: true},
		{name: "empty", window: &Window{Name: "w"}, wantErr: true},
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
