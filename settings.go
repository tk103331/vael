package main

import (
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"

	"github.com/egoist/mygo/ui"
	"github.com/egoist/mygo/ui/editor"
	"vael/internal/core"
	"vael/internal/workspace"
)

// The parts of the settings: how the editor looks, how it edits, and the
// agents it starts.
const (
	settingsLook   = "look"
	settingsEditor = "editor"
	settingsEnv    = "env"
	settingsLSP    = "lsp"
	settingsAgent  = "agent"
	settingsPlugin = "plugin"
	settingsAbout  = "about"
)

// settingsCustomAllowed tells whether a section whose settings are drawn
// by their own view fits the search the user typed.
func (a *app) settingsCustomAllowed(id string) bool {
	name, hint, _ := a.settingsPage(id)
	return a.settingsMatch(name, hint)
}

// resetSettingsSection puts the settings of a section back the way they
// were when Vael opened.
func (a *app) resetSettingsSection(id string) {
	switch id {
	case settingsLook:
		a.setTheme("vael-dark")
		a.iconID = "badges"
		a.fontFamily = "monospace"
		a.fontSize = DefaultFontSize
		a.lineHeight = 150
		a.activitySize = 28
	case settingsEditor:
		a.wrap = false
		a.guides, a.autoClose, a.numbers, a.minimap = true, true, true, true
		a.autoSave = autoSaveOff
		a.trimOnSave, a.finalNewline, a.formatOnSave = false, false, false
		a.tabSize = 0
		a.indent = int(editor.IndentAuto)
		a.whitespace = int(editor.WhitespaceSelection)
	}
	a.message, a.messageKind = "已恢复默认设置", "success"
}

// aboutSettings is what Vael is, and what it does with the user's work.
func (a *app) aboutSettings(c *ui.Context, p palette) {
	a.settingsRow(c, p, "Vael", "版本 "+core.Version+" · "+runtime.Version(), func() {})
	a.settingsRow(c, p, "隐私", "在本机运行：没有账号，没有遥测，也不上传你的使用数据。", func() {})
	a.settingsRow(c, p, "Agent 与插件", "通过 ACP 与本地插件工作；代码与对话只在你选择的工具之间流动。", func() {})
}

// settingsMatch tells whether a setting fits the search the user typed.
// An empty search fits everything.
func (a *app) settingsMatch(label, hint string) bool {
	query := strings.ToLower(strings.TrimSpace(a.settingsQuery))
	if query == "" {
		return true
	}
	return strings.Contains(strings.ToLower(label), query) ||
		strings.Contains(strings.ToLower(hint), query)
}

// settingsPage is the name, the description and the icon of a section.
func (a *app) settingsPage(id string) (string, string, *ui.SVG) {
	switch id {
	case settingsEditor:
		return "编辑器", "换行、缩进、字体与保存", codeIcon
	case settingsEnv:
		return "环境", "本机、SSH、WSL 与 Docker", globeIcon
	case settingsLSP:
		return "语言服务器", "诊断、补全、跳转与格式化", lspIcon
	case settingsAgent:
		return "ACP Agent", "接入在项目中工作的 Agent", agentIcon
	case settingsPlugin:
		return "插件", "插件商店、主题与图标主题", puzzleIcon
	case settingsAbout:
		return "关于", "版本与隐私", infoIcon
	}
	return "外观", "主题、图标与字号", sunIcon
}

