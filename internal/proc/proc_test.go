package proc

import (
	"reflect"
	"testing"
)

func TestParsePS(t *testing.T) {
	// `ps -ww -A -o pid=,ppid=,args=` style output: two integer columns then argv.
	out := "    1     0 /sbin/init\n" +
		" 1000   999 -bash\n" +
		" 1001  1000 ssh deploy@web-1.example.com\n" +
		" 1002  1000 ssh -p 2222 db-1\n"

	got := parsePS(out)
	if len(got) != 4 {
		t.Fatalf("want 4 processes, got %d (%+v)", len(got), got)
	}
	if got[2].PID != 1001 || got[2].PPID != 1000 {
		t.Errorf("row 2 pid/ppid wrong: %+v", got[2])
	}
	if !reflect.DeepEqual(got[2].Args, []string{"ssh", "deploy@web-1.example.com"}) {
		t.Errorf("row 2 args wrong: %v", got[2].Args)
	}
	if !reflect.DeepEqual(got[3].Args, []string{"ssh", "-p", "2222", "db-1"}) {
		t.Errorf("row 3 args wrong: %v", got[3].Args)
	}
}

func TestParsePS_SkipsMalformed(t *testing.T) {
	out := "\n" + // blank
		"notanumber 5 foo\n" + // non-integer pid
		"  10\n" + // too few fields
		"  20  10 bash\n"
	got := parsePS(out)
	if len(got) != 1 || got[0].PID != 20 {
		t.Fatalf("only the well-formed row should survive, got %+v", got)
	}
}

func TestForegroundCommand_DirectChild(t *testing.T) {
	// pane shell 1000 has an ssh child 1001.
	procs := []Process{
		{PID: 1000, PPID: 1, Args: []string{"-bash"}},
		{PID: 1001, PPID: 1000, Args: []string{"ssh", "web-1"}},
	}
	got := ForegroundCommand(procs, 1000, isSSH)
	if !reflect.DeepEqual(got, []string{"ssh", "web-1"}) {
		t.Errorf("want ssh child argv, got %v", got)
	}
}

func TestForegroundCommand_PaneItselfMatches(t *testing.T) {
	// shell exec'd into ssh, so the pane PID *is* the ssh process.
	procs := []Process{{PID: 1000, PPID: 1, Args: []string{"ssh", "web-1"}}}
	got := ForegroundCommand(procs, 1000, isSSH)
	if !reflect.DeepEqual(got, []string{"ssh", "web-1"}) {
		t.Errorf("want pane's own argv, got %v", got)
	}
}

func TestForegroundCommand_ShallowestWins(t *testing.T) {
	// ssh (1001) under shell (1000); ssh has its own ssh child (1002, e.g. a
	// ProxyJump helper). We want the interactive one closest to the shell.
	procs := []Process{
		{PID: 1000, PPID: 1, Args: []string{"-bash"}},
		{PID: 1001, PPID: 1000, Args: []string{"ssh", "web-1"}},
		{PID: 1002, PPID: 1001, Args: []string{"ssh", "jump-host"}},
	}
	got := ForegroundCommand(procs, 1000, isSSH)
	if !reflect.DeepEqual(got, []string{"ssh", "web-1"}) {
		t.Errorf("want shallowest ssh, got %v", got)
	}
}

func TestForegroundCommand_NoMatch(t *testing.T) {
	procs := []Process{
		{PID: 1000, PPID: 1, Args: []string{"-bash"}},
		{PID: 1001, PPID: 1000, Args: []string{"vim", "notes.txt"}},
	}
	if got := ForegroundCommand(procs, 1000, isSSH); got != nil {
		t.Errorf("want nil when no ssh descendant, got %v", got)
	}
}

// isSSH is a test matcher mirroring the real caller's predicate.
func isSSH(argv []string) bool { return len(argv) > 0 && argv[0] == "ssh" }
