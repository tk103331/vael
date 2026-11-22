package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"vael/internal/core"
)

func TestDiffBaseMarksAgainstGit(t *testing.T) {
	a, queue := testDispatchApp(t)
	root := t.TempDir()
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git is not available")
	}
	for _, args := range [][]string{{"init"}, {"config", "user.name", "t"}, {"config", "user.email", "t@e.invalid"}} {
		if out, err := exec.Command("git", append([]string{"-C", root}, args...)...).CombinedOutput(); err != nil {
			t.Fatalf("%v: %s", args, out)
		}
	}
	path := filepath.Join(root, "a.go")
	if err := os.WriteFile(path, []byte("package main\n\nfunc One() {}\n"), 0644); err != nil {
		t.Fatal(err)
	}
	if out, err := exec.Command("git", "-C", root, "add", "a.go").CombinedOutput(); err != nil {
		t.Fatalf("add: %v %s", err, out)
	}
	if out, err := exec.Command("git", "-C", root, "commit", "-m", "one").CombinedOutput(); err != nil {
		t.Fatalf("commit: %v %s", err, out)
	}
	a.useProject(core.NewLocal(), nil, root)
	a.openFile(path)
	pumpUntil(t, queue, func() bool {
		tab := a.tabForPath(path)
		return tab != nil && tab.state.DiffBase != nil
	})
	tab := a.tabForPath(path)
	if got := tab.state.DiffBase.String(); got != "package main\n\nfunc One() {}\n" {
		t.Fatalf("base: %q", got)
	}

	// A commit changes the base the gutter marks against.
	if err := os.WriteFile(path, []byte("package main\n\nfunc Two() {}\n"), 0644); err != nil {
		t.Fatal(err)
	}
	if out, err := exec.Command("git", "-C", root, "add", "a.go").CombinedOutput(); err != nil {
		t.Fatalf("add: %v %s", err, out)
	}
	if out, err := exec.Command("git", "-C", root, "commit", "-m", "two").CombinedOutput(); err != nil {
		t.Fatalf("commit: %v %s", err, out)
	}
	a.refreshGit()
	pumpUntil(t, queue, func() bool { return !a.refreshing })
	a.refreshDiffBases()
	pumpUntil(t, queue, func() bool {
		return tab.state.DiffBase != nil && tab.state.DiffBase.String() == "package main\n\nfunc Two() {}\n"
	})

	// A file Git does not track has no base.
	other := filepath.Join(root, "new.go")
	if err := os.WriteFile(other, []byte("package other\n"), 0644); err != nil {
		t.Fatal(err)
	}
	a.openFile(other)
	pumpUntil(t, queue, func() bool { return a.tabForPath(other) != nil })
	if tab2 := a.tabForPath(other); tab2.state.DiffBase != nil {
		t.Fatal("an untracked file was given a base")
	}
}

func TestSearchPanelReplacesAcrossTheProject(t *testing.T) {
	a, queue := testDispatchApp(t)
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "a.txt"), []byte("cat dog cat\n"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "b.md"), []byte("the cat\n"), 0644); err != nil {
		t.Fatal(err)
	}
	a.useProject(core.NewLocal(), nil, root)
	a.searchQuery = "cat"
	a.replaceWith = "fish"
	a.replaceAll()
	pumpUntil(t, queue, func() bool { return strings.Contains(a.message, "替换") })
	if !strings.Contains(a.message, "3 处") || !strings.Contains(a.message, "2 个文件") {
		t.Fatalf("the result was not reported: %q", a.message)
	}
	got, _ := os.ReadFile(filepath.Join(root, "a.txt"))
	if string(got) != "fish dog fish\n" {
		t.Fatalf("%q", got)
	}
	got, _ = os.ReadFile(filepath.Join(root, "b.md"))
	if string(got) != "the fish\n" {
		t.Fatalf("%q", got)
	}
}

func TestSendToTerminalNeedsAnOpenTerminal(t *testing.T) {
	a := newApp()
	defer a.shutdown()
	a.sendToTerminal("echo hi")
	if a.message == "" {
		t.Fatal("sending without a terminal was silent")
	}
}
