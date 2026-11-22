package main

import (
	"os"
	"path/filepath"
	"testing"

	"vael/internal/core"
	"vael/internal/workspace"
)

func TestOutlineFollowsTheFileThatIsOpen(t *testing.T) {
	a, queue := testDispatchApp(t)
	root := t.TempDir()
	path := filepath.Join(root, "main.go")
	source := "package main\n\nfunc Hello() {}\n\nfunc World() {}\n"
	if err := os.WriteFile(path, []byte(source), 0644); err != nil {
		t.Fatal(err)
	}
	a.useProject(core.NewLocal(), nil, root)
	a.openFile(path)
	pumpUntil(t, queue, func() bool { return a.tabForPath(path) != nil })

	symbols := workspace.SymbolsInFile(path, a.tabForPath(path).state.Buffer.String())
	if len(symbols) != 2 || symbols[0].Name != "Hello" {
		t.Fatalf("%+v", symbols)
	}
	// The caret goes to the declaration that was chosen.
	a.jumpToSymbol(a.tabForPath(path), symbols[1])
	if got := a.tabForPath(path).state.Cursor(); got != symbols[1].Offset {
		t.Fatalf("caret at %d, wanted %d", got, symbols[1].Offset)
	}
}

func TestGoToDefinitionUsesTheProjectSymbols(t *testing.T) {
	a, queue := testDispatchApp(t)
	root := t.TempDir()
	path := filepath.Join(root, "lib.go")
	if err := os.WriteFile(path, []byte("package main\n\nfunc Helper() {}\n"), 0644); err != nil {
		t.Fatal(err)
	}
	use := filepath.Join(root, "use.go")
	if err := os.WriteFile(use, []byte("package main\n\nfunc main() { Helper() }\n"), 0644); err != nil {
		t.Fatal(err)
	}
	a.useProject(core.NewLocal(), nil, root)
	a.openFile(use)
	pumpUntil(t, queue, func() bool { return a.tabForPath(use) != nil })

	a.symbols = []workspace.Symbol{{Path: "lib.go", Name: "Helper", Kind: "func", Line: 3, Offset: 14}}
	offset := len("package main\n\nfunc main() { ")
	a.goToDefinition(a.tabForPath(use), offset)
	pumpUntil(t, queue, func() bool { return a.tabForPath(filepath.Join(root, "lib.go")) != nil })
	if got := a.tabForPath(filepath.Join(root, "lib.go")).state.Cursor(); got != 14 {
		t.Fatalf("caret at %d", got)
	}
	// A word nobody declares is reported, not silently ignored.
	a.message = ""
	a.goToDefinition(a.tabForPath(use), 0)
	if a.message == "" {
		t.Fatal("an unknown symbol was not reported")
	}
}

func TestWordAtFindsTheIdentifierAroundTheOffset(t *testing.T) {
	text := "package main\n\nfunc Hello() {}\n"
	if got := wordAt(text, len("package main\n\nfunc Hel")); got != "Hello" {
		t.Fatalf("%q", got)
	}
	if got := wordAt(text, 0); got != "package" {
		t.Fatalf("%q", got)
	}
	if got := wordAt(text, len(text)); got != "" {
		t.Fatalf("%q", got)
	}
	if got := wordAt("变量名测试", 0); got != "变量名测试" {
		t.Fatalf("%q", got)
	}
}

func TestSymbolSearchPanelListsTheProjectSymbols(t *testing.T) {
	a, queue := testDispatchApp(t)
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "a.go"), []byte("package a\n\nfunc Alpha() {}\n"), 0644); err != nil {
		t.Fatal(err)
	}
	a.useProject(core.NewLocal(), nil, root)
	a.section = "symbols"
	a.loadSymbols()
	pumpUntil(t, queue, func() bool { return !a.symbolsLoading && len(a.symbols) > 0 })
	if a.symbols[0].Name != "Alpha" {
		t.Fatalf("%+v", a.symbols)
	}
	a.symbolQuery = "alp"
	if got := workspace.MatchSymbols(a.symbols, a.symbolQuery, 10); len(got) != 1 {
		t.Fatalf("%+v", got)
	}
}