func (a *app) settingsView(c *ui.Context, p palette) {
	if !a.settingsOpen {
		return
	}
	if a.settingsSection == "" {
		a.settingsSection = settingsLook
	}
	open := true
	a.settingsShown = 0
	ui.Modal(c, &open, func() {
		// The window keeps one height, whatever a section holds: the
		// room the content does not need is empty, and what it needs
		// past the room scrolls.
		ui.Column(c).Width(760).Height(560).Padding(20).Gap(12).Children(func() {
			ui.Row(c).FillWidth().AlignItems(ui.Center).Gap(10).Children(func() {
				ui.Text(c, "设置").FontSize(17).FontWeight(650)
				ui.Box(c).Grow(1)
				ui.TextInput(c, &a.settingsQuery).Width(240).Placeholder("搜索设置…").Label("搜索设置")
			})
			ui.Row(c).FillWidth().Grow(1).MinHeight(0).Gap(16).AlignItems(ui.Start).Children(func() {
				// The sections down the left, as an IDE's preferences, and
				// the one chosen beside them.
				ui.Column(c).Width(168).Shrink(0).FillHeight().Gap(2).Children(func() {
					for _, id := range []string{
						settingsLook, settingsEditor, settingsEnv,
						settingsLSP, settingsAgent, settingsPlugin, settingsAbout,
					} {
						label, hint, icon := a.settingsPage(id)
						if !a.settingsMatch(label, hint) && id != settingsLook && id != settingsEditor {
							// The rows of the look and editor pages are
							// filtered themselves, so their tabs stay.
							continue
						}
						a.settingsTab(c, p, label, id, icon)
					}
				})
				ui.Divider(c)
				ui.Column(c).Grow(1).MinWidth(0).FillHeight().Children(func() {
					name, hint, icon := a.settingsPage(a.settingsSection)
					ui.Row(c).FillWidth().AlignItems(ui.Center).Gap(8).Children(func() {
						ui.Icon(c, icon).Size(16, 16).TextColor(p.accent)
						ui.Column(c).Grow(1).MinWidth(0).Gap(1).Children(func() {
							ui.Text(c, name).FontSize(13).FontWeight(650).TextColor(p.text)
							ui.Text(c, hint).FontSize(10).TextColor(p.muted).SingleLine().Ellipsis("…")
						})
						switch a.settingsSection {
						case settingsLook, settingsEditor:
							if quiet(c, "恢复默认", false, p) {
								a.resetSettingsSection(a.settingsSection)
							}
						}
					})
					ui.Scroll(c).FillWidth().FillHeight().Children(func() {
						ui.Column(c).FillWidth().Gap(2).Children(func() {
							switch a.settingsSection {
							case settingsEditor:
								a.editorSettings(c, p)
							case settingsEnv:
								a.environmentsSettings(c, p)
							case settingsLSP:
								a.lspSettingsView(c, p)
							case settingsAgent:
								a.agentsSettings(c, p)
							case settingsPlugin:
								a.pluginSettings(c, p)
							case settingsAbout:
								a.aboutSettings(c, p)
							default:
								a.lookSettings(c, p)
							}
							if a.settingsQuery != "" && a.settingsShown == 0 {
								ui.Text(c, "没有匹配的设置。").FontSize(11.5).TextColor(p.muted).Padding(10, 8)
							}
						})
					})
				})
			})
			ui.Divider(c)
			ui.Row(c).FillWidth().AlignItems(ui.Center).Gap(8).Children(func() {
				ui.Text(c, "设置随会话保留。").FontSize(10.5).TextColor(p.muted).Grow(1)
				if quiet(c, "打开配置目录", false, p) {
					if dir, err := os.UserConfigDir(); err == nil {
						mygoShellOpenPath(filepath.Join(dir, "vael"))
					}
				}
				if ui.Button(c, "完成").Clicked() {
					open = false
				}
			})
		})
	})
	if !open {
		if a.settingsOpen {
			a.applyLSPSettings()
		}
		a.settingsOpen = false
	}
}

// settingsBadge is the count beside a section of the settings: how many
// servers, agents or plugins it holds.
func (a *app) settingsBadge(section string) string {
	var count int
	switch section {
	case settingsLSP:
		count = len(a.lspOn)
	case settingsAgent:
		count = len(a.agents.Items)
	case settingsPlugin:
		count = len(a.plugins)
	}
	if count == 0 {
		return ""
	}
	return strconv.Itoa(count)
}

