package core

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"vael/internal/workspace"
)

// TestSSHCloudMode verifies the SSH cloud mode against a real host. It runs
// only when the environment names one:
//
//	VAEL_SSH_HOST=ubuntu@host go test -run TestSSHCloudMode ./internal/core/
//
// The host needs nothing but an SSH account: the editor probes it, uploads
// the core that sits beside this test, and talks to it over the protocol.
func TestSSHCloudMode(t *testing.T) {
	target := os.Getenv("VAEL_SSH_HOST")
	if target == "" {
		t.Skip("VAEL_SSH_HOST is not set: the SSH cloud mode is verified against a real host")
	}
	local := os.Getenv("VAEL_CORE_LOCAL")
	if local == "" {
		local = filepath.Join("..", "..", "dist", "vael-core-linux-amd64")
	}
	if _, err := os.Stat(local); err != nil {
		t.Skipf("the core to upload is not built: %v", err)
	}

	root := "/tmp/vael-verify-" + strings.ReplaceAll(target, "@", "-")
	c := Connection{Kind: KindSSH, Target: target, Root: root, Binary: ""}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()

	// The host is asked what it is.
	info, err := c.Probe(ctx)
	if err != nil {
		t.Fatalf("probe: %v", err)
	}
	t.Logf("host: %s/%s", info.Platform, info.Arch)
	if info.Platform == "" || info.Arch == "" {
		t.Fatal("the host did not say what it is")
	}

	// The project is put where the user keeps their work, which the
	// editor would open.
	command, err := c.hostCommand("echo -n $HOME")
	if err != nil {
		t.Fatal(err)
	}
	home, err := runCommand(ctx, command)
	if err != nil {
		t.Fatal(err)
	}
	root = strings.TrimSpace(home) + "/vael-verify"
	c.Root = root

	// The core is sent to the host, and its version is checked there.
	installed, err := c.EnsureCore(ctx, local)
	if err != nil {
		t.Fatalf("ensure core: %v", err)
	}
	t.Logf("core: %s", installed)
	command, err = c.hostCommand(shellPath(installed) + " --version")
	if err != nil {
		t.Fatal(err)
	}
	out, err := runCommand(ctx, command)
	if err != nil || strings.TrimSpace(out) != "vael-core "+Version {
		t.Fatalf("the core on the host: %q %v", out, err)
	}

	// The SSH master connection is shared, so that a second command does
	// not shake hands again.
	master, err := exec.Command("sh", "-c", "ls ~/.ssh/cm_* 2>/dev/null | head -1").Output()
	if err != nil || len(strings.TrimSpace(string(master))) == 0 {
		t.Logf("no control socket found: %v %q", err, master)
	}

	// The project is made on the host, and the core is started there.
	command, err = c.hostCommand("rm -rf " + shellPath(root) + " && mkdir -p " + shellPath(root))
	if err != nil {
		t.Fatal(err)
	}
	if _, err = runCommand(ctx, command); err != nil {
		t.Fatal(err)
	}
	backend, err := Connect(ctx, c)
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	defer backend.Close()
	handshake, err := backend.Info(ctx)
	if err != nil || handshake.Protocol != ProtocolVersion {
		t.Fatalf("handshake: %+v %v", handshake, err)
	}

	// The files of the project are read and written through the tunnel.
	stat, err := backend.Stat(ctx, root)
	if err != nil || !stat.Directory {
		t.Fatalf("stat: %+v %v", stat, err)
	}
	path := root + "/你好.go"
	source := "package main\n\n// 你好\nfunc Hello() {}\n"
	if err = backend.Create(ctx, root, "你好.go", false); err != nil {
		t.Fatal(err)
	}
	if _, err = backend.Save(ctx, path, "", source); err != nil {
		t.Fatal(err)
	}
	text, stamp, err := backend.ReadFile(ctx, path)
	if err != nil || text != source {
		t.Fatalf("read: %q %v", text, err)
	}
	entries, err := backend.List(ctx, root)
	if err != nil || len(entries) != 1 {
		t.Fatalf("list: %+v %v", entries, err)
	}

	// A command runs on the host.
	run, err := backend.Run(ctx, root, "echo from-$HOSTNAME && exit 3", time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(run.Output, "from-") || run.Code != 3 {
		t.Fatalf("run: %+v", run)
	}

	// The core watches a file and pushes what it sees.
	watch, err := backend.Watch(ctx, []string{path}, map[string]workspace.Stamp{path: stamp})
	if err != nil {
		t.Fatal(err)
	}
	defer watch.Close()
	time.Sleep(600 * time.Millisecond)
	command, err = c.hostCommand("echo changed >> " + shellPath(path))
	if err != nil {
		t.Fatal(err)
	}
	if _, err = runCommand(ctx, command); err != nil {
		t.Fatal(err)
	}
	select {
	case change := <-watch.Changes():
		t.Logf("pushed: %s deleted=%v", change.Path, change.Deleted)
		if change.Deleted {
			t.Fatalf("%+v", change)
		}
	case <-time.After(15 * time.Second):
		t.Fatal("the change was not pushed")
	}

	// The shell of the host is where the terminal of the editor would go.
	term, err := c.TerminalCommand()
	if err != nil {
		t.Fatal(err)
	}
	shell := exec.Command(term.Program, term.Args...)
	shell.Stdin = strings.NewReader("echo PTY_OK $(pwd)\nexit\n")
	output, err := shell.CombinedOutput()
	if err != nil && !strings.Contains(string(output), "PTY_OK") {
		t.Fatalf("terminal: %v\n%s", err, output)
	}
	if !strings.Contains(string(output), "PTY_OK") {
		t.Fatalf("terminal output: %s", output)
	}

	// The editor leaves nothing behind but the core it installed.
	command, err = c.hostCommand("rm -rf " + shellPath(root))
	if err != nil {
		t.Fatal(err)
	}
	runCommand(ctx, command)
	t.Log("SSH cloud mode verified")
}
