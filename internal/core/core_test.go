package core

import (
	"context"
	"encoding/json"
	"errors"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"vael/internal/workspace"
)

func streamClient(t *testing.T) *Client {
	t.Helper()
	server, client := net.Pipe()
	done := make(chan error, 1)
	go func() { done <- Serve(context.Background(), server, server); server.Close() }()
	stream := NewStream(client, client, nil)
	t.Cleanup(func() {
		stream.Close()
		select {
		case err := <-done:
			if err != nil {
				t.Error(err)
			}
		case <-time.After(5 * time.Second):
			t.Error("core did not stop on disconnect")
		}
	})
	return NewClient(stream)
}
func exerciseClient(t *testing.T, c *Client) {
	t.Helper()
	ctx := context.Background()
	root := t.TempDir()
	info, err := c.Info(ctx)
	if err != nil || info.Protocol != ProtocolVersion {
		t.Fatalf("%+v %v", info, err)
	}
	stat, err := c.Stat(ctx, root)
	if err != nil || !stat.Directory {
		t.Fatalf("%+v %v", stat, err)
	}
	if err := c.Create(ctx, root, "hello.go", false); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(root, "hello.go")
	source := "package main\n\n// 你好\nfunc Hello() {}\n"
	if _, err := c.Save(ctx, path, "", source); err != nil {
		t.Fatal(err)
	}
	text, err := c.Read(ctx, path)
	if err != nil || text != source {
		t.Fatalf("%q %v", text, err)
	}
	if _, err := c.Save(ctx, path, "stale", "replacement"); err == nil {
		t.Fatal("save conflict was lost by transport")
	}
	files, err := c.List(ctx, root)
	if err != nil || len(files) != 1 {
		t.Fatalf("%+v %v", files, err)
	}
	index, err := c.Index(ctx, root)
	if err != nil || len(index.Files) != 1 {
		t.Fatalf("%+v %v", index, err)
	}
	matches, err := c.Search(ctx, root, index.Files, "你好", true)
	if err != nil || len(matches.Matches) != 1 {
		t.Fatalf("%+v %v", matches, err)
	}
	symbols, err := c.Symbols(ctx, root, index.Files, 100)
	if err != nil || len(symbols) != 1 || symbols[0].Name != "Hello" {
		t.Fatalf("%+v %v", symbols, err)
	}
	replaced, err := c.ReplaceAll(ctx, root, index.Files, "Hello", "Howdy", true)
	if err != nil || replaced.Files != 1 || replaced.Occurrences != 1 {
		t.Fatalf("%+v %v", replaced, err)
	}
	if text, err := c.Read(ctx, path); err != nil || !strings.Contains(text, "Howdy") {
		t.Fatalf("%q %v", text, err)
	}
	if _, err := c.Read(ctx, filepath.Join(root, "missing")); err == nil {
		t.Fatal("missing file error lost")
	}
	if _, err := c.Git(ctx, root, "init"); err != nil {
		t.Fatal(err)
	}
	repo, err := c.Status(ctx, root)
	if err != nil || len(repo.Changes) != 1 {
		t.Fatalf("%+v %v", repo, err)
	}
	if err := c.Stage(ctx, root, repo.Changes[0]); err != nil {
		t.Fatal(err)
	}
	repo, err = c.Status(ctx, root)
	if err != nil || !repo.Changes[0].Staged() {
		t.Fatalf("%+v %v", repo, err)
	}
	if err := c.Unstage(ctx, root, repo.Changes[0]); err != nil {
		t.Fatal(err)
	}
	// The file operations of the core travel over the transport as well.
	if err := c.Rename(ctx, root, path, "renamed.go"); err != nil {
		t.Fatal(err)
	}
	renamed := filepath.Join(root, "renamed.go")
	if _, err := os.Stat(renamed); err != nil {
		t.Fatal(err)
	}
	if err := c.Rename(ctx, root, renamed, "sub/name.go"); err == nil {
		t.Fatal("transport accepted a path separator")
	}
	copied, err := c.Duplicate(ctx, root, renamed)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(copied); err != nil {
		t.Fatal(err)
	}
	if err := c.Remove(ctx, root, copied); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(copied); !os.IsNotExist(err) {
		t.Fatal("copy was not removed")
	}
	// A command runs in the project through the transport as well.
	run, err := c.Run(ctx, root, "echo from-core", 30*time.Second)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(run.Output, "from-core") || run.Code != 0 {
		t.Fatalf("%+v", run)
	}
	// The core pushes the changes of the files it watches.
	watchCtx, cancelWatch := context.WithTimeout(ctx, 20*time.Second)
	defer cancelWatch()
	stamp, err := workspace.StampOf(renamed)
	if err != nil {
		t.Fatal(err)
	}
	watch, err := c.Watch(watchCtx, []string{renamed}, map[string]workspace.Stamp{renamed: stamp})
	if err != nil {
		t.Fatal(err)
	}
	defer watch.Close()
	time.Sleep(300 * time.Millisecond)
	if err := os.WriteFile(renamed, []byte("changed"), 0644); err != nil {
		t.Fatal(err)
	}
	select {
	case change := <-watch.Changes():
		if change.Path != renamed || change.Deleted {
			t.Fatalf("%+v", change)
		}
	case <-time.After(15 * time.Second):
		t.Fatal("watch over the transport reported nothing")
	}
}
func TestLocalAndStreamHaveSameBehavior(t *testing.T) {
	t.Run("local", func(t *testing.T) { exerciseClient(t, NewLocal()) })
	t.Run("stream", func(t *testing.T) { exerciseClient(t, streamClient(t)) })
}
func TestConcurrentRequestsAndCancellation(t *testing.T) {
	c := streamClient(t)
	var wg sync.WaitGroup
	for range 8 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for range 10 {
				if _, err := c.Info(context.Background()); err != nil {
					t.Error(err)
					return
				}
			}
		}()
	}
	wg.Wait()
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := c.Index(ctx, t.TempDir()); !errors.Is(err, context.Canceled) {
		t.Fatalf("%v", err)
	}
	if _, err := c.Info(context.Background()); err != nil {
		t.Fatal("cancel broke connection", err)
	}
}
func TestServerRejectsUnknownMethod(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := (Service{}).Handle(ctx, "info", nil); !errors.Is(err, context.Canceled) {
		t.Fatal(err)
	}
	if err := streamClient(t).transport.Call(context.Background(), "unknown", nil, nil); err == nil {
		t.Fatal("unknown method accepted")
	}
	// An error that comes back carries its code.
	_, err := (Service{}).Handle(context.Background(), "read", json.RawMessage(`{"path":"/definitely/not/here"}`))
	if ErrorCode(err) != CodeNotFound {
		t.Fatalf("the code of the error: %v", err)
	}
}
func TestCoreProcess(t *testing.T) {
	if os.Getenv("VAEL_TEST_CORE") == "1" {
		if err := Serve(context.Background(), os.Stdin, os.Stdout); err != nil {
			os.Exit(1)
		}
		os.Exit(0)
	}
	cmd := exec.Command(os.Args[0], "-test.run=^TestCoreProcess$")
	cmd.Env = append(os.Environ(), "VAEL_TEST_CORE=1")
	input, err := cmd.StdinPipe()
	if err != nil {
		t.Fatal(err)
	}
	output, err := cmd.StdoutPipe()
	if err != nil {
		t.Fatal(err)
	}
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	c := NewClient(NewStream(input, output, func() error { _ = cmd.Process.Kill(); _ = cmd.Wait(); return nil }))
	defer c.Close()
	exerciseClient(t, c)
}
func TestConnectionCommands(t *testing.T) {
	for _, test := range []struct {
		config         Connection
		core, terminal Command
	}{
		{Connection{KindSSH, "devbox", "/srv/a'b", "/opt/vael-core", 0},
			Command{"ssh", append(append([]string{"-T"}, []string{"-o", "ControlMaster=auto", "-o", "ControlPath=~/.ssh/cm_%C", "-o", "ControlPersist=30m", "-o", "ConnectTimeout=15"}...), "devbox", "'/opt/vael-core' --stdio")},
			Command{"ssh", append(append([]string{"-tt"}, []string{"-o", "ControlMaster=auto", "-o", "ControlPath=~/.ssh/cm_%C", "-o", "ControlPersist=30m", "-o", "ConnectTimeout=15"}...), "devbox", `cd '/srv/a'"'"'b' && exec "${SHELL:-/bin/sh}" -l`)}},
		{Connection{KindSSH, "devbox", "/srv/a'b", "/opt/vael-core", 2222},
			Command{"ssh", append(append(append([]string{"-T"}, []string{"-o", "ControlMaster=auto", "-o", "ControlPath=~/.ssh/cm_%C", "-o", "ControlPersist=30m", "-o", "ConnectTimeout=15"}...), "-p", "2222"), "devbox", "'/opt/vael-core' --stdio")},
			Command{"ssh", append(append(append([]string{"-tt"}, []string{"-o", "ControlMaster=auto", "-o", "ControlPath=~/.ssh/cm_%C", "-o", "ControlPersist=30m", "-o", "ConnectTimeout=15"}...), "-p", "2222"), "devbox", `cd '/srv/a'"'"'b' && exec "${SHELL:-/bin/sh}" -l`)}},
		{Connection{KindDocker, "dev-container", "/workspace", "vael-core", 0}, Command{"docker", []string{"exec", "-i", "--", "dev-container", "/bin/sh", "-c", "'vael-core' --stdio"}}, Command{"docker", []string{"exec", "-it", "--workdir", "/workspace", "--", "dev-container", "/bin/sh", "-l"}}},
		{Connection{KindWSL, "Ubuntu", "/home/dev/project", "/usr/local/bin/vael-core", 0}, Command{"wsl.exe", []string{"--distribution", "Ubuntu", "--exec", "/bin/sh", "-c", "'/usr/local/bin/vael-core' --stdio"}}, Command{"wsl.exe", []string{"--distribution", "Ubuntu", "--cd", "/home/dev/project", "--exec", "/bin/sh", "-l"}}},
		{Connection{KindWSL, "", "/home/dev/project", "vael-core", 0}, Command{"wsl.exe", []string{"--exec", "/bin/sh", "-c", "'vael-core' --stdio"}}, Command{"wsl.exe", []string{"--cd", "/home/dev/project", "--exec", "/bin/sh", "-l"}}},
	} {
		got, err := test.config.CoreCommand()
		if err != nil {
			t.Fatal(err)
		}
		if !got.sameProgramAs(test.core) {
			t.Fatalf("core command: %v", got)
		}
		term, err := test.config.TerminalCommand()
		if err != nil {
			t.Fatal(err)
		}
		if !term.sameProgramAs(test.terminal) {
			t.Fatalf("terminal command: %v", term)
		}
		// A port is passed to SSH, and to nothing else.
		if test.config.Port > 0 && test.config.Kind == KindSSH {
			if !slices.Contains(got.Args, "-p") {
				t.Fatalf("the port was dropped: %v", got)
			}
		}
	}
	for _, c := range []Connection{{KindSSH, "-bad", "/srv", "core", 0}, {KindDocker, "c", "relative", "core", 0}, {"unknown", "c", "/srv", "core", 0}, {KindSSH, "a b", "/srv", "core", 0}} {
		if _, err := c.CoreCommand(); err == nil {
			t.Fatalf("accepted %+v", c)
		}
	}
}