// settingsTab is a section of the settings, down the left of it: an icon,
// its name, and the way it looks when it is the one showing.
func (a *app) settingsTab(c *ui.Context, p palette, label, section string, icon *ui.SVG) {
	active := a.settingsSection == section
	b := ui.ButtonBase(c).FillWidth().Height(28).Padding(0, 8).Radius(5).Gap(7).
		AlignItems(ui.Center).Label(label)
	if active {
		b.Background(p.selected)
	} else if b.Hovered() {
		b.Background(p.edge)
	}
	b.Children(func() {
		color := p.muted
		if active {
			color = p.accent
		}
		ui.Icon(c, icon).Size(13, 13).TextColor(color)
		nameColor := p.text
		if !active {
			nameColor = p.muted
		}
		ui.Text(c, label).FontSize(12).FontWeight(600).TextColor(nameColor).
			SingleLine().Ellipsis("…")
		if count := a.settingsBadge(section); count != "" {
			ui.Text(c, count).FontSize(9).FontFeatures("tnum").TextColor(p.muted)
		}
	})
	if b.Clicked() {
		a.settingsSection = section
		a.agentAddOpen = false
		a.agentRegistryOpen = false
	}
}

// settingsRow is one setting: what it is, what it does, and the control
// that changes it.
func (a *app) settingsRow(c *ui.Context, p palette, label, hint string, control func()) {
	if !a.settingsMatch(label, hint) {
		return
	}
	a.settingsShown++
	ui.Row(c).FillWidth().Padding(6, 4).Gap(10).AlignItems(ui.Center).Children(func() {
		ui.Column(c).Grow(1).MinWidth(0).Gap(2).Children(func() {
			ui.Text(c, label).FontSize(12.5).TextColor(p.text).SingleLine().Ellipsis("…")
			if hint != "" {
				ui.Text(c, hint).FontSize(10.5).TextColor(p.muted).SingleLine().Ellipsis("…")
			}
		})
		control()
	})
}

// settingsSwitch is a setting that is on or off.
func (a *app) settingsSwitch(c *ui.Context, p palette, label, hint string, on *bool) {
	a.settingsRow(c, p, label, hint, func() {
		// The switch is named apart from the words of its row, which
		// assistive technology reads as the setting it changes.
		ui.Switch(c, on).Label(label + "开关")
	})
}

// settingsChoice is a setting that takes one of a few values.
func (a *app) settingsChoice(c *ui.Context, p palette, label, hint string, options []settingsOption, value *int) {
	a.settingsRow(c, p, label, hint, func() {
		ui.Row(c).Gap(2).Children(func() {
			for _, option := range options {
				option := option
				chosen := *value == option.value
				b := ui.ButtonBase(c).Height(24).Padding(0, 9).Radius(5).AlignItems(ui.Center).
					Label(label + "：" + option.label)
				if chosen {
					b.Background(p.selected)
				} else if b.Hovered() {
					b.Background(p.edge)
				}
				b.Children(func() {
					color := p.muted
					if chosen {
						color = p.text
					}
					ui.Text(c, option.label).FontSize(11.5).TextColor(color)
				})
				if b.Clicked() {
					*value = option.value
				}
			}
		})
	})
}

// settingsOption is a value a setting can take.
type settingsOption struct {
	label string
	value int
}

