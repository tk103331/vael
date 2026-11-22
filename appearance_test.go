package main

import (
	"path/filepath"
	"testing"

	"github.com/egoist/mygo/ui"
	"github.com/egoist/mygo/ui/editor"
	"vael/internal/workspace"
)

func TestZoomResizesOpenEditorsAndPersists(t *testing.T) {
	a := newApp()
	defer a.shutdown()
	a.fontSize = 0
	tab := &fileTab{state: &editor.State{Buffer: editor.NewBuffer("package main\n")}}
	diff := &fileTab{diff: &editor.DiffState{}}
	a.tabs = []*fileTab{tab, diff}
	a.selected = 0

	a.zoom(1)
	if a.fontSize != 14 {
		t.Fatalf("size %v", a.fontSize)
	}
	if tab.state.Font.Size != 14 || diff.diff.Font.Size != 14 {
		t.Fatal("the editors were not resized")
	}
	if got := a.snapshot().FontSize; got != 14 {
		t.Fatalf("the session keeps %v", got)
	}

	restored := newApp()
	defer restored.shutdown()
	restored.applyLayout(workspace.Session{FontSize: 17})
	if restored.fontSize != 17 {
		t.Fatalf("the layout was not restored: %v", restored.fontSize)
	}
	opened := &editor.State{Buffer: editor.NewBuffer("x")}
	restored.tabs = []*fileTab{{state: opened}}
	restored.applyFontSize()
	if opened.Font.Size != 17 {
		t.Fatalf("a restored size was not applied: %v", opened.Font.Size)
	}

	a.zoom(-10)
	if a.fontSize != 9 {
		t.Fatalf("the smallest size is not respected: %v", a.fontSize)
	}
	a.zoom(100)
	if a.fontSize != 24 {
		t.Fatalf("the largest size is not respected: %v", a.fontSize)
	}
	a.zoomReset()
	if a.fontSize != 13 {
		t.Fatalf("reset: %v", a.fontSize)
	}
}

func TestTabCyclingAndBulkClosing(t *testing.T) {
	a := newApp()
	defer a.shutdown()
	a.root = t.TempDir()
	first := &editor.State{Buffer: editor.NewBuffer("one\n")}
	second := &editor.State{Buffer: editor.NewBuffer("two\n")}
	dirtyBuffer := editor.NewBuffer("draft\n")
	dirtyBuffer.Set("draft changed\n")
	dirty := &editor.State{Buffer: dirtyBuffer}
	a.tabs = []*fileTab{
		{path: filepath.Join(a.root, "a.go"), state: first},
		{path: filepath.Join(a.root, "b.go"), state: second},
		{path: filepath.Join(a.root, "c.go"), state: dirty},
	}
	a.selected = 0

	a.nextTab()
	if a.selected != 1 {
		t.Fatalf("next: %d", a.selected)
	}
	a.prevTab()
	if a.selected != 0 {
		t.Fatalf("prev: %d", a.selected)
	}
	a.selected = len(a.tabs) - 1
	a.nextTab()
	if a.selected != 0 {
		t.Fatalf("next from the last tab: %d", a.selected)
	}

	// Closing the others asks about the changes of the tabs it would drop.
	a.selected = 0
	a.closeOtherTabs(0)
	if len(a.tabs) != 3 || a.confirm == nil {
		t.Fatal("dirty tabs were closed without asking")
	}
	a.confirm = nil // the user dismissed the dialog
	if len(a.tabs) != 3 {
		t.Fatalf("dismissal dropped tabs: %+v", a.tabs)
	}

	// Confirmed, the others go, and the kept tab keeps its buffer.
	a.closeOtherTabs(1)
	action := a.confirm
	a.confirm = nil
	action.action()
	if len(a.tabs) != 1 || a.tabs[0].path != filepath.Join(a.root, "b.go") || a.selected != 0 {
		t.Fatalf("close others: %+v selected %d", a.tabs, a.selected)
	}
	if second.Buffer.String() != "two\n" {
		t.Fatal("the kept tab's buffer was replaced")
	}

	// Closing all of them empties the editor, and asks when one is dirty.
	second.Buffer.Set("two changed\n")
	a.closeAllTabs()
	if a.confirm == nil {
		t.Fatal("closing a dirty tab did not ask")
	}
	action = a.confirm
	a.confirm = nil
	action.action()
	if len(a.tabs) != 0 || a.selected != -1 {
		t.Fatalf("close all: %+v selected %d", a.tabs, a.selected)
	}
}

func TestGlyphsOfTheFileTree(t *testing.T) {
	if g := glyphFor("main.go"); g.text != "GO" {
		t.Fatalf("go: %+v", g)
	}
	if g := glyphFor("app.TEST.go"); g.text != "GO" {
		t.Fatalf("case: %+v", g)
	}
	if g := glyphFor("notes.md"); g.text != "MD" {
		t.Fatalf("md: %+v", g)
	}
	if g := glyphFor("Makefile"); g.text != "·" {
		t.Fatalf("unknown: %+v", g)
	}
	if g := glyphFor(".gitignore"); g.text != "·" {
		t.Fatalf("dotfile: %+v", g)
	}
}

func TestWelcomeShowsTheRecentProjects(t *testing.T) {
	a := newApp()
	defer a.shutdown()
	a.environments.Recent = []workspace.Environment{
		{Root: "/tmp/one"},
		{Kind: "ssh", Target: "dev", Root: "/srv/two", Binary: "vael-core"},
	}
	// The welcome screen renders the recents without an open project.
	a.root = ""
	a.tabs = nil
	tt := ui.NewTester(a.view, 1280, 840)
	tt.Frame()
}
