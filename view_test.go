package main

import (
	"path/filepath"
	"testing"

	"github.com/egoist/mygo/ui"
	"github.com/egoist/mygo/ui/editor"
	"vael/internal/core"
	"vael/internal/workspace"
)

func TestWorkspaceLayoutAndTheme(t *testing.T) {
	a := newApp()
	a.root = t.TempDir()
	path := filepath.Join(a.root, "main.go")
	a.entries[a.root] = []workspace.Entry{{Name: "main.go", Path: path}}
	a.tabs = []*fileTab{{path: path, state: &editor.State{Buffer: editor.NewBuffer("package main\n")}}}
	a.selected = 0
	tt := ui.NewTester(a.view, 1280, 840)
	if err := tt.Click("切换到深色"); err != nil {
		t.Fatal(err)
	}
	if !a.dark {
		t.Fatal("theme did not change")
	}
	if err := tt.Click("显示/隐藏底部面板"); err != nil {
		t.Fatal(err)
	}
	if !a.showBottom {
		t.Fatal("terminal pane did not open")
	}
	if err := tt.Click("显示/隐藏侧栏"); err != nil {
		t.Fatal(err)
	}
	if a.sidebar {
		t.Fatal("sidebar did not hide")
	}
	if err := tt.Click("文件"); err != nil {
		t.Fatal(err)
	}
	if !a.sidebar {
		t.Fatal("activity bar did not restore sidebar")
	}
}

func TestClosingModifiedFileRequiresConfirmation(t *testing.T) {
	a := newApp()
	a.root = t.TempDir()
	b := editor.NewBuffer("package main\n")
	a.tabs = []*fileTab{{path: filepath.Join(a.root, "main.go"), state: &editor.State{Buffer: b}}}
	a.selected = 0
	tt := ui.NewTester(a.view, 1000, 720)
	if err := tt.Click("main.go"); err != nil {
		t.Fatal(err)
	}
	b.Set("package changed\n")
	a.closeTab(0)
	tt.Frame()
	if len(a.tabs) != 1 || a.confirm == nil {
		t.Fatal("modified file closed without confirmation")
	}
	if err := tt.Click("取消"); err != nil {
		t.Fatal(err)
	}
	if len(a.tabs) != 1 || a.confirm != nil {
		t.Fatal("cancel did not preserve file")
	}
	a.closeTab(0)
	tt.Frame()
	if err := tt.Click("继续"); err != nil {
		t.Fatal(err)
	}
	if len(a.tabs) != 0 {
		t.Fatal("confirmed file did not close")
	}
}

func TestQuickOpenAndSearchPanel(t *testing.T) {
	a := newApp()
	a.root = t.TempDir()
	a.index = workspace.FileIndex{Files: []string{"main.go", "src/model.go"}}
	tt := ui.NewTester(a.view, 1280, 840)
	if err := tt.Click("快速打开文件…"); err != nil {
		t.Fatal(err)
	}
	if !a.quickOpen {
		t.Fatal("quick open did not open")
	}
	a.quickOpen = false
	tt.Frame()
	if err := tt.Click("Commit"); err != nil {
		t.Fatal(err)
	}
	if a.section != "commit" || !a.sidebar {
		t.Fatal("the commit panel was not selected")
	}
}

func TestSessionSnapshotPreservesOnlyEditorFiles(t *testing.T) {
	a := newApp()
	a.root = t.TempDir()
	a.dark = true
	a.sideWidth = 310
	path := filepath.Join(a.root, "main.go")
	state := &editor.State{Buffer: editor.NewBuffer("package main\n")}
	state.Select(8, 8)
	a.tabs = []*fileTab{{path: path, state: state}, {path: "diff", diff: &editor.DiffState{}}}
	a.selected = 0
	s := a.snapshot()
	if len(s.Files) != 1 || s.Files[0].Cursor != 8 || s.Active != path {
		t.Fatalf("%+v", s)
	}
	restored := newApp()
	restored.applyLayout(s)
	if !restored.dark || restored.sideWidth != 310 {
		t.Fatal("layout not restored")
	}
	s.SideWidth = -10
	s.EditorShare = 1000
	restored.applyLayout(s)
	if restored.sideWidth != 310 || restored.editorShare != 62 {
		t.Fatal("invalid dimensions accepted")
	}
}

