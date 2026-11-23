package main

import (
	"context"
	"sort"
	"strings"
	"time"

	"github.com/egoist/mygo/ui"
)

// The sessions an agent knows about, as ACP names them: the button in the
// bar lists them with session/list, and a click loads one with session/load
// if the agent supports it.
// https://agentclientprotocol.com/protocol/v1/session-list

// agentHistoryButton is the button of the sessions in the bar of the
// agent, with the list that opens under it.
func (a *app) agentHistoryButton(c *ui.Context, p palette) {
	wasOpen := a.agentHistoryOpen
	open := wasOpen
	b := ui.ButtonBase(c).Size(24, 24).Radius(4).Center().Label("会话历史")
	if open {
		b.Background(p.selected)
	} else if b.Hovered() {
		b.Background(p.edge)
	}
	b.Children(func() {
		ui.Icon(c, historyIcon).Size(13, 13).TextColor(p.muted)
	})
	if b.Clicked() {
		if open {
			a.agentHistoryOpen = false
		} else {
			a.agentHistoryOpen = true
			a.loadAgentSessions()
		}
	}
	ui.Popover(c, b, &open, func() { a.agentHistoryMenu(c, p) })
	if wasOpen && !open {
		a.agentHistoryOpen = false
	}
}

// agentHistoryMenu is the list of sessions the agent told us of.
func (a *app) agentHistoryMenu(c *ui.Context, p palette) {
	ui.Column(c).Width(340).Padding(6).Gap(2).Children(func() {
		ui.Row(c).FillWidth().AlignItems(ui.Center).Gap(6).Children(func() {
			ui.Text(c, "会话历史").FontSize(11.5).FontWeight(650).TextColor(p.text).Grow(1)
			if quiet(c, "刷新", false, p) {
				a.loadAgentSessions()
			}
		})
		switch {
		case !a.agentCanList:
			ui.Text(c, "这个 Agent 不支持列出会话（session/list）。").
				FontSize(10).TextColor(p.muted).Padding(4, 4)
		case a.agentHistoryLoading:
			ui.Text(c, "读取中…").FontSize(10).TextColor(p.muted).Padding(4, 4)
		case a.agentHistoryErr != "":
			ui.Text(c, a.agentHistoryErr).FontSize(10).TextColor(p.danger).Padding(4, 4)
		case len(a.agentSessions) == 0:
			ui.Text(c, "这个项目里还没有历史会话。").FontSize(10).TextColor(p.muted).Padding(4, 4)
		default:
			ui.Scroll(c).FillWidth().MaxHeight(300).Children(func() {
				ui.Column(c).FillWidth().Gap(1).Children(func() {
					for _, session := range a.agentSessions {
						session := session
						title := session.Title
						if strings.TrimSpace(title) == "" {
							title = shortSessionID(session.SessionID)
						}
						b := ui.ButtonBase(c).FillWidth().Height(38).Padding(0, 8).Gap(6).
							Radius(5).AlignItems(ui.Center).Label("打开 " + title)
						if b.Hovered() {
							b.Background(p.edge)
						}
						b.Children(func() {
							ui.Column(c).Grow(1).MinWidth(0).Gap(1).Children(func() {
								ui.Text(c, title).FontSize(11).TextColor(p.text).SingleLine().Ellipsis("…")
								ui.Text(c, shortSessionID(session.SessionID)+" · "+sessionWhen(session.UpdatedAt)).
									FontSize(9).TextColor(p.muted).SingleLine().Ellipsis("…")
							})
							if session.SessionID == a.agentSession {
								ui.Icon(c, checkIcon).Size(11, 11).TextColor(p.accent)
							}
						})
						if b.Clicked() {
							a.agentHistoryOpen = false
							a.loadAgentSession(session)
						}
					}
				})
			})
		}
		if !a.agentCanLoad {
			ui.Text(c, "这个 Agent 不支持加载历史会话。").FontSize(9.5).
				TextColor(p.muted).Padding(4, 2)
		}
	})
}

// loadAgentSessions asks the agent for the sessions of this project.
func (a *app) loadAgentSessions() {
	if !a.agentCanList || a.agentConn == nil {
		return
	}
	a.agentHistoryLoading = true
	a.agentHistoryErr = ""
	conn, run, cwd := a.agentConn, a.agentRun, a.root
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		var list acpSessionList
		err := conn.call(ctx, "session/list", map[string]any{"cwd": cwd}, &list)
		a.update(func() {
			if run != a.agentRun {
				return
			}
			a.agentHistoryLoading = false
			if err != nil {
				a.agentHistoryErr = "读取失败：" + err.Error()
				return
			}
			sessions := list.Sessions
			sort.SliceStable(sessions, func(i, j int) bool {
				return sessions[i].UpdatedAt > sessions[j].UpdatedAt
			})
			a.agentSessions = sessions
		})
	}()
}

// loadAgentSession brings a previous session back: the agent replays its
// conversation over session/update while it answers.
func (a *app) loadAgentSession(session acpSessionInfo) {
	if !a.agentCanLoad || a.agentConn == nil {
		a.message = "这个 Agent 不支持加载历史会话。"
		return
	}
	conn, run, cwd := a.agentConn, a.agentRun, a.root
	a.agentLoading = true
	a.agentLines = nil
	a.agentStatus = "加载会话中…"
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
		defer cancel()
		var loaded acpSession
		err := conn.call(ctx, "session/load", map[string]any{
			"sessionId":  session.SessionID,
			"cwd":        cwd,
			"mcpServers": []any{},
		}, &loaded)
		a.update(func() {
			if run != a.agentRun {
				return
			}
			a.agentLoading = false
			if err != nil {
				a.agentStatus = "加载失败"
				a.agentLines = append(a.agentLines, agentLine{Kind: "error", Text: err.Error()})
				return
			}
			if loaded.SessionID != "" {
				a.agentSession = loaded.SessionID
			} else {
				a.agentSession = session.SessionID
			}
			a.agentConfig = loaded.ConfigOptions
			a.agentModes = loaded.Modes.AvailableModes
			a.agentModeID = loaded.Modes.CurrentModeID
			a.agentStatus = "已连接"
			a.agentLines = append(a.agentLines, agentLine{Kind: "note", Text: "已加载会话：" + session.Title})
			a.followAgent()
		})
	}()
}

// shortSessionID is a session's ID in a row, without making it the whole
// row: sessions are long tokens.
func shortSessionID(id string) string {
	if len(id) <= 12 {
		return id
	}
	return id[:8] + "…" + id[len(id)-4:]
}

// sessionWhen is when a session was last written, in short words.
func sessionWhen(updated string) string {
	if updated == "" {
		return ""
	}
	if at, err := time.Parse(time.RFC3339, updated); err == nil {
		since := time.Since(at)
		switch {
		case since < time.Minute:
			return "刚刚"
		case since < time.Hour:
			return itoa(int(since.Minutes())) + " 分钟前"
		case since < 24*time.Hour:
			return itoa(int(since.Hours())) + " 小时前"
		case since < 7*24*time.Hour:
			return itoa(int(since.Hours()/24)) + " 天前"
		}
		return at.Format("2006-01-02")
	}
	if len(updated) > 16 {
		return strings.ReplaceAll(updated[:16], "T", " ")
	}
	return updated
}
