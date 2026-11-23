package main

import (
	"context"
	"fmt"
	"path/filepath"
	"unicode"

	"github.com/egoist/mygo/ui"
	"github.com/egoist/mygo/ui/editor"
	"vael/internal/workspace"
)

// loadSymbols asks the core for the declarations of the project, so that
// they can be searched and jumped to.
func (a *app) loadSymbols() {
	if !a.requireProject() {
		return
	}
	if a.symbolsLoading {
		return
	}
	a.symbolsLoading = true
	ctx, cancel := context.WithCancel(a.projectCtx)
	a.symbolCancel = cancel
	generation, root := a.generation, a.root
	backend := a.backend
	go func() {
		index, err := backend.Index(ctx, root)
		var symbols []workspace.Symbol
		if err == nil {
			symbols, err = backend.Symbols(ctx, root, index.Files, 4000)
		}
		a.update(func() {
			if generation != a.generation {
				return
			}
			a.symbolsLoading = false
			if ctx.Err() != nil {
				return
			}
			a.report(err)
			if err == nil {
				a.symbols = symbols
				a.index = index
			}
		})
	}()
}

// outlineView shows the declarations of the file that is open. It follows
// the edits, as it reads the text the editor shows.
func (a *app) outlineView(c *ui.Context, p palette) {
	f := a.active()
	if f == nil || f.state == nil {
		ui.Text(c, "打开一个文件以查看它的结构。").FontSize(12).TextColor(p.muted)
		return
	}
	symbols := workspace.SymbolsInFile(f.path, f.state.Buffer.String())
	name := filepath.Base(f.path)
	ui.Text(c, "大纲 · "+name).FontSize(12).FontWeight(650).TextColor(p.text)
	if len(symbols) == 0 {
		ui.Text(c, "该文件没有可识别的声明。").FontSize(12).TextColor(p.muted)
		return
	}
	ui.Scroll(c).FillWidth().Grow(1).Children(func() {
		ui.Column(c).FillWidth().Gap(1).Children(func() {
			for _, symbol := range symbols {
				symbol := symbol
				row := ui.ButtonBase(c).FillWidth().Height(26).Padding(0, 6).Radius(4).Label(symbol.Name)
				if row.Hovered() {
					row.Background(p.edge)
				}
				row.Children(func() {
					ui.Text(c, symbol.Kind).FontSize(9.5).TextColor(p.accent).Width(38)
					ui.Text(c, symbol.Name).FontSize(12).TextColor(p.text).Grow(1).SingleLine().Ellipsis("…")
					ui.Text(c, fmt.Sprintf("%d", symbol.Line)).FontSize(10).TextColor(p.muted)
				})
				if row.Clicked() {
					a.jumpToSymbol(f, symbol)
				}
			}
		})
	})
}

// jumpToSymbol puts the caret on a declaration of the file that is open.
func (a *app) jumpToSymbol(f *fileTab, symbol workspace.Symbol) {
	if f.state == nil {
		return
	}
	f.state.Select(symbol.Offset, symbol.Offset)
}

// goToDefinition jumps to the declaration of the word under the caret, as
// the project symbols know it.
func (a *app) goToDefinition(f *fileTab, offset int) {
	if f == nil || f.state == nil {
		return
	}
	text := f.state.Buffer.String()
	word := wordAt(text, offset)
	if word == "" {
		return
	}
	if len(a.symbols) == 0 {
		a.loadSymbols()
	}
	for _, symbol := range a.symbols {
		if symbol.Name != word {
			continue
		}
		if symbol.Path == f.path {
			f.state.Select(symbol.Offset, symbol.Offset)
			return
		}
		a.openFileAt(a.joinPath(a.root, symbol.Path), symbol.Offset, symbol.Offset)
		return
	}
	a.message = "项目中没有找到 “" + word + "” 的声明。"
}

// wordAt is the identifier around an offset of a text.
func wordAt(text string, offset int) string {
	if offset < 0 || offset > len(text) {
		return ""
	}
	isWord := func(r rune) bool { return unicode.IsLetter(r) || unicode.IsDigit(r) || r == '_' }
	start := offset
	for start > 0 {
		r, size := decodeRune(text[start-1:])
		if !isWord(r) {
			break
		}
		start -= size
	}
	end := offset
	for end < len(text) {
		r, size := decodeRune(text[end:])
		if !isWord(r) {
			break
		}
		end += size
	}
	return text[start:end]
}

func decodeRune(text string) (rune, int) {
	for _, r := range text {
		return r, len(string(r))
	}
	return 0, 0
}

// symbolSearchView shows the declarations of the project that hold a query.
func (a *app) symbolSearchView(c *ui.Context, p palette) {
	ui.Text(c, "项目符号").FontSize(13).FontWeight(650).TextColor(p.text)
	input := ui.TextInput(c, &a.symbolQuery).FillWidth().Placeholder("搜索函数、类型或类…").Label("项目符号搜索")
	submitted := input.Submitted()
	ui.Row(c).FillWidth().Gap(4).Children(func() {
		if a.symbolsLoading {
			ui.Text(c, "正在读取符号…").FontSize(11).TextColor(p.muted)
		} else {
			ui.Text(c, fmt.Sprintf("%d 个符号", len(a.symbols))).FontSize(11).TextColor(p.muted)
		}
		ui.Spacer(c)
		if quiet(c, "刷新", false, p) || submitted {
			a.loadSymbols()
		}
	})
	matches := workspace.MatchSymbols(a.symbols, a.symbolQuery, 200)
	ui.Scroll(c).FillWidth().Grow(1).Children(func() {
		ui.Column(c).FillWidth().Gap(2).Children(func() {
			for _, symbol := range matches {
				symbol := symbol
				row := ui.ButtonBase(c).FillWidth().Height(28).Padding(0, 6).Radius(4).Label(symbol.Name)
				if row.Hovered() {
					row.Background(p.edge)
				}
				row.Children(func() {
					ui.Text(c, symbol.Kind).FontSize(9.5).TextColor(p.accent).Width(38)
					ui.Text(c, symbol.Name).FontSize(12).TextColor(p.text).Grow(1).SingleLine().Ellipsis("…")
					ui.Text(c, symbol.Path+":"+fmt.Sprintf("%d", symbol.Line)).FontSize(10).TextColor(p.muted).SingleLine().Ellipsis("…")
				})
				if row.Clicked() {
					a.openSymbol(symbol)
				}
			}
		})
	})
	if len(matches) == 0 && !a.symbolsLoading {
		ui.Text(c, "没有匹配的符号。").FontSize(12).TextColor(p.muted)
	}
}

