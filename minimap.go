package main

import (
	"strings"

	"github.com/egoist/mygo/ui"
)

// The minimap: the whole text of the file, shrunk into a narrow column
// beside the editor, with the part in view marked. A click or a drag in it
// scrolls the editor, as VS Code's minimap does.

const (
	// minimapWidth is how wide the column is.
	minimapWidth = 78
	// minimapCols is how many characters the column shows across.
	minimapCols = 140
	// minimapRows is the most rows drawn: a longer text is sampled.
	minimapRows = 620
)

// miniLineInk is where a line's text starts and how long it is, as
// fractions of the minimap's width.
type miniLineInk struct {
	start float32
	width float32
}

// minimapView is the column beside a file's editor.
func (a *app) minimapView(c *ui.Context, p palette, f *fileTab) {
	if f == nil || f.state == nil {
		return
	}
	m := ui.ButtonBase(c).Width(minimapWidth).FillHeight().Shrink(0).Label("缩略图")
	m.Cursor(ui.CursorPointer).FocusRing(false).Tooltip("缩略图：点击或拖动以跳转")
	m.Draw(func(painter *ui.Painter, r ui.Rect) {
		a.drawMinimap(painter, r, p, f)
	})
	// A press anywhere in the column, the pointer's moves while it lasts,
	// and the click it ends with put the middle of the view on the line
	// there.
	if m.Pressed() || m.Clicked() {
		_, y, over := m.PointerPosition()
		if over || m.Pressed() {
			a.minimapScroll(f, y)
		}
	}
}

// drawMinimap draws the text's lines and the view, which a click in the
// column moves.
func (a *app) drawMinimap(painter *ui.Painter, r ui.Rect, p palette, f *fileTab) {
	if f == nil || f.state == nil {
		return
	}
	f.miniHeight = r.H
	count := f.state.Buffer.LineCount()
	if count <= 0 || r.H <= 0 {
		return
	}
	rows := min(minimapRows, count)
	ink := a.minimapInk(f, rows)
	// A line is no taller than a few points: a short file shows its shape
	// at the top of the column rather than a few giant blocks. What the
	// lines take is what they are mapped to, for the band and for a click.
	step := min(3, r.H/float32(rows))
	contentH := step * float32(rows)
	if contentH <= 0 {
		return
	}
	f.miniContent = contentH
	height := max(1, step-0.4)
	for i, line := range ink {
		if line.width <= 0 {
			continue
		}
		painter.Fill(ui.Rect{
			X: r.X + line.start*r.W,
			Y: r.Y + float32(i)*step,
			W: max(1, line.width*r.W),
			H: height,
		}, p.text.Alpha(0.30), 0)
	}
	// What is in view wears a band, as the editor's own scrollbar does.
	first, last := f.state.VisibleLines()
	if first < 0 || last < first {
		return
	}
	y0 := float32(float64(first) / float64(count) * float64(contentH))
	y1 := float32(float64(last+1) / float64(count) * float64(contentH))
	if y1-y0 < 10 {
		// The band stays big enough to be seen and to be pressed.
		middle := (y0 + y1) / 2
		y0, y1 = middle-5, middle+5
	}
	y0 = max(0, y0)
	y1 = min(contentH, y1)
	if y1 <= y0 {
		return
	}
	band := ui.Rect{X: r.X, Y: r.Y + y0, W: r.W, H: y1 - y0}
	painter.Fill(band, p.accent.Alpha(0.16), 3)
	painter.Line(band.X, band.Y, band.X+band.W, band.Y, 1, p.accent.Alpha(0.45))
	painter.Line(band.X, band.Y+band.H, band.X+band.W, band.Y+band.H, 1, p.accent.Alpha(0.45))
}

// minimapInk is where the lines of a text start and end across the column.
// It is read from the buffer once a version, not at every frame.
func (a *app) minimapInk(f *fileTab, rows int) []miniLineInk {
	buffer := f.state.Buffer
	version := buffer.Version()
	if f.miniBuffer == buffer && f.miniVersion == version && f.miniRows == rows && f.miniInk != nil {
		return f.miniInk
	}
	count := buffer.LineCount()
	ink := make([]miniLineInk, rows)
	for row := range rows {
		line := row * count / rows
		if line >= count {
			continue
		}
		text := []rune(strings.TrimRight(buffer.Line(line), " \t\r"))
		start := 0
		for start < len(text) && (text[start] == ' ' || text[start] == '\t') {
			start++
		}
		if start == len(text) {
			continue
		}
		if start >= minimapCols {
			start = minimapCols - 1
		}
		width := len(text) - start
		if width > minimapCols-start {
			width = minimapCols - start
		}
		if width <= 0 {
			continue
		}
		ink[row] = miniLineInk{
			start: float32(start) / minimapCols,
			width: float32(width) / minimapCols,
		}
	}
	f.miniBuffer, f.miniVersion, f.miniRows, f.miniInk = buffer, version, rows, ink
	return ink
}

// minimapScroll puts the middle of the view on the line at y, from the top
// of the column, whose height was last drawn.
func (a *app) minimapScroll(f *fileTab, y float32) {
	if f == nil || f.state == nil || f.miniHeight <= 0 {
		return
	}
	count := f.state.Buffer.LineCount()
	if count <= 0 {
		return
	}
	height := min(f.miniHeight, f.miniContent)
	if height <= 0 {
		return
	}
	y = max(0, min(height, y))
	line := int(float64(y) / float64(height) * float64(count))
	first, last := f.state.VisibleLines()
	visible := last - first + 1
	if visible < 1 {
		visible = 1
	}
	f.state.ScrollTo(max(0, min(count-1, line-visible/2)))
}
