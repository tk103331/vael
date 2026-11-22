package main

import (
	"strings"

	"github.com/egoist/mygo/ui"
	"vael/internal/core"
	"vael/internal/workspace"
)

func (a *app) currentEnvironment() workspace.Environment {
	e := workspace.Environment{Root: a.root}
	if a.connection != nil {
		e.Kind = a.connection.Kind
		e.Target = a.connection.Target
		e.Binary = a.connection.Binary
	}
	return e
}
func (a *app) saveEnvironments() {
	if a.environmentsPath != "" {
		a.report(workspace.SaveEnvironments(a.environmentsPath, a.environments))
	}
}
func (a *app) rememberEnvironment() {
	if a.root == "" {
		return
	}
	a.environments.Remember(a.currentEnvironment())
	a.saveEnvironments()
}
func (a *app) saveConnectionProfile(c core.Connection) {
	if err := c.Validate(); err != nil {
		a.report(err)
		return
	}
	e := workspace.Environment{Name: a.profileName, Kind: c.Kind, Target: c.Target, Root: c.Root, Binary: c.Binary}
	if err := a.environments.SaveProfile(e); err != nil {
		a.report(err)
		return
	}
	a.saveEnvironments()
}
func (a *app) openEnvironment(e workspace.Environment) {
	a.environmentsOpen = false
	if e.Kind == "" {
		a.openProject(e.Root)
		return
	}
	a.connectEnvironment(core.Connection{Kind: e.Kind, Target: e.Target, Root: e.Root, Binary: e.Binary})
}

// environmentMenu is the dropdown of the title bar's environment button: the
// saved environments, and the ways to make a new one.
func (a *app) environmentMenu(c *ui.Context, p palette) {
	ui.Column(c).Width(300).Padding(6).Gap(2).Children(func() {
		if len(a.environments.Profiles) == 0 {
			ui.Text(c, "还没有保存的环境").FontSize(11).TextColor(p.muted).Padding(6, 8)
		}
		for _, profile := range a.environments.Profiles {
			profile := profile
			b := ui.ButtonBase(c).FillWidth().Height(38).Padding(0, 8).Gap(8).Radius(5).AlignItems(ui.Center).Label(profile.Label())
			if b.Hovered() {
				b.Background(p.edge)
			}
			b.Children(func() {
				ui.Icon(c, environmentIcon(profile.Kind)).Size(14, 14).TextColor(p.muted)
				ui.Column(c).Grow(1).MinWidth(0).Gap(1).Children(func() {
					ui.Text(c, profile.Label()).FontSize(12).TextColor(p.text).SingleLine().Ellipsis("…")
					detail := "本地"
					if profile.Kind != "" {
						detail = strings.ToUpper(profile.Kind) + " · " + profile.Target
					}
					ui.Text(c, detail).FontSize(10).TextColor(p.muted).SingleLine().Ellipsis("…")
				})
			})
			if b.Clicked() {
				a.envMenuOpen = false
				a.openEnvironment(profile)
			}
		}
		if len(a.environments.Profiles) > 0 {
			ui.Divider(c).Margin(4, 0)
		}
		a.environmentMenuItem(c, p, plusIcon, "连接新环境…", func() {
			a.envMenuOpen = false
			a.showConnectionPrompt()
		})
		a.environmentMenuItem(c, p, gearIcon, "管理工作环境…", func() {
			a.envMenuOpen = false
			a.environmentsOpen = true
		})
	})
}

// environmentMenuItem is a choice of the environment menu.
func (a *app) environmentMenuItem(c *ui.Context, p palette, icon *ui.SVG, text string, fn func()) {
	b := ui.ButtonBase(c).FillWidth().Height(28).Padding(0, 8).Gap(8).Radius(5).AlignItems(ui.Center).Label(text)
	if b.Hovered() {
		b.Background(p.edge)
	}
	b.Children(func() {
		ui.Icon(c, icon).Size(13, 13).TextColor(p.muted)
		ui.Text(c, text).FontSize(12).TextColor(p.text).Grow(1).SingleLine().Ellipsis("…")
	})
	if b.Clicked() {
		fn()
	}
}