// symbolsCommand is the palette's command that opens the symbol search.
func (a *app) symbolsCommand() {
	a.sidebar = true
	a.section = "symbols"
	if len(a.symbols) == 0 {
		a.loadSymbols()
	}
}

// workspaceSymbolsCommand opens the project's declarations in a window of
// their own, as Zed's and VS Code's Go to Symbol in Workspace does.
func (a *app) workspaceSymbolsCommand() {
	a.workspaceSymbolsOpen = true
	a.workspaceSymbolQuery = ""
	a.workspaceSymbolSelected = 0
	if len(a.symbols) == 0 && !a.symbolsLoading {
		a.loadSymbols()
	}
}

// workspaceSymbolPicker is the window of the project's declarations: type
// to filter them, Enter to go to the one chosen.
func (a *app) workspaceSymbolPicker(c *ui.Context, p palette) {
	if !a.workspaceSymbolsOpen {
		return
	}
	matches := workspace.MatchSymbols(a.symbols, a.workspaceSymbolQuery, 200)
	if a.workspaceSymbolQuery != a.workspaceSymbolShown {
		a.workspaceSymbolShown = a.workspaceSymbolQuery
		a.workspaceSymbolSelected = 0
	}
	if a.workspaceSymbolSelected >= len(matches) {
		a.workspaceSymbolSelected = max(0, len(matches)-1)
	}
	open := func(index int) {
		if index < 0 || index >= len(matches) {
			return
		}
		a.workspaceSymbolsOpen = false
		a.openSymbol(matches[index])
	}
	ui.Modal(c, &a.workspaceSymbolsOpen, func() {
		ui.Column(c).Width(680).Padding(16).Gap(10).Children(func() {
			ui.Text(c, "工作区符号").FontSize(15).FontWeight(650)
			input := ui.TextInput(c, &a.workspaceSymbolQuery).FillWidth().
				Placeholder("搜索函数、类型或类…").Label("工作区符号搜索").AutoFocus()
			ui.Row(c).FillWidth().AlignItems(ui.Center).Children(func() {
				if a.symbolsLoading {
					ui.Text(c, "正在读取符号…").FontSize(11).TextColor(p.muted).Grow(1)
				} else {
					ui.Textf(c, "%d 个符号", len(a.symbols)).FontSize(11).TextColor(p.muted).Grow(1)
				}
				if quiet(c, "刷新", false, p) {
					a.loadSymbols()
				}
			})
			state := ui.ListState{
				Selected: &a.workspaceSymbolSelected,
				Key:      func(i int) any { return matches[i].Path + "|" + matches[i].Name + "|" + fmt.Sprint(matches[i].Line) },
				Label:    func(i int) string { return matches[i].Name },
			}
			list := ui.List(c, &state, len(matches), func(i int) {
				symbol := matches[i]
				b := ui.ButtonBase(c).FillWidth().Height(30).Padding(0, 8).Gap(6).Radius(5).Label(symbol.Name)
				if b.Hovered() {
					b.Background(p.edge)
				}
				// The kind and the place are faint beside the name, but on
				// the chosen row's color they must still read.
				kindColor, pathColor := p.accent, p.muted
				if i == a.workspaceSymbolSelected {
					kindColor, pathColor = p.text, p.text
				}
				b.Children(func() {
					ui.Text(c, symbol.Kind).FontSize(9.5).TextColor(kindColor).Width(38)
					ui.Text(c, symbol.Name).FontSize(12).TextColor(p.text).Grow(1).SingleLine().Ellipsis("…")
					ui.Text(c, symbol.Path+":"+fmt.Sprint(symbol.Line)).FontSize(10).TextColor(pathColor).SingleLine().Ellipsis("…")
				})
				if b.Clicked() {
					open(i)
				}
			})
			if list.Submitted() {
				open(a.workspaceSymbolSelected)
			} else if input.Submitted() {
				open(a.workspaceSymbolSelected)
			}
			if len(matches) == 0 && !a.symbolsLoading {
				ui.Text(c, "没有匹配的符号。").FontSize(12).TextColor(p.muted)
			}
			ui.Text(c, "↑↓ 选择 · Enter 跳转 · Esc 关闭").FontSize(10).TextColor(p.muted)
		})
	})
}

// openSymbol opens the file of a declaration and puts the caret on it.
func (a *app) openSymbol(symbol workspace.Symbol) {
	full := a.joinPath(a.root, symbol.Path)
	if symbol.Line > 0 {
		a.pendingCaret[full] = editor.Pos{Line: symbol.Line - 1, Col: symbol.Column - 1}
	}
	a.openFile(full)
}