// sameProgramAs says whether two commands run the same program with the
// same arguments.
func (c Command) sameProgramAs(other Command) bool {
	return c.Program == other.Program && slices.Equal(c.Args, other.Args)
}

func TestHostProbeAndCorePath(t *testing.T) {
	c := Connection{KindSSH, "devbox", "/srv", "", 0}
	if got := c.CorePath(); got != DefaultCorePath {
		t.Fatalf("core path: %q", got)
	}
	// A leading ~ becomes $HOME, which the shell of the host expands.
	if got := (Connection{KindSSH, "devbox", "/srv", "~/tools/vael-core", 0}).CorePath(); got != "$HOME/tools/vael-core" {
		t.Fatalf("tilde core path: %q", got)
	}
	// A path with $ is quoted with double quotes, so that $HOME expands.
	if got := shellPath(DefaultCorePath); got != `"$HOME/.local/share/vael/core/vael-core"` {
		t.Fatalf("quoted core path: %q", got)
	}
	if got := shellPath("/opt/vae'l-core"); got != `'/opt/vae'"'"'l-core'` {
		t.Fatalf("quoted plain path: %q", got)
	}
	if got := (Connection{KindSSH, "devbox", "/srv", "/opt/vael-core", 0}).CorePath(); got != "/opt/vael-core" {
		t.Fatalf("explicit core path: %q", got)
	}
	if got := (Connection{KindSSH, "devbox", "/srv", "vael-core", 0}).CorePath(); got != "vael-core" {
		t.Fatalf("command core path: %q", got)
	}
	probe, err := c.hostCommand("uname -s; uname -m")
	if err != nil {
		t.Fatal(err)
	}
	if probe.Program != "ssh" || probe.Args[len(probe.Args)-1] != "uname -s; uname -m" {
		t.Fatalf("probe: %v", probe)
	}
	upload, err := c.hostCommand("mkdir -p '~' && cat > '~' && chmod +x '~'")
	if err != nil {
		t.Fatal(err)
	}
	if upload.Program != "ssh" {
		t.Fatalf("upload: %v", upload)
	}
}
func TestClosedStreamAndMalformedRequest(t *testing.T) {
	c := streamClient(t)
	done := c.Done()
	if done == nil {
		t.Fatal("stream state is not observable")
	}
	c.Close()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("disconnect not signaled")
	}
	if c.Err() == nil {
		t.Fatal("disconnect cause missing")
	}
	if _, err := c.Info(context.Background()); err == nil {
		t.Fatal("closed stream accepted request")
	}
	_, err := (Service{}).Handle(context.Background(), "read", json.RawMessage(`{bad`))
	if err == nil {
		t.Fatal("malformed params accepted")
	}
}
