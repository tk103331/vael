package main

import (
	"strings"
	"testing"

	"github.com/egoist/mygo/ui"
	"vael/internal/workspace"
)

func TestTheSettingsKeepOneHeight(t *testing.T) {
	a := newApp()
	defer a.shutdown()
	a.root = t.TempDir()
	a.loadLSPSettings()
	a.settingsOpen = true
	tt := ui.NewTester(a.view, 1280, 840)
	tt.Frame()
	done, ok := tt.Find("完成")
	if !ok {
		t.Fatal("the settings are not drawn")
	}
	for _, section := range []string{settingsEditor, settingsEnv, settingsLSP, settingsAgent} {
		a.settingsSection = section
		tt.Frame()
		other, ok := tt.Find("完成")
		if !ok {
			t.Fatalf("%q is not drawn", section)
		}
		if other.Y != done.Y {
			t.Fatalf("%q moved the window: %v, not %v", section, other.Y, done.Y)
		}
	}
}

func TestTheSettingsCanBeSearchedAndReset(t *testing.T) {
	a := newApp()
	defer a.shutdown()
	a.settingsOpen = true
	a.settingsSection = settingsEditor
	tt := ui.NewTester(a.view, 1280, 840)
	tt.Frame()
	if !tt.HasText("自动换行") || !tt.HasText("缩略图") {
		t.Fatalf("the settings are not drawn: %v", tt.Texts())
	}
	a.settingsQuery = "缩略图"
	tt.Frame()
	if !tt.HasText("缩略图") || tt.HasText("自动换行") {
		t.Fatalf("the search did not filter: %v", tt.Texts())
	}
	if !tt.HasText("编辑器") {
		t.Fatalf("the section of the setting went away: %v", tt.Texts())
	}
	a.settingsQuery = "没有这种设置"
	tt.Frame()
	if !tt.HasText("没有匹配的设置") {
		t.Fatalf("nothing says there is no match: %v", tt.Texts())
	}
	// Reset puts the section back the way it was.
	a.settingsQuery = ""
	a.wrap, a.minimap = true, false
	a.settingsSection = settingsEditor
	a.resetSettingsSection(settingsEditor)
	if a.wrap || !a.minimap || a.tabSize != 0 {
		t.Fatalf("the editor was not reset: wrap=%v mini=%v tabs=%d", a.wrap, a.minimap, a.tabSize)
	}
	a.settingsSection = settingsLook
	tt.Frame()
	if !tt.HasText("恢复默认") || !tt.HasText("打开配置目录") {
		t.Fatalf("the settings actions are not drawn: %v", tt.Texts())
	}
	// The about page says what Vael does with the user's work.
	a.settingsSection = settingsAbout
	tt.Frame()
	if !tt.HasText("没有遥测") {
		t.Fatalf("the about page: %v", tt.Texts())
	}
}

func TestTheActivityBarSizeIsOfferedAndKept(t *testing.T) {
	a := newApp()
	defer a.shutdown()
	a.settingsOpen = true
	a.settingsSection = settingsLook
	tt := ui.NewTester(a.view, 1280, 840)
	tt.Frame()
	if !tt.HasText("图标栏大小") || !tt.HasText("大") {
		t.Fatalf("the setting is not offered: %v", tt.Texts())
	}
	a.activitySize = 32
	saved := a.snapshot()
	if saved.ActivitySize != 32 {
		t.Fatalf("the session: %+v", saved)
	}
	opened := newApp()
	defer opened.shutdown()
	opened.applyLayout(saved)
	if opened.activitySize != 32 {
		t.Fatalf("the size after reopening: %d", opened.activitySize)
	}
}

func TestTheSettingsScrollWhatIsTooLong(t *testing.T) {
	a := newApp()
	defer a.shutdown()
	a.root = t.TempDir()
	a.lspPath = ""
	a.loadLSPSettings()
	// Enough agents that the list of them is longer than the window.
	for i := 0; i < 16; i++ {
		name := "Agent " + strings.Repeat("长", 3) + itoa(i)
		if err := a.agents.Add(workspace.Agent{Name: name, Command: "acp --stdio"}); err != nil {
			t.Fatal(err)
		}
	}
	a.agentRegistryOpen = false
	a.settingsOpen = true
	a.settingsSection = settingsAgent
	tt := ui.NewTester(a.view, 1280, 840)
	tt.Frame()
	first, ok := tt.Find("Agent 长长长0")
	if !ok {
		t.Fatalf("the agents are not drawn: %v", tt.Texts())
	}
	// Scrolling the section brings in the agents the window cannot hold
	// at once.
	for range 4 {
		tt.Scroll(900, 400, 0, 120)
		tt.Frame()
	}
	firstAgain, ok := tt.Find("Agent 长长长0")
	if !ok || firstAgain.Y >= first.Y {
		t.Fatalf("the section did not scroll: %+v, not %+v", firstAgain, first)
	}
	last, ok := tt.Find("Agent 长长长15")
	if !ok || last.Y > 700 {
		t.Fatalf("the last agent is not in the window: %v %+v", ok, last)
	}
}
