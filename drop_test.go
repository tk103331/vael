package main

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/egoist/mygo/ui"
	"vael/internal/core"
)

func TestDropOpensFoldersAndFiles(t *testing.T) {
	a, queue := testDispatchApp(t)
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "outside.txt"), []byte("x"), 0644); err != nil {
		t.Fatal(err)
	}
	other := t.TempDir()

	// A dropped folder becomes the project.
	a.handleDrop([]string{other})
	pumpUntil(t, queue, func() bool { return a.root == other })
	if a.connection != nil {
		t.Fatal("a local drop connected somewhere")
	}

	// A dropped file opens in the editor.
	a.handleDrop([]string{filepath.Join(root, "outside.txt")})
	pumpUntil(t, queue, func() bool {
		return a.tabForPath(filepath.Join(root, "outside.txt")) != nil
	})

	// A missing path is reported, not ignored.
	a.message = ""
	a.handleDrop([]string{filepath.Join(root, "nope.txt")})
	if a.message == "" {
		t.Fatal("a missing drop was silent")
	}
	// Nothing dropped, nothing happens.
	a.handleDrop(nil)
}

func TestDropWithARemoteConnectionIsExplained(t *testing.T) {
	a, _ := testDispatchApp(t)
	a.connection = &core.Connection{Kind: core.KindSSH, Target: "box", Root: "/srv", Binary: "vael-core"}
	a.handleDrop([]string{t.TempDir()})
	if a.message == "" {
		t.Fatal("a drop while connected was silent")
	}
}

func TestSettingsDialogChangesThePreferences(t *testing.T) {
	a := newApp()
	defer a.shutdown()
	a.fontSize = 13
	a.root = t.TempDir()
	tt := ui.NewTester(a.view, 1280, 840)
	if err := tt.Click("设置"); err != nil {
		t.Fatal(err)
	}
	if !a.settingsOpen {
		t.Fatal("the settings dialog did not open")
	}
	if err := tt.Click("放大字号"); err != nil {
		t.Fatal(err)
	}
	if a.fontSize != 14 {
		t.Fatalf("size %v", a.fontSize)
	}
	if err := tt.Click("深色"); err != nil {
		t.Fatal(err)
	}
	if !a.dark {
		t.Fatal("theme did not change")
	}
	if err := tt.Click("编辑器"); err != nil {
		t.Fatal(err)
	}
	if err := tt.Click("自动换行开关"); err != nil {
		t.Fatal(err)
	}
	if !a.wrap {
		t.Fatal("wrap did not change")
	}
	if err := tt.Click("Tab 宽度：2"); err != nil {
		t.Fatal(err)
	}
	if a.tabSize != 2 {
		t.Fatalf("tab size %v", a.tabSize)
	}
	if err := tt.Click("行号开关"); err != nil {
		t.Fatal(err)
	}
	if a.numbers {
		t.Fatal("line numbers did not change")
	}
	if err := tt.Click("完成"); err != nil {
		t.Fatal(err)
	}
	if a.settingsOpen {
		t.Fatal("the settings dialog did not close")
	}
}

func TestTitleBarDrawsTheWindowControls(t *testing.T) {
	a := newApp()
	defer a.shutdown()
	a.root = t.TempDir()
	tt := ui.NewTester(a.view, 1280, 840)
	tt.Frame()
	// The window's controls are drawn by the app, and they answer to a
	// click without a window behind them.
	if err := tt.Click("关闭窗口"); err != nil {
		t.Fatal(err)
	}
	if err := tt.Click("最大化窗口"); err != nil {
		t.Fatal(err)
	}
	// The title bar is the app's own: the logo drags the window.
	if err := tt.Click("VAEL"); err != nil {
		t.Fatal(err)
	}
}
