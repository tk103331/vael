//go:build !windows

// Package proc runs commands whose children go away with them: a shell
// that is killed otherwise leaves what it ran behind, holding the pipes
// of the editor.
package proc

import (
	"os/exec"
	"syscall"
)

// SetGroup puts a command in a group of processes of its own, so that
// KillGroup takes it and what it starts.
func SetGroup(cmd *exec.Cmd) {
	if cmd.SysProcAttr == nil {
		cmd.SysProcAttr = &syscall.SysProcAttr{}
	}
	cmd.SysProcAttr.Setpgid = true
}

// KillGroup kills a command and everything it started.
func KillGroup(cmd *exec.Cmd) {
	if cmd.Process == nil {
		return
	}
	// The group's id is that of its first process.
	_ = syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
	_ = cmd.Process.Kill()
}
