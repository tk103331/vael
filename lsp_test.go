package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"
	"github.com/egoist/mygo/ui/editor"
	"vael/internal/core"
)

// TestLSPHelper is the language server the test in lsp_test.go starts: a
// process that speaks LSP on its standard input and output, and answers
// what the editor asks of it. It does nothing unless the test asked for it
// by name.
func TestLSPHelper(t *testing.T) {
	if os.Getenv("VAEL_LSP_SERVER") == "" {
		return
	}
	in := bufio.NewReader(os.Stdin)
	out := os.Stdout
	root := ""
	write := func(value any) {
		data, _ := json.Marshal(value)
		fmt.Fprintf(out, "Content-Length: %d\r\n\r\n%s", len(data), data)
	}
	answer := func(id *int, result any) {
		if id == nil {
			return
		}
		write(map[string]any{"jsonrpc": "2.0", "id": *id, "result": result})
	}
	for {
		// A message is framed by a header holding its length.
		length := 0
		for {
			line, err := in.ReadString('\n')
			if err != nil {
				return
			}
			line = strings.TrimSpace(line)
			if line == "" {
				break
			}
			if key, value, ok := strings.Cut(line, ":"); ok && strings.EqualFold(strings.TrimSpace(key), "content-length") {
				length, _ = strconv.Atoi(strings.TrimSpace(value))
			}
		}
		body := make([]byte, length)
		if _, err := io.ReadFull(in, body); err != nil {
			return
		}
		var message struct {
			ID     *int            `json:"id"`
			Method string          `json:"method"`
			Params json.RawMessage `json:"params"`
		}
		if json.Unmarshal(body, &message) != nil {
			continue
		}
		switch message.Method {
		case "initialize":
			var params struct {
				RootURI string `json:"rootUri"`
			}
			json.Unmarshal(message.Params, &params)
			if strings.HasPrefix(params.RootURI, "file://") {
				root = params.RootURI[len("file://"):]
			}
			answer(message.ID, map[string]any{
				"capabilities": map[string]any{
					"positionEncoding":           "utf-16",
					"hoverProvider":              true,
					"completionProvider":         map[string]any{},
					"definitionProvider":         true,
					"documentFormattingProvider": true,
				},
				"serverInfo": map[string]any{"name": "Fake", "version": "1"},
			})
		case "textDocument/didOpen":
			var params struct {
				TextDocument struct {
					URI string `json:"uri"`
				} `json:"textDocument"`
			}
			json.Unmarshal(message.Params, &params)
			write(map[string]any{
				"jsonrpc": "2.0", "method": "textDocument/publishDiagnostics",
				"params": map[string]any{
					"uri":     params.TextDocument.URI,
					"version": 1,
					"diagnostics": []any{map[string]any{
						"range":    map[string]any{"start": map[string]int{"line": 0, "character": 0}, "end": map[string]int{"line": 0, "character": 7}},
						"severity": 1,
						"message":  "假的错误",
						"source":   "fake",
					}},
				},
			})
		case "textDocument/completion":
			answer(message.ID, map[string]any{"items": []any{
				map[string]any{"label": "println", "kind": 3, "detail": "func(a ...any)"},
				map[string]any{"label": "print", "kind": 3, "detail": "func(a ...any)"},
			}})
		case "textDocument/hover":
			answer(message.ID, map[string]any{
				"contents": map[string]any{"kind": "markdown", "value": "`println(a ...any)`"},
			})
		case "textDocument/formatting":
			answer(message.ID, []any{map[string]any{
				"range":   map[string]any{"start": map[string]int{"line": 0, "character": 0}, "end": map[string]int{"line": 0, "character": 0}},
				"newText": "// 已格式化\n",
			}})
		case "textDocument/definition":
			answer(message.ID, map[string]any{
				"uri":   "file://" + root + "/other.go",
				"range": map[string]any{"start": map[string]int{"line": 2, "character": 4}},
			})
		case "shutdown":
			answer(message.ID, nil)
		case "exit":
			return
		}
	}
}

// lspTestApp is an app with the fake language server turned on for Go.
func lspTestApp(t *testing.T, root string) (*app, chan func()) {
	t.Helper()
	a, queue := testDispatchApp(t)
	command := "VAEL_LSP_SERVER=1 " + os.Args[0] + " -test.run=TestLSPHelper"
	a.lspSet = lspSettings{Languages: map[string]lspSetting{
		"go":         {Enabled: true, Command: command},
		"typescript": {Enabled: false, Command: "typescript-language-server --stdio"},
	}}
	a.lspOn = map[string]bool{"go": true}
	a.lspCmd = map[string]*string{}
	a.section = "files"
	a.useProject(core.NewLocal(), nil, root)
	pumpUntil(t, queue, func() bool { return len(a.entries[a.root]) > 0 })
	return a, queue
}

// pumpTexts runs the editor and the work it queued until the window shows
// the text, which the answers of a server arrive asynchronously as.
func pumpTexts(t *testing.T, queue chan func(), tt *ui.Tester, text string) bool {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		for drained := false; !drained; {
			select {
			case fn := <-queue:
				fn()
			default:
				drained = true
			}
		}
		tt.Frame()
		for _, shown := range tt.Texts() {
			if shown == text {
				return true
			}
		}
		time.Sleep(10 * time.Millisecond)
	}
	return false
}

func TestTheLanguageServerAnswersTheEditor(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(root+"/main.go", []byte("package main\n\nfunc main() {\n}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(root+"/other.go", []byte("package main\n\nvar target = 1\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	a, queue := lspTestApp(t, root)
	a.openFile(root + "/main.go")
	pumpUntil(t, queue, func() bool { return a.tabOf(root+"/main.go") != nil })
	tab := a.tabOf(root + "/main.go")

	// The server starts with the file, and tells of what it found.
	pumpUntil(t, queue, func() bool { return len(tab.state.Diagnostics()) > 0 })
	diagnostics := tab.state.Diagnostics()
	if diagnostics[0].Message != "假的错误" || diagnostics[0].Source != "fake" {
		t.Fatalf("diagnostics: %+v", diagnostics)
	}
	if session := a.lspCurrent(); session == nil || a.lspStatus() == "" {
		t.Fatalf("no server: %+v", session)
	}

	// Completions come to the editor's list, from what the server said.
	a.lspComplete(tab, editor.CompletionRequest{Offset: 4, Version: tab.state.Buffer.Version()})
	tt := ui.NewTester(a.view, 1280, 840)
	if !pumpTexts(t, queue, tt, "println") {
		t.Fatalf("the completions are not drawn: %v", tt.Texts())
	}

	// Formatting puts what the server said in the buffer, in one change.
	before := tab.state.Buffer.String()
	a.lspFormat()
	pumpUntil(t, queue, func() bool {
		return strings.HasPrefix(tab.state.Buffer.String(), "// 已格式化")
	})
	if tab.state.Buffer.String() == before {
		t.Fatal("the document was not formatted")
	}

	// A definition opens the file the server names, at the place.
	a.defineAt(tab, 4)
	pumpUntil(t, queue, func() bool { return a.tabOf(root+"/other.go") != nil })
	target := a.tabOf(root + "/other.go")
	if target == nil || target.state == nil {
		t.Fatal("the definition did not open")
	}
	if pos := target.state.CursorPos(); pos.Line != 2 || pos.Col != 4 {
		t.Fatalf("the caret is at %+v", pos)
	}
}
