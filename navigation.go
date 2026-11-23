package main

import (
	"context"
	"fmt"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/egoist/mygo/ui"
	"github.com/egoist/mygo/ui/editor"
	"vael/internal/workspace"
)

func (a *app) refreshIndex() {
	if a.root == "" || a.disconnected {
		return
	}
	if a.indexCancel != nil {
		a.indexCancel()
	}
	ctx, cancel := context.WithCancel(a.projectCtx)
	a.indexCancel = cancel
	a.indexVersion++
	version, generation, root := a.indexVersion, a.generation, a.root
	a.indexing = true
	backend := a.backend
	go func() {
		index, err := backend.Index(ctx, root)
		a.update(func() {
			if generation != a.generation || version != a.indexVersion {
				return
			}
			a.indexing = false
			if ctx.Err() != nil {
				return
			}
			a.report(err)
			if err == nil {
				a.index = index
			}
		})
	}()
}
func (a *app) beginSearch() {
	if !a.requireProject() {
		return
	}
	if a.searchCancel != nil {
		a.searchCancel()
	}
	a.searchVersion++
	version := a.searchVersion
	a.searchResults = workspace.SearchResults{}
	if strings.TrimSpace(a.searchQuery) == "" {
		a.searching = false
		return
	}
	ctx, cancel := context.WithCancel(a.projectCtx)
	a.searchCancel = cancel
	generation, root, query, matchCase := a.generation, a.root, a.searchQuery, a.searchCase
	a.searching = true
	backend := a.backend
	go func() {
		// Refresh the candidate list so newly created files and ignore rules apply.
		index, err := backend.Index(ctx, root)
		var results workspace.SearchResults
		if err == nil {
			results, err = backend.Search(ctx, root, index.Files, query, matchCase)
			results.Limited = results.Limited || index.Limited
		}
		a.update(func() {
			if generation != a.generation || version != a.searchVersion {
				return
			}
			a.searching = false
			if ctx.Err() != nil {
				return
			}
			a.report(err)
			if err == nil {
				a.index = index
				a.searchResults = results
			}
		})
	}()
}

// replaceAll replaces the query of the search panel in the files of the
// project, and says what changed.
func (a *app) replaceAll() {
	if !a.requireProject() {
		return
	}
	query := strings.TrimSpace(a.searchQuery)
	if query == "" {
		return
	}
	ctx, cancel := context.WithCancel(a.projectCtx)
	a.searchCancel = cancel
	generation, root, replacement, matchCase := a.generation, a.root, a.replaceWith, a.searchCase
	backend := a.backend
	go func() {
		index, err := backend.Index(ctx, root)
		var result workspace.ReplaceResult
		if err == nil {
			result, err = backend.ReplaceAll(ctx, root, index.Files, query, replacement, matchCase)
		}
		a.update(func() {
			if generation != a.generation {
				return
			}
			a.report(err)
			if err == nil {
				a.message = fmt.Sprintf("替换了 %d 处（%d 个文件）", result.Occurrences, result.Files)
				a.refreshGit()
				a.refreshIndex()
			}
		})
	}()
}