// environmentIcon is the icon of a kind of environment.
func environmentIcon(kind string) *ui.SVG {
	switch kind {
	case "ssh":
		return globeIcon
	case "wsl", "docker":
		return monitorIcon
	default:
		return folderIcon
	}
}

func (a *app) environmentsDialog(c *ui.Context, p palette) {
	if !a.environmentsOpen {
		return
	}
	ui.Modal(c, &a.environmentsOpen, func() {
		ui.Column(c).Width(620).Padding(20).Gap(12).Children(func() {
			ui.Text(c, "工作环境与最近项目").FontSize(18).FontWeight(650)
			ui.Scroll(c).FillWidth().MaxHeight(440).Children(func() {
				ui.Column(c).FillWidth().Gap(5).Children(func() {
					ui.Text(c, "已保存的配置").FontSize(12).FontWeight(650).TextColor(p.muted)
					if len(a.environments.Profiles) == 0 {
						ui.Text(c, "在连接对话框中命名并保存，或收藏当前项目。").FontSize(12).TextColor(p.muted)
					}
					for _, profile := range a.environments.Profiles {
						profile := profile
						ui.Row(c).FillWidth().Gap(4).Children(func() {
							a.environmentRow(c, p, profile)
							if quiet(c, "编辑", false, p) {
								a.environmentsOpen = false
								if profile.Kind == "" {
									a.message = "本地项目可直接从列表打开。"
								} else {
									a.profileName = profile.Name
									a.connectionPrompt = &core.Connection{Kind: profile.Kind, Target: profile.Target, Root: profile.Root, Binary: profile.Binary}
								}
							}
							if quiet(c, "移除", false, p) {
								a.environments.RemoveProfile(profile.Name)
								a.saveEnvironments()
							}
						})
					}
					ui.Divider(c).Margin(6, 0)
					ui.Text(c, "最近项目").FontSize(12).FontWeight(650).TextColor(p.muted)
					if len(a.environments.Recent) == 0 {
						ui.Text(c, "还没有打开过项目").FontSize(12).TextColor(p.muted)
					}
					for _, recent := range a.environments.Recent {
						a.environmentRow(c, p, recent)
					}
				})
			})
			ui.Row(c).FillWidth().Gap(8).Children(func() {
				if quiet(c, "新建连接", false, p) {
					a.environmentsOpen = false
					a.showConnectionPrompt()
				}
				if a.root != "" && quiet(c, "收藏当前项目", false, p) {
					e := a.currentEnvironment()
					e.Name = e.Label()
					a.report(a.environments.SaveProfile(e))
					a.saveEnvironments()
				}
				ui.Spacer(c)
				if quiet(c, "清空最近记录", false, p) {
					a.environments.Recent = nil
					a.saveEnvironments()
				}
				if ui.Button(c, "关闭").Clicked() {
					a.environmentsOpen = false
				}
			})
		})
	})
}
func (a *app) environmentRow(c *ui.Context, p palette, e workspace.Environment) {
	b := ui.ButtonBase(c).Grow(1).FillWidth().MinWidth(0).Height(48).Padding(6, 8).Radius(5).Label(e.Label())
	if b.Hovered() {
		b.Background(p.selected)
	}
	b.Children(func() {
		ui.Column(c).FillWidth().Gap(3).Children(func() {
			ui.Text(c, e.Label()).FontSize(12).FontWeight(550).TextColor(p.text).SingleLine().Ellipsis("…")
			detail := "本地 · " + e.Root
			if e.Kind != "" {
				detail = strings.ToUpper(e.Kind) + " · " + e.Target + " · " + e.Root
			}
			ui.Text(c, detail).FontSize(10).TextColor(p.muted).SingleLine().Ellipsis("…")
		})
	})
	if b.Clicked() {
		a.openEnvironment(e)
	}
}