// lookSettings is how the editor looks: its theme, its text, and how much
// room its lines take.
func (a *app) lookSettings(c *ui.Context, p palette) {
	ui.Column(c).FillWidth().Gap(2).Children(func() {
		// The themes, each as a swatch of its own colours.
		if a.settingsMatch("主题", "界面与编辑器的配色") {
			ui.Row(c).FillWidth().Padding(6, 4).Gap(8).AlignItems(ui.Center).Children(func() {
				ui.Column(c).Grow(1).MinWidth(0).Gap(2).Children(func() {
					ui.Text(c, "主题").FontSize(12.5).TextColor(p.text)
					ui.Text(c, "界面与编辑器的配色").FontSize(10.5).TextColor(p.muted)
				})
			})
		}
		if a.settingsMatch("主题", "界面与编辑器的配色") {
			a.settingsShown++
			ui.Box(c).FillWidth().Padding(0, 4).Children(func() {
				ui.Row(c).FillWidth().Wrap().Gap(6).Children(func() {
					for _, theme := range append(append([]uiTheme(nil), uiThemes...), a.pluginThemes...) {
						theme := theme
						a.themeSwatch(c, p, theme)
					}
				})
			})
		}
		a.settingsChoice(c, p, "图标栏大小", "左侧与右侧图标栏的按钮大小", []settingsOption{
			{label: "小", value: 24}, {label: "标准", value: 28}, {label: "大", value: 32},
		}, &a.activitySize)
		// The icon theme: the marks of the files of the tree and the tabs.
		a.settingsRow(c, p, "图标主题", "文件树与标签页的标记", func() {
			for _, icons := range pluginIconChoices(a) {
				icons := icons
				chosen := a.iconThemeFor() == icons.ID
				b := ui.ButtonBase(c).Height(24).Padding(0, 9).Radius(5).AlignItems(ui.Center).
					Label("图标主题：" + icons.Name)
				if chosen {
					b.Background(p.selected)
				} else if b.Hovered() {
					b.Background(p.edge)
				}
				b.Children(func() {
					color := p.muted
					if chosen {
						color = p.text
					}
					ui.Text(c, icons.Name).FontSize(11.5).TextColor(color)
				})
				if b.Clicked() {
					a.iconID = icons.ID
				}
			}
		})
		a.settingsRow(c, p, "字体", "编辑器的等宽字体", func() {
			b := ui.ButtonBase(c).Height(24).Padding(0, 9).Radius(5).Gap(4).
				AlignItems(ui.Center).Label("字体：" + a.fontName())
			if a.fontMenuOpen {
				b.Background(p.selected)
			} else if b.Hovered() {
				b.Background(p.edge)
			}
			b.Children(func() {
				ui.Text(c, a.fontName()).FontSize(11.5).TextColor(p.text).SingleLine().Ellipsis("…")
				ui.Icon(c, chevronDownIcon).Size(9, 9).TextColor(p.muted)
			})
			if b.Clicked() {
				a.fontMenuOpen = !a.fontMenuOpen
				a.fontQuery = ""
				if a.fontMenuOpen {
					// The families of the system are asked for once, and
					// only when the list shows: it takes a moment.
					a.monospacedFonts()
				}
			}
			ui.Popover(c, b, &a.fontMenuOpen, func() { a.fontMenu(c, p) })
		})
		a.settingsRow(c, p, "字号", "编辑器的字号，9 到 24", func() {
			if iconButton(c, minusIcon, "缩小字号", false, p) {
				a.zoom(-1)
			}
			size := a.fontSize
			if size == 0 {
				size = DefaultFontSize
			}
			ui.Textf(c, "%d", int(size)).FontSize(13).FontWeight(650).TextColor(p.text).
				FontFeatures("tnum").Width(26).SingleLine().TextAlign(ui.Center)
			if iconButton(c, plusIcon, "放大字号", false, p) {
				a.zoom(1)
			}
		})
		a.settingsRow(c, p, "行高", "行与行之间的距离", func() {
			for _, option := range []struct {
				label string
				value float32
			}{{"紧凑", 120}, {"标准", 150}, {"宽松", 180}} {
				option := option
				chosen := a.lineHeightValue() == option.value
				b := ui.ButtonBase(c).Height(24).Padding(0, 9).Radius(5).AlignItems(ui.Center).
					Label("行高：" + option.label)
				if chosen {
					b.Background(p.selected)
				} else if b.Hovered() {
					b.Background(p.edge)
				}
				b.Children(func() {
					color := p.muted
					if chosen {
						color = p.text
					}
					ui.Text(c, option.label).FontSize(11.5).TextColor(color)
				})
				if b.Clicked() {
					a.lineHeight = option.value
				}
			}
		})
	})
}

// lineHeightValue is the height of a line as a percentage, 150 when the
// editor's own.
func (a *app) lineHeightValue() float32 {
	if a.lineHeight == 0 {
		return 150
	}
	return a.lineHeight
}

