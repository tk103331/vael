package main

import (
	"testing"

	"github.com/egoist/mygo/ui"
	"vael/internal/workspace"
)

func TestThemesChooseTheColoursOfTheWindow(t *testing.T) {
	a := newApp()
	defer a.shutdown()
	a.root = t.TempDir()

	dark := a.colors()
	a.setTheme("vael-light")
	light := a.colors()
	if a.dark {
		t.Fatal("a light theme still says dark")
	}
	if dark.card == light.card || dark.text == light.text {
		t.Fatalf("the themes share their colours: %+v %+v", dark, light)
	}
	if a.editorTheme().Background != light.card {
		t.Fatalf("the editor does not draw with the theme: %+v", a.editorTheme().Background)
	}

	// Cmd+Shift+L goes to the other side of the same family.
	a.toggleDark()
	if !a.dark || a.themeID != "vael-dark" {
		t.Fatalf("the toggle did not come back: %v %q", a.dark, a.themeID)
	}
	a.setTheme("solarized-light")
	a.toggleDark()
	if a.themeID != "solarized-dark" {
		t.Fatalf("the toggle left the family: %q", a.themeID)
	}

	// A theme of another family has a colour of its own.
	nord := themeByID("nord")
	if nord.Palette.accent == themeByID("vael-dark").Palette.accent {
		t.Fatal("two themes share an accent")
	}
	// An unknown theme falls back to the default one, rather than nothing.
	a.setTheme("no-such-theme")
	if a.themeID != "vael-dark" {
		t.Fatalf("the fallback is %q", a.themeID)
	}
}

func TestTheSessionKeepsTheThemeAndTheIcons(t *testing.T) {
	a := newApp()
	defer a.shutdown()
	a.root = t.TempDir()
	a.setTheme("nord")
	a.iconID = "glyphs"
	s := a.snapshot()
	if s.Theme != "nord" || s.Icons != "glyphs" {
		t.Fatalf("the session holds %q %q", s.Theme, s.Icons)
	}
	restored := newApp()
	defer restored.shutdown()
	restored.applyLayout(s)
	if restored.themeID != "nord" || restored.iconID != "glyphs" || !restored.dark {
		t.Fatalf("the layout came back as %q %q %v", restored.themeID, restored.iconID, restored.dark)
	}
	// A session of before themes has only whether it was dark.
	old := newApp()
	defer old.shutdown()
	old.applyLayout(workspace.Session{Dark: true})
	if old.themeID != "vael-dark" || !old.dark {
		t.Fatalf("an old session came back as %q %v", old.themeID, old.dark)
	}
}

func TestTheIconThemeMarksTheFiles(t *testing.T) {
	a := newApp()
	defer a.shutdown()
	a.root = "/项目"
	badges := a.markFor("main.go")
	if badges.text != "GO" || badges.icon != nil {
		t.Fatalf("the badge theme: %+v", badges)
	}
	a.iconID = "glyphs"
	glyphs := a.markFor("main.go")
	if glyphs.icon == nil || glyphs.text != "" {
		t.Fatalf("the icon theme: %+v", glyphs)
	}
	if glyphs.color(a.colors()) != a.colors().accent {
		t.Fatal("a code file is not the accent's")
	}
	// A kind without a mark of its own keeps its badge.
	if unknown := a.markFor("go.mod"); unknown.text != "·" {
		t.Fatalf("an unknown kind: %+v", unknown)
	}
	if len(iconThemes) < 2 || iconThemes[0].ID != "badges" {
		t.Fatalf("the icon themes: %+v", iconThemes)
	}
	_ = ui.Color{}
}