func (a *app) searchView(c *ui.Context, p palette) {
	ui.Text(c, "项目搜索").FontSize(13).FontWeight(650).TextColor(p.text)
	input := ui.TextInput(c, &a.searchQuery).FillWidth().Placeholder("搜索文本，按 Enter 开始").Label("项目搜索文本")
	submitted := input.Submitted()
	ui.TextInput(c, &a.replaceWith).FillWidth().Placeholder("替换为…").Label("项目替换文本")
	ui.Row(c).FillWidth().Gap(4).Children(func() {
		if strings.TrimSpace(a.searchQuery) != "" {
			if quiet(c, "全部替换", false, p) {
				a.confirm = &confirmation{"全部替换？", "把项目文件中的 “" + strings.TrimSpace(a.searchQuery) + "” 全部替换为 “" + a.replaceWith + "”。", func() { a.replaceAll() }}
			}
		}
	})
	ui.Row(c).FillWidth().Gap(4).Children(func() {
		if quiet(c, "Aa", a.searchCase, p) {
			a.searchCase = !a.searchCase
		}
		ui.Spacer(c)
		if a.searching {
			if quiet(c, "停止", false, p) {
				if a.searchCancel != nil {
					a.searchCancel()
				}
				a.searchVersion++
				a.searching = false
			}
		}
		if quiet(c, "搜索", false, p) || submitted {
			a.beginSearch()
		}
	})
	status := fmt.Sprintf("%d 处匹配", len(a.searchResults.Matches))
	if a.searching {
		status = "正在搜索…"
	}
	ui.Text(c, status).FontSize(11).TextColor(p.muted)
	if a.searchResults.Limited {
		ui.Text(c, "结果已截断：最多 500 处匹配、64 MiB 文本。").FontSize(11).TextColor(p.muted)
	}
	if a.searchResults.Skipped > 0 {
		ui.Textf(c, "跳过 %d 个非文本、过大或不可读文件", a.searchResults.Skipped).FontSize(10).TextColor(p.muted)
	}
	ui.Scroll(c).FillWidth().Grow(1).Children(func() {
		ui.Column(c).FillWidth().Gap(3).Children(func() {
			previous := ""
			for _, match := range a.searchResults.Matches {
				match := match
				if previous != match.Path {
					ui.Text(c, match.Path).FontSize(11).FontWeight(650).TextColor(p.text).Padding(6, 2).SingleLine().Ellipsis("…")
					previous = match.Path
				}
				b := ui.ButtonBase(c).FillWidth().MinHeight(28).Padding(5).Radius(4).Label(fmt.Sprintf("%s:%d", match.Path, match.Line))
				if b.Hovered() {
					b.Background(p.edge)
				}
				b.Children(func() {
					ui.Text(c, fmt.Sprintf("%d  %s", match.Line, match.Text)).FontSize(11).TextColor(p.muted).SingleLine().Ellipsis("…")
				})
				if b.Clicked() {
					a.openSearchResult(match)
				}
			}
		})
	})
	ui.Text(c, "搜索磁盘文件 · 遵循 Git 忽略规则").FontSize(10).TextColor(p.muted)
}
func (a *app) openSearchResult(match workspace.SearchMatch) {
	path := a.joinPath(a.root, match.Path)
	// Search offsets are from disk; refuse to select a stale range in dirty buffers.
	for _, f := range a.tabs {
		if f.path == path && f.state != nil && f.state.Buffer.Modified() {
			a.openFile(path)
			a.message = "此文件有未保存的修改，搜索位置基于磁盘版本，请在编辑器中查找对应文本。"
			return
		}
	}
	a.openFileAt(path, match.Start, match.End)
}
func (a *app) quickOpenView(c *ui.Context, p palette) {
	if !a.quickOpen {
		return
	}
	parts := parseQuickQuery(a.quickQuery)
	if parts.symbolOnly {
		a.symbolQuickOpen(c, p, parts)
		return
	}
	matches := workspace.MatchFiles(a.index.Files, parts.file, 80)
	if a.quickQuery != a.quickShown {
		// New results, new choice: the first match is what Enter opens.
		a.quickShown = a.quickQuery
		a.quickSelected = 0
	}
	if a.quickSelected >= len(matches) {
		a.quickSelected = max(0, len(matches)-1)
	}
	ui.Modal(c, &a.quickOpen, func() {
		ui.Column(c).Width(620).Padding(16).Gap(10).Children(func() {
			ui.Text(c, "快速打开文件").FontSize(15).FontWeight(650)
			input := ui.TextInput(c, &a.quickQuery).FillWidth().
				Placeholder("文件名或路径，可加 :行号，或 @符号…").Label("快速打开文件名").AutoFocus()
			if a.indexing {
				ui.Text(c, "正在索引项目…").FontSize(11).TextColor(p.muted)
			}
			state := ui.ListState{
				Selected: &a.quickSelected,
				Key:      func(i int) any { return matches[i] },
				Label:    func(i int) string { return matches[i] },
			}
			list := ui.List(c, &state, len(matches), func(i int) {
				path := matches[i]
				b := ui.ButtonBase(c).FillWidth().Height(32).Padding(0, 8).Radius(5).Label(path).AlignItems(ui.Start)
				b.Children(func() {
					ui.Box(c).Width(24).Children(func() {
						a.markView(c, p, a.markFor(path))
					})
					ui.Text(c, path).FontSize(12).TextColor(p.text).Grow(1).SingleLine().Ellipsis("…")
				})
			})
			if list.Submitted() && a.quickSelected >= 0 && a.quickSelected < len(matches) {
				a.quickOpen = false
				a.quickOpenFile(matches[a.quickSelected], parts.line, parts.symbol)
			} else if input.Submitted() && len(matches) > 0 {
				a.quickOpen = false
				a.quickOpenFile(matches[max(0, a.quickSelected)], parts.line, parts.symbol)
			}
			if len(matches) == 0 && !a.indexing {
				ui.Text(c, "没有匹配的文件").FontSize(12).TextColor(p.muted)
			}
			count := fmt.Sprintf("%d 个项目文件 · ↑↓ 选择 · Enter 打开 · Esc 关闭", len(a.index.Files))
			if a.index.Limited {
				count = "索引已达到 100000 个文件上限 · " + count
			}
			ui.Text(c, count).FontSize(10).TextColor(p.muted)
		})
	})
}

