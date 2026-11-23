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
