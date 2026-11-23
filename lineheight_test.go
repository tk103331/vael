package main

import (
	"path/filepath"
	"testing"

	"github.com/egoist/mygo/ui"
	"github.com/egoist/mygo/ui/editor"
)

// TestTheLineHeightIsAPercentage checks that the settings' 120/150/180 are
// percentages, which the editor takes as a multiple of the font's size.
func TestTheLineHeightIsAPercentage(t *testing.T) {
	a := newApp()
	defer a.shutdown()
	state := &editor.State{Buffer: editor.NewBuffer("a\nb\n")}
	for _, c := range []struct {
		percent float32
		want    float32
	}{{0, 1.5}, {120, 1.2}, {150, 1.5}, {180, 1.8}} {
		a.lineHeight = c.percent
		a.styleEditor(state)
		if state.LineHeight != c.want {
			t.Fatalf("%v%% became %v, not %v", c.percent, state.LineHeight, c.want)
		}
	}
}

// TestTheLineHeightLeavesTheLinesInView checks the editor with the setting
// at 150%: a file of twelve lines is twelve lines, not one taller than the
// window, as it was when the percentage was taken for a multiple.
func TestTheLineHeightLeavesTheLinesInView(t *testing.T) {
	a := newApp()
	defer a.shutdown()
	a.root = t.TempDir()
	a.fontSize = 9
	a.lineHeight = 150
	path := filepath.Join(a.root, "a.go")
	a.tabs = []*fileTab{{path: path, state: &editor.State{Buffer: editor.NewBuffer(longText(12)), KeepScrollbars: true}}}
	a.selected = 0
	a.sidebar = false
	a.rightPanel = false
	a.showBottom = false
	tt := ui.NewTester(a.view, 951, 940)
	tt.Frame()
	first, last := a.tabs[0].state.VisibleLines()
	if first != 0 || last < 10 {
		t.Fatalf("the lines in view: %d..%d", first, last)
	}
	if got := a.tabs[0].state.LineHeight; got != 1.5 {
		t.Fatalf("the editor's line height is %v", got)
	}
	if _, ok := tt.Find("Code editor"); !ok {
		t.Fatal("the editor is not drawn")
	}
}

// TestTheDiffTakesTheLineHeight checks that a difference views its lines at
// the setting's height, as the editor does.
func TestTheDiffTakesTheLineHeight(t *testing.T) {
	a := newApp()
	defer a.shutdown()
	a.lineHeight = 180
	tab := &fileTab{path: "a.go · 工作区", diff: &editor.DiffState{Old: editor.NewBuffer("a\n"), New: editor.NewBuffer("b\n")}}
	a.tabs = []*fileTab{tab}
	a.applyFontSize()
	if tab.diff.LineHeight != 1.8 {
		t.Fatalf("the difference's line height is %v", tab.diff.LineHeight)
	}
}
