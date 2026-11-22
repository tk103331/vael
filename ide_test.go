package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/egoist/mygo/ui"
	"github.com/egoist/mygo/ui/editor"
)

func TestDetectIndent(t *testing.T) {
	cases := []struct {
		text  string
		tabs  bool
		width int
	}{
		{"package main\n\nfunc main() {\n    println()\n}\n", false, 4},
		{"a:\n  b\n  c\n", false, 2},
		{"a:\n\tb\n\tc\n", true, 4},
		{"", false, 4},
	}
	for _, c := range cases {
		tabs, width := detectIndent(c.text)
		if tabs != c.tabs || width != c.width {
			t.Fatalf("%q: tabs=%v width=%d", c.text, tabs, width)
		}
	}
}

func TestConvertIndent(t *testing.T) {
	text := "func main() {\n    if true {\n        println()\n    }\n}\n"
	tabs := convertIndent(text, true, 4)
	if !strings.Contains(tabs, "\tif true {") || !strings.Contains(tabs, "\t\tprintln()") {
		t.Fatalf("tabs: %q", tabs)
	}
	if back := convertIndent(tabs, false, 4); back != text {
		t.Fatalf("round trip: %q", back)
	}
}

func TestIndentWidgetConvertsTheFile(t *testing.T) {
	a := newApp()
	defer a.shutdown()
	a.root = t.TempDir()
	text := "func main() {\n    println()\n}\n"
	path := filepath.Join(a.root, "main.go")
	if err := os.WriteFile(path, []byte(text), 0644); err != nil {
		t.Fatal(err)
	}
	a.tabs = []*fileTab{{path: path, state: &editor.State{Buffer: editor.NewBuffer(text)}}}
	a.selected = 0
	tt := ui.NewTester(a.view, 1280, 840)
	tt.Frame()
	if err := tt.Click("空格: 4"); err != nil {
		t.Fatal(err)
	}
	if err := tt.ChooseMenuItem("制表符"); err != nil {
		t.Fatal(err)
	}
	f := a.active()
	if got := f.state.Buffer.String(); !strings.Contains(got, "\tprintln()") {
		t.Fatalf("converted: %q", got)
	}
	if !f.state.Buffer.Modified() {
		t.Fatal("the conversion did not mark the buffer modified")
	}
	// The conversion is one edit, which Undo takes back.
	f.state.Buffer.Undo()
	if got := f.state.Buffer.String(); got != text {
		t.Fatalf("undo: %q", got)
	}
	// The widget says what the file uses now.
	if tabs, width := f.indentOf(); tabs || width != 4 {
		t.Fatalf("detected tabs=%v width=%d", tabs, width)
	}
}

func TestLineEndingWidgetSwitchesTheEnding(t *testing.T) {
	a := newApp()
	defer a.shutdown()
	a.root = t.TempDir()
	path := filepath.Join(a.root, "main.go")
	if err := os.WriteFile(path, []byte("a\nb\n"), 0644); err != nil {
		t.Fatal(err)
	}
	a.tabs = []*fileTab{{path: path, state: &editor.State{Buffer: editor.NewBuffer("a\nb\n")}}}
	a.selected = 0
	tt := ui.NewTester(a.view, 1280, 840)
	tt.Frame()
	if err := tt.Click("LF"); err != nil {
		t.Fatal(err)
	}
	if err := tt.ChooseMenuItem("CRLF（Windows）"); err != nil {
		t.Fatal(err)
	}
	if got := a.active().state.Buffer.LineEnding(); got != "\r\n" {
		t.Fatalf("ending %q", got)
	}
	if err := tt.Click("CRLF"); err != nil {
		t.Fatal(err)
	}
	if err := tt.ChooseMenuItem("LF（Unix）"); err != nil {
		t.Fatal(err)
	}
	if got := a.active().state.Buffer.LineEnding(); got != "\n" {
		t.Fatalf("ending %q", got)
	}
}

func TestRecentFilesKeepTheNewestFirst(t *testing.T) {
	a, queue := testDispatchApp(t)
	a.root = t.TempDir()
	first := filepath.Join(a.root, "a.go")
	second := filepath.Join(a.root, "b.go")
	for _, path := range []string{first, second} {
		if err := os.WriteFile(path, []byte("package main\n"), 0644); err != nil {
			t.Fatal(err)
		}
	}
	a.openFile(first)
	pumpUntil(t, queue, func() bool { return a.tabForPath(first) != nil })
	a.openFile(second)
	pumpUntil(t, queue, func() bool { return a.tabForPath(second) != nil })
	if len(a.recentFiles) != 2 || a.recentFiles[0] != second || a.recentFiles[1] != first {
		t.Fatalf("recent: %v", a.recentFiles)
	}
	// Opening a file again moves it to the front, without a duplicate.
	a.openFile(first)
	pumpUntil(t, queue, func() bool { return a.recentFiles[0] == first })
	if len(a.recentFiles) != 2 {
		t.Fatalf("recent: %v", a.recentFiles)
	}
	// The query filters by relative path.
	a.recentQuery = "b.go"
	matches := a.recentMatches()
	if len(matches) != 1 || matches[0] != second {
		t.Fatalf("matches: %v", matches)
	}
	// The list survives a session.
	s := a.snapshot()
	other := newApp()
	defer other.shutdown()
	other.applyLayout(s)
	if len(other.recentFiles) != 2 || other.recentFiles[0] != first {
		t.Fatalf("restored: %v", other.recentFiles)
	}
}

func TestRecentFilesDialogOpensAFile(t *testing.T) {
	a, queue := testDispatchApp(t)
	a.root = t.TempDir()
	path := filepath.Join(a.root, "main.go")
	if err := os.WriteFile(path, []byte("package main\n"), 0644); err != nil {
		t.Fatal(err)
	}
	a.recentFiles = []string{path}
	a.recentOpen = true
	tt := ui.NewTester(a.view, 1280, 840)
	tt.Frame()
	if err := tt.Click("main.go"); err != nil {
		t.Fatal(err)
	}
	tt.Key(0, ui.KeyEnter)
	pumpUntil(t, queue, func() bool { return a.tabForPath(path) != nil })
	if a.recentOpen {
		t.Fatal("the dialog did not close")
	}
}

func TestZenModeHidesTheToolWindows(t *testing.T) {
	a := newApp()
	defer a.shutdown()
	a.root = t.TempDir()
	a.sidebar = true
	a.showBottom = true
	a.rightPanel = true
	tt := ui.NewTester(a.view, 1280, 840)
	tt.Frame()
	if err := tt.Click("Commit"); err != nil {
		t.Fatalf("the sidebar should show: %v", err)
	}
	a.zen = true
	tt.Frame()
	if err := tt.Click("Commit"); err == nil {
		t.Fatal("zen mode still shows the sidebar")
	}
	if err := tt.Click("显示/隐藏大纲"); err == nil {
		t.Fatal("zen mode still shows the bar of the right")
	}
	// The editor stays.
	if err := tt.Click("Vael"); err != nil {
		t.Fatalf("the welcome screen should show: %v", err)
	}
	a.zen = false
	tt.Frame()
	if err := tt.Click("Commit"); err != nil {
		t.Fatalf("the sidebar should come back: %v", err)
	}
}

func TestRecentFilesDialogWithNothingRecent(t *testing.T) {
	a := newApp()
	defer a.shutdown()
	a.root = t.TempDir()
	a.recentOpen = true
	tt := ui.NewTester(a.view, 1280, 840)
	tt.Frame()
	if err := tt.Click("还没有最近打开的文件"); err != nil {
		t.Fatal(err)
	}
}
