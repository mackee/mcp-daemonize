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
	Name      string
	Commands  []string
	Logger    Logger
	Workdir   string
	cmd       *exec.Cmd
	mu        sync.Mutex
	exitError error
	done      chan struct{}
}

func NewDaemon(name string, commands []string, workdir string) *Daemon {
	logger := NewMemoryLogger()
	return &Daemon{
		Name:      name,
		Commands:  commands,
		Logger:    logger,
		Workdir:   workdir,
		mu:        sync.Mutex{},
		exitError: nil,
		done:      make(chan struct{}),
	}
}

func (d *Daemon) Start(ctx context.Context) error {
	dctx := context.WithoutCancel(ctx)
	d.cmd = exec.CommandContext(dctx, d.Commands[0], d.Commands[1:]...)
	d.cmd.Stdout = d.Logger
	d.cmd.Stderr = d.Logger
	d.cmd.Dir = d.Workdir
	d.cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	if err := d.cmd.Start(); err != nil {
		return fmt.Errorf("failed to start daemon %s: %w", d.Name, err)
	}
	go func() {
		select {
		case <-ctx.Done():
			if status, err := d.Status(); err != nil {
				slog.ErrorContext(ctx, "failed to get daemon status", slog.String("name", d.Name), slog.Any("error", err))
			} else if status != DaemonStatusRunning {
				slog.DebugContext(ctx, "daemon already stopped", slog.String("name", d.Name), slog.String("status", string(status)))
				return
			}
			if err := d.Stop(ctx); err != nil {
				slog.ErrorContext(ctx, "failed to stop daemon", slog.String("name", d.Name), slog.Any("error", err))
			} else {
				slog.InfoContext(ctx, "daemon stopped successfully", slog.String("name", d.Name))
			}
		case <-d.done:
			slog.DebugContext(ctx, "daemon already stopped", slog.String("name", d.Name))
		}
	}()
	go func() {
		defer close(d.done)
		if err := d.cmd.Wait(); err != nil {
			var ee *exec.ExitError
			if errors.As(err, &ee) {
				ws, ok := ee.Sys().(syscall.WaitStatus)
				if ok && ws.Signaled() {
					slog.DebugContext(ctx, "daemon stopped by signal", slog.String("name", d.Name))
					return
				}
				if ee.Exited() && ee.ExitCode() == 0 {
					slog.InfoContext(ctx, "daemon exited successfully", slog.String("name", d.Name))
					return
				}
				slog.ErrorContext(ctx, "daemon exited with error", slog.String("name", d.Name), slog.Any("error", err))
				d.exitError = fmt.Errorf("daemon %s exited with error: %w", d.Name, err)
				return
			}
			slog.ErrorContext(ctx, "daemon exited with error", slog.String("name", d.Name), slog.Any("error", err))
		} else {
			slog.InfoContext(ctx, "daemon exited successfully", slog.String("name", d.Name))
		}
	}()

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

	// Graceful-stop
	if err := syscall.Kill(-pgid, syscall.SIGINT); err != nil && !errors.Is(err, os.ErrProcessDone) {
		return fmt.Errorf("sigterm: %w", err)
	}

	select {
	case <-ctx.Done():
		// 呼び出し側が辛抱切れ → SIGKILL
		_ = syscall.Kill(-pgid, syscall.SIGKILL)
		<-d.done
		slog.InfoContext(ctx, "daemon %s stopped", slog.Any("error", ctx.Err()))
		return ctx.Err()
	case <-d.done:
		if d.exitError != nil {
			return d.exitError
		}
		return nil
	case <-time.After(10 * time.Second):
		_ = syscall.Kill(-pgid, syscall.SIGKILL)
		<-d.done
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