// recentMatches are the files opened lately that belong to this project,
// filtered by the dialog's query, newest first.
func (a *app) recentMatches() []string {
	query := strings.ToLower(a.recentQuery)
	out := make([]string, 0, len(a.recentFiles))
	for _, path := range a.recentFiles {
		rel, err := a.relativePath(a.root, path)
		if err != nil {
			continue
		}
		if query != "" && !strings.Contains(strings.ToLower(rel), query) {
			continue
		}
		out = append(out, path)
	}
	return out
}

// recentView is the dialog of the files opened lately, as an IDE's Recent
// Files: type to filter, Enter to open.
func (a *app) recentView(c *ui.Context, p palette) {
	if !a.recentOpen {
		return
	}
	matches := a.recentMatches()
	if a.recentQuery != a.recentShown {
		a.recentShown = a.recentQuery
		a.recentSelected = 0
	}
	if a.recentSelected >= len(matches) {
		a.recentSelected = max(0, len(matches)-1)
	}
	ui.Modal(c, &a.recentOpen, func() {
		ui.Column(c).Width(620).Padding(16).Gap(10).Children(func() {
			ui.Text(c, "最近的文件").FontSize(15).FontWeight(650)
			input := ui.TextInput(c, &a.recentQuery).FillWidth().Placeholder("输入文件名或路径…").Label("最近文件筛选").AutoFocus()
			state := ui.ListState{
				Selected: &a.recentSelected,
				Key:      func(i int) any { return matches[i] },
				Label:    func(i int) string { return matches[i] },
			}
			list := ui.List(c, &state, len(matches), func(i int) {
				path := matches[i]
				rel, _ := a.relativePath(a.root, path)
				b := ui.ButtonBase(c).FillWidth().Height(34).Padding(0, 8).Radius(5).Label(rel).AlignItems(ui.Start)
				b.Children(func() {
					ui.Box(c).Width(24).Children(func() {
						a.markView(c, p, a.markFor(path))
					})
					ui.Column(c).Grow(1).Gap(1).Children(func() {
						ui.Text(c, filepath.Base(path)).FontSize(12).TextColor(p.text).SingleLine().Ellipsis("…")
						if dir := filepath.Dir(rel); dir != "" && dir != "." {
							ui.Text(c, dir).FontSize(10).TextColor(p.muted).SingleLine().Ellipsis("…")
						}
					})
				})
			})
			if list.Submitted() && a.recentSelected >= 0 && a.recentSelected < len(matches) {
				a.recentOpen = false
				a.openFile(matches[a.recentSelected])
			} else if input.Submitted() && len(matches) > 0 {
				a.recentOpen = false
				a.openFile(matches[max(0, a.recentSelected)])
			}
			if len(matches) == 0 {
				ui.Text(c, "还没有最近打开的文件").FontSize(12).TextColor(p.muted)
			}
			ui.Text(c, "↑↓ 选择 · Enter 打开 · Esc 关闭").FontSize(10).TextColor(p.muted)
		})
	})
}