func TestEnvironmentDialogAndRemotePaths(t *testing.T) {
	a := newApp()
	a.root = t.TempDir()
	tt := ui.NewTester(a.view, 1280, 840)
	tt.Frame()
	if err := tt.Click("连接环境"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if err := tt.Click("连接新环境…"); err != nil {
		t.Fatal(err)
	}
	if a.connectionPrompt == nil || a.connectionPrompt.Kind != "ssh" {
		t.Fatal("environment dialog did not open")
	}
	if err := tt.Click("WSL"); err != nil {
		t.Fatal(err)
	}
	if a.connectionPrompt.Kind != "wsl" {
		t.Fatal("WSL was not selected")
	}
	if err := tt.Click("DOCKER"); err != nil {
		t.Fatal(err)
	}
	if a.connectionPrompt.Kind != "docker" {
		t.Fatal("Docker was not selected")
	}
	if err := tt.Click("取消"); err != nil {
		t.Fatal(err)
	}
	if a.connectionPrompt != nil {
		t.Fatal("dialog did not close")
	}
	a.connection = &core.Connection{Kind: "wsl", Target: "Ubuntu", Root: "/home/dev", Binary: "vael-core"}
	if got := a.joinPath("/home/dev", "src/main.go"); got != "/home/dev/src/main.go" {
		t.Fatal(got)
	}
	if rel, err := a.relativePath("/home/dev", "/home/dev/src/main.go"); err != nil || rel != "src/main.go" {
		t.Fatalf("%q %v", rel, err)
	}
	s := a.snapshot()
	if s.ConnectionKind != "wsl" || s.Target != "Ubuntu" {
		t.Fatal("environment not persisted")
	}
}

func TestSavedEnvironmentManager(t *testing.T) {
	a := newApp()
	defer a.shutdown()
	a.root = t.TempDir()
	a.environmentsPath = filepath.Join(t.TempDir(), "environments.json")
	a.profileName = "开发容器"
	a.saveConnectionProfile(core.Connection{Kind: "docker", Target: "dev", Root: "/workspace", Binary: "vael-core"})
	if len(a.environments.Profiles) != 1 {
		t.Fatal("profile was not saved")
	}
	tt := ui.NewTester(a.view, 1280, 840)
	tt.Frame()
	if err := tt.Click("连接环境"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if err := tt.Click("管理工作环境…"); err != nil {
		t.Fatal(err)
	}
	if !a.settingsOpen || a.settingsSection != settingsEnv {
		t.Fatalf("the environments did not open in the settings: %v %q", a.settingsOpen, a.settingsSection)
	}
	if err := tt.Click("收藏当前环境"); err != nil {
		t.Fatal(err)
	}
	if len(a.environments.Profiles) != 2 {
		t.Fatal("the current environment was not saved")
	}
	if err := tt.Click("完成"); err != nil {
		t.Fatal(err)
	}
	if a.settingsOpen {
		t.Fatal("the settings did not close")
	}
	saved, err := workspace.LoadEnvironments(a.environmentsPath)
	if err != nil || len(saved.Profiles) != 2 {
		t.Fatalf("profiles not persisted: %+v %v", saved, err)
	}
}

func TestTheActivityBarLeavesTheToolWindowToItsStrip(t *testing.T) {
	a := newApp()
	defer a.shutdown()
	a.root = t.TempDir()
	a.sidebar = true
	tt := ui.NewTester(a.view, 1280, 840)
	tt.Frame()
	for _, label := range []string{"Git 日志", "终端"} {
		if err := tt.Click(label); err == nil {
			t.Fatalf("the activity bar still shows %q", label)
		}
	}
}

func TestTheToolWindowSitsUnderTheWorkspace(t *testing.T) {
	a := newApp()
	defer a.shutdown()
	a.root = t.TempDir()
	a.sidebar = true
	a.showLog()
	tt := ui.NewTester(a.view, 1280, 840)
	tt.Frame()
	title, ok := tt.Find("当前文件夹没有可用的 Git 仓库。")
	if !ok {
		t.Fatal("the log is not there")
	}
	// The panel starts after the activity bar and the sidebar, not under
	// them.
	if title.X < 46+float32(a.sideWidth)-4 {
		t.Fatalf("the log starts under the sidebar: %+v", title)
	}
	hide, ok := tt.Find("隐藏面板")
	if !ok {
		t.Fatal("the hide button is not there")
	}
	// The panel at the bottom stops before the panels of the right.
	if hide.X+hide.W > 1280-34 {
		t.Fatalf("the log reaches under the bar of the right: %+v", hide)
	}
}

func TestTheBarOfTheRightHoldsThePanels(t *testing.T) {
	a := newApp()
	defer a.shutdown()
	a.root = t.TempDir()
	a.sidebar = true
	tt := ui.NewTester(a.view, 1280, 840)
	tt.Frame()
	if err := tt.Click("显示/隐藏大纲"); err != nil {
		t.Fatal(err)
	}
	if !a.rightPanel || a.rightSection != "outline" {
		t.Fatalf("the structure did not open: %v %q", a.rightPanel, a.rightSection)
	}
	tt.Frame()
	if err := tt.Click("显示/隐藏 Agent"); err != nil {
		t.Fatal(err)
	}
	if !a.rightPanel || a.rightSection != "agent" {
		t.Fatalf("the agent did not open: %v %q", a.rightPanel, a.rightSection)
	}
	tt.Frame()
	if err := tt.Click("显示/隐藏 Agent"); err != nil {
		t.Fatal(err)
	}
	if a.rightPanel {
		t.Fatal("the panel did not close")
	}
}

func TestTheBottomStripSwitchesThePanel(t *testing.T) {
	a := newApp()
	defer a.shutdown()
	a.root = t.TempDir()
	a.sidebar = true
	a.showTerminal()
	tt := ui.NewTester(a.view, 1280, 840)
	tt.Frame()
	if err := tt.Click("面板：Git"); err != nil {
		t.Fatal(err)
	}
	if a.bottomTab != "git" {
		t.Fatalf("tab %q", a.bottomTab)
	}
	tt.Frame()
	if _, ok := tt.Find("Git"); !ok {
		t.Fatal("the title of the log is not Git")
	}
	if err := tt.Click("面板：终端"); err != nil {
		t.Fatal(err)
	}
	if a.bottomTab != "terminal" {
		t.Fatalf("tab %q", a.bottomTab)
	}
}

func TestThePanelsAreValues(t *testing.T) {
	want := []struct {
		id       string
		position panelPosition
	}{
		{"files", panelLeft}, {"commit", panelLeft},
		{"outline", panelRight}, {"agent", panelRight},
		{"terminal", panelBottom}, {"git", panelBottom}, {"problems", panelBottom},
	}
	for _, w := range want {
		item := panelAt(w.id, w.position)
		if item == nil {
			t.Fatalf("%q is not a panel of its position", w.id)
		}
		if item.Icon == nil || item.Name == "" || item.Tip == "" || item.View == nil {
			t.Fatalf("%q is not whole: %+v", w.id, item)
		}
	}
	for position, want := range map[panelPosition]int{
		panelLeft: 2, panelRight: 2, panelBottom: 3,
	} {
		if got := len(panelsAt(position)); got != want {
			t.Fatalf("the bar of %v shows %d panels, not %d", position, got, want)
		}
	}
	if panelAt("terminal", panelLeft) != nil {
		t.Fatal("a panel is at the wrong position")
	}
	if d := defaultPanel(panelRight); d == nil || d.ID != "outline" {
		t.Fatalf("the default panel of the right is %+v", d)
	}
	// The panels the bars never show are the ones the palette opens.
	for _, id := range []string{"search", "symbols", "tasks"} {
		item := panelAt(id, panelLeft)
		if item == nil || !item.Hidden {
			t.Fatalf("%q should be a hidden panel of the left", id)
		}
	}
}
