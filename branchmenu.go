package main

import (
	"strings"

	"github.com/egoist/mygo/ui"
)

// branchPrompt is the dialog that creates a branch.
type branchPrompt struct{ name string }

// revisionPrompt is the dialog that checks out a tag or a revision.
type revisionPrompt struct{ revision string }

// branchMenu is the switcher of branches, as an IDE's: the Git actions,
// the branches of late, and the local and remote ones, filtered by the
// query.
func (a *app) branchMenu(c *ui.Context, p palette) {
	if a.repo == nil {
		return
	}
	ui.Column(c).Width(420).Padding(4).Gap(1).Children(func() {
		input := ui.TextInput(c, &a.branchQuery).FillWidth().Height(26).Padding(2, 6).Placeholder("搜索分支与操作…").Label("搜索分支").AutoFocus()
		query := strings.ToLower(strings.TrimSpace(a.branchQuery))
		matches := func(name string) bool {
			return query == "" || strings.Contains(strings.ToLower(name), query)
		}
		if query == "" {
			menuRow(c, p, downloadIcon, "更新项目…", "拉取", func() {
				a.branchMenuOpen = false
				a.gitRemote("pull", "--ff-only")
			})
			menuRow(c, p, checkIcon, "提交…", "", func() {
				a.branchMenuOpen = false
				a.sidebar = true
				a.section = "commit"
			})
			menuRow(c, p, uploadIcon, "推送…", "", func() {
				a.branchMenuOpen = false
				a.gitRemote("push")
			})
			ui.Divider(c)
			menuRow(c, p, plusIcon, "新建分支…", "", func() {
				a.branchMenuOpen = false
				a.branchPrompt = &branchPrompt{}
			})
			menuRow(c, p, tagIcon, "检出标签或修订…", "", func() {
				a.branchMenuOpen = false
				a.revisionPrompt = &revisionPrompt{}
			})
			ui.Divider(c)
		}
		ui.Scroll(c).FillWidth().MaxHeight(380).Children(func() {
			ui.Column(c).FillWidth().Children(func() {
				if query == "" {
					menuHeader(c, p, "最近")
					a.branchRow(c, p, a.repo.Branch, true)
					for _, name := range a.recentBranches {
						if name == a.repo.Branch || !matches(name) {
							continue
						}
						a.branchRow(c, p, name, false)
					}
				}
				if a.branchSection(c, p, "本地", &a.branchLocalOpen, len(a.branches), query) {
					found := 0
					for _, name := range a.branches {
						if !matches(name) {
							continue
						}
						found++
						a.branchRow(c, p, name, name == a.repo.Branch)
					}
					if found == 0 {
						ui.Text(c, "没有匹配的本地分支").FontSize(10.5).TextColor(p.muted).Padding(6, 6, 3, 6)
					}
				}
				if a.branchSection(c, p, "远程", &a.branchRemoteOpen, len(a.remoteBranches), query) {
					found := 0
					for _, name := range a.remoteBranches {
						if !matches(name) {
							continue
						}
						found++
						a.remoteBranchRow(c, p, name)
					}
					if found == 0 {
						ui.Text(c, "没有匹配的远程分支").FontSize(10.5).TextColor(p.muted).Padding(6, 6, 3, 6)
					}
				}
			})
		})
		if input.Submitted() {
			for _, name := range a.branches {
				if matches(name) {
					a.branchMenuOpen = false
					a.switchBranch(name)
					break
				}
			}
		}
	})
}

// branchSection is the title of a section of branches, which opens and
// closes, as an IDE's. A query opens it.
func (a *app) branchSection(c *ui.Context, p palette, label string, open *bool, count int, query string) bool {
	expanded := *open || query != ""
	b := ui.ButtonBase(c).FillWidth().Height(22).Padding(0, 6).Radius(5).Gap(5).AlignItems(ui.Center).Label(label + "分支")
	if b.Hovered() {
		b.Background(p.edge)
	}
	b.Children(func() {
		chevron := chevronRightIcon
		if expanded {
			chevron = chevronDownIcon
		}
		ui.Icon(c, chevron).Size(10, 10).TextColor(p.muted)
		ui.Text(c, label).FontSize(10.5).FontWeight(650).TextColor(p.muted).Grow(1)
		if count > 0 {
			ui.Textf(c, "%d", count).FontSize(9.5).TextColor(p.muted).FontFeatures("tnum")
		}
	})
	if b.Clicked() && query == "" {
		*open = !*open
	}
	return expanded
}

