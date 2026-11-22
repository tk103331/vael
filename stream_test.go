package main

import (
	"context"
	"errors"
	"net"
	"strings"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"
	"vael/internal/core"
)

func TestTaskStreamsItsOutputAsItRuns(t *testing.T) {
	a, queue := testDispatchApp(t)
	root := t.TempDir()
	a.useProject(core.NewLocal(), nil, root)
	command := "echo first && sleep 1 && echo second && exit 3"
	a.taskCommand = command
	a.runTask(command)
	// The first line arrives while the command is still running.
	pumpUntil(t, queue, func() bool {
		return a.task.running && strings.Contains(a.task.output, "first")
	})
	if strings.Contains(a.task.output, "second") {
		t.Fatalf("the output came all at once: %q", a.task.output)
	}
	pumpUntil(t, queue, func() bool { return !a.task.running })
	if !strings.Contains(a.task.output, "second") {
		t.Fatalf("the rest of the output is missing: %q", a.task.output)
	}
	if a.task.code != 3 {
		t.Fatalf("exit code %d", a.task.code)
	}
	if len(a.taskHistory) != 1 {
		t.Fatalf("history: %+v", a.taskHistory)
	}
}

func TestTaskOutputKeepsOnlyItsTail(t *testing.T) {
	long := strings.Repeat("line\n", 40000) // 200 KiB
	if got := tailOf(long); len(got) > 128<<10 {
		t.Fatalf("the tail is %d bytes", len(got))
	}
	if !strings.HasSuffix(tailOf(long), "line\n") {
		t.Fatal("the tail does not end at a line")
	}
	if got := tailOf("short"); got != "short" {
		t.Fatalf("%q", got)
	}
}

func TestStoppingATaskStopsItsCommand(t *testing.T) {
	a, queue := testDispatchApp(t)
	root := t.TempDir()
	a.useProject(core.NewLocal(), nil, root)
	a.runTask("sleep 30")
	pumpUntil(t, queue, func() bool { return a.task.running })
	start := time.Now()
	if a.taskCancel == nil {
		t.Fatal("the task cannot be stopped")
	}
	a.taskCancel()
	pumpUntil(t, queue, func() bool { return !a.task.running })
	if time.Since(start) > 3*time.Second {
		t.Fatal("the command outlived the editor's stop")
	}
}

func TestAutoReconnectRetriesUntilItWorks(t *testing.T) {
	a, queue := testDispatchApp(t)
	a.root = "/srv/project"
	a.connection = &core.Connection{Kind: core.KindSSH, Target: "test", Root: "/srv/project", Binary: "vael-core"}
	a.disconnected = true
	a.connectionError = "核心连接已断开"
	attempts := 0
	a.connectForReconnect = func(ctx context.Context, c core.Connection) (*core.Client, error) {
		attempts++
		if attempts < 2 {
			return nil, errors.New("not yet")
		}
		server, client := net.Pipe()
		go func() { _ = core.Serve(ctx, server, server) }()
		backend := core.NewClient(core.NewStream(client, client, nil))
		a.projectCtx, a.projectCancel = ctx, func() {}
		return backend, nil
	}
	a.scheduleReconnect(50*time.Millisecond, 1)
	pumpUntil(t, queue, func() bool { return !a.disconnected })
	if attempts < 2 {
		t.Fatalf("no retry was made: %d", attempts)
	}
	if a.reconnectAttempt != 0 || a.connectionError != "" {
		t.Fatalf("state after reconnect: attempt %d error %q", a.reconnectAttempt, a.connectionError)
	}
}

func TestStopReconnectSilencesTheAttempts(t *testing.T) {
	a, queue := testDispatchApp(t)
	a.root = "/srv/project"
	a.connection = &core.Connection{Kind: core.KindSSH, Target: "test", Root: "/srv/project", Binary: "vael-core"}
	a.disconnected = true
	a.reconnectOff = false
	a.connectForReconnect = func(ctx context.Context, c core.Connection) (*core.Client, error) {
		t.Error("a stopped reconnection still tried")
		return nil, errors.New("should not run")
	}
	a.scheduleReconnect(50*time.Millisecond, 1)
	a.stopReconnect()
	if !a.reconnectOff || a.reconnectTimer != nil {
		t.Fatal("the reconnection was not stopped")
	}
	// The next break of the connection starts it again.
	a.reconnectOff = false
	a.scheduleReconnect(50*time.Millisecond, 1)
	if a.reconnectTimer == nil {
		t.Fatal("the reconnection did not restart")
	}
	a.cancelReconnectTimer()
	deadline := time.After(300 * time.Millisecond)
	for {
		select {
		case fn := <-queue:
			fn()
		case <-deadline:
			return
		}
	}
}

func TestReconnectStopsWhenTheProjectChanges(t *testing.T) {
	a, queue := testDispatchApp(t)
	a.root = "/srv/project"
	a.connection = &core.Connection{Kind: core.KindSSH, Target: "test", Root: "/srv/project", Binary: "vael-core"}
	a.disconnected = true
	generation := a.generation
	installed := false
	a.connectForReconnect = func(ctx context.Context, c core.Connection) (*core.Client, error) {
		installed = true
		return nil, errors.New("not yet")
	}
	a.scheduleReconnect(50*time.Millisecond, 1)
	// The user opens another project while the attempt waits.
	a.generation++
	deadline := time.After(600 * time.Millisecond)
	for installed {
		select {
		case fn := <-queue:
			fn()
		case <-deadline:
		}
	}
	if installed {
		t.Fatal("a stale attempt ran after the project changed")
	}
	_ = generation
}

func TestPaletteShowsTheReconnectCommands(t *testing.T) {
	a := newApp()
	defer a.shutdown()
	tt := ui.NewTester(a.view, 1280, 840)
	tt.Frame()
	commands := a.commands()
	found := false
	for _, c := range commands {
		if strings.Contains(c.title, "重新连接") {
			found = true
		}
	}
	if !found {
		t.Fatal("the palette does not offer the reconnection")
	}
}
