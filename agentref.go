package main

import (
	"bytes"
	"encoding/base64"
	"fmt"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"unicode/utf8"

	"github.com/egoist/mygo/ui"
)

// A reference is something the user pointed at, to travel with the next
// message: a file or a folder of the project, text selected in an editor,
// or an image pasted or dropped into the conversation. The panel shows
// them as chips; the prompt carries them as content blocks that an ACP
// agent understands.
type agentRef struct {
	Kind, Path, Rel, Text, Line string
	// Data is the bytes of an image, and Mime what they are, for the
	// block an agent reads it as. Src is the same image decoded, for the
	// thumbnail of its chip.
	Data []byte
	Mime string
	Src  ui.ImageSource
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
		if item.Kind == ref.Kind && item.Path == ref.Path && item.Line == ref.Line &&
			item.Text == ref.Text && bytes.Equal(item.Data, ref.Data) {
			return
		}
	}
	a.agentRefs = append(a.agentRefs, ref)
	a.rightPanel, a.rightSection = true, "agent"
	a.message, a.messageKind = "已添加到对话", "success"
}

// addAgentImage attaches a picture to the next message: one pasted from
// the clipboard, or a file dropped on the box. It is sent as the image
// block of ACP, which agents with a vision model read.
func (a *app) addAgentImage(data []byte, name string) {
	if len(data) == 0 {
		return
	}
	src, err := ui.DecodeImage(data)
	if err != nil {
		a.report(fmt.Errorf("无法读取图片“%s”：%v", name, err))
		return
	}
	label := strings.TrimSpace(name)
	if label == "" {
		label = "图片"
	}
	if w, h := ui.ImageSize(src); w > 0 && h > 0 {
		label = fmt.Sprintf("%s · %d×%d", label, int(w), int(h))
	}
	a.addAgentRef(agentRef{Kind: "image", Rel: label, Data: data, Mime: imageMime(data), Src: src})
}

// imageMime names the format of a picture, as its image block says.
func imageMime(data []byte) string {
	switch kind := http.DetectContentType(data); kind {
	case "image/png", "image/jpeg", "image/gif", "image/webp", "image/bmp":
		return kind
	}
	return "image/png"
}

// isImagePath tells whether a dropped file is a picture the conversation
// can carry, by its extension.
func isImagePath(path string) bool {
	switch strings.ToLower(filepath.Ext(path)) {
	case ".png", ".jpg", ".jpeg", ".gif", ".webp", ".bmp":
		return true
	}
	return false
}

// attachAgentImages takes the pictures among dropped files into the
// conversation, and gives the others back to the editor, which opens
// them as it does any drop.
func (a *app) attachAgentImages(paths []string) {
	var rest []string
	for _, item := range paths {
		if !isImagePath(item) {
			rest = append(rest, item)
			continue
		}
		item := item
		go func() {
			data, err := os.ReadFile(item)
			a.update(func() {
				if err != nil {
					a.report(err)
					return
				}
				a.addAgentImage(data, filepath.Base(item))
			})
		}()
	}
	if len(rest) > 0 {
		a.handleDrop(rest)
	}
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
			switch ref.Kind {
			case "dir":
				icon = folderIcon
			case "selection":
				icon = codeIcon
			case "image":
				icon = imageIcon
			}
			chip.Children(func() {
				if ref.Kind == "image" && ref.Src != nil {
					ui.Image(c, ref.Src).Size(18, 18).Radius(3)
				} else {
					ui.Icon(c, icon).Size(11, 11).TextColor(p.accent)
				}
				ui.Text(c, ref.label()).FontSize(10).TextColor(p.text).MaxWidth(170).SingleLine().Ellipsis("…")
				if smallIconButton(c, xIcon, "移除引用 "+ref.label(), p) {
					a.agentRefs = append(a.agentRefs[:i], a.agentRefs[i+1:]...)
				}
			})
		}
	})
}

// agentQueueChips are the messages typed while the agent works, in the
// order they will go. A click takes one back to the box to edit it, the
// arrow sends it at once, and the cross drops it.
func (a *app) agentQueueChips(c *ui.Context, p palette) {
	if len(a.agentQueue) == 0 {
		return
	}
	ui.Column(c).FillWidth().Gap(2).Children(func() {
		ui.Row(c).FillWidth().AlignItems(ui.Center).Children(func() {
			ui.Textf(c, "待发送 %d 条", len(a.agentQueue)).FontSize(9.5).TextColor(p.accent)
			ui.Box(c).Grow(1)
			ui.Text(c, "Agent 完成后依次发送").FontSize(9).TextColor(p.muted)
		})
		for i, item := range a.agentQueue {
			i, item := i, item
			row := ui.Row(c).FillWidth().Height(24).Padding(0, 5).Gap(5).Radius(4).
				Background(p.wash).AlignItems(ui.Center)
			row.Children(func() {
				ui.Icon(c, agentIcon).Size(10, 10).TextColor(p.muted)
				label := strings.Join(strings.Fields(item.Text), " ")
				if label == "" {
					label = "（只有引用）"
				}
				if len(item.Refs) > 0 {
					label += fmt.Sprintf(" · %d 个引用", len(item.Refs))
				}
				body := ui.ButtonBase(c).Grow(1).MinWidth(0).Height(20).Padding(0, 2).
					Radius(3).AlignItems(ui.Center).Label("编辑待发送消息")
				if body.Hovered() {
					body.Background(p.edge)
				}
				body.Children(func() {
					ui.Text(c, label).FontSize(10).TextColor(p.text).SingleLine().Ellipsis("…")
				})
				body.Tooltip("取回输入框编辑")
				if body.Clicked() {
					a.editQueuedAgent(i)
				}
				if smallIconButton(c, arrowUpIcon, "插队发送", p) {
					a.steerAgent(i)
				}
				if smallIconButton(c, xIcon, "移除待发送消息", p) {
					a.agentQueue = append(a.agentQueue[:i], a.agentQueue[i+1:]...)
				}
			})
		}
	})
}

var agentAtToken = regexp.MustCompile(`@([^\s@]+)`)

// agentPromptBlocks is the message and its references, as ACP content
// blocks: text, a link for each file or folder mentioned with @, and an
// image block for each picture attached. Selected code goes in a text
// block so an agent that only reads text still sees it.
func (a *app) agentPromptBlocks(text string) []any {
	return a.agentPromptBlocksWith(text, a.agentRefs)
}

// agentPromptBlocksWith is agentPromptBlocks for references that may not
// be the ones in the box any more, as a message waiting in its queue.
func (a *app) agentPromptBlocksWith(text string, refs []agentRef) []any {
	var blocks []any
	if strings.TrimSpace(text) != "" {
		blocks = append(blocks, map[string]any{"type": "text", "text": text})
	}
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
	for _, ref := range refs {
		switch ref.Kind {
		case "image":
			if len(ref.Data) == 0 {
				continue
			}
			mime := ref.Mime
			if mime == "" {
				mime = imageMime(ref.Data)
			}
			blocks = append(blocks, map[string]any{
				"type":     "image",
				"data":     base64.StdEncoding.EncodeToString(ref.Data),
				"mimeType": mime,
			})
		case "selection":
			header := "引用 " + ref.Rel
			if ref.Line != "" {
				header += ":" + ref.Line
			}
			blocks = append(blocks, map[string]any{"type": "text", "text": header + "\n```\n" + ref.Text + "\n```"})
		default:
			link(ref.Path, ref.Rel)
		}
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
