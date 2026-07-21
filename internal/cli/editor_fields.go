package cli

// The form's row table. Each fieldDef couples a config field with its edit
// behavior and a one-line help text (condensed from the schema docs in
// internal/config/types.go). The UI never reaches into config.Session
// directly — it goes through these closures, always against the draft copy.

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/bthall/mox/internal/config"
)

type fieldKind int

const (
	fieldText      fieldKind = iota // inline text input
	fieldCycle                      // Enter/Space advances: bool, tri-state, enum
	fieldNumber                     // inline input, digits validated on commit
	fieldList                       // opens the list sub-editor
	fieldStructure                  // complex-mode windows: read-only, 'o' opens $EDITOR
)

type fieldDef struct {
	key  string
	kind fieldKind
	help string

	display func(s *config.Session) string          // value shown in the row
	text    func(s *config.Session) string          // seed for inline editing
	set     func(s *config.Session, v string) error // commit for text/number
	cycle   func(s *config.Session)                 // advance for cycle fields
	list    func(s *config.Session) *[]string       // backing slice for list fields
}

func onOff(b bool) string {
	if b {
		return "on"
	}
	return "off"
}

func orDash(s string) string {
	if s == "" {
		return "—"
	}
	return s
}

func holdDisplay(h *bool) string {
	switch {
	case h == nil:
		return "default (on)"
	case *h:
		return "on"
	default:
		return "off"
	}
}

// cycleHold advances nil (default on) → on → off → nil.
func cycleHold(h **bool) {
	switch {
	case *h == nil:
		v := true
		*h = &v
	case **h:
		v := false
		*h = &v
	default:
		*h = nil
	}
}

// nextArrange cycles "" → tiled → … → main-vertical → "".
func nextArrange(cur string) string {
	if cur == "" {
		return arrangeLayouts[0]
	}
	for i, a := range arrangeLayouts {
		if a == cur {
			if i == len(arrangeLayouts)-1 {
				return ""
			}
			return arrangeLayouts[i+1]
		}
	}
	return ""
}

func listSummary(items []string) string {
	if len(items) == 0 {
		return "—"
	}
	return strings.Join(items, ", ")
}

// sessionFields builds the form rows for a session. Simple-mode-only fields
// (hosts, commands) are omitted for complex sessions, which get a read-only
// structure row instead.
func sessionFields(s *config.Session) []fieldDef {
	isComplex := len(s.Windows) > 0
	var fields []fieldDef

	if !isComplex {
		fields = append(fields, fieldDef{
			key: "hosts", kind: fieldList,
			help:    "Hosts, one pane per host. An entry starting with @ expands the cluster into its members when committed.",
			display: func(s *config.Session) string { return listSummary(s.Hosts) },
			list:    func(s *config.Session) *[]string { return &s.Hosts },
		})
	}
	fields = append(fields,
		fieldDef{
			key: "connect", kind: fieldText,
			help:    "Template used to connect to each host; {{host}} is substituted. Empty = 'ssh {{host}}'.",
			display: func(s *config.Session) string { return orDash(s.Connect) },
			text:    func(s *config.Session) string { return s.Connect },
			set:     func(s *config.Session, v string) error { s.Connect = v; return nil },
		},
		fieldDef{
			key: "ssh_user", kind: fieldText,
			help:    "SSH as this user in the default template ('ssh USER@host'). Ignored when connect is set.",
			display: func(s *config.Session) string { return orDash(s.SSHUser) },
			text:    func(s *config.Session) string { return s.SSHUser },
			set:     func(s *config.Session, v string) error { s.SSHUser = v; return nil },
		},
		fieldDef{
			key: "sync", kind: fieldCycle,
			help:    "Broadcast typed input to every pane (tmux synchronize-panes).",
			display: func(s *config.Session) string { return onOff(s.Sync) },
			cycle:   func(s *config.Session) { s.Sync = !s.Sync },
		},
		fieldDef{
			key: "arrange", kind: fieldCycle,
			help:    "Built-in tmux layout applied after panes are created.",
			display: func(s *config.Session) string { return orDash(s.Arrange) },
			cycle:   func(s *config.Session) { s.Arrange = nextArrange(s.Arrange) },
		},
		fieldDef{
			key: "hold", kind: fieldCycle,
			help:    "When a connection ends, hold the pane (notice + wait for Enter) instead of dropping to a local shell.",
			display: func(s *config.Session) string { return holdDisplay(s.Hold) },
			cycle:   func(s *config.Session) { cycleHold(&s.Hold) },
		},
		fieldDef{
			key: "retry", kind: fieldNumber,
			help:    "Re-run a failed connect up to N extra times, 3s apart (0-10; 0 disables).",
			display: func(s *config.Session) string { return strconv.Itoa(s.Retry) },
			text:    func(s *config.Session) string { return strconv.Itoa(s.Retry) },
			set: func(s *config.Session, v string) error {
				if v == "" {
					s.Retry = 0
					return nil
				}
				n, err := strconv.Atoi(v)
				if err != nil {
					return fmt.Errorf("retry must be a number")
				}
				if n < 0 || n > 10 {
					return fmt.Errorf("retry must be between 0 and 10")
				}
				s.Retry = n
				return nil
			},
		},
		fieldDef{
			key: "root", kind: fieldText,
			help:    "Working directory for the session's panes.",
			display: func(s *config.Session) string { return orDash(s.Root) },
			text:    func(s *config.Session) string { return s.Root },
			set:     func(s *config.Session, v string) error { s.Root = v; return nil },
		},
		fieldDef{
			key: "pre", kind: fieldList,
			help:    "Commands prepended to every pane in this session (environment setup).",
			display: func(s *config.Session) string { return listSummary(s.Pre) },
			list:    func(s *config.Session) *[]string { return &s.Pre },
		},
	)
	if !isComplex {
		fields = append(fields, fieldDef{
			key: "commands", kind: fieldList,
			help:    "Commands sent to each host pane after connect, in order.",
			display: func(s *config.Session) string { return listSummary(s.Commands) },
			list:    func(s *config.Session) *[]string { return &s.Commands },
		})
	}
	fields = append(fields,
		fieldDef{
			key: "on_start", kind: fieldList,
			help:    "Local commands run before the session is built; a failure aborts creation.",
			display: func(s *config.Session) string { return listSummary(s.OnStart) },
			list:    func(s *config.Session) *[]string { return &s.OnStart },
		},
		fieldDef{
			key: "on_stop", kind: fieldList,
			help:    "Local commands run after 'mox kill' destroys the session.",
			display: func(s *config.Session) string { return listSummary(s.OnStop) },
			list:    func(s *config.Session) *[]string { return &s.OnStop },
		},
	)
	if isComplex {
		fields = append(fields, fieldDef{
			key: "windows", kind: fieldStructure,
			help:    "Window/pane structure is edited as YAML — press o to open the config in $EDITOR.",
			display: func(s *config.Session) string { return pluralize(len(s.Windows), "window") },
		})
	}
	return fields
}
