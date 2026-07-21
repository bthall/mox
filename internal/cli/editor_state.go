package cli

// editor_state is the non-UI half of the config editor: the loaded config in
// both typed and yaml.Node form, per-session working drafts, and the save
// path that patches only the affected session so the rest of the file —
// comments, ordering, the modeline — survives byte-for-byte.

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"time"

	"gopkg.in/yaml.v3"

	"github.com/bthall/mox/internal/config"
)

// errStaleConfig means the file changed on disk after the editor loaded it.
var errStaleConfig = errors.New("config file changed on disk since the editor loaded it")

// editorState holds the on-disk side of the editor. mtime is recorded at
// load (and after each save) for staleness detection.
type editorState struct {
	path  string
	mtime time.Time
	root  *yaml.Node // retained document tree, patched on save
	cfg   *config.Config
}

// loadEditorState reads, parses (strictly), and validates the config at
// path, keeping both the typed form and the raw node tree.
func loadEditorState(path string) (*editorState, error) {
	data, err := os.ReadFile(path) //nolint:gosec // user-supplied config path is intentional
	if err != nil {
		return nil, fmt.Errorf("read config %s: %w", path, err)
	}
	fi, err := os.Stat(path)
	if err != nil {
		return nil, err
	}

	var root yaml.Node
	if err := yaml.Unmarshal(data, &root); err != nil {
		return nil, fmt.Errorf("parse config %s: %w", path, err)
	}
	if len(root.Content) == 0 || root.Content[0].Kind != yaml.MappingNode {
		return nil, fmt.Errorf("unexpected config structure in %s (root is not a mapping)", path)
	}

	dec := yaml.NewDecoder(bytes.NewReader(data))
	dec.KnownFields(true)
	var cfg config.Config
	if err := dec.Decode(&cfg); err != nil {
		return nil, fmt.Errorf("parse config %s: %w", path, err)
	}
	if err := cfg.Validate(); err != nil {
		return nil, fmt.Errorf("invalid config %s: %w\n\nFix it first: 'mox validate' shows details, 'mox edit' opens $EDITOR", path, err)
	}

	return &editorState{path: path, mtime: fi.ModTime(), root: &root, cfg: &cfg}, nil
}

// sessionDraft is one buffered, unsaved change set for a single session.
// Exactly one draft is active in the editor at a time; guards force a
// save/discard decision before it can be abandoned.
type sessionDraft struct {
	orig    string          // config name this draft edits; "" for brand-new sessions
	name    string          // current name (differs from orig after a rename)
	sess    *config.Session // working copy; ignored when deleted
	deleted bool
	added   bool // brand-new (wizard/duplicate): no orig entry to replace
}

// newDraft starts a clean draft for an existing configured session.
func newDraft(cfg *config.Config, name string) *sessionDraft {
	return &sessionDraft{orig: name, name: name, sess: cloneSession(cfg.Sessions[name])}
}

// cloneSession deep-copies a session via YAML round-trip — the same
// representation equality (sameSessionYAML) is defined over.
func cloneSession(s *config.Session) *config.Session {
	if s == nil {
		return &config.Session{}
	}
	data, err := yaml.Marshal(s)
	if err != nil {
		return &config.Session{}
	}
	var out config.Session
	if err := yaml.Unmarshal(data, &out); err != nil {
		return &config.Session{}
	}
	return &out
}

// dirty reports whether the draft differs from the loaded config.
func (d *sessionDraft) dirty(cfg *config.Config) bool {
	if d == nil {
		return false
	}
	if d.deleted || d.added || d.name != d.orig {
		return true
	}
	return !sameSessionYAML(d.sess, cfg.Sessions[d.orig])
}

// nextConfig returns a copy of the loaded config with the draft applied —
// what the file would hold after saving. Used for pre-save validation.
func (st *editorState) nextConfig(d *sessionDraft) *config.Config {
	next := &config.Config{
		Layouts:  st.cfg.Layouts,
		Sessions: make(map[string]*config.Session, len(st.cfg.Sessions)+1),
	}
	for k, v := range st.cfg.Sessions {
		next.Sessions[k] = v
	}
	if d.deleted {
		delete(next.Sessions, d.orig)
		return next
	}
	if d.orig != "" {
		delete(next.Sessions, d.orig)
	}
	next.Sessions[d.name] = d.sess
	return next
}

// checkStale returns errStaleConfig when the file changed since load.
func (st *editorState) checkStale() error {
	fi, err := os.Stat(st.path)
	if err != nil {
		return err
	}
	if !fi.ModTime().Equal(st.mtime) {
		return errStaleConfig
	}
	return nil
}

// applyDraft validates the draft, re-checks staleness, patches the retained
// node tree, writes the file atomically, and syncs the typed config. On any
// error the file is untouched.
func (st *editorState) applyDraft(d *sessionDraft) error {
	next := st.nextConfig(d)
	if err := next.Validate(); err != nil {
		return err
	}
	if err := st.checkStale(); err != nil {
		return err
	}

	sessMap := findOrCreateMapKey(st.root.Content[0], "sessions")
	switch {
	case d.deleted:
		removeMapKey(sessMap, d.orig)
	default:
		var sn yaml.Node
		if err := sn.Encode(d.sess); err != nil {
			return fmt.Errorf("encode session: %w", err)
		}
		if d.orig == "" || !setMapValue(sessMap, d.orig, &sn) {
			sessMap.Content = append(sessMap.Content,
				&yaml.Node{Kind: yaml.ScalarNode, Value: d.name},
				&sn,
			)
		} else if d.name != d.orig {
			renameMapKey(sessMap, d.orig, d.name)
		}
	}

	if err := writeYAMLNode(st.path, st.root, false); err != nil {
		return err
	}
	st.cfg = next
	if fi, err := os.Stat(st.path); err == nil {
		st.mtime = fi.ModTime()
	}
	return nil
}
