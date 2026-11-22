package core

import (
	"context"
	"os"
	"os/exec"
	"strings"
	"testing"
	"time"

	"vael/internal/workspace"
)

// TestDockerCloudMode verifies the Docker cloud mode against a real
// container. It runs only when the environment asks for one:
//
//	VAEL_DOCKER=1 go test -run TestDockerCloudMode ./internal/core/
//
// The test builds a throwaway container, installs the core in it the way a
// user would, and then talks to it over the protocol: the files, a command,
// the push of a change, and the shell of the container.
func TestDockerCloudMode(t *testing.T) {
	if os.Getenv("VAEL_DOCKER") == "" {
		t.Skip("VAEL_DOCKER is not set: Docker is verified against a real container")
	}
	local := os.Getenv("VAEL_CORE_LOCAL")
	if local == "" {
		local = "../../dist/vael-core-linux-arm64"
	}
	if _, err := os.Stat(local); err != nil {
		t.Skipf("the core to install is not built: %v", err)
	}
	if _, err := exec.LookPath("docker"); err != nil {
		t.Skip("docker is not available")
	}

	name := "vael-verify"
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()
	docker := func(args ...string) (string, error) {
		out, err := exec.CommandContext(ctx, "docker", args...).CombinedOutput()
		return string(out), err
	}

	// A container of the user's own, with nothing but the core in it.
	if out, err := docker("rm", "-f", name); err != nil && !strings.Contains(out, "No such container") {
		t.Logf("removing an older container: %s", strings.TrimSpace(out))
	}
	if out, err := docker("run", "-d", "--name", name, "debian:stable-slim", "sleep", "infinity"); err != nil {
		t.Skipf("starting a container: %v: %s", err, strings.TrimSpace(out))
	}
	defer docker("rm", "-f", name)

	// The core is installed the way the editor's instructions say.
	if out, err := docker("cp", local, name+":/tmp/vael-core"); err != nil {
		t.Fatalf("copying the core: %v: %s", err, strings.TrimSpace(out))
	}
	install := `mkdir -p "$HOME/.local/share/vael/core" && mv /tmp/vael-core "$HOME/.local/share/vael/core/vael-core" && chmod +x "$HOME/.local/share/vael/core/vael-core"`
	if out, err := docker("exec", "--", name, "/bin/sh", "-c", install); err != nil {
		t.Fatalf("installing the core: %v: %s", err, strings.TrimSpace(out))
	}

	c := Connection{Kind: KindDocker, Target: name, Root: "/tmp/vael-verify", Binary: ""}
	installed, err := c.EnsureCore(ctx, "")
	if err != nil {
		t.Fatalf("ensure core: %v", err)
	}
	t.Logf("core: %s", installed)

	backend, err := Connect(ctx, c)
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	defer backend.Close()
	handshake, err := backend.Info(ctx)
	if err != nil || handshake.Protocol != ProtocolVersion {
		t.Fatalf("handshake: %+v %v", handshake, err)
	}

	// The files of the project, through the container.
	if _, err = backend.Stat(ctx, "/tmp"); err != nil {
		t.Fatalf("stat: %v", err)
	}
	if out, err := docker("exec", "--", name, "/bin/sh", "-c", "mkdir -p "+c.Root); err != nil {
		t.Fatalf("making the project: %v: %s", err, strings.TrimSpace(out))
	}
	if err = backend.Create(ctx, c.Root, "你好.go", false); err != nil {
		t.Fatal(err)
	}
	path := c.Root + "/你好.go"
	source := "package main\n\n// 你好\nfunc Hello() {}\n"
	if _, err = backend.Save(ctx, path, "", source); err != nil {
		t.Fatal(err)
	}
	text, stamp, err := backend.ReadFile(ctx, path)
	if err != nil || text != source {
		t.Fatalf("read: %q %v", text, err)
	}
	entries, err := backend.List(ctx, c.Root)
	if err != nil || len(entries) != 1 {
		t.Fatalf("list: %+v %v", entries, err)
	}

	// A command runs in the container.
	run, err := backend.Run(ctx, c.Root, "echo docker-ok && exit 3", time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(run.Output, "docker-ok") || run.Code != 3 {
		t.Fatalf("run: %+v", run)
	}

	// The core watches a file and pushes what it sees.
	watch, err := backend.Watch(ctx, []string{path}, map[string]workspace.Stamp{path: stamp})
	if err != nil {
		t.Fatal(err)
	}
	defer watch.Close()
	time.Sleep(600 * time.Millisecond)
	if out, err := docker("exec", "--", name, "/bin/sh", "-c", "echo changed >> "+path); err != nil {
		t.Fatalf("changing the file: %v: %s", err, strings.TrimSpace(out))
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

	// The shell of the container is where the terminal of the editor goes.
	// The editor drives a real terminal; the CLI of docker refuses a pipe
	// with -it, so a pseudo terminal in the container stands in for one.
	term, err := c.TerminalCommand()
	if err != nil {
		t.Fatal(err)
	}
	shell := exec.Command(term.Program, term.Args...)
	shell.Stdin = strings.NewReader("echo PTY_OK $(pwd)\nexit\n")
	output, err := shell.CombinedOutput()
	if !strings.Contains(string(output), "PTY_OK") && strings.Contains(string(output), "stdin is not a terminal") {
		pseudo := exec.Command("docker", "exec", "-i", "--", name, "/bin/sh", "-c",
			"cd "+c.Root+` && script -qec "/bin/sh -l" /dev/null`)
		pseudo.Stdin = strings.NewReader("echo PTY_OK $(pwd)\nexit\n")
		output, err = pseudo.CombinedOutput()
	}
	if !strings.Contains(string(output), "PTY_OK") {
		t.Fatalf("terminal: %v\n%s", err, output)
	}
	t.Log("Docker cloud mode verified")
}
