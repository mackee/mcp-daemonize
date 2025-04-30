package daemonize_test

import (
	"context"
	"errors"
	"io"
	"strconv"
	"syscall"
	"testing"
	"time"

	daemonize "github.com/mackee/mcp-daemonize"
)

// TestMemoryLogger verifies that the in-memory logger records and returns lines correctly.
func TestMemoryLogger(t *testing.T) {
	logger := daemonize.NewMemoryLogger()
	// Write a line and verify count
	n, err := logger.Write([]byte("hello world\n"))
	if err != nil {
		t.Fatalf("Write error: %v", err)
	}
	if n != len("hello world\n") {
		t.Errorf("Write returned %d, want %d", n, len("hello world\n"))
	}
	if got := logger.Lines(); got != 1 {
		t.Errorf("Lines() = %d, want 1", got)
	}
	// Read lines from offset 0
	lines, err := logger.ReadLine(0)
	if err != nil {
		t.Fatalf("ReadLine error: %v", err)
	}
	if len(lines) != 1 || lines[0] != "hello world" {
		t.Errorf("ReadLine returned %v, want [\"hello world\"]", lines)
	}
	// After reading, logger should be empty
	if got := logger.Lines(); got != 0 {
		t.Errorf("After ReadLine, Lines() = %d, want 0", got)
	}
	// Read with invalid offsets
	if _, err := logger.ReadLine(0); err != io.EOF {
		t.Errorf("ReadLine on empty logger returned %v, want io.EOF", err)
	}
	if _, err := logger.ReadLine(-1); err != io.EOF {
		t.Errorf("ReadLine with negative offset returned %v, want io.EOF", err)
	}
	// Close should be no-op
	if err := logger.Close(); err != nil {
		t.Errorf("Close() = %v, want nil", err)
	}
}

// TestNewDaemon ensures NewDaemon initializes fields correctly and status is stopped.
func TestNewDaemon(t *testing.T) {
	name := "testd"
	cmds := []string{"echo", "hi"}
	d := daemonize.NewDaemon(name, cmds, t.TempDir())
	if d.Name != name {
		t.Errorf("Name = %q, want %q", d.Name, name)
	}
	if len(d.Commands) != len(cmds) {
		t.Fatalf("Commands length = %d, want %d", len(d.Commands), len(cmds))
	}
	for i, c := range cmds {
		if d.Commands[i] != c {
			t.Errorf("Commands[%d] = %q, want %q", i, d.Commands[i], c)
		}
	}
	// Initial status should be stopped
	status, err := d.Status()
	if err != nil {
		t.Fatalf("Status error: %v", err)
	}
	if status != daemonize.DaemonStatusStopped {
		t.Errorf("Initial Status() = %q, want %q", status, daemonize.DaemonStatusStopped)
	}
}

// TestStopNotRunning ensures stopping a non-started daemon returns ErrDaemonNotRunning.
func TestStopNotRunning(t *testing.T) {
	d := daemonize.NewDaemon("x", []string{"doesnotmatter"}, t.TempDir())
	err := d.Stop(context.Background())
	if err != daemonize.ErrDaemonNotRunning {
		t.Errorf("Stop on non-running daemon returned %v, want ErrDaemonNotRunning", err)
	}
}

// TestStartStop runs a simple long-lived command, checks status changes, and stops it.
func TestStartStop(t *testing.T) {
	// Use cat as a long-running command that waits for input
	d := daemonize.NewDaemon(
		"catproc",
		[]string{"cat"},
		t.TempDir(),
	)
	ctx := context.Background()
	// Start should succeed
	if err := d.Start(ctx); err != nil {
		t.Fatalf("Start error: %v", err)
	}
	// Status should be running
	status, err := d.Status()
	if err != nil {
		t.Fatalf("Status error: %v", err)
	}
	if status != daemonize.DaemonStatusRunning {
		t.Errorf("After Start, Status() = %q, want %q", status, daemonize.DaemonStatusRunning)
	}
	// Stop should succeed
	if err := d.Stop(ctx); err != nil {
		t.Fatalf("Stop error: %v", err)
	}
	// After Stop, status should be stopped
	status, err = d.Status()
	if err != nil {
		t.Fatalf("Status error: %v", err)
	}
	if status != daemonize.DaemonStatusStopped {
		t.Errorf("After Stop, Status() = %q, want %q", status, daemonize.DaemonStatusStopped)
	}
}

// TestStopKillsDescendants ensures that Stop kills both the daemon and its descendant processes.
func TestStopKillsDescendants(t *testing.T) {
	// Spawn a shell that backgrounds a sleep and prints its PID.
	d := daemonize.NewDaemon(
		"withchild",
		[]string{"sh", "-c", "sh -c 'sleep 100 & echo $!; wait'"},
		t.TempDir(),
	)
	logger := d.Logger
	ctx := context.Background()
	if err := d.Start(ctx); err != nil {
		t.Fatalf("Start error: %v", err)
	}
	// Wait for the child PID to be logged
	var childPid int
	for range 50 {
		if logger.Lines() > 0 {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	if logger.Lines() == 0 {
		t.Fatal("timeout waiting for child PID")
	}
	lines, err := logger.ReadLine(0)
	if err != nil {
		t.Fatalf("ReadLine error: %v", err)
	}
	if len(lines) < 1 {
		t.Fatalf("expected at least one log line for child PID, got: %v", lines)
	}
	childPid, err = strconv.Atoi(lines[0])
	if err != nil {
		t.Fatalf("parsing child PID: %v", err)
	}
	// Stop the daemon (should kill both parent and grandchild)
	if err := d.Stop(ctx); err != nil {
		t.Fatalf("Stop error: %v", err)
	}
	// Give the OS a moment to clean up processes
	time.Sleep(50 * time.Millisecond)
	// Verify the child (sleep) process is gone
	err = syscall.Kill(childPid, 0)
	if err == nil {
		t.Errorf("child process %d is still running", childPid)
	} else if !errors.Is(err, syscall.ESRCH) {
		t.Errorf("unexpected error checking child process: %v", err)
	}
}

