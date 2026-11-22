package main

import (
	"image"
	"image/png"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"
	"vael/internal/core"
)

// TestScreenshot renders the editor offscreen and saves it as a PNG, for
// the README. It runs only when the environment names an output file:
//
//	VAEL_SHOT=/path/shot.png [VAEL_SHOT_PROJECT=/path/project] go test -run TestScreenshot .
func TestScreenshot(t *testing.T) {
	out := os.Getenv("VAEL_SHOT")
	if out == "" {
		t.Skip("VAEL_SHOT is not set")
	}
	root := os.Getenv("VAEL_SHOT_PROJECT")
	if root == "" {
		root = "/tmp/vael-demo"
	}
	if info, err := os.Stat(root); err != nil || !info.IsDir() {
		t.Skipf("the project to shoot is not there: %v", err)
	}

	a := newApp()
	defer a.shutdown()
	queue := make(chan func(), 256)
	a.dispatch = func(fn func()) { queue <- fn }

	// The workspace of the shot: the dark theme, the tree and the terminal.
	a.dark = true
	a.sidebar = true
	a.showBottom = true
	a.rightPanel = true
	a.editorShare = 62
	a.sideWidth = 252

	tt := ui.NewTester(a.view, 1280, 800)
	tt.SetScale(2)
	pump := func(ms int) {
		deadline := time.Now().Add(time.Duration(ms) * time.Millisecond)
		for time.Now().Before(deadline) {
			select {
			case fn := <-queue:
				fn()
			default:
				tt.Frame()
				time.Sleep(20 * time.Millisecond)
			}
		}
	}

	a.useProject(core.NewLocal(), nil, root)
	pump(700)
	a.openFile(filepath.Join(root, "main.go"))
	pump(400)
	a.openFile(filepath.Join(root, "internal", "store", "store.go"))
	pump(400)
	a.selected = 0
	a.newTerminal()
	pump(2600)
	tt.Frame()
	save(t, tt.Image(), out)

	// The Git panel of the same workspace.
	a.section = "commit"
	a.refreshGit()
	pump(600)
	tt.Frame()
	save(t, tt.Image(), strings.TrimSuffix(out, ".png")+"-git.png")

	// The quick-open panel over the editor.
	a.quickOpen = true
	pump(300)
	tt.Frame()
	save(t, tt.Image(), strings.TrimSuffix(out, ".png")+"-quick.png")
	a.quickOpen = false

	// The recent files dialog.
	a.recentOpen = true
	pump(300)
	tt.Frame()
	save(t, tt.Image(), strings.TrimSuffix(out, ".png")+"-recent.png")
	a.recentOpen = false

	// The project switcher.
	a.projectMenuOpen = true
	pump(300)
	tt.Frame()
	save(t, tt.Image(), strings.TrimSuffix(out, ".png")+"-projects.png")
	a.projectMenuOpen = false

	// The branch switcher.
	a.branchMenuOpen = true
	a.loadBranches()
	pump(500)
	tt.Frame()
	save(t, tt.Image(), strings.TrimSuffix(out, ".png")+"-branches.png")
	a.branchMenuOpen = false

	// The Git log, on a repository with a history.
	logRoot := logDemoRepo(t)
	a.useProject(core.NewLocal(), nil, logRoot)
	pump(500)
	a.bottomTab = "git"
	a.editorShare = 52
	a.logLoaded = false
	a.loadLog()
	pump(900)
	tt.Frame()
	save(t, tt.Image(), strings.TrimSuffix(out, ".png")+"-log.png")
}

// logDemoRepo makes a repository with a history, for the log's shot.
func logDemoRepo(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	run := func(args ...string) {
		cmd := exec.Command("git", append([]string{"-C", root}, args...)...)
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("%v: %s", args, out)
		}
	}
	run("init", "-b", "main")
	run("config", "user.name", "Vael Demo")
	run("config", "user.email", "demo@example.invalid")
	write := func(name, text string) {
		path := filepath.Join(root, name)
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(text), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	commit := func(message string) {
		run("add", "-A")
		run("commit", "-m", message)
	}
	write("README.md", "# demo\n")
	commit("初始化项目结构")
	write("main.go", "package main\n\nfunc main() {}\n")
	commit("添加程序入口")
	write("internal/store/store.go", "package store\n\n// Store keeps the counters.\ntype Store struct{}\n")
	commit("添加计数器存储")
	run("checkout", "-b", "feature/api")
	write("internal/api/api.go", "package api\n\n// Serve starts the API.\nfunc Serve() {}\n")
	commit("添加 HTTP 接口")
	write("internal/api/api_test.go", "package api\n")
	commit("补充接口测试")
	run("checkout", "main")
	write("docs/design.md", "# 设计\n")
	commit("补充设计文档")
	run("merge", "--no-ff", "feature/api", "-m", "合并 feature/api")
	run("tag", "v0.1.0")
	run("update-ref", "refs/remotes/origin/main", "HEAD")
	run("update-ref", "refs/remotes/origin/feature/api", "feature/api")
	write("go.mod", "module demo\n")
	commit("添加模块定义")
	return root
}

func save(t *testing.T, img *image.RGBA, path string) {
	f, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	if err := png.Encode(f, img); err != nil {
		t.Fatal(err)
	}
	t.Log("saved", path)
}
