package main

import (
	"encoding/json"

	"github.com/egoist/mygo/ui"
	"github.com/egoist/mygo/ui/editor"
)

// What a language server offers of code: the fixes it can apply where the
// caret is, and where a name is used, as an IDE's quick fix and Find Usages
// do.

// codeAction is a fix or a refactoring a server offers.
type codeAction struct {
	title   string
	kind    string
	edit    []lspWorkspaceEdit
	command *lspCommand
}

// lspCommand is a command of the server that does what an action means.
type lspCommand struct {
	Command   string          `json:"command"`
	Arguments json.RawMessage `json:"arguments,omitempty"`
}

// actionList is the actions a server offered, which the editor lists.
type actionList struct {
	items    []codeAction
	selected int
}

// referenceList is the places a name is used, which the editor lists.
type referenceList struct {
	items    []lspPlace
	selected int
}

// lspRangeOf is the range of the caret's line, or of the selection, which a
// request about code is about.
func lspRangeOf(state *editor.State, encoding string) lspRange {
	selection := state.Selection()
	if selection.Anchor != selection.Head {
		text := state.Buffer.String()
		start, end := selection.Anchor, selection.Head
		if end < start {
			start, end = end, start
		}
		return lspRange{
			Start: positionIn(text, start, encoding),
			End:   positionIn(text, end, encoding),
		}
	}
	text := state.Buffer.String()
	offset := state.Cursor()
	line := positionIn(text, offset, encoding).Line
	return lspRange{
		Start: lspPosition{Line: line, Character: 0},
		End:   lspPosition{Line: line + 1, Character: 0},
	}
}

// diagnosticsIn is what the file has in a range, as a server asked for the
// problems of a request wants them.
func diagnosticsIn(state *editor.State, at lspRange, encoding string) []any {
	text := state.Buffer.String()
	from := offsetIn(text, at.Start, encoding)
	to := offsetIn(text, at.End, encoding)
	var found []any
	for _, diagnostic := range state.Diagnostics() {
		if diagnostic.End < from || diagnostic.Start > to {
			continue
		}
		found = append(found, map[string]any{
			"range": map[string]any{
				"start": positionIn(text, diagnostic.Start, encoding),
				"end":   positionIn(text, diagnostic.End, encoding),
			},
			"severity": severityOf(diagnostic.Severity),
			"message":  diagnostic.Message,
			"source":   diagnostic.Source,
		})
	}
	return found
}

// severityOf is how bad a problem is, as LSP numbers it.
func severityOf(severity editor.Severity) int {
	switch severity {
	case editor.SeverityError:
		return 1
	case editor.SeverityWarning:
		return 2
	case editor.SeverityInfo:
		return 3
	}
	return 4
}

// quickFixes asks the server of the file what it can do where the caret is,
// and lists it.
func (a *app) quickFixes() {
	tab := a.active()
	if tab == nil || tab.state == nil || tab.diff != nil {
		return
	}
	server := a.lspServerFor(tab.path)
	if server == nil {
		a.message = "这个文件没有语言服务器"
		return
	}
	session := a.lsps[server.Language]
	if session == nil || session.wire == nil {
		a.message = "语言服务器还没就绪"
		return
	}
	at := lspRangeOf(tab.state, session.encoding)
	state, path := tab.state, tab.path
	session.wire.request("textDocument/codeAction", map[string]any{
		"textDocument": map[string]any{"uri": fileURI(path)},
		"range":        at,
		"context": map[string]any{
			"diagnostics": diagnosticsIn(state, at, session.encoding),
			"only":        nil,
		},
	}, func(result json.RawMessage, err error) {
		if err != nil {
			a.message = "没有可用的修复：" + err.Error()
			return
		}
		actions := lspCodeActions(result)
		if len(actions) == 0 {
			a.message = "这里没有可用的修复"
			return
		}
		a.actions = &actionList{items: actions}
	})
}

// lspCodeActions reads what a server offered with a code action request.
func lspCodeActions(result json.RawMessage) []codeAction {
	var items []struct {
		Title   string          `json:"title"`
		Kind    string          `json:"kind"`
		Edit    json.RawMessage `json:"edit"`
		Command *lspCommand     `json:"command"`
	}
	if json.Unmarshal(result, &items) != nil {
		return nil
	}
	var actions []codeAction
	for _, item := range items {
		action := codeAction{title: item.Title, kind: item.Kind, command: item.Command}
		if len(item.Edit) > 0 {
			action.edit = lspWorkspaceEdits(item.Edit)
		}
		if action.edit == nil && action.command == nil {
			continue
		}
		actions = append(actions, action)
	}
	return actions
}