// branchRow is a branch of the switcher: the one that is checked out has
// a star, and its upstream with how far it is ahead and behind.
func (a *app) branchRow(c *ui.Context, p palette, name string, current bool) {
	b := ui.ButtonBase(c).FillWidth().Height(24).Padding(0, 6).Radius(5).Gap(5).AlignItems(ui.Center).Label(name)
	if current {
		b.Background(p.selected)
	} else if b.Hovered() {
		b.Background(p.edge)
	}
	b.Children(func() {
		if current {
			ui.Icon(c, starIcon).Size(12, 12).TextColor(p.warn)
		} else {
			ui.Box(c).Width(12)
		}
		ui.Text(c, name).FontSize(11.5).TextColor(p.text).Grow(1).SingleLine().Ellipsis("…")
		if current {
			if a.upstream != "" {
				ui.Text(c, a.upstream).FontSize(9.5).TextColor(p.muted).SingleLine().Ellipsis("…")
			}
			if a.branchAhead > 0 {
				ui.Textf(c, "↑%d", a.branchAhead).FontSize(9.5).TextColor(p.success).FontFeatures("tnum")
			}
			if a.branchBehind > 0 {
				ui.Textf(c, "↓%d", a.branchBehind).FontSize(9.5).TextColor(p.warn).FontFeatures("tnum")
			}
		}
	})
	if b.Clicked() {
		a.branchMenuOpen = false
		if !current {
			a.switchBranch(name)
		}
	}
}

// remoteBranchRow is a branch of a remote. Choosing it checks out a local
// branch that tracks it.
func (a *app) remoteBranchRow(c *ui.Context, p palette, name string) {
	b := ui.ButtonBase(c).FillWidth().Height(24).Padding(0, 6).Radius(5).Gap(5).AlignItems(ui.Center).Label(name)
	if b.Hovered() {
		b.Background(p.edge)
	}
	b.Children(func() {
		ui.Box(c).Width(12)
		ui.Text(c, name).FontSize(12).TextColor(p.text).Grow(1).SingleLine().Ellipsis("…")
	})
	if b.Clicked() {
		a.branchMenuOpen = false
		a.checkoutRemote(name)
	}
}

// branchPromptView is the dialog that creates a branch.
func (a *app) branchPromptView(c *ui.Context, p palette) {
	if a.branchPrompt == nil {
		return
	}
	prompt := a.branchPrompt
	open := true
	ui.Modal(c, &open, func() {
		ui.Column(c).Width(420).Padding(22).Gap(14).Children(func() {
			ui.Text(c, "新建分支").FontSize(17).FontWeight(650)
			from := ""
			if a.repo != nil {
				from = "从 " + a.repo.Branch + " 创建并切换"
			}
			ui.Text(c, from).FontSize(11).TextColor(p.muted)
			input := ui.TextInput(c, &prompt.name).FillWidth().Placeholder("feature/…").Label("分支名称").AutoFocus()
			submitted := input.Submitted()
			ui.Row(c).FillWidth().Gap(8).Children(func() {
				ui.Spacer(c)
				if ui.Button(c, "取消").Clicked() {
					open = false
				}
				if ui.PrimaryButton(c, "创建").Clicked() || submitted {
					open = false
					a.createBranch(prompt.name)
				}
			})
		})
	})
	if !open {
		a.branchPrompt = nil
	}
}

// revisionPromptView is the dialog that checks out a tag or a revision.
func (a *app) revisionPromptView(c *ui.Context, p palette) {
	if a.revisionPrompt == nil {
		return
	}
	prompt := a.revisionPrompt
	open := true
	ui.Modal(c, &open, func() {
		ui.Column(c).Width(420).Padding(22).Gap(14).Children(func() {
			ui.Text(c, "检出标签或修订").FontSize(17).FontWeight(650)
			ui.Text(c, "标签、提交或远程分支，如 v1.2.0 或 origin/main").FontSize(11).TextColor(p.muted)
			input := ui.TextInput(c, &prompt.revision).FillWidth().Placeholder("v1.2.0").Label("标签或修订").AutoFocus()
			submitted := input.Submitted()
			ui.Row(c).FillWidth().Gap(8).Children(func() {
				ui.Spacer(c)
				if ui.Button(c, "取消").Clicked() {
					open = false
				}
				if ui.PrimaryButton(c, "检出").Clicked() || submitted {
					open = false
					a.checkoutRevision(prompt.revision)
				}
			})
		})
	})
	if !open {
		a.revisionPrompt = nil
	}
}
