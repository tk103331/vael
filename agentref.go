package main

import (
	"fmt"
	"path"
	"regexp"
	"sort"
	"strings"
	"unicode/utf8"

	"github.com/egoist/mygo/ui"
)

// A reference is something the user pointed at, to travel with the next
// message: a file or a folder of the project, or text selected in an
// editor. The panel shows them as chips; the prompt carries them as
// content blocks that an ACP agent understands.
type agentRef struct {
	Kind, Path, Rel, Text, Line string
}

func (r agentRef) label() string {
	if r.Kind == "selection" {
		if r.Line != "" {
			return r.Rel + ":" + r.Line
		}
		return r.Rel + " · 选中"
	}
	if r.Kind == "dir" {
		return r.Rel + "/"
	}
	return r.Rel
}

func (a *app) addAgentRef(ref agentRef) {
	if ref.Rel == "" {
		ref.Rel = shortPath(ref.Path)
	}
	if ref.Kind == "" {
		ref.Kind = "file"
	}
	for _, item := range a.agentRefs {
		if item.Kind == ref.Kind && item.Path == ref.Path && item.Line == ref.Line && item.Text == ref.Text {
			return
		}
	}
	a.agentRefs = append(a.agentRefs, ref)
	a.rightPanel, a.rightSection = true, "agent"
	a.message, a.messageKind = "已添加到对话", "success"
}

// addSelectionToAgent takes the text selected in an editor into the
// conversation, with the file and the line it starts at.
func (a *app) addSelectionToAgent(tab *fileTab) {
	if tab == nil || tab.state == nil {
		return
	}
	selected := tab.state.SelectedText()
	if strings.TrimSpace(selected) == "" {
		a.addAgentRef(agentRef{Kind: "file", Path: tab.path, Rel: a.relOf(tab.path)})
		return
	}
	offset := tab.state.Selection().Start()
	line := tab.state.Buffer.Pos(offset).Line + 1
	a.addAgentRef(agentRef{Kind: "selection", Path: tab.path, Rel: a.relOf(tab.path),
		Text: selected, Line: fmt.Sprint(line)})
}

func (a *app) relOf(full string) string {
	if rel, err := a.relativePath(a.root, full); err == nil {
		return rel
	}
	return shortPath(full)
}

// agentToken is the command or the @ mention being typed before the caret.
func agentToken(text string, caret int) (kind byte, query string, start, end int) {
	runes := []rune(text)
	if caret < 0 {
		caret = 0
	}
	if caret > len(runes) {
		caret = len(runes)
	}
	i := caret - 1
	for i >= 0 && runes[i] != ' ' && runes[i] != '\n' && runes[i] != '\t' {
		i--
	}
	start, end = i+1, caret
	word := string(runes[start:end])
	if strings.HasPrefix(word, "@") {
		return '@', word[1:], start, end
	}
	if strings.HasPrefix(word, "/") && !strings.Contains(word, " ") {
		return '/', word[1:], start, end
	}
	return 0, "", 0, 0
}

// updateAgentToken follows the word at the caret, opening the candidates
// for @ and / and closing them once the word is gone or dismissed.
func (a *app) updateAgentToken() {
	kind, query, start, end := agentToken(a.agentInput, a.agentCaret)
	a.agentMentionKind, a.agentMentionQuery = kind, query
	a.agentTokenStart, a.agentTokenEnd = start, end
	a.agentMentionDir = false
	if kind == 0 {
		a.agentMentionOpen, a.agentMentionDismissed = false, ""
		return
	}
	a.agentMentionOpen = a.agentMentionDismissed != string(kind)+query
}

// insertAgentText replaces the word being typed and leaves the caret
// after what was inserted.
func (a *app) insertAgentText(start, end int, text string) {
	runes := []rune(a.agentInput)
	if start < 0 || start > len(runes) {
		start = len(runes)
	}
	if end < start || end > len(runes) {
		end = len(runes)
	}
	a.agentInput = string(runes[:start]) + text + string(runes[end:])
	a.agentCaretPending = start + utf8.RuneCountInString(text)
	a.agentMentionDismissed = string(a.agentMentionKind) + a.agentMentionQuery
	a.agentMentionOpen = false
}

