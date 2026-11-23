package main

import (
	"path/filepath"
	"sort"

	"github.com/egoist/mygo/ui"
	"github.com/egoist/mygo/ui/editor"
)

// Problems: what the language servers found in the files that are open,
// as an IDE lists them, in a tool window at the bottom.

// problem is one thing a server found: where it is, and what it says.
type problem struct {
	path     string
	line     int
	column   int
	start    int
	end      int
	severity editor.Severity
	message  string
	source   string
}

// problemsOf collects what the files that are open have, in the order of
// their paths and their lines.
func (a *app) problemsOf() []problem {
	var found []problem
	for _, tab := range a.tabs {
		if tab.state == nil || tab.diff != nil {
			continue
		}
		for _, diagnostic := range tab.state.Diagnostics() {
			at := tab.state.Buffer.Pos(diagnostic.Start)
			found = append(found, problem{
				path:     tab.path,
				line:     at.Line,
				column:   at.Col,
				start:    diagnostic.Start,
				end:      diagnostic.End,
				severity: diagnostic.Severity,
				message:  diagnostic.Message,
				source:   diagnostic.Source,
			})
		}
	}
	sort.SliceStable(found, func(i, j int) bool {
		if found[i].path != found[j].path {
			return found[i].path < found[j].path
		}
		return found[i].start < found[j].start
	})
	return found
}

// problems counts what the files that are open have.
func (a *app) problems() int {
	count := 0
	for _, tab := range a.tabs {
		if tab.state != nil && tab.diff == nil {
			count += len(tab.state.Diagnostics())
		}
	}
	return count
}

// problemBadge is what the strip of the tool window shows of them.
func (a *app) problemBadge() string {
	count := a.problems()
	if count == 0 {
		return ""
	}
	return itoa(count)
}

// itoa writes a count.
func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	var digits []byte
	for n > 0 {
		digits = append([]byte{byte('0' + n%10)}, digits...)
		n /= 10
	}
	return string(digits)
}

// problemMark is how bad a problem is: an error's cross, a warning's bang,
// or a note's dot.
func (a *app) problemMark(c *ui.Context, p palette, severity editor.Severity) {
	switch severity {
	case editor.SeverityError:
		ui.Icon(c, xIcon).Size(11, 11).TextColor(p.danger)
	case editor.SeverityWarning:
		ui.Icon(c, alertIcon).Size(11, 11).TextColor(p.warn)
	case editor.SeverityInfo:
		ui.Box(c).Size(7, 7).Radius(3.5).Background(p.accent)
	default:
		ui.Box(c).Size(7, 7).Radius(3.5).Border(1, p.muted)
	}
}

// problemView is the tool window of the problems: the files that have
// them, and what each says, as an IDE lists them.
func (a *app) problemView(c *ui.Context, p palette) {
	found := a.problemsOf()
	if len(found) == 0 {
		ui.Column(c).FillWidth().Padding(12).Gap(4).Children(func() {
			ui.Text(c, "没有发现问题").FontSize(12).TextColor(p.text)
			ui.Text(c, "语言服务器把文件里的问题报到这里；保存后自动更新。").
				FontSize(10.5).TextColor(p.muted)
		})
		return
	}
	ui.Scroll(c).FillWidth().Grow(1).Children(func() {
		ui.Column(c).FillWidth().Padding(0, 4).Children(func() {
			file := ""
			for _, item := range found {
				item := item
				if item.path != file {
					file = item.path
					a.problemFileRow(c, p, file, found)
				}
				a.problemRow(c, p, item)
			}
		})
	})
}

// problemFileRow is the file of a group of problems, with how many it
// has.
func (a *app) problemFileRow(c *ui.Context, p palette, path string, found []problem) {
	count := 0
	for _, item := range found {
		if item.path == path {
			count++
		}
	}
	name := path
	if rel, err := a.relativePath(a.root, path); err == nil {
		name = rel
	}
	ui.Row(c).FillWidth().Height(24).Padding(6, 6).Gap(5).AlignItems(ui.Center).Children(func() {
		ui.Icon(c, fileIcon).Size(11, 11).TextColor(p.muted)
		ui.Text(c, name).FontSize(11).FontWeight(600).TextColor(p.text).Grow(1).
			MinWidth(0).SingleLine().Ellipsis("…")
		ui.Textf(c, "%d", count).FontSize(9.5).TextColor(p.muted).FontFeatures("tnum")
	})
}

// problemRow is one problem: how bad it is, what it says, and where.
func (a *app) problemRow(c *ui.Context, p palette, item problem) {
	row := ui.ButtonBase(c).FillWidth().Height(24).Padding(6, 24).Gap(6).
		Radius(4).AlignItems(ui.Center).Label(item.message)
	if row.Hovered() {
		row.Background(p.edge)
	}
	row.Children(func() {
		a.problemMark(c, p, item.severity)
		ui.Text(c, item.message).FontSize(11.5).TextColor(p.text).Grow(1).
			MinWidth(0).SingleLine().Ellipsis("…")
		where := itoa(item.line+1) + ":" + itoa(item.column+1)
		if item.source != "" {
			where = item.source + " · " + where
		}
		ui.Text(c, where).FontSize(10).TextColor(p.muted).SingleLine()
	})
	row.Cursor(ui.CursorPointer)
	if row.Clicked() {
		a.openFileAt(item.path, item.start, item.end)
	}
}

// problemFile is the name of a problem's file, for a tooltip.
func (a *app) problemFile(path string) string {
	if rel, err := a.relativePath(a.root, path); err == nil {
		return rel
	}
	return filepath.Base(path)
}