// editorSettings is how the editor edits: wrapping, indentation, and what
// it shows.
func (a *app) editorSettings(c *ui.Context, p palette) {
	ui.Column(c).FillWidth().Gap(2).Children(func() {
		a.settingsSwitch(c, p, "自动换行", "长行在窗口宽度处折行", &a.wrap)
		a.settingsChoice(c, p, "Tab 宽度", "一个制表符与一级缩进占几列", []settingsOption{
			{"跟随语言", 0}, {"2", 2}, {"4", 4}, {"8", 8},
		}, &a.tabSize)
		a.settingsChoice(c, p, "缩进", "按 Tab 时插入什么", []settingsOption{
			{"跟随语言", int(editor.IndentAuto)},
			{"空格", int(editor.IndentSpaces)},
			{"制表符", int(editor.IndentTabs)},
		}, &a.indent)
		a.settingsChoice(c, p, "显示空白", "空格与制表符何时显示为点与箭头", []settingsOption{
			{"选中时", int(editor.WhitespaceSelection)},
			{"从不", int(editor.WhitespaceNone)},
			{"总是", int(editor.WhitespaceAll)},
			{"行尾", int(editor.WhitespaceTrailing)},
		}, &a.whitespace)
		a.settingsSwitch(c, p, "缩进参考线", "用竖线标出缩进层级", &a.guides)
		a.settingsSwitch(c, p, "自动闭合括号", "输入括号或引号时成对插入", &a.autoClose)
		a.settingsSwitch(c, p, "行号", "在左侧显示行号", &a.numbers)
		a.settingsSwitch(c, p, "缩略图", "在编辑器右侧显示整份文件的缩略图，可点击或拖动跳转", &a.minimap)
		a.savingSettings(c, p)
	})
}

// agentsSettings is the ACP agents the editor starts: those it knows, and
// the ways to add one.
func (a *app) agentsSettings(c *ui.Context, p palette) {
	if !a.settingsCustomAllowed(settingsAgent) {
		return
	}
	ui.Column(c).FillWidth().Gap(10).Children(func() {
		ui.Text(c, "Agent 通过 ACP（Agent Client Protocol）在你自己的机器上启动，"+
			"在你的项目里开会话：读写文件、运行命令都在本机完成。").
			FontSize(10.5).TextColor(p.muted)
		a.settingsSwitch(c, p, "Agent 完成时通知", "窗口不在前面时，用系统通知告诉你这一轮结束了", &a.notifyAgent)
		ui.Divider(c)
		if len(a.agents.Items) == 0 {
			ui.Text(c, "还没有接入 Agent。从下面的 Registry 选一个，或自己填启动命令。").
				FontSize(11.5).TextColor(p.muted).Padding(4, 2)
		}
		for _, agent := range a.agents.Items {
			agent := agent
			ui.Row(c).FillWidth().Gap(4).Children(func() {
				a.agentSettingsRow(c, p, agent)
			})
		}
		ui.Divider(c)
		ui.Row(c).FillWidth().Gap(6).Children(func() {
			if quiet(c, "从 ACP Registry 添加…", false, p) {
				a.agentRegistryOpen = !a.agentRegistryOpen
				a.agentAddOpen = false
				if a.agentRegistryOpen {
					a.loadRegistry(false)
				}
			}
			if quiet(c, "手动添加…", false, p) {
				a.agentAddOpen = !a.agentAddOpen
				a.agentRegistryOpen = false
				if a.agentAddOpen {
					a.agentAddName, a.agentAddCommand = "", ""
				}
			}
		})
		if a.agentRegistryOpen {
			a.agentRegistryView(c, p)
		}
		if a.agentAddOpen {
			a.agentAddView(c, p)
		}
	})
}

// agentSettingsRow is an agent of the list, with what it takes to choose
// it and to take it away.
func (a *app) agentSettingsRow(c *ui.Context, p palette, agent workspace.Agent) {
	chosen := agent.Name == a.agents.Default
	b := ui.ButtonBase(c).Grow(1).MinWidth(0).Height(40).Padding(0, 8).Gap(8).Radius(5).
		AlignItems(ui.Center).Label(agent.Name)
	if b.Hovered() {
		b.Background(p.edge)
	}
	b.Children(func() {
		if svg := a.agentSVG(agent); svg != nil {
			ui.Icon(c, svg).Size(14, 14).TextColor(p.muted)
		}
		ui.Column(c).Grow(1).MinWidth(0).Gap(2).Children(func() {
			ui.Text(c, agent.Name).FontSize(12).TextColor(p.text).SingleLine().Ellipsis("…")
			ui.Text(c, agent.Command).FontSize(10).TextColor(p.muted).
				Font("monospace").SingleLine().Ellipsis("…")
		})
		if chosen {
			ui.Icon(c, checkIcon).Size(11, 11).TextColor(p.accent)
		}
	})
	if b.Clicked() {
		a.agents.Default = agent.Name
		a.agentChosen = agent.Name
		a.saveAgents()
	}
	if smallIconButton(c, xIcon, "移除 "+agent.Name, p) {
		if a.agents.Default == agent.Name {
			a.agents.Default = ""
		}
		if a.agentChosen == agent.Name {
			a.agentChosen = ""
		}
		a.agents.Remove(agent.Name)
		a.saveAgents()
	}
}