type agentCandidate struct {
	label, insert string
	ref           *agentRef
	command       bool
}

// agentCandidates ranks the files, folders and slash commands that fit
// what is typed. A folder gets a trailing slash so that it stays a
// folder when it is inserted.
func (a *app) agentCandidates() []agentCandidate {
	query := strings.TrimSpace(a.agentMentionQuery)
	if a.agentMentionKind == '/' {
		var out []agentCandidate
		for _, command := range a.agentCommands {
			if !strings.Contains(strings.ToLower(command.Name+" "+command.Description), strings.ToLower(query)) {
				continue
			}
			c := agentCandidate{label: "/" + command.Name, insert: "/" + command.Name + " ", command: true}
			if command.Description != "" {
				c.label += " · " + command.Description
			}
			out = append(out, c)
		}
		sort.SliceStable(out, func(i, j int) bool { return out[i].label < out[j].label })
		if len(out) > 30 {
			out = out[:30]
		}
		return out
	}
	if a.agentMentionKind != '@' {
		return nil
	}
	seen := map[string]bool{}
	var out []agentCandidate
	add := func(rel string, dir bool) {
		rel = strings.TrimSuffix(rel, "/")
		if rel == "" || seen[rel] || strings.HasPrefix(rel, ".") {
			return
		}
		seen[rel] = true
		if query != "" && !strings.Contains(strings.ToLower(rel), strings.ToLower(query)) {
			return
		}
		label, insert := rel, "@"+rel
		kind := "file"
		if dir {
			label, insert, kind = rel+"/", "@"+rel+"/", "dir"
		}
		ref := agentRef{Kind: kind, Path: a.joinPath(a.root, rel), Rel: rel}
		out = append(out, agentCandidate{label: label, insert: insert, ref: &ref})
	}
	for _, file := range a.index.Files {
		add(file, false)
	}
	for dir := range a.entries {
		if dir == a.root {
			continue
		}
		if rel, err := a.relativePath(a.root, dir); err == nil {
			add(rel, true)
		}
	}
	for _, file := range a.index.Files {
		for p := path.Dir(file); p != "." && p != "/"; p = path.Dir(p) {
			add(p, true)
		}
	}
	// The files open now are candidates even when the index has not
	// come back yet, as from a new project.
	for _, tab := range a.tabs {
		if tab.state == nil || tab.diff != nil {
			continue
		}
		if rel, err := a.relativePath(a.root, tab.path); err == nil {
			add(rel, false)
		}
	}
	sort.SliceStable(out, func(i, j int) bool {
		a, b := out[i].ref, out[j].ref
		if a.Kind != b.Kind {
			return a.Kind == "file"
		}
		return strings.ToLower(a.Rel) < strings.ToLower(b.Rel)
	})
	if len(out) > 30 {
		out = out[:30]
	}
	return out
}

func (a *app) chooseAgentCandidate(candidate agentCandidate) {
	a.insertAgentText(a.agentTokenStart, a.agentTokenEnd, candidate.insert)
	if candidate.ref != nil {
		ref := *candidate.ref
		a.agentRefs = append(a.agentRefs, ref)
	}
	if candidate.command {
		a.agentCommand = strings.TrimPrefix(candidate.insert, "/")
		a.agentCommand = strings.TrimSpace(a.agentCommand)
	}
}

