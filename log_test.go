package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/egoist/mygo/ui"
	"vael/internal/core"
)

func TestParseLogReadsTheFields(t *testing.T) {
	line := "abc\x1fdef ghi\x1fVael Test\x1f2026/11/22 10:00\x1f添加接口\x1fHEAD -> main, origin/main"
	commits := parseLog(line + "\n")
	if len(commits) != 1 {
		t.Fatalf("%+v", commits)
	}
	commit := commits[0]
	if commit.Hash != "abc" || commit.Subject != "添加接口" || commit.Author != "Vael Test" {
		t.Fatalf("%+v", commit)
	}
	if len(commit.Parents) != 2 || commit.Parents[0] != "def" || commit.Parents[1] != "ghi" {
		t.Fatalf("parents %v", commit.Parents)
	}
	if commit.Date != "2026/11/22 10:00" || commit.Refs != "HEAD -> main, origin/main" {
		t.Fatalf("%+v", commit)
	}
}

func TestAssignLanesFollowsAMerge(t *testing.T) {
	commits := []logCommit{
		{Hash: "m", Parents: []string{"a", "b"}},
		{Hash: "a", Parents: []string{"c"}},
		{Hash: "b", Parents: []string{"c"}},
		{Hash: "c"},
	}
	assignLanes(commits)
	if commits[0].Lane != 0 || commits[0].Lanes != 2 {
		t.Fatalf("merge: %+v", commits[0])
	}
	if commits[1].Lane != 0 {
		t.Fatalf("first parent: %+v", commits[1])
	}
	if commits[2].Lane != 1 {
		t.Fatalf("second parent: %+v", commits[2])
	}
	// Both branches wait for the root, so its row joins the second lane
	// back into the first.
	if commits[3].Lane != 0 || commits[3].Lanes != 2 || len(commits[3].Joins) != 1 || commits[3].Joins[0] != (logJoin{Lane: 1, Up: true}) {
		t.Fatalf("root: %+v", commits[3])
	}
}

func TestLogRefsNamesBranchesAndTags(t *testing.T) {
	refs := logRefs("HEAD -> main, origin/main, tag: v1.0.0")
	want := []logRef{{Name: "main"}, {Name: "v1.0.0", Tag: true}, {Name: "origin/main"}}
	if len(refs) != len(want) {
		t.Fatalf("%v", refs)
	}
	for i := range want {
		if refs[i] != want[i] {
			t.Fatalf("%v", refs)
		}
	}
}

func TestLogFiltersByTextAndAuthor(t *testing.T) {
	a := newApp()
	defer a.shutdown()
	commit := logCommit{Hash: "abcdef1234", Subject: "添加 HTTP 接口", Author: "Vael Demo"}
	a.logQuery = "http"
	if !a.logMatches(commit) {
		t.Fatal("the subject did not match")
	}
	a.logQuery = "abcdef"
	if !a.logMatches(commit) {
		t.Fatal("the hash did not match")
	}
	a.logQuery = "missing"
	if a.logMatches(commit) {
		t.Fatal("a commit that does not match was shown")
	}
	a.logQuery = ""
	a.logAuthor = "demo"
	if !a.logMatches(commit) {
		t.Fatal("the author did not match")
	}
	a.logAuthor = "someone"
	if a.logMatches(commit) {
		t.Fatal("a commit of another author was shown")
	}
}