// agentRegistryView offers the agents of the ACP registry: the index is
// fetched from its site, and each agent is added as its npx or uvx package
// or as the binary for this platform.
func (a *app) agentRegistryView(c *ui.Context, p palette) {
	ui.Column(c).FillWidth().Padding(6).Gap(4).Background(p.wash).Radius(6).Children(func() {
		ui.Row(c).FillWidth().AlignItems(ui.Center).Gap(6).Children(func() {
			ui.Text(c, "ACP Registry").FontSize(11.5).FontWeight(650).TextColor(p.text).Grow(1)
			switch {
			case a.registryLoading:
				ui.Text(c, "读取中…").FontSize(10).TextColor(p.muted)
			case len(a.registry) > 0:
				ui.Textf(c, "%d 个 Agent", len(a.registry)).FontSize(10).TextColor(p.muted)
			}
			if quiet(c, "刷新", false, p) {
				a.loadRegistry(true)
			}
		})
		if a.registryErr != "" {
			ui.Row(c).FillWidth().AlignItems(ui.Center).Gap(6).Children(func() {
				ui.Text(c, a.registryErr).FontSize(10).TextColor(p.danger).Grow(1).SingleLine().Ellipsis("…")
				if quiet(c, "重试", false, p) {
					a.loadRegistry(true)
				}
			})
		}
		if len(a.registry) > 0 {
			ui.TextInput(c, &a.registryQuery).FillWidth().Height(26).Padding(2, 6).
				Placeholder("搜索名字或描述…").Label("搜索 ACP Registry")
			matches := a.registryMatches()
			ui.Scroll(c).FillWidth().MaxHeight(300).Children(func() {
				ui.Column(c).FillWidth().Gap(1).Children(func() {
					for _, agent := range matches {
						agent := agent
						a.registryRow(c, p, agent)
					}
				})
			})
			if len(matches) == 0 {
				ui.Text(c, "没有匹配的 Agent。").FontSize(10.5).TextColor(p.muted)
			}
			ui.Text(c, "npx／uvx 直接以包启动，二进制会下载到配置目录；点一个即加入。").
				FontSize(9.5).TextColor(p.muted).Padding(4, 2)
		}
	})
}

// registryRow is an agent of the registry: its name, its description and
// how it would be added here.
func (a *app) registryRow(c *ui.Context, p palette, agent acpRegistryAgent) {
	installed := a.agents.Has(agent.Name)
	kind := acpRegistryKind(agent, runtime.GOOS, runtime.GOARCH, a.hasRegistryTool)
	status := a.registryStatus[agent.ID]
	b := ui.ButtonBase(c).FillWidth().Height(40).Padding(0, 8).Gap(8).Radius(5).
		AlignItems(ui.Center).Label(agent.Name)
	if b.Hovered() && !installed {
		b.Background(p.edge)
	}
	b.Children(func() {
		if svg := a.agentSVG(workspace.Agent{Icon: agent.Icon}); svg != nil {
			ui.Icon(c, svg).Size(14, 14).TextColor(p.muted)
		}
		ui.Column(c).Grow(1).MinWidth(0).Gap(1).Children(func() {
			ui.Row(c).FillWidth().Gap(6).AlignItems(ui.Center).Children(func() {
				ui.Text(c, agent.Name).FontSize(12).TextColor(p.text).SingleLine().Ellipsis("…")
				ui.Text(c, "v"+agent.Version).FontSize(9).TextColor(p.muted)
			})
			ui.Text(c, registryDescription(agent)).FontSize(9.5).TextColor(p.muted).
				SingleLine().Ellipsis("…")
		})
		switch {
		case installed:
			ui.Icon(c, checkIcon).Size(11, 11).TextColor(p.accent)
		case status != "":
			ui.Text(c, status).FontSize(9.5).TextColor(p.muted).SingleLine().Ellipsis("…")
		default:
			ui.Text(c, registryKindLabel(kind)).FontSize(9.5).TextColor(p.muted)
			ui.Icon(c, plusIcon).Size(10, 10).TextColor(p.muted)
		}
	})
	if b.Clicked() && !installed && status != "下载中…" {
		a.addRegistryAgent(agent, kind)
	}
	b.Tooltip(registryTooltip(agent, kind))
}

