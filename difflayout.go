package main

import (
	"github.com/egoist/mygo/ui"
	"github.com/egoist/mygo/ui/editor"
)

var inlineDiffIcon = lineIcon(`<rect x="3" y="3" width="18" height="18" rx="2"/><line x1="3" y1="12" x2="21" y2="12"/><line x1="7" y1="7" x2="17" y2="7"/><line x1="7" y1="17" x2="17" y2="17"/>`)

// All file and commit diffs share the last chosen layout, including files
// that arrive after a commit's toolbar has been used while loading.
func (a *app) diffLayoutControls(c *ui.Context, p palette) {
	if iconButton(c, inlineDiffIcon, "行内对比", a.diffLayout == editor.DiffUnified, p) {
		a.diffLayout = editor.DiffUnified
	}
	if iconButton(c, splitIcon, "分栏对比", a.diffLayout == editor.DiffSplit, p) {
		a.diffLayout = editor.DiffSplit
	}
}

func (a *app) fileDiffToolbar(c *ui.Context, p palette) {
	ui.Row(c).FillWidth().Height(32).Shrink(0).Padding(0, 6).Gap(4).AlignItems(ui.Center).Children(func() {
		ui.Text(c, "文件差异").FontSize(11).TextColor(p.muted).Grow(1)
		a.diffLayoutControls(c, p)
	})
	ui.Divider(c)
}