// applyAction does what an action means: the edits it holds, or the command
// of the server.
func (a *app) applyAction(action codeAction) {
	for _, file := range action.edit {
		a.renameInFile(file.path, file.edits, a.lspEncodingOf(file.path))
	}
	if action.command != nil {
		tab := a.active()
		if tab == nil {
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
		session.wire.request("workspace/executeCommand", map[string]any{
			"command":   action.command.Command,
			"arguments": action.command.Arguments,
		}, func(json.RawMessage, error) {})
	}
	if action.edit == nil && action.command == nil {
		a.message = "这条修复没有可执行的内容"
	}
}

// lspEncodingOf is the encoding the server of a file chose, UTF-16 when no
// server of it runs.
func (a *app) lspEncodingOf(path string) string {
	server := a.lspServerFor(path)
	if server == nil {
		return "utf-16"
	}
	if session := a.lsps[server.Language]; session != nil {
		session.mu.Lock()
		defer session.mu.Unlock()
		if session.encoding != "" {
			return session.encoding
		}
	}
	return "utf-16"
}

// actionView lists the fixes of the server for what is chosen.
func (a *app) actionView(c *ui.Context, p palette) {
	if a.actions == nil {
		return
	}
	open := true
	list := a.actions
	choose := func(index int) {
		if index < 0 || index >= len(list.items) {
			return
		}
		action := list.items[index]
		open = false
		a.actions = nil
		a.applyAction(action)
	}
	ui.Modal(c, &open, func() {
		ui.Column(c).Width(520).Padding(16).Gap(10).Children(func() {
			ui.Text(c, "代码操作").FontSize(15).FontWeight(650)
			state := ui.ListState{
				Selected: &list.selected,
				Key:      func(i int) any { return list.items[i].title },
				Label:    func(i int) string { return list.items[i].title },
			}
			rows := ui.List(c, &state, len(list.items), func(i int) {
				item := list.items[i]
				chosen := i == list.selected
				b := ui.ButtonBase(c).FillWidth().Height(30).Padding(0, 8).Gap(6).
					Radius(5).AlignItems(ui.Center).Label(item.title)
				if b.Hovered() {
					b.Background(p.edge)
				}
				// The kind of a fix is faint beside its title, but it must
				// still read on the chosen row's color.
				kindColor := p.muted
				if chosen {
					kindColor = p.text
				}
				b.Children(func() {
					ui.Text(c, item.title).FontSize(12).TextColor(p.text).Grow(1).
						SingleLine().Ellipsis("…")
					if item.kind != "" {
						ui.Text(c, item.kind).FontSize(9.5).TextColor(kindColor).SingleLine()
					}
				})
				if b.Clicked() {
					choose(i)
				}
			})
			if rows.Submitted() {
				choose(list.selected)
			}
			ui.Text(c, "↑↓ 选择 · Enter 执行 · Esc 关闭").FontSize(10).TextColor(p.muted)
		})
	})
	if !open {
		a.actions = nil
	}
}

// findReferences asks the server of the file where a name is used, and
// lists the places.
func (a *app) findReferences() {
	tab := a.active()
	if tab == nil || tab.state == nil || tab.diff != nil {
		return
	}
	server := a.lspServerFor(tab.path)
	if server == nil {
		a.message = "这个文件没有语言服务器"
		return
	}
	session := a.lsps[server.Language]
	if session == nil || session.wire == nil {
		a.message = "语言服务器还没就绪"
		return
	}
	text := tab.state.Buffer.String()
	at := positionIn(text, tab.state.Cursor(), session.encoding)
	session.wire.request("textDocument/references", map[string]any{
		"textDocument": map[string]any{"uri": fileURI(tab.path)},
		"position":     at,
		"context":      map[string]any{"includeDeclaration": true},
	}, func(result json.RawMessage, err error) {
		if err != nil {
			a.message = "查找引用失败：" + err.Error()
			return
		}
		places := lspPlaces(result)
		if len(places) == 0 {
			a.message = "没有找到引用"
			return
		}
		a.refs = &referenceList{items: places}
	})
}

// lspPlaces reads the places a server listed: for references, and for what
// else it names so.
func lspPlaces(result json.RawMessage) []lspPlace {
	var items []struct {
		URI   string   `json:"uri"`
		Range lspRange `json:"range"`
	}
	if json.Unmarshal(result, &items) != nil {
		return nil
	}
	var places []lspPlace
	for _, item := range items {
		if path := filePath(item.URI); path != "" {
			places = append(places, lspPlace{
				path:      path,
				line:      item.Range.Start.Line,
				character: item.Range.Start.Character,
			})
		}
	}
	return places
}

// referenceView lists where a name is used, and goes to the one chosen.
func (a *app) referenceView(c *ui.Context, p palette) {
	if a.refs == nil {
		return
	}
	open := true
	list := a.refs
	ui.Modal(c, &open, func() {
		ui.Column(c).Width(560).Padding(16).Gap(10).Children(func() {
			ui.Row(c).FillWidth().AlignItems(ui.Center).Children(func() {
				ui.Text(c, "引用").FontSize(15).FontWeight(650).Grow(1)
				ui.Textf(c, "%d 处", len(list.items)).FontSize(11).TextColor(p.muted)
			})
			ui.Scroll(c).FillWidth().MaxHeight(360).Children(func() {
				ui.Column(c).FillWidth().Gap(1).Children(func() {
					for i, place := range list.items {
						place := place
						name := place.path
						if rel, err := a.relativePath(a.root, place.path); err == nil {
							name = rel
						}
						label := name + ":" + itoa(place.line+1)
						b := ui.ButtonBase(c).FillWidth().Height(28).Padding(0, 8).Gap(6).
							Radius(4).AlignItems(ui.Center).Label(label)
						if i == list.selected {
							b.Background(p.selected)
						} else if b.Hovered() {
							b.Background(p.edge)
						}
						b.Children(func() {
							ui.Text(c, label).FontSize(11.5).TextColor(p.text).Grow(1).
								SingleLine().Ellipsis("…")
							ui.Text(c, "列 "+itoa(place.character+1)).FontSize(9.5).
								TextColor(p.muted).SingleLine()
						})
						if b.Clicked() {
							open = false
							a.refs = nil
							a.openAtPlace(place)
						}
					}
				})
			})
			ui.Text(c, "点一行即跳到那里 · Esc 关闭").FontSize(10).TextColor(p.muted)
		})
	})
	if !open {
		a.refs = nil
	}
}

// openAtPlace opens a file at a place of it.
func (a *app) openAtPlace(place lspPlace) {
	a.pendingCaret[place.path] = editor.Pos{Line: place.line, Col: place.character}
	a.openFile(place.path)
}