// registryDescription is what a row says of an agent: what it does, or
// where it comes from when it says nothing.
func registryDescription(agent acpRegistryAgent) string {
	if agent.Description != "" {
		return agent.Description
	}
	if agent.Repository != "" {
		return agent.Repository
	}
	return agent.ID
}

// registryKindLabel names how an agent is added.
func registryKindLabel(kind string) string {
	switch kind {
	case "npx", "uvx":
		return kind
	case "binary":
		return "二进制"
	}
	return "此平台无分发"
}

// registryTooltip is the longer word of a row: the package or binary, and
// what it runs.
func registryTooltip(agent acpRegistryAgent, kind string) string {
	switch kind {
	case "npx":
		return acpPackageCommand("npx", agent.Distribution.Npx)
	case "uvx":
		return acpPackageCommand("uvx", agent.Distribution.Uvx)
	case "binary":
		target, platform, ok := acpBinaryFor(agent, runtime.GOOS, runtime.GOARCH)
		if ok {
			return platform + " · " + target.Archive
		}
	}
	return registryDescription(agent)
}

// registryMatches are the agents of the index that hold what is searched.
func (a *app) registryMatches() []acpRegistryAgent {
	query := strings.ToLower(strings.TrimSpace(a.registryQuery))
	if query == "" {
		return a.registry
	}
	var matches []acpRegistryAgent
	for _, agent := range a.registry {
		if strings.Contains(strings.ToLower(agent.Name), query) ||
			strings.Contains(strings.ToLower(agent.ID), query) ||
			strings.Contains(strings.ToLower(agent.Description), query) {
			matches = append(matches, agent)
		}
	}
	return matches
}

// agentAddView is the form that adds an agent of the user's own.
func (a *app) agentAddView(c *ui.Context, p palette) {
	ui.Column(c).FillWidth().Padding(8).Gap(8).Background(p.wash).Radius(6).Children(func() {
		name := ui.TextInput(c, &a.agentAddName).FillWidth().Height(28).Padding(2, 6).
			Placeholder("名称，如 Claude Code").Label("Agent 名称")
		command := ui.TextInput(c, &a.agentAddCommand).FillWidth().Height(28).Padding(2, 6).
			Placeholder("启动命令，如 npx -y @zed-industries/claude-code-acp").
			Label("启动命令")
		submitted := name.Submitted() || command.Submitted()
		ui.Row(c).FillWidth().Gap(6).AlignItems(ui.Center).Children(func() {
			ui.Text(c, "命令在项目目录中执行，标准输入输出讲 ACP。").
				FontSize(9.5).TextColor(p.muted).Grow(1)
			if ui.PrimaryButton(c, "添加").Clicked() || submitted {
				a.addAgentFromForm()
			}
		})
	})
}

// addAgentFromForm puts what the form holds among the agents.
func (a *app) addAgentFromForm() {
	name := strings.TrimSpace(a.agentAddName)
	command := strings.TrimSpace(a.agentAddCommand)
	if name == "" || command == "" {
		a.message = "请填写 Agent 名称与启动命令。"
		return
	}
	if err := a.agents.Add(workspace.Agent{Name: name, Command: command}); err != nil {
		a.report(err)
		return
	}
	if a.agents.Default == "" {
		a.agents.Default = name
	}
	a.agentChosen = name
	a.agentAddOpen = false
	a.agentAddName, a.agentAddCommand = "", ""
	a.saveAgents()
}
