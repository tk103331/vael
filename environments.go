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
	a.settingsOpen = false
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
			a.settingsOpen = true
			a.settingsSection = settingsEnv
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

// environmentsSettings is the remote environments the editor opens
// projects in: those it saved, and the projects it opened lately.
func (a *app) environmentsSettings(c *ui.Context, p palette) {
	if !a.settingsCustomAllowed(settingsEnv) {
		return
	}
	ui.Column(c).FillWidth().Gap(10).Children(func() {
		ui.Text(c, "SSH、WSL 或 Docker：核心在目标环境里运行，文件、命令与终端都在那边。").
			FontSize(10.5).TextColor(p.muted)
		ui.Row(c).FillWidth().AlignItems(ui.Center).Gap(8).Children(func() {
			ui.Text(c, "已保存的环境").FontSize(12).FontWeight(650).TextColor(p.text).Grow(1)
			if quiet(c, "新建连接…", false, p) {
				a.settingsOpen = false
				a.showConnectionPrompt()
			}
		})
		if len(a.environments.Profiles) == 0 {
			ui.Text(c, "还没有保存的环境：连接时在对话框里命名并保存，或收藏当前环境。").
				FontSize(11).TextColor(p.muted).Padding(4, 2)
		}
		for _, profile := range a.environments.Profiles {
			profile := profile
			ui.Row(c).FillWidth().Gap(4).Children(func() {
				a.environmentRow(c, p, profile)
				if quiet(c, "编辑", false, p) {
					if profile.Kind == "" {
						a.message = "本地项目可直接从列表打开。"
					} else {
						a.settingsOpen = false
						a.profileName = profile.Name
						a.connectionPrompt = &core.Connection{
							Kind: profile.Kind, Target: profile.Target,
							Root: profile.Root, Binary: profile.Binary,
						}
					}
				}
				if quiet(c, "移除", false, p) {
					a.environments.RemoveProfile(profile.Name)
					a.saveEnvironments()
				}
			})
		}
		ui.Row(c).FillWidth().Children(func() {
			if a.root != "" && quiet(c, "收藏当前环境", false, p) {
				environment := a.currentEnvironment()
				environment.Name = environment.Label()
				a.report(a.environments.SaveProfile(environment))
				a.saveEnvironments()
			}
		})
		ui.Divider(c)
		ui.Row(c).FillWidth().AlignItems(ui.Center).Gap(8).Children(func() {
			ui.Text(c, "最近项目").FontSize(12).FontWeight(650).TextColor(p.text).Grow(1)
			if len(a.environments.Recent) > 0 && quiet(c, "清空", false, p) {
				a.environments.Recent = nil
				a.saveEnvironments()
			}
		})
		if len(a.environments.Recent) == 0 {
			ui.Text(c, "还没有打开过项目").FontSize(11).TextColor(p.muted).Padding(4, 2)
		}
		for _, recent := range a.environments.Recent {
			a.environmentRow(c, p, recent)
		}
	})
}

func (a *app) environmentRow(c *ui.Context, p palette, e workspace.Environment) {
	b := ui.ButtonBase(c).Grow(1).FillWidth().MinWidth(0).Height(48).Padding(6, 8).Radius(5).Label(e.Label())
	if b.Hovered() {
		b.Background(p.selected)
	}
	b.Children(func() {
		ui.Column(c).FillWidth().Gap(3).Children(func() {
			// A saved environment is known by its name, a project by its
			// path, which the line under adds what it is opened in.
			title := e.Name
			if title == "" {
				title = e.Root
			}
			detail := "本地"
			if e.Kind != "" {
				detail = strings.ToUpper(e.Kind) + " · " + e.Target
				if e.Name != "" {
					detail += " · " + e.Root
				}
			}
			ui.Text(c, title).FontSize(12).FontWeight(550).TextColor(p.text).SingleLine().Ellipsis("…")
			ui.Text(c, detail).FontSize(10).TextColor(p.muted).SingleLine().Ellipsis("…")
		})
	})
	if b.Clicked() {
		a.openEnvironment(e)
	}
}
