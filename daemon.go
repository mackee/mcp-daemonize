package daemonize

import (
	"context"
	"fmt"
	"os/exec"
	"syscall"
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
	cmd      *exec.Cmd
}

func NewDaemon(name string, commands []string) *Daemon {
	logger := NewMemoryLogger()
	return &Daemon{
		Name:     name,
		Commands: commands,
		Logger:   logger,
	}
}

func (d *Daemon) Start(ctx context.Context) error {
	d.cmd = exec.CommandContext(ctx, d.Commands[0], d.Commands[1:]...)
	d.cmd.Stdout = d.Logger
	d.cmd.Stderr = d.Logger
	d.cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	if err := d.cmd.Start(); err != nil {
		return fmt.Errorf("failed to start daemon %s: %w", d.Name, err)
	}

	return nil
}

var ErrDaemonNotRunning = fmt.Errorf("daemon not running")

func (d *Daemon) Stop(ctx context.Context) error {
	if d.cmd == nil || d.cmd.Process == nil {
		return ErrDaemonNotRunning
	}
	ch := make(chan error)
	defer close(ch)
	go func() {
		if err := d.cmd.Wait(); err != nil {
			ch <- fmt.Errorf("failed to wait for daemon %s: %w", d.Name, err)
		}
	}()
	tctx, cancel := context.WithTimeout(ctx, 5)
	defer cancel()
	pgid, err := syscall.Getpgid(d.cmd.Process.Pid)
	if err != nil {
		return fmt.Errorf("failed to get pgid of daemon %s: %w", d.Name, err)
	}
	if err := syscall.Kill(-pgid, syscall.SIGTERM); err != nil {
		return fmt.Errorf("failed to stop daemon %s: %w", d.Name, err)
	}
	select {
	case <-tctx.Done():
		if err := d.cmd.Process.Kill(); err != nil {
			return fmt.Errorf("failed to kill daemon %s: %w", d.Name, err)
		}
	case err := <-ch:
		if err != nil {
			return err
		}
	}
	d.cmd = nil
	d.Logger = NewMemoryLogger()

	return nil
}

func (d *Daemon) Status() DaemonStatus {
	if d.cmd == nil || d.cmd.Process == nil {
		return DaemonStatusStopped
	}
	return DaemonStatusRunning
}
