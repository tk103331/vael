package main

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/egoist/mygo/ui"
	"github.com/egoist/mygo/ui/editor"
	"vael/internal/core"
	"vael/internal/workspace"
)

func TestTheQuickOpenReadsFilesSymbolsAndLines(t *testing.T) {
	parts := parseQuickQuery("@main")
	if !parts.symbolOnly || parts.symbol != "main" || parts.file != "" {
		t.Fatalf("a symbol alone: %+v", parts)
	}
	parts = parseQuickQuery("internal/a.go@Run")
	if parts.symbolOnly || parts.file != "internal/a.go" || parts.symbol != "Run" || parts.line != 0 {
		t.Fatalf("a file and a symbol: %+v", parts)
	}
	parts = parseQuickQuery("main.go:42")
	if parts.file != "main.go" || parts.line != 42 || parts.symbol != "" || parts.symbolOnly {
		t.Fatalf("a file and a line: %+v", parts)
	}
	parts = parseQuickQuery("main.go:42@main")
	if parts.file != "main.go" || parts.line != 42 || parts.symbol != "main" {
		t.Fatalf("both: %+v", parts)
	}
}

func TestTheQuickOpenGoesToASymbolOfAFile(t *testing.T) {
	a, queue := testDispatchApp(t)
	root := t.TempDir()
	path := filepath.Join(root, "main.go")
	if err := os.WriteFile(path, []byte("package main\n\nfunc main() {\n}\n\nfunc helper() {}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	a.useProject(core.NewLocal(), nil, root)
	pumpUntil(t, queue, func() bool { return len(a.index.Files) > 0 })
	a.quickQuery = "main.go@helper"
	a.quickOpen = true
	tt := ui.NewTester(a.view, 1280, 840)
	tt.Frame()
	tt.Key(0, ui.KeyEnter)
	pumpUntil(t, queue, func() bool { return a.tabOf(path) != nil })
	tab := a.tabOf(path)
	if tab == nil {
		t.Fatal("the file did not open")
	}
	if pos := tab.state.CursorPos(); pos.Line != 5 || pos.Col != 5 {
		t.Fatalf("the caret is at %+v", pos)
	}
}

func TestTheQuickOpenListsTheSymbolsOfTheFile(t *testing.T) {
	a := newApp()
	defer a.shutdown()
	a.root = t.TempDir()
	path := filepath.Join(a.root, "main.go")
	a.tabs = []*fileTab{{path: path, state: &editor.State{Buffer: editor.NewBuffer("package main\n\nfunc main() {}\n\nfunc helper() {}\n")}}}
	a.selected = 0
	a.quickQuery = "@help"
	a.quickOpen = true
	tt := ui.NewTester(a.view, 1280, 840)
	tt.Frame()
	if _, ok := tt.Find("helper"); !ok {
		t.Fatalf("the symbol is not drawn: %v", tt.Texts())
	}
	// "main" is out of the query.
	for _, shown := range tt.Texts() {
		if shown == "main" {
			t.Fatalf("a symbol out of the query is drawn: %v", tt.Texts())
		}
	}
	if err := tt.Click("helper"); err != nil {
		t.Fatal(err)
	}
	if pos := a.tabs[0].state.CursorPos(); pos.Line != 4 || pos.Col != 5 {
		t.Fatalf("the caret is at %+v", pos)
	}
	if a.quickOpen {
		t.Fatal("the window did not close")
	}
}

func TestTheWorkspaceSymbolsListAndOpen(t *testing.T) {
	a, queue := testDispatchApp(t)
	root := t.TempDir()
	path := filepath.Join(root, "main.go")
	text := "package main\n\nfunc alpha() {}\n\nfunc beta() {}\n"
	if err := os.WriteFile(path, []byte(text), 0o644); err != nil {
		t.Fatal(err)
	}
	a.useProject(core.NewLocal(), nil, root)
	pumpUntil(t, queue, func() bool { return len(a.index.Files) > 0 })
	a.symbols = workspace.SymbolsInFile("main.go", text)
	a.workspaceSymbolsCommand()
	a.workspaceSymbolQuery = "beta"
	tt := ui.NewTester(a.view, 1280, 840)
	tt.Frame()
	if _, ok := tt.Find("beta"); !ok {
		t.Fatalf("the symbol is not drawn: %v", tt.Texts())
	}
	if err := tt.Click("beta"); err != nil {
		t.Fatal(err)
	}
	pumpUntil(t, queue, func() bool { return a.tabOf(path) != nil })
	tab := a.tabOf(path)
	if tab == nil {
		t.Fatal("the file did not open")
	}
	if pos := tab.state.CursorPos(); pos.Line != 4 || pos.Col != 5 {
		t.Fatalf("the caret is at %+v", pos)
	}
}

func TestTheEditorSplitsInTwo(t *testing.T) {
	a := newApp()
	defer a.shutdown()
	a.root = t.TempDir()
	one := filepath.Join(a.root, "one.go")
	two := filepath.Join(a.root, "two.go")
	a.tabs = []*fileTab{
		{path: one, state: &editor.State{Buffer: editor.NewBuffer("package one\n")}},
		{path: two, state: &editor.State{Buffer: editor.NewBuffer("package two\n")}},
	}
	a.selected = 0
	a.splitEditor()
	if !a.split || a.splitPath != two {
		t.Fatalf("the split: %v %q", a.split, a.splitPath)
	}
	tt := ui.NewTester(a.view, 1280, 840)
	tt.Frame()
	if _, ok := tt.Find("取消拆分"); !ok {
		t.Fatalf("the second pane is not drawn: %v", tt.Texts())
	}
	// Each pane's strip lists the tabs, and each chooses its own.
	if err := tt.Click("one.go"); err != nil {
		t.Fatal(err)
	}
	if a.focusPane != 0 || a.splitPath != two {
		t.Fatalf("a click on the left strip moved the right pane: %v %q", a.focusPane, a.splitPath)
	}
	// The right pane takes the file chosen in its own strip, and the
	// keyboard.
	a.focusPaneOn(1)
	a.focusTab(0)
	if a.splitPath != one {
		t.Fatalf("the right pane shows %q", a.splitPath)
	}
	if a.selected != 0 || a.active() != a.tabs[0] {
		t.Fatalf("the left pane moved: %v", a.selected)
	}
	// Closing a tab that the right pane shows leaves the split sound.
	a.closeTab(0)
	if a.splitPath != "" && a.splitIndex() < 0 {
		t.Fatalf("the right pane kept a file that is gone: %q", a.splitPath)
	}
}

func TestTheQuickOpenOpensInThePaneWithTheKeyboard(t *testing.T) {
	a, queue := testDispatchApp(t)
	root := t.TempDir()
	one := filepath.Join(root, "one.go")
	two := filepath.Join(root, "two.go")
	three := filepath.Join(root, "three.go")
	for _, path := range []string{one, two, three} {
		if err := os.WriteFile(path, []byte("package x\n\nfunc x() {}\n"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	a.useProject(core.NewLocal(), nil, root)
	pumpUntil(t, queue, func() bool { return len(a.index.Files) > 0 })
	a.openFile(one)
	pumpUntil(t, queue, func() bool { return a.tabOf(one) != nil })
	a.openFile(two)
	pumpUntil(t, queue, func() bool { return a.tabOf(two) != nil })
	a.selected = 0
	a.splitEditor()
	a.focusPaneOn(1)
	a.quickQuery = "three.go"
	a.quickOpen = true
	tt := ui.NewTester(a.view, 1280, 840)
	tt.Frame()
	tt.Key(0, ui.KeyEnter)
	pumpUntil(t, queue, func() bool { return a.tabOf(three) != nil && a.splitPath == three })
	if a.selected != 0 || a.tabs[0].path != one {
		t.Fatalf("the left pane moved: %v", a.selected)
	}
	if a.paneTab(1) != a.tabOf(three) {
		t.Fatalf("the right pane does not show the file: %q", a.splitPath)
	}
}

// TestTheSplitPaneShowsItsOwnFile checks that both panes draw an editor.
func TestTheSplitPaneShowsItsOwnFile(t *testing.T) {
	a := newApp()
	defer a.shutdown()
	a.root = t.TempDir()
	one := filepath.Join(a.root, "one.go")
	two := filepath.Join(a.root, "two.go")
	a.tabs = []*fileTab{
		{path: one, state: &editor.State{Buffer: editor.NewBuffer("package one\n")}},
		{path: two, state: &editor.State{Buffer: editor.NewBuffer("package two\n")}},
	}
	a.selected = 0
	a.splitEditor()
	tt := ui.NewTester(a.view, 1280, 840)
	tt.Frame()
	editors := 0
	for _, shown := range tt.Texts() {
		if shown == "Code editor" {
			editors++
		}
	}
	if editors != 2 {
		t.Fatalf("%d editors are drawn: %v", editors, tt.Texts())
	}
}
