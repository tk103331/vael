package main

import (
	"strings"

	"github.com/egoist/mygo/ui"
	"vael/internal/workspace"
)

var discardIcon = lineIcon(`<path d="M3 10h11a6 6 0 0 1 0 12"/><polyline points="8 5 3 10 8 15"/>`)
var openFileIcon = lineIcon(`<path d="M14 3h7v7M21 3 10 14"/><path d="M10 3H5a2 2 0 0 0-2 2v14a2 2 0 0 0 2 2h14a2 2 0 0 0 2-2v-5"/>`)

// Porcelain's two columns describe the index and worktree separately.
// Show the status of this section, rather than the raw two-column record.
func gitChangeStatus(change workspace.Change, staged bool) (code, description string) {
	if change.Code == "??" {
		return "?", "未跟踪"
	}
	switch change.Code {
	case "DD", "AU", "UD", "UA", "DU", "AA", "UU":
		return "U", "合并冲突"
	}
	i := 1
	if staged {
		i = 0
	}
	if len(change.Code) <= i {
		return "?", "未知变更"
	}
	code = change.Code[i : i+1]
	description = map[string]string{
		"M": "修改", "A": "新增", "D": "删除", "R": "重命名", "C": "复制", "T": "类型变更",
	}[code]
	if description == "" {
		return "?", "未知变更"
	}
	return code, description
}

func (a *app) gitChangesView(c *ui.Context, p palette) {
	ui.Column(c).Fill().MinHeight(0).Gap(6).Children(func() {
		input := ui.TextInput(c, &a.commit).FillWidth().Height(32).Shrink(0).Padding(3, 8).Radius(5).
			Placeholder("提交信息…")
		if ui.PrimaryButton(c, "提交已暂存的更改").FillWidth().Height(30).Shrink(0).Radius(5).
			Disabled(a.gitBusy || strings.TrimSpace(a.commit) == "").Clicked() || input.Submitted() && strings.TrimSpace(a.commit) != "" {
			backend, ctx, root, message := a.backend, a.projectCtx, a.repo.Root, a.commit
			a.gitAction(func() error { _, err := backend.Git(ctx, root, "commit", "-m", message); return err })
		}
		ui.Scroll(c).Label("变更文件列表").FillWidth().Grow(1).MinHeight(0).Children(func() {
			ui.Column(c).FillWidth().Gap(6).Children(func() {
				for _, staged := range []bool{true, false} {
					ui.Column(c).FillWidth().Gap(1).Children(func() {
						title := "工作区更改"
						if staged {
							title = "已暂存的更改"
						}
						a.gitSectionCount(c, p, title, a.changeCount(staged))
						for _, change := range a.repo.Changes {
							if staged && change.Staged() || !staged && change.Unstaged() {
								a.gitChangeRow(c, p, change, staged)
							}
						}
						if a.changeCount(staged) == 0 {
							ui.Text(c, "没有更改").FontSize(11).TextColor(p.muted).Padding(2)
						}
					})
				}
			})
		})
	})
}

func (a *app) gitChangeRow(c *ui.Context, p palette, change workspace.Change, staged bool) {
	code, description := gitChangeStatus(change, staged)
	color := logCodeColor(code, p)
	ui.Row(c).FillWidth().Height(24).Gap(2).AlignItems(ui.Center).Children(func() {
		b := ui.ButtonBase(c).Grow(1).MinWidth(0).Height(24).Padding(0, 4).Gap(6).Radius(4).
			AlignItems(ui.Center).Justify(ui.Start).Label(change.Path)
		if b.Hovered() {
			b.Background(p.edge)
		}
		b.Tooltip(description + " · " + change.Path)
		b.Children(func() {
			ui.Text(c, code).Width(14).Shrink(0).Font("monospace").FontSize(10.5).FontWeight(650).
				TextColor(color).TextAlign(ui.Center).Label(description)
			ui.Text(c, change.Path).Grow(1).MinWidth(0).FontSize(11).TextColor(p.text).SingleLine().Ellipsis("…")
		})
		if b.Clicked() {
			a.openDiff(change, staged)
		}
		backend, ctx, root := a.backend, a.projectCtx, a.repo.Root
		icon, tip := plusIcon, "暂存 "+change.Path
		if staged {
			icon, tip = minusIcon, "取消暂存 "+change.Path
		}
		if !a.gitBusy && smallIconButton(c, icon, tip, p) {
			a.gitAction(func() error {
				if staged {
					return backend.Unstage(ctx, root, change)
				}
				return backend.Stage(ctx, root, change)
			})
		}
		if !staged && !a.gitBusy && smallIconButton(c, discardIcon, "放弃更改 "+change.Path, p) {
			a.confirm = &confirmation{"放弃更改？", "“" + change.Path + "”在工作区中的修改将被丢弃，无法撤销。", func() { a.discardChange(change) }}
		}
		if smallIconButton(c, openFileIcon, "打开文件 "+change.Path, p) {
			a.openFile(a.joinPath(root, change.Path))
		}
	})
}

func (a *app) gitHistoryView(c *ui.Context, p palette) {
	ui.Column(c).Fill().MinHeight(0).Gap(3).Children(func() {
		a.gitSectionCount(c, p, "提交历史", len(a.commits))
		ui.Scroll(c).Label("提交历史列表").FillWidth().Grow(1).MinHeight(0).Children(func() {
			ui.Column(c).FillWidth().Gap(0).Children(func() {
				if a.commitsLoading {
					ui.Text(c, "正在读取提交…").FontSize(11).TextColor(p.muted).Padding(2)
				}
				for _, line := range a.commits {
					hash, subject := line, ""
					if i := strings.Index(line, " "); i > 0 {
						hash, subject = line[:i], strings.TrimSpace(line[i+1:])
					}
					row := ui.ButtonBase(c).FillWidth().Height(22).Padding(0, 4).Gap(6).Radius(3).
						AlignItems(ui.Center).Label("提交：" + line).Cursor(ui.CursorPointer)
					if row.Hovered() {
						row.Background(p.edge)
					}
					row.Tooltip(subject + " · 双击查看提交 Diff")
					row.Children(func() {
						ui.Text(c, hash).Font("monospace").FontSize(10).TextColor(p.accent).Width(54).Shrink(0)
						ui.Text(c, subject).FontSize(11).TextColor(p.text).Grow(1).MinWidth(0).SingleLine().Ellipsis("…")
					})
					if row.DoubleClicked() {
						a.openCommitDiff(hash, subject)
					}
				}
				if len(a.commits) == 0 && !a.commitsLoading {
					ui.Text(c, "没有提交").FontSize(11).TextColor(p.muted).Padding(2)
				}
			})
		})
	})
}
