package workspace

import (
	"bufio"
	"context"
	"errors"
	"io"
	"os/exec"
	"strings"
	"time"

	"vael/internal/proc"
)

// RunResult is what a command left behind: what it printed, and how it ended.
type RunResult struct {
	Output   string
	Code     int
	Duration int64 // milliseconds
	TimedOut bool
}

var errEmptyCommand = errors.New("请输入要运行的命令")

// Run runs a command in a directory and waits for it. The command is run by
// the system's shell, so that it may hold pipes and redirections.
func Run(ctx context.Context, dir, command string, timeout time.Duration) (RunResult, error) {
	return RunStream(ctx, dir, command, timeout, nil)
}

// RunStream runs a command in a directory and hands its output to a sink as
// the command prints it, so that a caller shows a build as it runs instead
// of after it. The result still holds all of the output.
func RunStream(ctx context.Context, dir, command string, timeout time.Duration, sink func([]byte)) (RunResult, error) {
	command = strings.TrimSpace(command)
	if command == "" {
		return RunResult{}, errEmptyCommand
	}
	if timeout <= 0 {
		timeout = 2 * time.Minute
	}
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	start := time.Now()
	cmd := exec.CommandContext(ctx, shell(), shellFlag(), command)
	cmd.Dir = dir
	// The command's children go with it when its time is up, and Wait
	// does not wait on pipes they hold.
	proc.SetGroup(cmd)
	cmd.Cancel = func() error {
		proc.KillGroup(cmd)
		return nil
	}
	cmd.WaitDelay = 2 * time.Second
	reader, writer := io.Pipe()
	cmd.Stdout = writer
	cmd.Stderr = writer
	if err := cmd.Start(); err != nil {
		return RunResult{Code: -1, Output: err.Error(), Duration: time.Since(start).Milliseconds()}, nil
	}
	drained := make(chan struct{})
	output := &strings.Builder{}
	go func() {
		defer close(drained)
		buffered := bufio.NewReaderSize(reader, 32<<10)
		for {
			chunk, err := buffered.ReadBytes('\n')
			if len(chunk) > 0 {
				output.Write(chunk)
				if sink != nil {
					sink(chunk)
				}
			}
			if err != nil {
				return
			}
		}
	}()
	err := cmd.Wait()
	// Wait has reaped the command; what it printed has all been written.
	_ = writer.Close()
	<-drained
	result := RunResult{Output: output.String(), Duration: time.Since(start).Milliseconds()}
	if ctx.Err() != nil && errors.Is(ctx.Err(), context.DeadlineExceeded) {
		result.TimedOut = true
	}
	switch {
	case cmd.ProcessState != nil:
		result.Code = cmd.ProcessState.ExitCode()
	case err != nil:
		result.Code = -1
	}
	return result, nil
}
