package main

import (
	"context"
	"strings"

	"github.com/egoist/mygo/ui"
)

// The session's selectors: the model, the mode, how much the agent
// thinks, and the other options it offers. They come with the session,
// change by session/set_config_option, and come back whole.
// https://agentclientprotocol.com/protocol/v1/session-config-options

// setAgentConfig asks the agent to change an option of the session. The
// answer holds the complete list, as one option may depend on another.
func (a *app) setAgentConfig(option acpConfigOption, value any) {
	if a.agentConn == nil || a.agentSession == "" {
		return
	}
	params := map[string]any{
		"sessionId": a.agentSession,
		"configId":  option.ID,
		"value":     value,
	}
	// The value of a switch is a boolean; that of a choice is an ID, as
	// the protocol's type says.
	if option.IsBoolean() {
		params["type"] = "boolean"
	} else {
		params["type"] = "id"
	}
	a.agentOptionOpen = ""
	conn, run := a.agentConn, a.agentRun
	go func() {
		var answer setConfigResult
		err := conn.call(context.Background(), "session/set_config_option", params, &answer)
		a.update(func() {
			if run != a.agentRun {
				return
			}
			if err != nil {
				a.agentLines = append(a.agentLines, agentLine{Kind: "error", Text: "设置失败：" + err.Error()})
				a.followAgent()
				return
			}
			if answer.ConfigOptions != nil {
				a.agentConfig = answer.ConfigOptions
			}
		})
	}()
}

// setAgentMode asks an agent of the older protocol, which names its modes
// rather than offering them as options, to work another way.
func (a *app) setAgentMode(modeID string) {
	if a.agentConn == nil || a.agentSession == "" || modeID == "" {
		return
	}
	a.agentOptionOpen = ""
	conn, run, session := a.agentConn, a.agentRun, a.agentSession
	go func() {
		err := conn.call(context.Background(), "session/set_mode", map[string]any{
			"sessionId": session,
			"modeId":    modeID,
		}, nil)
		a.update(func() {
			if run != a.agentRun {
				return
			}
			if err != nil {
				a.agentLines = append(a.agentLines, agentLine{Kind: "error", Text: "切换模式失败：" + err.Error()})
				a.followAgent()
				return
			}
			a.agentModeID = modeID
		})
	}()
}

// agentConfigBar is the row of selectors above the composer: the model
// first, then the mode and what else the agent offers.
func (a *app) agentConfigBar(c *ui.Context, p palette) {
	ui.Row(c).FillWidth().Wrap().Gap(4).Children(func() {
		shown := false
		for _, option := range agentConfigOrder(a.agentConfig) {
			option := option
			if !agentConfigShown(option) {
				continue
			}
			shown = true
			a.agentConfigChip(c, p, option)
		}
		// The protocol's modes are what an agent offers before session
		// config options: an agent that offers those has no chip of its
		// own for them, and showing both doubles what they name.
		if !shown && len(a.agentConfig) == 0 && len(a.agentModes) > 0 && a.agentModeID != "" {
			a.agentModeChip(c, p)
		}
	})
}

// agentConfigOrder is the options as the bar shows them: what chooses the
// model first, as the model is what one changes most, then the rest in
// the order the agent gave.
func agentConfigOrder(options []acpConfigOption) []acpConfigOption {
	ordered := make([]acpConfigOption, 0, len(options))
	for _, option := range options {
		if option.IsModel() {
			ordered = append(ordered, option)
		}
	}
	for _, option := range options {
		if !option.IsModel() {
			ordered = append(ordered, option)
		}
	}
	return ordered
}

// agentConfigShown tells whether an option is one the bar can show: a
// switch, or a choice the agent named. An option of a type this editor
// does not know is left to the agent's default.
func agentConfigShown(option acpConfigOption) bool {
	if option.ID == "" {
		return false
	}
	switch option.Type {
	case "", "select", "boolean":
		return true
	}
	return false
}

// agentConfigLabel names an option and the value it is on. A value the
// agent already names after the option, as "Thinking: medium", is not
// named twice.
func agentConfigLabel(option acpConfigOption) string {
	value := option.CurrentName()
	name := strings.TrimSpace(option.Name)
	if name == "" || value == "" {
		return value
	}
	lowerValue, lowerName := strings.ToLower(value), strings.ToLower(name)
	if strings.HasPrefix(lowerValue, lowerName+":") || strings.HasPrefix(lowerValue, lowerName+"：") {
		return value
	}
	return name + "：" + value
}

// agentConfigChip is one selector: its name and value, and a list of the
// values when it is clicked.
func (a *app) agentConfigChip(c *ui.Context, p palette, option acpConfigOption) {
	label := agentConfigLabel(option)
	open := a.agentOptionOpen == option.ID
	b := ui.ButtonBase(c).Height(22).Padding(0, 7).Radius(9).Gap(4).
		AlignItems(ui.Center).Label(label)
	if open {
		b.Background(p.selected)
	} else if b.Hovered() {
		b.Background(p.edge)
	}
	b.Children(func() {
		ui.Text(c, label).FontSize(10).TextColor(p.text).SingleLine().Ellipsis("…")
		if !option.IsBoolean() {
			ui.Icon(c, chevronDownIcon).Size(8, 8).TextColor(p.muted)
		}
	})
	b.Tooltip(agentConfigTooltip(option))
	if b.Clicked() {
		if option.IsBoolean() {
			a.setAgentConfig(option, !option.Bool())
			return
		}
		if open {
			a.agentOptionOpen = ""
		} else {
			a.agentOptionOpen = option.ID
		}
	}
	if option.IsBoolean() {
		return
	}
	wasOpen := open
	ui.Popover(c, b, &open, func() { a.agentConfigMenu(c, p, option) })
	// The window was open, and the user closed it by clicking away: the
	// click that opens it does not count as one.
	if wasOpen && !open {
		a.agentOptionOpen = ""
	}
}

