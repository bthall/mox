// Package proc inspects the local process table to recover the command running
// under a given PID. It exists so `mox import` can recover the SSH connection a
// tmux pane is running: tmux exposes only the foreground command's basename
// (#{pane_current_command} == "ssh"), not its arguments, so the target host has
// to be read from the OS process table instead.
//
// Everything here is best-effort. A caller that cannot capture the process
// table should degrade to structure-only behavior rather than fail.
package proc

import (
	"context"
	"os/exec"
	"strconv"
	"strings"
)

// Process is one entry from the process table.
type Process struct {
	PID  int
	PPID int
	Args []string // full argv; Args[0] is the executable
}

// Capture snapshots the process table via `ps`. The flags are chosen to work on
// both Linux (procps) and macOS/BSD: -A (all processes), -ww (don't truncate
// the argv column), and -o with trailing '=' to suppress headers.
func Capture(ctx context.Context) ([]Process, error) {
	out, err := exec.CommandContext(ctx, "ps", "-ww", "-A", "-o", "pid=,ppid=,args=").Output()
	if err != nil {
		return nil, err
	}
	return parsePS(string(out)), nil
}

// parsePS parses `ps -o pid=,ppid=,args=` output: two leading integer columns
// followed by the argv. Lines that don't begin with two integers are skipped.
func parsePS(out string) []Process {
	var procs []Process
	for _, line := range strings.Split(out, "\n") {
		fields := strings.Fields(line)
		if len(fields) < 3 {
			continue
		}
		pid, err1 := strconv.Atoi(fields[0])
		ppid, err2 := strconv.Atoi(fields[1])
		if err1 != nil || err2 != nil {
			continue
		}
		procs = append(procs, Process{PID: pid, PPID: ppid, Args: fields[2:]})
	}
	return procs
}

// ForegroundCommand returns the argv of the shallowest process that is pid
// itself or a descendant of pid and satisfies match. Searching breadth-first
// from pid means the process closest to the pane's shell wins — that is the
// interactive command, not any helper it may have spawned. Returns nil if
// nothing matches.
func ForegroundCommand(procs []Process, pid int, match func(argv []string) bool) []string {
	byPID := make(map[int]Process, len(procs))
	children := make(map[int][]int)
	for _, p := range procs {
		byPID[p.PID] = p
		children[p.PPID] = append(children[p.PPID], p.PID)
	}

	queue := []int{pid}
	seen := make(map[int]bool)
	for len(queue) > 0 {
		cur := queue[0]
		queue = queue[1:]
		if seen[cur] {
			continue
		}
		seen[cur] = true
		if p, ok := byPID[cur]; ok && match(p.Args) {
			return p.Args
		}
		queue = append(queue, children[cur]...)
	}
	return nil
}
