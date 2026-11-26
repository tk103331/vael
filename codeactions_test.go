package main

import (
	"encoding/json"
	"os"
	"strings"
	"testing"

	"github.com/egoist/mygo/ui"
	"github.com/egoist/mygo/ui/editor"
)

func TestTheCodeActionsAreRead(t *testing.T) {
	result := json.RawMessage(`[
		{"title": "加上 import", "kind": "quickfix",
		 "edit": {"changes": {"file:///a/main.go": [
			{"range": {"start": {"line": 2, "character": 0}, "end": {"line": 2, "character": 0}}, "newText": "import \"fmt\"\n"}]}}},
		{"title": "整理 import", "kind": "source.organizeImports", "command": {"command": "gopls.organize", "arguments": [1]}},
		{"title": "没有内容"},
		{"title": "只有编辑", "edit": {"changes": {}}}
	]`)
	actions := lspCodeActions(result)
	if len(actions) != 2 {
		t.Fatalf("actions: %+v", actions)
	}
	if actions[0].title != "加上 import" || len(actions[0].edit) != 1 || actions[0].edit[0].path != "/a/main.go" {
		t.Fatalf("the edit: %+v", actions[0])
	}
	if actions[1].command == nil || actions[1].command.Command != "gopls.organize" {
		t.Fatalf("the command: %+v", actions[1])
	}
}

func TestTheReferencesAreRead(t *testing.T) {
	result := json.RawMessage(`[
		{"uri": "file:///a/main.go", "range": {"start": {"line": 4, "character": 6}, "end": {"line": 4, "character": 10}}},
		{"uri": "file:///a/other.go", "range": {"start": {"line": 0, "character": 0}, "end": {"line": 0, "character": 4}}},
		{"uri": "", "range": {"start": {"line": 0, "character": 0}}}
	]`)
	places := lspPlaces(result)
	if len(places) != 2 {
		t.Fatalf("places: %+v", places)
	}
	if places[0].path != "/a/main.go" || places[0].line != 4 || places[0].character != 6 {
		t.Fatalf("place: %+v", places[0])
	}
	if places[1].path != "/a/other.go" || places[1].line != 0 {
		t.Fatalf("place: %+v", places[1])
	}
}

// TestTheCodeActionsListAndApply checks what a server offers through the
// editor, with a fake one running.
func TestTheCodeActionsListAndApply(t *testing.T) {
	root := t.TempDir()
	path := root + "/main.go"
	if err := os.WriteFile(path, []byte("package main\n\nfunc main() {\n}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	a, queue := lspTestApp(t, root)
	a.openFile(path)
	pumpUntil(t, queue, func() bool { return a.tabOf(path) != nil })
	tab := a.tabOf(path)
	pumpUntil(t, queue, func() bool { return len(tab.state.Diagnostics()) > 0 })

	a.quickFixes()
	pumpUntil(t, queue, func() bool { return a.actions != nil })
	if len(a.actions.items) != 2 {
		t.Fatalf("actions: %+v", a.actions.items)
	}
	tt := ui.NewTester(a.view, 1280, 840)
	tt.Frame()
	tt.Frame()
	if _, ok := tt.Find("改成 println"); !ok {
		t.Fatalf("the fix is not drawn: %v", tt.Texts())
	}
	if err := tt.Click("改成 println"); err != nil {
		t.Fatal(err)
	}
	pumpUntil(t, queue, func() bool {
		return strings.Contains(tab.state.Buffer.String(), "func println() {")
	})
	// The fix saves the file as well; wait for the write so that the
	// temporary project is not removed while it is still in flight.
	pumpUntil(t, queue, func() bool {
		data, err := os.ReadFile(path)
		return err == nil && strings.Contains(string(data), "func println() {")
	})
	if a.actions != nil {
		t.Fatal("the list did not close")
	}
}

// TestTheReferencesListAndOpen checks that where a name is used is listed,
// and that a choice opens it at the place.
func TestTheReferencesListAndOpen(t *testing.T) {
	root := t.TempDir()
	path := root + "/main.go"
	if err := os.WriteFile(path, []byte("package main\n\nfunc main() {\n}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	a, queue := lspTestApp(t, root)
	a.openFile(path)
	pumpUntil(t, queue, func() bool { return a.tabOf(path) != nil })
	tab := a.tabOf(path)
	pumpUntil(t, queue, func() bool { return len(tab.state.Diagnostics()) > 0 })

	a.findReferences()
	pumpUntil(t, queue, func() bool { return a.refs != nil })
	if len(a.refs.items) != 1 {
		t.Fatalf("places: %+v", a.refs.items)
	}
	tt := ui.NewTester(a.view, 1280, 840)
	tt.Frame()
	if _, ok := tt.Find("main.go:3"); !ok {
		t.Fatalf("the place is not drawn: %v", tt.Texts())
	}
	if err := tt.Click("main.go:3"); err != nil {
		t.Fatal(err)
	}
	if pos := tab.state.CursorPos(); pos.Line != 2 || pos.Col != 5 {
		t.Fatalf("the caret is at %+v", pos)
	}
	if a.refs != nil {
		t.Fatal("the list did not close")
	}
}

// TestARangeHoldsTheProblemsInIt checks what a code action request says of
// the problems of the range it is about.
func TestARangeHoldsTheProblemsInIt(t *testing.T) {
	buffer := editor.NewBuffer("a\nb\nc\n")
	state := &editor.State{Buffer: buffer}
	state.SetDiagnostics(buffer.Version(), []editor.Diagnostic{
		{Start: 0, End: 1, Message: "第一行的问题", Severity: editor.SeverityError},
		{Start: 4, End: 5, Message: "第三行的问题", Severity: editor.SeverityWarning},
	})
	at := lspRange{Start: lspPosition{Line: 2}, End: lspPosition{Line: 3}}
	problems := diagnosticsIn(state, at, "utf-16")
	if len(problems) != 1 {
		t.Fatalf("problems: %+v", problems)
	}
	problem := problems[0].(map[string]any)
	if problem["message"] != "第三行的问题" || problem["severity"] != 2 {
		t.Fatalf("problem: %+v", problem)
	}
	// The range of the caret's line holds that line, as a request asks.
	caret := &editor.State{Buffer: editor.NewBuffer("a\nb\nc\n")}
	caret.GoTo(editor.Pos{Line: 1})
	if span := lspRangeOf(caret, "utf-16"); span.Start.Line != 1 || span.End.Line != 2 {
		t.Fatalf("the range: %+v", span)
	}
}
