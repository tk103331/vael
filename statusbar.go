package main

import (
	"fmt"
	"strings"

	"github.com/egoist/mygo/ui"
)

// detectIndent reports how the text indents: with tabs, or with how many
// spaces. It reads the leading whitespace of the lines that have some, and
// takes the greatest common divisor of the space indents as the unit.
func detectIndent(text string) (tabs bool, width int) {
	width = 4
	tabLines, spaceLines, unit := 0, 0, 0
	for _, line := range strings.Split(text, "\n") {
		trimmed := strings.TrimLeft(line, " \t")
		if trimmed == "" {
			continue
		}
		lead := line[:len(line)-len(trimmed)]
		if strings.Contains(lead, "\t") {
			tabLines++
			continue
		}
		spaceLines++
		if n := len(lead); n > 0 {
			unit = gcd(unit, n)
		}
	}
	if tabLines > spaceLines {
		return true, 4
	}
	if unit >= 2 && unit <= 8 {
		return false, unit
	}
	return false, 4
}

// gcd is the greatest common divisor of a and b, with 0 as the identity.
func gcd(a, b int) int {
	for b != 0 {
		a, b = b, a%b
	}
	return a
}

// convertIndent rewrites the leading whitespace of every line: to width
// spaces, or to tabs of that width. The text of the lines is untouched.
func convertIndent(text string, tabs bool, width int) string {
	if width < 1 {
		width = 4
	}
	lines := strings.Split(text, "\n")
	for i, line := range lines {
		trimmed := strings.TrimLeft(line, " \t")
		lead := line[:len(line)-len(trimmed)]
		if lead == "" {
			continue
		}
		columns := 0
		for _, r := range lead {
			if r == '\t' {
				columns += width
			} else {
				columns++
			}
		}
		if tabs {
			lines[i] = strings.Repeat("\t", columns/width) + strings.Repeat(" ", columns%width) + trimmed
		} else {
			lines[i] = strings.Repeat(" ", columns) + trimmed
		}
	}
	return strings.Join(lines, "\n")
}

// indentOf reports the indentation of the tab's text, cached for the
// version of the buffer it was read from.
func (f *fileTab) indentOf() (tabs bool, width int) {
	version := f.state.Buffer.Version()
	if !f.indentDone || f.indentVersion != version {
		f.indentTabs, f.indentWidth = detectIndent(f.state.Buffer.String())
		f.indentVersion, f.indentDone = version, true
	}
	return f.indentTabs, f.indentWidth
}

// setIndent converts the indentation of the file that is open, as one edit
// that Undo takes back.
func (a *app) setIndent(f *fileTab, tabs bool, width int) {
	text := f.state.Buffer.String()
	next := convertIndent(text, tabs, width)
	if next == text {
		return
	}
	f.state.Buffer.Set(next)
}

// indentWidget is the status bar's indentation: what the file uses, and the
// menu that converts it, as an IDE's.
func (a *app) indentWidget(c *ui.Context, p palette, f *fileTab) {
	tabs, width := f.indentOf()
	label := fmt.Sprintf("空格: %d", width)
	if tabs {
		label = "制表符"
	}
	b := ui.ButtonBase(c).Height(20).Padding(0, 5).Radius(4).Label(label)
	b.Children(func() { ui.Text(c, label).FontSize(11).TextColor(p.muted) })
	b.Tooltip("缩进：点击转换")
	b.Menu(func(m *ui.Menu) {
		for _, w := range []int{2, 4, 8} {
			if m.Item(fmt.Sprintf("空格：%d", w)).Chosen() {
				a.setIndent(f, false, w)
			}
		}
		if m.Item("制表符").Chosen() {
			a.setIndent(f, true, width)
		}
	})
}

// lineEndingWidget is the status bar's line ending, and the menu that
// switches it between LF and CRLF.
func (a *app) lineEndingWidget(c *ui.Context, p palette, f *fileTab) {
	ending := "LF"
	if f.state.Buffer.LineEnding() == "\r\n" {
		ending = "CRLF"
	}
	b := ui.ButtonBase(c).Height(20).Padding(0, 5).Radius(4).Label(ending)
	b.Children(func() { ui.Text(c, ending).FontSize(11).TextColor(p.muted) })
	b.Tooltip("行尾符：点击切换")
	b.Menu(func(m *ui.Menu) {
		if m.Item("LF（Unix）").Chosen() {
			f.state.Buffer.SetLineEnding("\n")
		}
		if m.Item("CRLF（Windows）").Chosen() {
			f.state.Buffer.SetLineEnding("\r\n")
		}
	})
}
