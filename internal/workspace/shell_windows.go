//go:build windows

package workspace

// shell is the program a command runs by, and shellFlag is the argument
// that tells it to read the command.
func shell() string     { return "cmd" }
func shellFlag() string { return "/c" }
