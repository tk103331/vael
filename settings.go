package main

import (
	"strings"

	"github.com/egoist/mygo/ui"
	"github.com/egoist/mygo/ui/editor"
	"vael/internal/workspace"
)

// The parts of the settings: how the editor looks, how it edits, and the
// agents it starts.
const (
	settingsLook   = "look"
	settingsEditor = "editor"
	settingsEnv    = "env"
	settingsAgent  = "agent"
)

func (a *app) settingsView(c *ui.Context, p palette) {
	if !a.settingsOpen {
		return
	}
	if a.settingsSection == "" {
		a.settingsSection = settingsLook
	}
	open := true
	ui.Modal(c, &open, func() {
		ui.Column(c).Width(540).Padding(20).Gap(14).Children(func() {
			ui.Text(c, "设置").FontSize(17).FontWeight(650)
			ui.Row(c).FillWidth().Gap(2).Children(func() {
				a.settingsTab(c, p, "外观", settingsLook)
				a.settingsTab(c, p, "编辑器", settingsEditor)
				a.settingsTab(c, p, "环境", settingsEnv)
				a.settingsTab(c, p, "ACP Agent", settingsAgent)
			})
			ui.Divider(c)
			ui.Scroll(c).FillWidth().MaxHeight(460).Children(func() {
				ui.Column(c).FillWidth().Gap(2).Children(func() {
					switch a.settingsSection {
					case settingsEditor:
						a.editorSettings(c, p)
					case settingsEnv:
						a.environmentsSettings(c, p)
					case settingsAgent:
						a.agentsSettings(c, p)
					default:
						a.lookSettings(c, p)
					}
				})
			})
			ui.Row(c).FillWidth().Children(func() {
				ui.Text(c, "设置随会话保留。").FontSize(10.5).TextColor(p.muted).Grow(1)
				if ui.Button(c, "完成").Clicked() {
					open = false
				}
			})
		})
	})
	if !open {
		a.settingsOpen = false
	}
}

// settingsTab is a section of the settings.
func (a *app) settingsTab(c *ui.Context, p palette, label, section string) {
	active := a.settingsSection == section
	b := ui.ButtonBase(c).Height(26).Padding(0, 10).Radius(5).AlignItems(ui.Center).Label(label)
	if active {
		b.Background(p.selected)
	} else if b.Hovered() {
		b.Background(p.edge)
	}
	b.Children(func() {
		color := p.muted
		if active {
			color = p.text
		}
		ui.Text(c, label).FontSize(12).FontWeight(600).TextColor(color)
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
		a.settingsRow(c, p, "主题", "浅色或深色", func() {
			if quiet(c, "浅色", !a.dark, p) {
				a.dark = false
			}
			if quiet(c, "深色", a.dark, p) {
				a.dark = true
			}
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
	})
}

// agentsSettings is the ACP agents the editor starts: those it knows, and
// the ways to add one.
func (a *app) agentsSettings(c *ui.Context, p palette) {
	ui.Column(c).FillWidth().Gap(10).Children(func() {
		ui.Text(c, "Agent 通过 ACP（Agent Client Protocol）在你自己的机器上启动，"+
			"在你的项目里开会话：读写文件、运行命令都在本机完成。").
			FontSize(10.5).TextColor(p.muted)
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

// agentRegistryView offers the agents of the ACP registry.
func (a *app) agentRegistryView(c *ui.Context, p palette) {
	ui.Column(c).FillWidth().Padding(6).Gap(2).Background(p.wash).Radius(6).Children(func() {
		for _, agent := range acpRegistry {
			agent := agent
			installed := false
			for _, known := range a.agents.Items {
				if known.Name == agent.Name {
					installed = true
					break
				}
			}
			b := ui.ButtonBase(c).FillWidth().Height(34).Padding(0, 8).Gap(8).Radius(5).
				AlignItems(ui.Center).Label(agent.Name)
			if b.Hovered() {
				b.Background(p.edge)
			}
			b.Children(func() {
				ui.Column(c).Grow(1).MinWidth(0).Gap(1).Children(func() {
					ui.Text(c, agent.Name).FontSize(12).TextColor(p.text).SingleLine().Ellipsis("…")
					ui.Text(c, agent.Command).FontSize(9.5).TextColor(p.muted).
						Font("monospace").SingleLine().Ellipsis("…")
				})
				if installed {
					ui.Icon(c, checkIcon).Size(11, 11).TextColor(p.accent)
				} else {
					ui.Icon(c, plusIcon).Size(11, 11).TextColor(p.muted)
				}
			})
			if b.Clicked() && !installed {
				a.report(a.agents.Add(agent))
				if a.agents.Default == "" {
					a.agents.Default = agent.Name
				}
				a.saveAgents()
			}
		}
		ui.Text(c, "这些命令只是起点，接入后可在「手动添加」里改名或改命令。").
			FontSize(9.5).TextColor(p.muted).Padding(4, 6)
	})
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