func (a *app) snapshot() workspace.Session {
	s := workspace.Session{Root: a.root, SideWidth: a.sideWidth, EditorShare: a.editorShare, Sidebar: a.sidebar, Terminal: a.showBottom, RightPanel: a.rightPanel, Dark: a.dark, Wrap: a.wrap, Section: a.section, FontSize: a.fontSize,
		TabSize: a.tabSize, Indent: a.indent, Whitespace: a.whitespace, LineHeight: a.lineHeight,
		NoGuides: !a.guides, NoAutoClose: !a.autoClose, NoNumbers: !a.numbers,
		Theme: a.themeID, Icons: a.iconID, FontFamily: a.fontFamily,
		AutoSave: a.autoSave, TrimOnSave: a.trimOnSave, FinalNewline: a.finalNewline,
		FormatOnSave: a.formatOnSave, Minimap: &a.minimap}
	s.Recent = append([]string(nil), a.recentFiles...)
	if a.connection != nil {
		s.ConnectionKind = a.connection.Kind
		s.Target = a.connection.Target
		s.Binary = a.connection.Binary
		s.Port = a.connection.Port
	}
	for _, f := range a.tabs {
		if f.state != nil {
			s.Files = append(s.Files, workspace.OpenFile{Path: f.path, Cursor: f.state.Cursor()})
		}
	}
	if f := a.active(); f != nil && f.state != nil {
		s.Active = f.path
	}
	return s
}
func (a *app) applyLayout(s workspace.Session) {
	a.dark, a.wrap, a.sidebar, a.showBottom, a.rightPanel = s.Dark, s.Wrap, s.Sidebar, s.Terminal, s.RightPanel
	// A session of before themes has only whether it was dark.
	a.themeID = s.Theme
	if s.Theme == "" {
		a.themeID = themeOfDark("vael-dark", s.Dark)
	}
	a.setTheme(a.themeID)
	if s.Icons != "" {
		a.iconID = s.Icons
	}
	if s.FontFamily != "" {
		a.fontFamily = s.FontFamily
	}
	if s.Minimap != nil {
		a.minimap = *s.Minimap
	}
	if s.AutoSave >= autoSaveOff && s.AutoSave <= autoSaveAfterDelay {
		a.autoSave = s.AutoSave
	}
	a.trimOnSave, a.finalNewline, a.formatOnSave = s.TrimOnSave, s.FinalNewline, s.FormatOnSave
	a.recentFiles = append([]string(nil), s.Recent...)
	if s.SideWidth >= 160 && s.SideWidth <= 640 {
		a.sideWidth = s.SideWidth
	}
	if s.EditorShare >= 20 && s.EditorShare <= 90 {
		a.editorShare = s.EditorShare
	}
	if s.FontSize >= 9 && s.FontSize <= 24 {
		a.fontSize = s.FontSize
	}
	if s.TabSize >= 0 && s.TabSize <= 8 {
		a.tabSize = s.TabSize
	}
	if s.Indent >= 0 && s.Indent <= 2 {
		a.indent = s.Indent
	}
	if s.Whitespace >= 0 && s.Whitespace <= 4 {
		a.whitespace = s.Whitespace
	}
	if s.LineHeight >= 80 && s.LineHeight <= 220 {
		a.lineHeight = s.LineHeight
	}
	a.guides, a.autoClose, a.numbers = !s.NoGuides, !s.NoAutoClose, !s.NoNumbers
	if panelAt(s.Section, panelLeft) != nil {
		a.section = s.Section
	}
}
func (a *app) restoreFiles(s workspace.Session) {
	if a.connection == nil && s.ConnectionKind != "" {
		return
	}
	if a.connection != nil && (s.ConnectionKind != a.connection.Kind || s.Target != a.connection.Target) {
		return
	}
	if filepath.Clean(s.Root) != filepath.Clean(a.root) {
		return
	}
	generation := a.generation
	backend := a.backend
	remote := a.connection != nil
	ctx := a.projectCtx
	go func() {
		var tabs []*fileTab
		selected := 0
		seen := map[string]bool{}
		for _, file := range s.Files {
			if seen[file.Path] || (!remote && !filepath.IsAbs(file.Path)) || (remote && !strings.HasPrefix(file.Path, "/")) {
				continue
			}
			seen[file.Path] = true
			if ctx.Err() != nil {
				return
			}
			text, err := backend.Read(ctx, file.Path)
			if err != nil {
				continue
			}
			b := editor.NewBuffer(text)
			b.SetLanguage(editor.LanguageFor(file.Path))
			state := &editor.State{Buffer: b, KeepScrollbars: true}
			cursor := max(0, min(file.Cursor, len(text)))
			state.Select(cursor, cursor)
			if file.Path == s.Active {
				selected = len(tabs)
			}
			tab := &fileTab{path: file.Path, original: text, state: state}
			tab.state.OnDefinition = func(offset int) { a.defineAt(tab, offset) }
			tabs = append(tabs, tab)
		}
		a.update(func() {
			if generation != a.generation {
				return
			}
			// Files opened during startup remain selected and do not get duplicated.
			if len(a.tabs) > 0 {
				for _, tab := range tabs {
					found := false
					for _, existing := range a.tabs {
						if existing.path == tab.path {
							found = true
							break
						}
					}
					if !found {
						a.tabs = append(a.tabs, tab)
					}
				}
				return
			}
			a.tabs = tabs
			a.selected = -1
			if len(tabs) > 0 {
				a.selected = selected
			}
			for _, tab := range a.tabs {
				a.refreshDiffBase(tab)
			}
		})
	}()
}