// agentConfigMenu lists the values a selector takes, the one it is on
// marked.
func (a *app) agentConfigMenu(c *ui.Context, p palette, option acpConfigOption) {
	ui.Column(c).Width(300).Padding(6).Gap(2).Children(func() {
		ui.Text(c, option.Name).FontSize(10.5).FontWeight(650).TextColor(p.muted).Padding(4, 4)
		if option.Description != "" {
			ui.Text(c, option.Description).FontSize(9.5).TextColor(p.muted).Padding(4, 0).MaxWidth(280)
		}
		ui.Scroll(c).FillWidth().MaxHeight(260).Children(func() {
			ui.Column(c).FillWidth().Gap(1).Children(func() {
				for _, choice := range option.Options {
					choice := choice
					chosen := choice.Value == option.Value()
					row := ui.ButtonBase(c).FillWidth().Height(34).Padding(0, 8).Gap(6).
						Radius(5).AlignItems(ui.Center).Label("选择 " + choice.Name)
					if row.Hovered() {
						row.Background(p.edge)
					}
					row.Children(func() {
						ui.Column(c).Grow(1).MinWidth(0).Gap(1).Children(func() {
							ui.Text(c, choice.Name).FontSize(11.5).TextColor(p.text).SingleLine().Ellipsis("…")
							if choice.Description != "" {
								ui.Text(c, choice.Description).FontSize(9.5).TextColor(p.muted).
									SingleLine().Ellipsis("…")
							}
						})
						if chosen {
							ui.Icon(c, checkIcon).Size(11, 11).TextColor(p.accent)
						}
					})
					if row.Clicked() {
						a.setAgentConfig(option, choice.Value)
					}
				}
			})
		})
	})
}

// agentModeChip is the mode of an agent that names its modes rather than
// offering them as options.
func (a *app) agentModeChip(c *ui.Context, p palette) {
	label := "模式：" + a.agentModeName()
	open := a.agentOptionOpen == "mode"
	b := ui.ButtonBase(c).Height(22).Padding(0, 7).Radius(9).Gap(4).
		AlignItems(ui.Center).Label(label)
	if open {
		b.Background(p.selected)
	} else if b.Hovered() {
		b.Background(p.edge)
	}
	b.Children(func() {
		ui.Text(c, label).FontSize(10).TextColor(p.text).SingleLine().Ellipsis("…")
		ui.Icon(c, chevronDownIcon).Size(8, 8).TextColor(p.muted)
	})
	if b.Clicked() {
		if open {
			a.agentOptionOpen = ""
		} else {
			a.agentOptionOpen = "mode"
		}
	}
	wasOpen := open
	ui.Popover(c, b, &open, func() {
		ui.Column(c).Width(280).Padding(6).Gap(2).Children(func() {
			ui.Text(c, "工作模式").FontSize(10.5).FontWeight(650).TextColor(p.muted).Padding(4, 4)
			for _, mode := range a.agentModes {
				mode := mode
				chosen := mode.ID == a.agentModeID
				row := ui.ButtonBase(c).FillWidth().Height(34).Padding(0, 8).Gap(6).
					Radius(5).AlignItems(ui.Center).Label("模式 " + mode.Name)
				if row.Hovered() {
					row.Background(p.edge)
				}
				row.Children(func() {
					ui.Column(c).Grow(1).MinWidth(0).Gap(1).Children(func() {
						ui.Text(c, mode.Name).FontSize(11.5).TextColor(p.text).SingleLine().Ellipsis("…")
						if mode.Description != "" {
							ui.Text(c, mode.Description).FontSize(9.5).TextColor(p.muted).
								SingleLine().Ellipsis("…")
						}
					})
					if chosen {
						ui.Icon(c, checkIcon).Size(11, 11).TextColor(p.accent)
					}
				})
				if row.Clicked() {
					a.setAgentMode(mode.ID)
				}
			}
		})
	})
	if wasOpen && !open {
		a.agentOptionOpen = ""
	}
}

// agentModeName is the name of the mode the agent works in.
func (a *app) agentModeName() string {
	for _, mode := range a.agentModes {
		if mode.ID == a.agentModeID {
			if mode.Name != "" {
				return mode.Name
			}
			return mode.ID
		}
	}
	if a.agentModeID == "" {
		return "—"
	}
	return a.agentModeID
}

// agentConfigTooltip says of an option what its row cannot.
func agentConfigTooltip(option acpConfigOption) string {
	text := option.Name
	if option.Description != "" {
		text += "：" + option.Description
	}
	return text
}
