package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/egoist/mygo/ui"
	"github.com/egoist/mygo/ui/editor"
	"vael/internal/core"
)

func TestSavingTakesOffAndAddsWhatTheSettingsSay(t *testing.T) {
	if got := saveText("a  \n\tb\n", true, false); got != "a\n\tb\n" {
		t.Fatalf("trimmed: %q", got)
	}
	if got := saveText("a\nb", false, true); got != "a\nb\n" {
		t.Fatalf("final newline: %q", got)
	}
	if got := saveText("a  \nb", true, true); got != "a\nb\n" {
		t.Fatalf("both: %q", got)
	}
	if got := saveText("", false, true); got != "" {
		t.Fatalf("an empty file: %q", got)
	}
}

func TestASavedFileFollowsTheSettings(t *testing.T) {
	a, queue := testDispatchApp(t)
	root := t.TempDir()
	path := filepath.Join(root, "main.go")
	if err := os.WriteFile(path, []byte("package main  \n\nfunc main() {}"), 0o644); err != nil {
		t.Fatal(err)
	}
	a.useProject(core.NewLocal(), nil, root)
	pumpUntil(t, queue, func() bool { return len(a.entries[a.root]) > 0 })
	a.trimOnSave, a.finalNewline = true, true
	a.openFile(path)
	pumpUntil(t, queue, func() bool { return a.tabOf(path) != nil })
	tab := a.tabOf(path)
	a.save()
	pumpUntil(t, queue, func() bool { return !tab.saving && !tab.state.Buffer.Modified() })
	saved, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(saved) != "package main\n\nfunc main() {}\n" {
		t.Fatalf("saved: %q", saved)
	}
	if tab.state.Buffer.String() != string(saved) {
		t.Fatalf("the buffer has %q", tab.state.Buffer.String())
	}
}

func TestTheQuickOpenGoesToALine(t *testing.T) {
	if name, line := quickTarget("main.go:42"); name != "main.go" || line != 42 {
		t.Fatalf("%q %d", name, line)
	}
	if name, line := quickTarget("internal/a:b.go"); name != "internal/a:b.go" || line != 0 {
		t.Fatalf("%q %d", name, line)
	}
	if name, line := quickTarget("main.go:"); name != "main.go:" || line != 0 {
		t.Fatalf("%q %d", name, line)
	}

	a, queue := testDispatchApp(t)
	root := t.TempDir()
	path := filepath.Join(root, "main.go")
	lines := strings.Repeat("// 注释\n", 30)
	if err := os.WriteFile(path, []byte(lines), 0o644); err != nil {
		t.Fatal(err)
	}
	a.useProject(core.NewLocal(), nil, root)
	pumpUntil(t, queue, func() bool { return len(a.index.Files) > 0 })
	a.quickQuery = "main.go:20"
	a.quickOpen = true
	tt := ui.NewTester(a.view, 1280, 840)
	tt.Frame()
	tt.Key(0, ui.KeyEnter)
	pumpUntil(t, queue, func() bool { return a.tabOf(path) != nil })
	tab := a.tabOf(path)
	if pos := tab.state.CursorPos(); pos.Line != 19 {
		t.Fatalf("the caret is at %+v", pos)
	}
}

// TestTheRenameEditsTheFiles checks what a rename does with the answer of a
// server, without one running.
func TestTheRenameEditsTheFiles(t *testing.T) {
	a, queue := testDispatchApp(t)
	root := t.TempDir()
	path := filepath.Join(root, "main.go")
	if err := os.WriteFile(path, []byte("package main\n\nvar count = 1\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	other := filepath.Join(root, "other.go")
	if err := os.WriteFile(other, []byte("package main\n\nvar total = count + 1\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	a.useProject(core.NewLocal(), nil, root)
	pumpUntil(t, queue, func() bool { return len(a.entries[a.root]) > 0 })
	a.openFile(path)
	pumpUntil(t, queue, func() bool { return a.tabOf(path) != nil })
	tab := a.tabOf(path)

	result, _ := json.Marshal(map[string]any{
		"changes": map[string]any{
			fileURI(path): []any{map[string]any{
				"range":   map[string]any{"start": map[string]int{"line": 2, "character": 4}, "end": map[string]int{"line": 2, "character": 9}},
				"newText": "renamed",
			}},
			fileURI(other): []any{map[string]any{
				"range":   map[string]any{"start": map[string]int{"line": 2, "character": 12}, "end": map[string]int{"line": 2, "character": 17}},
				"newText": "renamed",
			}},
		},
	})
	edits := lspWorkspaceEdits(result)
	if len(edits) != 2 {
		t.Fatalf("edits: %+v", edits)
	}
	for _, file := range edits {
		a.renameInFile(file.path, file.edits, "utf-16")
	}
	pumpUntil(t, queue, func() bool { return !tab.saving })
	if got := tab.state.Buffer.String(); !strings.Contains(got, "var renamed = 1") {
		t.Fatalf("the open file has %q", got)
	}
	// The file that was not open is read, changed and written.
	pumpUntil(t, queue, func() bool {
		saved, err := os.ReadFile(other)
		return err == nil && strings.Contains(string(saved), "var total = renamed + 1")
	})
}

func TestARenameTakesTheWordAtTheCaret(t *testing.T) {
	text := "package main\n\nvar count = 1\n"
	span := wordRangeAt(text, strings.Index(text, "count")+2)
	if got := text[span[0]:span[1]]; got != "count" {
		t.Fatalf("the word is %q", got)
	}
	a := newApp()
	defer a.shutdown()
	a.root = t.TempDir()
	a.tabs = []*fileTab{{path: filepath.Join(a.root, "main.go"), state: &editor.State{Buffer: editor.NewBuffer(text)}}}
	a.selected = 0
	// Without a language server, the prompt does not open: the file's own
	// rename takes it, as F2 did.
	a.renameSymbol()
	if a.symbolPrompt != nil {
		t.Fatalf("a prompt without a server: %+v", a.symbolPrompt)
	}
}