// quickTarget splits what is typed in the quick open into the file it finds
// and the line to go to, as "main.go:42" says, as Sublime's Goto Anything
// opens a file at a line.
func quickTarget(query string) (string, int) {
	at := strings.LastIndex(query, ":")
	if at < 0 {
		return query, 0
	}
	line, err := strconv.Atoi(strings.TrimSpace(query[at+1:]))
	if err != nil || line <= 0 {
		return query, 0
	}
	return strings.TrimSpace(query[:at]), line
}

// quickOpenFile opens what the quick open found, at the line it named, and
// at the declaration it named, if any.
func (a *app) quickOpenFile(path string, line int, symbol string) {
	full := a.joinPath(a.root, path)
	if line > 0 {
		a.pendingCaret[full] = editor.Pos{Line: line - 1}
	}
	if symbol != "" {
		a.pendingSymbol = full + "|" + symbol
	}
	a.openFile(full)
}

// quickQueryParts is what the quick open reads of what is typed: the file
// to find, the line and the declaration to go to in it, as Sublime Text's
// Goto Anything takes "file:42" and "file@name".
type quickQueryParts struct {
	file       string
	line       int
	symbol     string
	symbolOnly bool
}

// parseQuickQuery reads what the quick open is given.
func parseQuickQuery(query string) quickQueryParts {
	parts := quickQueryParts{file: query}
	if at := strings.Index(query, "@"); at >= 0 {
		parts.symbol = strings.TrimSpace(query[at+1:])
		parts.file = strings.TrimSpace(query[:at])
		parts.symbolOnly = parts.file == ""
	}
	if !parts.symbolOnly {
		parts.file, parts.line = quickTarget(parts.file)
	}
	return parts
}

