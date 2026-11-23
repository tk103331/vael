package main

import (
	"path/filepath"
	"testing"

	"github.com/egoist/mygo/ui"
	"github.com/egoist/mygo/ui/editor"
)

func TestTheFontOfTheEditorIsChosen(t *testing.T) {
	a := newApp()
	defer a.shutdown()
	a.root = t.TempDir()
	path := filepath.Join(a.root, "main.go")
	a.tabs = []*fileTab{{path: path, state: &editor.State{Buffer: editor.NewBuffer("package main\n")}}}
	a.selected = 0
	// The families of a machine differ: the list is given for the test.
	a.fontFamilies = []ui.FontFamily{{Name: "Test Mono", Monospaced: true}}
	a.settingsOpen = true
	a.settingsSection = settingsLook
	tt := ui.NewTester(a.view, 1280, 840)
	tt.Frame()
	if err := tt.Click("字体：默认等宽"); err != nil {
		t.Fatal(err)
	}
	if !a.fontMenuOpen {
		t.Fatal("the fonts did not open")
	}
	tt.Frame()
	if err := tt.Click("字体：Test Mono"); err != nil {
		t.Fatal(err)
	}
	if a.fontFamily != "Test Mono" {
		t.Fatalf("the family is %q", a.fontFamily)
	}
	if a.fontMenuOpen {
		t.Fatal("the list did not close")
	}
	if a.tabs[0].state.Font.Family != "Test Mono" {
		t.Fatalf("the editor kept its font: %+v", a.tabs[0].state.Font)
	}
	if a.fontName() != "Test Mono" {
		t.Fatalf("the name shown is %q", a.fontName())
	}
	// The session keeps the family.
	s := a.snapshot()
	if s.FontFamily != "Test Mono" {
		t.Fatalf("the session holds %q", s.FontFamily)
	}
	restored := newApp()
	defer restored.shutdown()
	restored.applyLayout(s)
	if restored.fontFamily != "Test Mono" || restored.editorFont().Family != "Test Mono" {
		t.Fatalf("the family came back as %q", restored.fontFamily)
	}
}

func TestTheFontMenuFilters(t *testing.T) {
	a := newApp()
	defer a.shutdown()
	a.root = t.TempDir()
	a.fontFamilies = []ui.FontFamily{
		{Name: "Test Mono", Monospaced: true},
		{Name: "Other Mono", Monospaced: true},
	}
	a.settingsOpen = true
	a.settingsSection = settingsLook
	a.fontMenuOpen = true
	a.fontQuery = "other"
	tt := ui.NewTester(a.view, 1280, 840)
	tt.Frame()
	if _, ok := tt.Find("字体：Other Mono"); !ok {
		t.Fatalf("the match is not drawn: %v", tt.Texts())
	}
	for _, shown := range tt.Texts() {
		if shown == "字体：Test Mono" {
			t.Fatalf("a font out of the query is drawn: %v", tt.Texts())
		}
	}
}
