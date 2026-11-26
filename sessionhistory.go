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
		case a.agentConn == nil:
			ui.Text(c, "先启动 Agent 再查看会话历史。").
				FontSize(10).TextColor(p.muted).Padding(4, 4)
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
			now := time.Now()
			var pinned []acpSessionInfo
			for _, session := range a.agentSessions {
				if a.chatMetaOf(session.SessionID).Pinned {
					pinned = append(pinned, session)
				}
			}
			ui.Scroll(c).FillWidth().MaxHeight(340).Children(func() {
				ui.Column(c).FillWidth().Gap(1).Children(func() {
					if len(pinned) > 0 {
						a.agentHistoryHeader(c, p, "置顶", len(pinned))
						for _, session := range pinned {
							a.agentHistoryRow(c, p, session)
						}
					}
					for _, period := range chatPeriods {
						var group []acpSessionInfo
						for _, session := range a.agentSessions {
							if a.chatMetaOf(session.SessionID).Pinned {
								continue
							}
							if chatPeriod(session.UpdatedAt, now) == period {
								group = append(group, session)
							}
						}
						if len(group) == 0 {
							continue
						}
						a.agentHistoryHeader(c, p, period, len(group))
						for _, session := range group {
							a.agentHistoryRow(c, p, session)
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

// agentHistoryHeader is one group of the history list.
func (a *app) agentHistoryHeader(c *ui.Context, p palette, label string, count int) {
	ui.Row(c).FillWidth().Height(20).Padding(2, 6, 0, 6).
		AlignItems(ui.Center).Gap(4).Children(func() {
		ui.Text(c, label).FontSize(9.5).FontWeight(650).TextColor(p.muted).Grow(1)
		ui.Textf(c, "%d", count).FontSize(9).TextColor(p.muted).FontFeatures("tnum")
	})
}

// agentHistoryRow is one session of the history: its name, when it was
// written, whether it is open and pinned, and what a right click offers.
func (a *app) agentHistoryRow(c *ui.Context, p palette, session acpSessionInfo) {
	title := a.chatTitleOfSession(session)
	meta := a.chatMetaOf(session.SessionID)
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
		if meta.Pinned {
			ui.Icon(c, pinIcon).Size(10, 10).TextColor(p.warn)
		}
		if session.SessionID == a.agentSession {
			ui.Icon(c, checkIcon).Size(11, 11).TextColor(p.accent)
		}
	})
	b.ContextMenu(func(m *ui.Menu) {
		if m.Item("打开对话").Chosen() {
			a.agentHistoryOpen = false
			a.openHistorySession(session)
		}
		label := "置顶对话"
		if meta.Pinned {
			label = "取消置顶"
		}
		if m.Item(label).Chosen() {
			a.toggleChatPin(session.SessionID)
		}
		if m.Item("重命名…").Chosen() {
			a.chatRename = &chatRenamePrompt{session: session.SessionID, value: title}
		}
		if a.agentCanFork {
			if m.Item("分叉对话…").Chosen() {
				a.agentHistoryOpen = false
				a.forkAgentSession(session.SessionID)
			}
		}
	})
	if b.Clicked() {
		a.agentHistoryOpen = false
		a.openHistorySession(session)
	}
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
				a.agentSessionsLoaded = true
				return
			}
			a.agentSessionsLoaded = true
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
	a.ensureChats()
	tab := a.chatActive()
	a.chatSave()
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
			a.chatWithTab(tab, func() {
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
				if loaded.AvailableCommands != nil {
					a.agentCommands = loaded.AvailableCommands
				}
				a.agentStatus = "已连接"
				a.agentSessionsLoaded = false
				a.agentLines = append(a.agentLines, agentLine{Kind: "note", Text: "已加载会话：" + session.Title})
				a.followAgent()
			})
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
