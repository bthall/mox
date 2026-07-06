package history

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

// newTestStore returns a store backed by a temp file and a controllable clock.
func newTestStore(t *testing.T) (*store, *time.Time) {
	t.Helper()
	clock := time.Date(2026, 6, 2, 12, 0, 0, 0, time.UTC)
	s := &store{
		path: filepath.Join(t.TempDir(), "recent.json"),
		now:  func() time.Time { return clock },
	}
	return s, &clock
}

func TestStore_RecordAndLoadRoundTrip(t *testing.T) {
	s, clock := newTestStore(t)

	if err := s.record("dev", ActionCreated); err != nil {
		t.Fatalf("record: %v", err)
	}
	*clock = clock.Add(time.Minute)
	if err := s.record("prod", ActionAttached); err != nil {
		t.Fatalf("record: %v", err)
	}

	got, err := s.load()
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("want 2 entries, got %d (%+v)", len(got), got)
	}
	// Newest first.
	if got[0].Name != "prod" || got[0].Action != ActionAttached {
		t.Errorf("want prod/attached first, got %+v", got[0])
	}
	if got[1].Name != "dev" || got[1].Action != ActionCreated {
		t.Errorf("want dev/created second, got %+v", got[1])
	}
}

func TestStore_DedupKeepsNewestAction(t *testing.T) {
	s, clock := newTestStore(t)

	if err := s.record("dev", ActionCreated); err != nil {
		t.Fatalf("record: %v", err)
	}
	*clock = clock.Add(time.Hour)
	if err := s.record("dev", ActionAttached); err != nil {
		t.Fatalf("record: %v", err)
	}

	got, _ := s.load()
	if len(got) != 1 {
		t.Fatalf("want 1 deduped entry, got %d (%+v)", len(got), got)
	}
	if got[0].Action != ActionAttached {
		t.Errorf("want newest action attached, got %q", got[0].Action)
	}
}

func TestStore_CapsAtMaxEntries(t *testing.T) {
	s, clock := newTestStore(t)

	for i := 0; i < maxEntries+10; i++ {
		*clock = clock.Add(time.Minute)
		if err := s.record("session-"+string(rune('A'+i%26))+string(rune('0'+i/26)), ActionCreated); err != nil {
			t.Fatalf("record: %v", err)
		}
	}
	got, _ := s.load()
	if len(got) != maxEntries {
		t.Fatalf("want cap of %d, got %d", maxEntries, len(got))
	}
}

func TestStore_CorruptFileTreatedAsEmpty(t *testing.T) {
	s, _ := newTestStore(t)
	if err := os.MkdirAll(filepath.Dir(s.path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(s.path, []byte("not json{{"), 0o600); err != nil {
		t.Fatal(err)
	}

	got, err := s.load()
	if err != nil {
		t.Fatalf("load should not error on corrupt file: %v", err)
	}
	if len(got) != 0 {
		t.Errorf("want empty on corrupt file, got %+v", got)
	}

	// A subsequent record should recover by overwriting with valid JSON.
	if err := s.record("dev", ActionCreated); err != nil {
		t.Fatalf("record after corrupt: %v", err)
	}
	got, _ = s.load()
	if len(got) != 1 {
		t.Errorf("want 1 entry after recovery, got %d", len(got))
	}
}

func TestStore_MissingFileLoadsEmpty(t *testing.T) {
	s, _ := newTestStore(t)
	got, err := s.load()
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if len(got) != 0 {
		t.Errorf("want empty for missing file, got %+v", got)
	}
}

func TestStore_EmptyPathIsNoOp(t *testing.T) {
	s := &store{path: "", now: time.Now}
	if err := s.record("dev", ActionCreated); err != nil {
		t.Errorf("record with empty path should be a no-op, got %v", err)
	}
	got, err := s.load()
	if err != nil || len(got) != 0 {
		t.Errorf("load with empty path: got %v, %v", got, err)
	}
}

func TestDefaultPath_HonorsXDGStateHome(t *testing.T) {
	t.Setenv("XDG_STATE_HOME", "/custom/state")
	if got := defaultPath(); got != filepath.Join("/custom/state", "mox", "recent.json") {
		t.Errorf("defaultPath() = %q", got)
	}
}