// agentCompletion is the strip of candidates above the composer. It is
// compact, and only there while a word is being typed.
func (a *app) agentCompletion(c *ui.Context, p palette) {
	if !a.agentMentionOpen {
		return
	}
	candidates := a.agentCandidates()
	if len(candidates) == 0 {
		return
	}
	ui.Scroll(c).FillWidth().MaxHeight(150).Background(p.wash).Children(func() {
		ui.Column(c).FillWidth().Gap(1).Children(func() {
			for _, candidate := range candidates {
				candidate := candidate
				icon, color := fileIcon, p.muted
				if candidate.command {
					icon, color = taskIcon, p.accent
				} else if candidate.ref != nil && candidate.ref.Kind == "dir" {
					icon, color = folderIcon, p.accent
				}
				row := ui.ButtonBase(c).FillWidth().Height(24).Padding(0, 8).Gap(7).Radius(4).
					AlignItems(ui.Center).Label("插入 " + candidate.label)
				if row.Hovered() {
					row.Background(p.edge)
				}
				row.Children(func() {
					ui.Icon(c, icon).Size(12, 12).TextColor(color)
					ui.Text(c, candidate.label).FontSize(11).TextColor(p.text).Grow(1).
						MinWidth(0).SingleLine().Ellipsis("…")
				})
				if row.Clicked() {
					a.chooseAgentCandidate(candidate)
				}
			}
		})
	})
}

// agentRefChips are the files and the selected text riding with the next
// message, each with a way to take it back.
func (a *app) agentRefChips(c *ui.Context, p palette) {
	if len(a.agentRefs) == 0 {
		return
	}
	ui.Row(c).FillWidth().Wrap().Gap(4).Children(func() {
		for i, ref := range a.agentRefs {
			ref, i := ref, i
			chip := ui.Row(c).Height(22).Padding(0, 4, 0, 7).Gap(5).Radius(4).
				Background(p.edge).AlignItems(ui.Center)
			icon := fileIcon
			if ref.Kind == "dir" {
				icon = folderIcon
			} else if ref.Kind == "selection" {
				icon = codeIcon
			}
			chip.Children(func() {
				ui.Icon(c, icon).Size(11, 11).TextColor(p.accent)
				ui.Text(c, ref.label()).FontSize(10).TextColor(p.text).MaxWidth(170).SingleLine().Ellipsis("…")
				if smallIconButton(c, xIcon, "移除引用 "+ref.label(), p) {
					a.agentRefs = append(a.agentRefs[:i], a.agentRefs[i+1:]...)
				}
			})
		}
	})
}

var agentAtToken = regexp.MustCompile(`@([^\s@]+)`)

// agentPromptBlocks is the message and its references, as ACP content
// blocks: text, plus a link for each file or folder mentioned with @.
// Selected code goes in a text block so an agent that only reads text
// still sees it.
func (a *app) agentPromptBlocks(text string) []any {
	blocks := []any{map[string]any{"type": "text", "text": text}}
	seen := map[string]bool{}
	link := func(p, rel string) {
		key := strings.TrimSuffix(rel, "/")
		if key == "" || seen[key] {
			return
		}
		seen[key] = true
		clean := strings.ReplaceAll(p, "\\", "/")
		if !strings.HasPrefix(clean, "/") && !strings.Contains(clean, "://") {
			clean = "/" + clean
		}
		uri := "file://" + strings.ReplaceAll(clean, " ", "%20")
		blocks = append(blocks, map[string]any{"type": "resource_link", "uri": uri, "name": rel})
	}
	for _, ref := range a.agentRefs {
		if ref.Kind == "selection" {
			header := "引用 " + ref.Rel
			if ref.Line != "" {
				header += ":" + ref.Line
			}
			blocks = append(blocks, map[string]any{"type": "text", "text": header + "\n```\n" + ref.Text + "\n```"})
			continue
		}
		link(ref.Path, ref.Rel)
	}
	for _, match := range agentAtToken.FindAllStringSubmatch(text, -1) {
		rel := strings.Trim(match[1], "`,.:;)\"'")
		if rel == "" || strings.Contains(rel, "://") {
			continue
		}
		if seen[strings.TrimSuffix(rel, "/")] {
			continue
		}
		link(a.joinPath(a.root, rel), rel)
	}
	return blocks
}
