package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/egoist/mygo/ui"
	"vael/internal/core"
	"vael/internal/workspace"
)

func TestProjectSwitcherOpensARecentProject(t *testing.T) {
	a, queue := testDispatchApp(t)
	first := t.TempDir()
	second := t.TempDir()
	a.useProject(core.NewLocal(), nil, first)
	pumpUntil(t, queue, func() bool { return a.root == first })
	a.environments.Recent = []workspace.Environment{{Root: second}}

	tt := ui.NewTester(a.view, 1280, 840)
	tt.Frame()
	if err := tt.Click("项目：" + filepath.Base(first)); err != nil {
		t.Fatal(err)
	}
	if !a.projectMenuOpen {
		t.Fatal("the switcher did not open")
	}
	tt.Frame()
	if err := tt.Click(filepath.Base(second)); err != nil {
		t.Fatal(err)
	}
	pumpUntil(t, queue, func() bool { return a.root == second })
	if a.projectMenuOpen {
		t.Fatal("the switcher did not close")
	}
}

func TestProjectSwitcherShowsTheCurrentProject(t *testing.T) {
	a := newApp()
	defer a.shutdown()
	a.root = t.TempDir()
	a.projectMenuOpen = true
	tt := ui.NewTester(a.view, 1280, 840)
	tt.Frame()
	if err := tt.Click("打开的项目"); err != nil {
		t.Fatal(err)
	}
	if err := tt.Click(a.root); err != nil {
		t.Fatal(err)
	}
	// Choosing the project that is open changes nothing.
	if !a.projectMenuOpen {
		t.Fatal("the switcher closed on the current project")
	}
}

func TestCreateProjectMakesTheFolder(t *testing.T) {
	a, queue := testDispatchApp(t)
	parent := t.TempDir()
	a.createProject(&projectPrompt{name: "demo", parent: parent})
	path := filepath.Join(parent, "demo")
	pumpUntil(t, queue, func() bool { return a.root == path })
	if info, err := os.Stat(path); err != nil || !info.IsDir() {
		t.Fatalf("the folder was not made: %v", err)
	}
	// A name with a separator is refused.
	a.message = ""
	a.createProject(&projectPrompt{name: "a/b", parent: parent})
	if a.message == "" {
		t.Fatal("a name with a separator was accepted")
	}
}

func TestParentFolderFollowsTheEnvironment(t *testing.T) {
	a := newApp()
	defer a.shutdown()
	a.root = "/tmp/local/project"
	if got := a.parentFolder(); got != "/tmp/local" {
		t.Fatalf("local: %q", got)
	}
	// A remote project is made next to the project that is open there, not
	// on this machine.
	a.connection = &core.Connection{Root: "/home/user/project"}
	if got := a.parentFolder(); got != "/home/user" {
		t.Fatalf("remote: %q", got)
	}
}

func TestCloneName(t *testing.T) {
	cases := map[string]string{
		"https://github.com/user/repo.git": "repo",
		"git@github.com:user/repo.git":     "repo",
		"https://example.com/repo/":        "repo",
		"":                                 "repository",
	}
	for url, want := range cases {
		if got := cloneName(url); got != want {
			t.Fatalf("%q: %q", url, got)
		}
	}
}

func TestProjectBadgeNamesTheLanguage(t *testing.T) {
	a := newApp()
	defer a.shutdown()
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "go.mod"), []byte("module demo\n"), 0644); err != nil {
		t.Fatal(err)
	}
	if text, _, ok := a.projectBadge(root); !ok || text != "GO" {
		t.Fatalf("badge %q %v", text, ok)
	}
	empty := t.TempDir()
	if _, _, ok := a.projectBadge(empty); ok {
		t.Fatal("an unknown project got a badge")
	}
}

func TestBranchSwitcherChecksOutABranch(t *testing.T) {
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
	if out, err := exec.Command("git", "-C", root, "branch", "feature").CombinedOutput(); err != nil {
		t.Fatalf("%s", out)
	}
	a.useProject(core.NewLocal(), nil, root)
	pumpUntil(t, queue, func() bool { return a.repo != nil })
	a.loadBranches()
	pumpUntil(t, queue, func() bool { return !a.branchesLoading })
	if len(a.branches) < 2 {
		t.Fatalf("branches: %v", a.branches)
	}

	a.branchMenuOpen = true
	tt := ui.NewTester(a.view, 1280, 840)
	tt.Frame()
	if err := tt.Click("本地分支"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if err := tt.Click("feature"); err != nil {
		t.Fatal(err)
	}
	pumpUntil(t, queue, func() bool { return a.repo != nil && a.repo.Branch == "feature" })
	if len(a.recentBranches) == 0 || a.recentBranches[0] != "feature" {
		t.Fatalf("recent branches: %v", a.recentBranches)
	}
}

func TestCreateBranchChecksItOut(t *testing.T) {
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
	a.createBranch("feature/new")
	pumpUntil(t, queue, func() bool { return a.repo != nil && a.repo.Branch == "feature/new" })
	// An invalid name is refused before Git runs.
	a.message = ""
	a.createBranch("bad name")
	if a.message == "" {
		t.Fatal("an invalid branch name was accepted")
	}
}

func TestBranchMenuFiltersByQuery(t *testing.T) {
	a := newApp()
	defer a.shutdown()
	a.root = t.TempDir()
	a.repo = &workspace.Repository{Root: a.root, Branch: "main"}
	a.branches = []string{"main", "feature/login", "fix/typo"}
	a.remoteBranches = []string{"origin/main", "origin/feature/login"}
	a.branchQuery = "login"
	tt := ui.NewTester(a.view, 1280, 840)
	open := func() {
		a.branchMenuOpen = true
		tt.Frame()
	}
	open()
	if err := tt.Click("feature/login"); err != nil {
		t.Fatal(err)
	}
	open()
	if err := tt.Click("origin/feature/login"); err != nil {
		t.Fatal(err)
	}
	open()
	if err := tt.Click("fix/typo"); err == nil {
		t.Fatal("a branch that does not match the query is shown")
	}
}