func TestLogTabShowsAndChoosesCommits(t *testing.T) {
	a, queue := testDispatchApp(t)
	root := gitRepository(t)
	write := func(name, text string) {
		if err := os.WriteFile(filepath.Join(root, name), []byte(text), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	for _, message := range []string{"first", "second"} {
		name := message + ".txt"
		write(name, message)
		if out, err := exec.Command("git", "-C", root, "add", name).CombinedOutput(); err != nil {
			t.Fatalf("%s", out)
		}
		if out, err := exec.Command("git", "-C", root, "commit", "-m", message).CombinedOutput(); err != nil {
			t.Fatalf("%s", out)
		}
	}
	a.useProject(core.NewLocal(), nil, root)
	pumpUntil(t, queue, func() bool { return a.repo != nil })
	a.showTerminal()
	tt := ui.NewTester(a.view, 1280, 840)
	tt.Frame()
	if err := tt.Click("面板：Git"); err != nil {
		t.Fatal(err)
	}
	if a.bottomTab != "git" {
		t.Fatalf("tab %q", a.bottomTab)
	}
	pumpUntil(t, queue, func() bool { return a.logLoaded && !a.logLoading })
	tt.Frame()
	if err := tt.Click("first"); err != nil {
		t.Fatal(err)
	}
	pumpUntil(t, queue, func() bool { return a.logSelected == 1 })
	pumpUntil(t, queue, func() bool { return a.logDetail != nil && !a.logDetailLoading })
	if len(a.logDetail.Files) != 1 || a.logDetail.Files[0].Path != "first.txt" {
		t.Fatalf("files: %+v", a.logDetail.Files)
	}
}

func TestLogReadsCommitsAndTheirFiles(t *testing.T) {
	a, queue := testDispatchApp(t)
	root := gitRepository(t)
	write := func(name, text string) {
		if err := os.WriteFile(filepath.Join(root, name), []byte(text), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	write("a.txt", "one")
	if out, err := exec.Command("git", "-C", root, "add", "a.txt").CombinedOutput(); err != nil {
		t.Fatalf("%s", out)
	}
	if out, err := exec.Command("git", "-C", root, "commit", "-m", "first").CombinedOutput(); err != nil {
		t.Fatalf("%s", out)
	}
	write("b.txt", "two")
	if out, err := exec.Command("git", "-C", root, "add", "b.txt").CombinedOutput(); err != nil {
		t.Fatalf("%s", out)
	}
	if out, err := exec.Command("git", "-C", root, "commit", "-m", "second").CombinedOutput(); err != nil {
		t.Fatalf("%s", out)
	}
	a.useProject(core.NewLocal(), nil, root)
	pumpUntil(t, queue, func() bool { return a.repo != nil })
	a.loadLog()
	pumpUntil(t, queue, func() bool { return a.logLoaded && !a.logLoading })
	if len(a.logCommits) != 2 {
		t.Fatalf("commits: %+v", a.logCommits)
	}
	if a.logCommits[0].Subject != "second" || a.logCommits[1].Subject != "first" {
		t.Fatalf("commits: %+v", a.logCommits)
	}
	if a.logSelected != 0 {
		t.Fatalf("selected %d", a.logSelected)
	}
	pumpUntil(t, queue, func() bool { return a.logDetail != nil && !a.logDetailLoading })
	if len(a.logDetail.Files) != 1 || a.logDetail.Files[0].Path != "b.txt" {
		t.Fatalf("files: %+v", a.logDetail.Files)
	}
	if a.logDetail.Files[0].Code != "A" {
		t.Fatalf("code: %+v", a.logDetail.Files[0])
	}
	if len(a.logDetail.Branches) == 0 {
		t.Fatalf("branches: %+v", a.logDetail.Branches)
	}

	// The path filter reads only the commits that touch a path.
	a.logPath, a.logPathInput = "a.txt", "a.txt"
	a.logLoaded = false
	a.loadLog()
	pumpUntil(t, queue, func() bool { return a.logLoaded && !a.logLoading })
	if len(a.logCommits) != 1 || a.logCommits[0].Subject != "first" {
		t.Fatalf("path filter: %+v", a.logCommits)
	}
}

func TestLogFileGroupsSortTheRootFirst(t *testing.T) {
	groups := logFileGroups([]logFile{
		{Code: "M", Path: "internal/api/api.go"},
		{Code: "A", Path: "go.mod"},
		{Code: "M", Path: "internal/api/api_test.go"},
	})
	if len(groups) != 2 {
		t.Fatalf("%+v", groups)
	}
	if groups[0].Dir != "." || len(groups[0].Files) != 1 {
		t.Fatalf("%+v", groups[0])
	}
	if groups[1].Dir != "internal/api" || len(groups[1].Files) != 2 {
		t.Fatalf("%+v", groups[1])
	}
}

func TestLogBranchPanelChoosesABranch(t *testing.T) {
	a, queue := testDispatchApp(t)
	root := gitRepository(t)
	write := func(name, text string) {
		if err := os.WriteFile(filepath.Join(root, name), []byte(text), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	commit := func(message string) {
		if out, err := exec.Command("git", "-C", root, "add", "-A").CombinedOutput(); err != nil {
			t.Fatalf("%s", out)
		}
		if out, err := exec.Command("git", "-C", root, "commit", "-m", message).CombinedOutput(); err != nil {
			t.Fatalf("%s", out)
		}
	}
	write("main.txt", "main")
	commit("on main")
	if out, err := exec.Command("git", "-C", root, "checkout", "-b", "feature/api").CombinedOutput(); err != nil {
		t.Fatalf("%s", out)
	}
	write("api.txt", "api")
	commit("on the branch")
	a.useProject(core.NewLocal(), nil, root)
	pumpUntil(t, queue, func() bool { return a.repo != nil })
	a.showLog()
	pumpUntil(t, queue, func() bool { return a.logLoaded && !a.logLoading })
	tt := ui.NewTester(a.view, 1280, 840)
	tt.Frame()
	pumpUntil(t, queue, func() bool { return len(a.branches) > 0 })
	tt.Frame()
	if err := tt.Click("日志分支：feature/api"); err != nil {
		t.Fatal(err)
	}
	pumpUntil(t, queue, func() bool { return a.logBranch == "feature/api" })
	pumpUntil(t, queue, func() bool { return a.logLoaded && !a.logLoading })
	if len(a.logCommits) == 0 || a.logCommits[0].Subject != "on the branch" {
		t.Fatalf("commits: %+v", a.logCommits)
	}
}

func TestLogSearchReadsRegularExpressions(t *testing.T) {
	a := newApp()
	defer a.shutdown()
	commit := logCommit{Hash: "abc123", Subject: "fix(WikiTask): 移除重复代码", Author: "Vael Demo"}
	a.logQuery = `fix\(Wiki\w+\)`
	if a.logMatches(commit) {
		t.Fatal("a pattern matched without the flag")
	}
	a.logRegex = true
	if !a.logMatches(commit) {
		t.Fatal("the pattern did not match")
	}
	a.logCase = true
	a.logQuery = "wiki"
	if a.logMatches(commit) {
		t.Fatal("case was ignored")
	}
}

func TestLogToolbarHidesTheGraphAndTheDetail(t *testing.T) {
	a, queue := testDispatchApp(t)
	root := gitRepository(t)
	if err := os.WriteFile(filepath.Join(root, "main.txt"), []byte("main"), 0o644); err != nil {
		t.Fatal(err)
	}
	for _, args := range [][]string{{"add", "-A"}, {"commit", "-m", "on main"}} {
		if out, err := exec.Command("git", append([]string{"-C", root}, args...)...).CombinedOutput(); err != nil {
			t.Fatalf("%s", out)
		}
	}
	a.useProject(core.NewLocal(), nil, root)
	pumpUntil(t, queue, func() bool { return a.repo != nil })
	a.showLog()
	pumpUntil(t, queue, func() bool { return a.logLoaded && !a.logLoading })
	tt := ui.NewTester(a.view, 1280, 840)
	tt.Frame()
	if err := tt.Click("显示提交图"); err != nil {
		t.Fatal(err)
	}
	if a.logGraphOn {
		t.Fatal("the graph did not hide")
	}
	tt.Frame()
	if err := tt.Click("显示提交详情"); err != nil {
		t.Fatal(err)
	}
	if a.logDetailOn {
		t.Fatal("the detail did not hide")
	}
}

func TestAssignLanesJoinsTheBranchesOfAMerge(t *testing.T) {
	commits := []logCommit{
		{Hash: "m", Parents: []string{"a", "b"}},
		{Hash: "a", Parents: []string{"c"}},
		{Hash: "b", Parents: []string{"c"}},
		{Hash: "c"},
	}
	assignLanes(commits)
	if len(commits[0].Joins) != 1 || commits[0].Joins[0] != (logJoin{Lane: 1}) {
		t.Fatalf("the merge does not join its second parent: %+v", commits[0])
	}
	if len(commits[0].Out) != 2 || commits[0].Out[0] != "a" || commits[0].Out[1] != "b" {
		t.Fatalf("out: %+v", commits[0].Out)
	}
	if logColumns(commits) != 2 {
		t.Fatalf("columns: %d", logColumns(commits))
	}
	// The lanes that wait for the same commit join it, and leave nothing
	// behind.
	if len(commits[3].Out) != 0 {
		t.Fatalf("the root leaves lanes: %+v", commits[3].Out)
	}
}

func TestTheLogKeepsTheCommitsTheFiltersAllow(t *testing.T) {
	a := newApp()
	defer a.shutdown()
	a.logCommits = []logCommit{
		{Hash: "a", Subject: "first"},
		{Hash: "b", Subject: "second"},
		{Hash: "c", Subject: "third"},
	}
	a.logQuery = "second"
	rows := a.logShownCommits()
	if len(rows) != 1 || rows[0].Hash != "b" || rows[0].Row != 1 {
		t.Fatalf("%+v", rows)
	}
	a.logQuery = ""
	rows = a.logShownCommits()
	if len(rows) != 3 {
		t.Fatalf("%+v", rows)
	}
}

func TestTheLogMakesABranchAtACommit(t *testing.T) {
	a, queue := testDispatchApp(t)
	root := gitRepository(t)
	if err := os.WriteFile(filepath.Join(root, "main.txt"), []byte("main"), 0o644); err != nil {
		t.Fatal(err)
	}
	for _, args := range [][]string{{"add", "-A"}, {"commit", "-m", "on main"}} {
		if out, err := exec.Command("git", append([]string{"-C", root}, args...)...).CombinedOutput(); err != nil {
			t.Fatalf("%s", out)
		}
	}
	a.useProject(core.NewLocal(), nil, root)
	pumpUntil(t, queue, func() bool { return a.repo != nil })
	hash, err := exec.Command("git", "-C", root, "rev-parse", "HEAD").Output()
	if err != nil {
		t.Fatal(err)
	}
	a.createBranchFrom("feature/from-log", strings.TrimSpace(string(hash)))
	pumpUntil(t, queue, func() bool { return !a.gitBusy })
	out, err := exec.Command("git", "-C", root, "branch", "--format=%(refname:short)").Output()
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(out), "feature/from-log") {
		t.Fatalf("the branch was not made: %s", out)
	}
}
