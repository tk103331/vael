package main

import (
	"encoding/json"
	"sort"

	"github.com/egoist/mygo/ui"
	"github.com/egoist/mygo/ui/editor"
)

// Renaming a symbol: the language server of the file says where the name is
// written, and the editor changes every one of them, as Zed's F2 does.

// symbolRename is the name a symbol is given, and where it is.
type symbolRename struct {
	from, to string
	path     string
	offset   int
}

// renameSymbol renames what the caret is on, through the language server of
// the file. A file without one keeps the rename of the file itself.
func (a *app) renameSymbol() {
	tab := a.active()
	if tab == nil || tab.state == nil || tab.diff != nil {
		return
	}
	server := a.lspServerFor(tab.path)
	if server == nil {
		a.renameActiveFile()
		return
	}
	session := a.lsps[server.Language]
	if session == nil || session.wire == nil {
		a.message = "语言服务器还没就绪"
		return
	}
	offset := tab.state.Cursor()
	span := wordRangeAt(tab.state.Buffer.String(), offset)
	word := tab.state.Buffer.String()[span[0]:span[1]]
	if word == "" {
		a.message = "光标不在可以重命名的名字上"
		return
	}
	a.symbolPrompt = &symbolRename{from: word, to: word, path: tab.path, offset: offset}
}

// renameTo asks the server where a name is written, and changes every one
// of them.
func (a *app) renameTo(name string) {
	prompt := a.symbolPrompt
	a.symbolPrompt = nil
	tab := a.tabOf(prompt.path)
	if tab == nil || tab.state == nil {
		return
	}
	server := a.lspServerFor(tab.path)
	if server == nil {
		return
	}
	session := a.lsps[server.Language]
	if session == nil || session.wire == nil {
		return
	}
	text := tab.state.Buffer.String()
	at := positionIn(text, prompt.offset, session.encoding)
	session.wire.request("textDocument/rename", map[string]any{
		"textDocument": map[string]any{"uri": fileURI(tab.path)},
		"position":     at,
		"newName":      name,
	}, func(result json.RawMessage, err error) {
		if err != nil {
			a.message = "重命名失败：" + err.Error()
			return
		}
		edits := lspWorkspaceEdits(result)
		if len(edits) == 0 {
			a.message = "服务器没有给出要改的位置"
			return
		}
		count := 0
		for _, file := range edits {
			count += len(file.edits)
			a.renameInFile(file.path, file.edits, session.encoding)
		}
		a.message = "重命名 " + prompt.from + " → " + name + "：" + itoa(count) + " 处"
	})
}

// lspWorkspaceEdit is the files an edit names, and the edits of each.
type lspWorkspaceEdit struct {
	path  string
	edits []lspTextEdit
}

// lspTextEdit is a change of a range of a file.
type lspTextEdit struct {
	Range   lspRange `json:"range"`
	NewText string   `json:"newText"`
}

// lspWorkspaceEdits reads what a rename answered: the changes by URI, or the
// document changes of a server that names them so.
func lspWorkspaceEdits(result []byte) []lspWorkspaceEdit {
	var answer struct {
		Changes         map[string][]lspTextEdit `json:"changes"`
		DocumentChanges []struct {
			TextDocument struct {
				URI string `json:"uri"`
			} `json:"textDocument"`
			Edits []lspTextEdit `json:"edits"`
		} `json:"documentChanges"`
	}
	if json.Unmarshal(result, &answer) != nil {
		return nil
	}
	var edits []lspWorkspaceEdit
	for uri, list := range answer.Changes {
		if len(list) > 0 {
			edits = append(edits, lspWorkspaceEdit{path: filePath(uri), edits: list})
		}
	}
	for _, change := range answer.DocumentChanges {
		if len(change.Edits) > 0 {
			edits = append(edits, lspWorkspaceEdit{path: filePath(change.TextDocument.URI), edits: change.Edits})
		}
	}
	sort.Slice(edits, func(i, j int) bool { return edits[i].path < edits[j].path })
	return edits
}

// renameInFile changes a file: its buffer when it is open, and the file
// itself when it is not, as both are the same file.
func (a *app) renameInFile(path string, edits []lspTextEdit, encoding string) {
	if path == "" {
		return
	}
	if tab := a.tabOf(path); tab != nil && tab.state != nil {
		text := tab.state.Buffer.String()
		tab.state.Buffer.Edit(lspEdits(text, edits, encoding)...)
		a.saveTab(tab)
		return
	}
	generation, backend, ctx := a.generation, a.backend, a.projectCtx
	go func() {
		text, _, err := backend.ReadFile(ctx, path)
		if err != nil {
			return
		}
		edited := applyLSPEdits(text, edits, encoding)
		if edited == text {
			return
		}
		_, err = backend.Save(ctx, path, text, edited)
		a.update(func() {
			if generation != a.generation {
				return
			}
			a.report(err)
			if err == nil {
				a.refreshGit()
			}
		})
	}()
}

// lspEdits turns the edits of a file into the changes the editor takes,
// the last first, so that each offset is still the one the server meant.
func lspEdits(text string, edits []lspTextEdit, encoding string) []editor.Edit {
	changes := make([]editor.Edit, 0, len(edits))
	for _, edit := range edits {
		start := offsetIn(text, edit.Range.Start, encoding)
		end := offsetIn(text, edit.Range.End, encoding)
		if end < start {
			start, end = end, start
		}
		changes = append(changes, editor.Edit{Start: start, End: end, Text: edit.NewText})
	}
	sort.Slice(changes, func(i, j int) bool { return changes[i].Start > changes[j].Start })
	return changes
}

// applyLSPEdits is the text of a file after its edits.
func applyLSPEdits(text string, edits []lspTextEdit, encoding string) string {
	changes := lspEdits(text, edits, encoding)
	out := text
	for _, change := range changes {
		if change.Start < 0 || change.End > len(out) || change.Start > change.End {
			continue
		}
		out = out[:change.Start] + change.Text + out[change.End:]
	}
	return out
}

// symbolPromptView is the box a renamed symbol is typed in.
func (a *app) symbolPromptView(c *ui.Context, p palette) {
	if a.symbolPrompt == nil {
		return
	}
	open := true
	prompt := a.symbolPrompt
	ui.Modal(c, &open, func() {
		ui.Column(c).Width(420).Padding(22).Gap(14).Children(func() {
			ui.Text(c, "重命名符号").FontSize(17).FontWeight(650)
			ui.Text(c, prompt.from).FontSize(11).TextColor(p.muted)
			input := ui.TextInput(c, &prompt.to).FillWidth().Placeholder("新名字").Label("新名字").AutoFocus()
			submitted := input.Submitted()
			ui.Row(c).FillWidth().Gap(8).Children(func() {
				ui.Text(c, "语言服务器会改掉它写到的每一处。").FontSize(10).TextColor(p.muted).Grow(1)
				if ui.Button(c, "取消").Clicked() {
					open = false
				}
				name := prompt.to
				if ui.PrimaryButton(c, "重命名").Clicked() || submitted {
					a.renameTo(name)
				}
			})
		})
	})
	if !open {
		a.symbolPrompt = nil
	}
}
