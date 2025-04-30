package daemonize

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"sync"
	"syscall"
	"time"
)

type DaemonStatus string

const (
	DaemonStatusRunning DaemonStatus = "running"
	DaemonStatusStopped DaemonStatus = "stopped"
)

type Daemon struct {
	Name     string
	Commands []string
	Logger   Logger
	Workdir  string
	cmd      *exec.Cmd
	mu       sync.Mutex
}

func NewDaemon(name string, commands []string, workdir string) *Daemon {
	logger := NewMemoryLogger()
	return &Daemon{
		Name:     name,
		Commands: commands,
		Logger:   logger,
		Workdir:  workdir,
		mu:       sync.Mutex{},
	}
}

func (d *Daemon) Start(ctx context.Context) error {
	d.cmd = exec.CommandContext(ctx, d.Commands[0], d.Commands[1:]...)
	d.cmd.Stdout = d.Logger
	d.cmd.Stderr = d.Logger
	d.cmd.Dir = d.Workdir
	d.cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	if err := d.cmd.Start(); err != nil {
		return fmt.Errorf("failed to start daemon %s: %w", d.Name, err)
	}

	return nil
}

var ErrDaemonNotRunning = fmt.Errorf("daemon not running")

func (d *Daemon) pgid() (int, error) {
	if d.cmd == nil || d.cmd.Process == nil {
		return -1, ErrDaemonNotRunning
	}
	return syscall.Getpgid(d.cmd.Process.Pid)
}

func (d *Daemon) Stop(ctx context.Context) error {
	d.mu.Lock()
	defer d.mu.Unlock()

	if d.cmd == nil || d.cmd.Process == nil {
		return ErrDaemonNotRunning
	}

	pgid, err := d.pgid()
	if err != nil {
		return fmt.Errorf("pgid: %w", err)
	}

	done := make(chan error, 1)
	go func() { done <- d.cmd.Wait() }() // Zombie 化防止
	// Graceful-stop
	if err := syscall.Kill(-pgid, syscall.SIGTERM); err != nil && !errors.Is(err, os.ErrProcessDone) {
		return fmt.Errorf("sigterm: %w", err)
	}

	select {
	case <-ctx.Done():
		// 呼び出し側が辛抱切れ → SIGKILL
		_ = syscall.Kill(-pgid, syscall.SIGKILL)
		<-done
		slog.InfoContext(ctx, "daemon %s stopped", slog.Any("error", ctx.Err()))
		return ctx.Err()
	case err := <-done:
		var ee *exec.ExitError
		if errors.As(err, &ee) {
			ws, ok := ee.Sys().(syscall.WaitStatus)
			if ok && ws.Signaled() {
				return nil
			}
			if ee.Exited() && ee.ExitCode() == 0 {
				return nil
			}
			return fmt.Errorf("daemon %s exited with error: exitCode=%d, %w", d.Name, ee.ExitCode(), err)
		}
		return fmt.Errorf("daemon %s exited with error: %w", d.Name, err)
	case <-time.After(10 * time.Second):
		// デフォルト猶予
		_ = syscall.Kill(-pgid, syscall.SIGKILL)
		<-done
		return errors.New("graceful shutdown timed out")
	}
}

func (d *Daemon) Status() (DaemonStatus, error) {
	if d.cmd == nil || d.cmd.Process == nil {
		return DaemonStatusStopped, nil
	}
	pgid, err := d.pgid()
	if err != nil {
		// no such process
		if errors.Is(err, syscall.ESRCH) {
			d.cmd = nil
			return DaemonStatusStopped, nil
		}
		return DaemonStatusStopped, fmt.Errorf("pgid: %w", err)
	}
	if err := syscall.Kill(-pgid, 0); err != nil {
		if errors.Is(err, os.ErrProcessDone) {
			d.cmd = nil
			return DaemonStatusStopped, nil
		}
		return DaemonStatusStopped, fmt.Errorf("daemon %s is not running: %w", d.Name, err)
	}
	return DaemonStatusRunning, nil
}
