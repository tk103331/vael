package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"vael/internal/core"
	"vael/internal/workspace"
)

func gitRepository(t *testing.T) string {
	t.Helper()
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git is not available")
	}
	root := t.TempDir()
	for _, args := range [][]string{{"init"}, {"config", "user.name", "Vael Test"}, {"config", "user.email", "test@example.invalid"}} {
		cmd := exec.Command("git", append([]string{"-C", root}, args...)...)
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("%v: %s", args, out)
		}
	}
	return root
}

func TestTaskRunsInTheProjectAndKeepsItsOutput(t *testing.T) {
	a, queue := testDispatchApp(t)
	root := t.TempDir()
	a.useProject(core.NewLocal(), nil, root)
	a.taskCommand = "echo vael-task"
	a.runTask(a.taskCommand)
	pumpUntil(t, queue, func() bool { return !a.task.running })
	if !strings.Contains(a.task.output, "vael-task") {
		t.Fatalf("%q", a.task.output)
	}
	if a.task.code != 0 {
		t.Fatalf("exit code %d", a.task.code)
	}
	if len(a.taskHistory) != 1 || a.taskHistory[0] != "echo vael-task" {
		t.Fatalf("%+v", a.taskHistory)
	}
	// The same command is not remembered twice.
	a.runTask(a.taskCommand)
	pumpUntil(t, queue, func() bool { return !a.task.running })
	if len(a.taskHistory) != 1 {
		t.Fatalf("%+v", a.taskHistory)
	}
}

func TestGitPanelReadsBranchesAndCommits(t *testing.T) {
	a, queue := testDispatchApp(t)
	root := gitRepository(t)
	if err := os.WriteFile(filepath.Join(root, "a.txt"), []byte("one"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := exec.Command("git", "-C", root, "add", "a.txt").Run(); err != nil {
		t.Fatal(err)
	}
	if err := exec.Command("git", "-C", root, "commit", "-m", "first").Run(); err != nil {
		t.Fatal(err)
	}
	a.useProject(core.NewLocal(), nil, root)
	pumpUntil(t, queue, func() bool { return a.repo != nil })
	a.loadBranches()
	pumpUntil(t, queue, func() bool { return !a.branchesLoading })
	if len(a.branches) == 0 {
		t.Fatal("no branches were read")
	}
	a.loadCommits()
	pumpUntil(t, queue, func() bool { return !a.commitsLoading })
	if len(a.commits) == 0 || !strings.Contains(a.commits[0], "first") {
		t.Fatalf("%+v", a.commits)
	}
}

func TestDiscardChangeThrowsTheEditAway(t *testing.T) {
	a, queue := testDispatchApp(t)
	root := gitRepository(t)
	path := filepath.Join(root, "a.txt")
	if err := os.WriteFile(path, []byte("one"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := exec.Command("git", "-C", root, "add", "a.txt").Run(); err != nil {
		t.Fatal(err)
	}
	if err := exec.Command("git", "-C", root, "commit", "-m", "first").Run(); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("two"), 0644); err != nil {
		t.Fatal(err)
	}
	a.useProject(core.NewLocal(), nil, root)
	pumpUntil(t, queue, func() bool { return a.repo != nil && len(a.repo.Changes) > 0 })
	change := a.repo.Changes[0]
	a.discardChange(change)
	pumpUntil(t, queue, func() bool { return !a.gitBusy })
	content, err := os.ReadFile(path)
	if err != nil || string(content) != "one" {
		t.Fatalf("%q %v", content, err)
	}
}

func TestUntrackedFilesAreRemovedWhenDiscarded(t *testing.T) {
	a, queue := testDispatchApp(t)
	root := gitRepository(t)
	path := filepath.Join(root, "new.txt")
	if err := os.WriteFile(path, []byte("x"), 0644); err != nil {
		t.Fatal(err)
	}
	a.useProject(core.NewLocal(), nil, root)
	pumpUntil(t, queue, func() bool { return a.repo != nil && len(a.repo.Changes) > 0 })
	change := workspace.Change{Code: "??", Path: "new.txt"}
	a.discardChange(change)
	pumpUntil(t, queue, func() bool { return !a.gitBusy })
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatal("the untracked file was not removed")
	}
}