// symbolQuickOpen lists the declarations of the file that is open, for
// typing "@name" in the quick open.
func (a *app) symbolQuickOpen(c *ui.Context, p palette, parts quickQueryParts) {
	var symbols []workspace.Symbol
	f := a.active()
	if f != nil && f.state != nil {
		symbols = workspace.SymbolsInFile(f.path, f.state.Buffer.String())
	}
	matches := workspace.MatchSymbols(symbols, parts.symbol, 80)
	if a.quickQuery != a.quickShown {
		a.quickShown = a.quickQuery
		a.quickSelected = 0
	}
	if a.quickSelected >= len(matches) {
		a.quickSelected = max(0, len(matches)-1)
	}
	ui.Modal(c, &a.quickOpen, func() {
		ui.Column(c).Width(560).Padding(16).Gap(10).Children(func() {
			ui.Text(c, "转到符号").FontSize(15).FontWeight(650)
			input := ui.TextInput(c, &a.quickQuery).FillWidth().
				Placeholder("输入符号名…（@ 后）").Label("转到文件中的符号").AutoFocus()
			ui.Row(c).FillWidth().AlignItems(ui.Center).Children(func() {
				name := "没有打开的文件"
				if f != nil {
					name = filepath.Base(f.path)
				}
				ui.Text(c, "文件："+name).FontSize(11).TextColor(p.muted).Grow(1).SingleLine().Ellipsis("…")
			})
			state := ui.ListState{
				Selected: &a.quickSelected,
				Key:      func(i int) any { return matches[i].Path + "|" + matches[i].Name },
				Label:    func(i int) string { return matches[i].Name },
			}
			list := ui.List(c, &state, len(matches), func(i int) {
				symbol := matches[i]
				b := ui.ButtonBase(c).FillWidth().Height(30).Padding(0, 8).Gap(6).Radius(5).Label(symbol.Name)
				if b.Hovered() {
					b.Background(p.edge)
				}
				kindColor, lineColor := p.accent, p.muted
				if i == a.quickSelected {
					kindColor, lineColor = p.text, p.text
				}
				b.Children(func() {
					ui.Text(c, symbol.Kind).FontSize(9.5).TextColor(kindColor).Width(38)
					ui.Text(c, symbol.Name).FontSize(12).TextColor(p.text).Grow(1).SingleLine()
					ui.Text(c, "第 "+itoa(symbol.Line)+" 行").FontSize(10).TextColor(lineColor)
				})
				if b.Clicked() {
					a.quickOpen = false
					a.symbolJump(f, symbol)
				}
			})
			if list.Submitted() && a.quickSelected >= 0 && a.quickSelected < len(matches) {
				a.quickOpen = false
				a.symbolJump(f, matches[a.quickSelected])
			} else if input.Submitted() && len(matches) > 0 {
				a.quickOpen = false
				a.symbolJump(f, matches[max(0, a.quickSelected)])
			}
			if len(matches) == 0 {
				ui.Text(c, "没有匹配的符号。").FontSize(12).TextColor(p.muted)
			}
			ui.Text(c, "↑↓ 选择 · Enter 跳转 · Esc 关闭").FontSize(10).TextColor(p.muted)
		})
	})
}

// symbolJump puts the caret on a declaration of the file that is open.
func (a *app) symbolJump(tab *fileTab, symbol workspace.Symbol) {
	if tab == nil || tab.state == nil {
		return
	}
	tab.state.GoTo(editor.Pos{Line: symbol.Line - 1, Col: symbol.Column - 1})
}
