package main

import (
	"context"
	"net"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/egoist/mygo/ui/editor"
	"vael/internal/core"
)

func testDispatchApp(t *testing.T) (*app, chan func()) {
	t.Helper()
	a := newApp()
	queue := make(chan func(), 64)
	a.dispatch = func(fn func()) { queue <- fn }
	t.Cleanup(a.shutdown)
	return a, queue
}
func pumpUntil(t *testing.T, queue chan func(), condition func() bool) {
	t.Helper()
	timeout := time.NewTimer(5 * time.Second)
	defer timeout.Stop()
	for !condition() {
		select {
		case fn := <-queue:
			fn()
		case <-timeout.C:
			t.Fatal("UI operation did not complete")
		}
	}
}
func testStream(t *testing.T) (*core.Client, net.Conn) {
	t.Helper()
	server, client := net.Pipe()
	done := make(chan struct{})
	go func() { _ = core.Serve(context.Background(), server, server); server.Close(); close(done) }()
	backend := core.NewClient(core.NewStream(client, client, nil))
	t.Cleanup(func() {
		backend.Close()
		server.Close()
		select {
		case <-done:
		case <-time.After(5 * time.Second):
			t.Error("core did not stop")
		}
	})
	return backend, server
}
func TestDisconnectAndReconnectPreservesDirtyBufferAndSaveConflict(t *testing.T) {
	a, queue := testDispatchApp(t)
	a.root = t.TempDir()
	path := filepath.Join(a.root, "main.go")
	original := "package main\n"
	if err := os.WriteFile(path, []byte(original), 0644); err != nil {
		t.Fatal(err)
	}
	b := editor.NewBuffer(original)
	b.Set("package edited\n")
	state := &editor.State{Buffer: b}
	state.Select(8, 8)
	tab := &fileTab{path: path, original: original, state: state}
	a.tabs = []*fileTab{tab}
	a.selected = 0
	backend, server := testStream(t)
	a.backend = backend
	oldCtx := a.projectCtx
	a.watchBackend(backend)
	_ = server.Close()
	pumpUntil(t, queue, func() bool { return a.disconnected })
	if oldCtx.Err() == nil {
		t.Fatal("disconnect did not cancel requests")
	}
	if a.tabs[0] != tab || !b.Modified() || b.String() != "package edited\n" {
		t.Fatal("disconnect lost edits")
	}
	a.save()
	if a.message == "" || tab.saving {
		t.Fatal("offline save was not explained")
	}
	a.message = ""
	next, _ := testStream(t)
	a.installReconnected(next)
	pumpUntil(t, queue, func() bool { return !a.refreshing && !a.indexing && !a.loading[a.root] })
	if a.disconnected || a.tabs[0] != tab || state.Cursor() != 8 || tab.original != original {
		t.Fatal("reconnect replaced editor state")
	}
	if err := os.WriteFile(path, []byte("package external\n"), 0644); err != nil {
		t.Fatal(err)
	}
	a.save()
	pumpUntil(t, queue, func() bool { return !tab.saving })
	if a.message == "" || !b.Modified() {
		t.Fatal("external conflict was not reported")
	}
	actual, _ := os.ReadFile(path)
	if string(actual) != "package external\n" {
		t.Fatal("reconnect save overwrote external edits")
	}
}
func TestLatestProjectRequestWins(t *testing.T) {
	a, queue := testDispatchApp(t)
	first, second := t.TempDir(), t.TempDir()
	a.openProject(first)
	// Wait until the first result is queued, but delay applying it until a newer
	// project request exists, just as two concurrent connection attempts can race.
	var firstResult func()
	select {
	case firstResult = <-queue:
	case <-time.After(5 * time.Second):
		t.Fatal("first stat did not complete")
	}
	a.openProject(second)
	firstResult()
	pumpUntil(t, queue, func() bool { return a.root == second && !a.indexing && !a.refreshing && !a.loading[second] })
	if a.root != second {
		t.Fatal("older response won")
	}
	if len(a.environments.Recent) != 1 || a.environments.Recent[0].Root != second {
		t.Fatal("superseded project added to history")
	}
}
func TestCancelAndShutdownIgnoreQueuedCallbacks(t *testing.T) {
	a, queue := testDispatchApp(t)
	version, ctx := a.beginConnection()
	a.cancelConnection()
	if ctx.Err() == nil || a.connecting || version == a.connectionVersion {
		t.Fatal("connection was not canceled")
	}
	applied, cleaned := false, false
	a.update(func() { applied = true }, func() { cleaned = true })
	a.shutdown()
	(<-queue)()
	if applied || !cleaned {
		t.Fatal("late callback applied after shutdown or resource leaked")
	}
}
func TestSwitchDetectsEditsMadeAfterConfirmation(t *testing.T) {
	a := newApp()
	defer a.shutdown()
	b := editor.NewBuffer("saved")
	b.Set("first edit")
	a.tabs = []*fileTab{{state: &editor.State{Buffer: b}}}
	snapshot := a.captureEdits()
	if a.newEditsSince(snapshot) {
		t.Fatal("unchanged edits require another confirmation")
	}
	b.Set("later edit")
	if !a.newEditsSince(snapshot) {
		t.Fatal("later edit would be silently discarded")
	}
}
