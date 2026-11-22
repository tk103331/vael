//go:build windows

package proc

import "os/exec"

// SetGroup does nothing on Windows, where a command's children are not
// killed with it.
func SetGroup(cmd *exec.Cmd) {}

// KillGroup kills a command, leaving what it started to its own means.
func KillGroup(cmd *exec.Cmd) {
	if cmd.Process != nil {
		_ = cmd.Process.Kill()
	}
}
