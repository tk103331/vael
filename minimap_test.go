package main

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/egoist/mygo/ui"
	"github.com/egoist/mygo/ui/editor"
)

// longText is a text long enough that its whole does not show at once.
func longText(lines int) string {
	var b strings.Builder
	for i := 0; i < lines; i++ {
		switch i % 4 {
		case 0:
			b.WriteString("func line")
		case 1:
			b.WriteString("\tif line")
		case 2:
			b.WriteString("// a comment on line")
		default:
			b.WriteString("var value")
		}
		b.WriteString(itoa(i))
		b.WriteString(" = 0\n")
	}
	return b.String()
}

func TestTheMinimapFollowsTheEditorsText(t *testing.T) {
	a := newApp()
	defer a.shutdown()
	a.root = t.TempDir()
	path := filepath.Join(a.root, "long.go")
	a.tabs = []*fileTab{{path: path, state: &editor.State{Buffer: editor.NewBuffer(longText(400)), KeepScrollbars: true}}}
	a.selected = 0
	tt := ui.NewTester(a.view, 1000, 640)
	tt.Frame()
	if _, ok := tt.Find("缩略图"); !ok {
		t.Fatalf("the minimap is not drawn: %v", tt.Texts())
	}
	// A click in the middle of the column scrolls to the middle of the
	// text.
	if err := tt.Click("缩略图"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	first, last := a.tabs[0].state.VisibleLines()
	if first == 0 {
		t.Fatalf("the click did not scroll: %d..%d", first, last)
	}
	if first < 150 {
		t.Fatalf("the click scrolled too little: %d..%d", first, last)
	}
	// A click at the top goes back to the beginning.
	box, _ := tt.Find("缩略图")
	tt.ClickAt(box.X+box.W/2, box.Y+1)
	tt.Frame()
	if first, _ := a.tabs[0].state.VisibleLines(); first != 0 {
		t.Fatalf("the second click did not go back: %d", first)
	}
}

// TestTheMinimapIsTheTextsInk checks that the column shows where lines start
// and how long they are, and that it is read again as the text changes.
func TestTheMinimapIsTheTextsInk(t *testing.T) {
	a := newApp()
	defer a.shutdown()
	a.root = t.TempDir()
	buffer := editor.NewBuffer("func a() {}\n\tif b {\n\n")
	tab := &fileTab{path: filepath.Join(a.root, "a.go"), state: &editor.State{Buffer: buffer}}
	ink := a.minimapInk(tab, 3)
	if ink[0].start != 0 || ink[0].width <= 0 {
		t.Fatalf("the first line: %+v", ink[0])
	}
	if ink[1].start <= 0 {
		t.Fatalf("the indented line starts at 0: %+v", ink[1])
	}
	if ink[1].width >= ink[0].width {
		// "if b {" is shorter than "func a() {}".
		t.Fatalf("the lengths: %+v %+v", ink[0], ink[1])
	}
	if ink[2].width != 0 {
		t.Fatalf("an empty line has ink: %+v", ink[2])
	}
	// A change reads the ink again.
	buffer.Edit(editor.Edit{Start: 0, End: buffer.Len(), Text: "// a longer comment line than before\n"})
	fresh := a.minimapInk(tab, 3)
	if fresh[0].width <= ink[0].width {
		t.Fatalf("the ink was not read again: %+v, not %+v", fresh[0], ink[0])
	}
}

func TestTheMinimapSettingHidesIt(t *testing.T) {
	a := newApp()
	defer a.shutdown()
	a.root = t.TempDir()
	a.tabs = []*fileTab{{path: filepath.Join(a.root, "a.go"), state: &editor.State{Buffer: editor.NewBuffer("package a\n")}}}
	a.selected = 0
	tt := ui.NewTester(a.view, 1000, 640)
	tt.Frame()
	if _, ok := tt.Find("缩略图"); !ok {
		t.Fatalf("the minimap is off by default: %v", tt.Texts())
	}
	a.minimap = false
	tt.Frame()
	if _, ok := tt.Find("缩略图"); ok {
		t.Fatal("the minimap is still drawn when the setting is off")
	}
	// The session keeps the setting, and an old one without it keeps the
	// default.
	s := a.snapshot()
	if s.Minimap == nil || *s.Minimap {
		t.Fatalf("the session holds %+v", s.Minimap)
	}
	restored := newApp()
	defer restored.shutdown()
	restored.applyLayout(s)
	if restored.minimap {
		t.Fatal("the setting did not come back")
	}
	old := s
	old.Minimap = nil
	kept := newApp()
	defer kept.shutdown()
	kept.applyLayout(old)
	if !kept.minimap {
		t.Fatal("a session without the setting turned the minimap off")
	}
}
