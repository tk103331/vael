package main

import (
	"time"
)

// The agent panel keeps its conversations as tabs of its own. Past
// sessions come from the agent's session/list and open in a tab when
// chosen; the history list groups them by how long ago they were written.

// chatPeriods are the groups of the history list, newest first.
var chatPeriods = []string{"今天", "昨天", "最近 7 天", "最近 30 天", "更早"}

// chatPeriod is the group a time belongs to.
func chatPeriod(when string, now time.Time) string {
	at, err := time.Parse(time.RFC3339, when)
	if err != nil {
		return "更早"
	}
	at, now = at.In(now.Location()), now.In(now.Location())
	day := func(t time.Time) time.Time {
		return time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, t.Location())
	}
	switch days := int(day(now).Sub(day(at)).Hours() / 24); {
	case days <= 0:
		return "今天"
	case days == 1:
		return "昨天"
	case days < 7:
		return "最近 7 天"
	case days < 30:
		return "最近 30 天"
	}
	return "更早"
}

// openHistorySession opens a past session in a new tab of the agent
// panel, then loads it: the agent replays it over session/update.
func (a *app) openHistorySession(session acpSessionInfo) {
	a.ensureChats()
	if !a.agentCanLoad || a.agentConn == nil {
		a.message = "这个 Agent 不支持加载历史会话。"
		return
	}
	for _, tab := range a.chatTabs {
		if tab.state.session == session.SessionID {
			a.chatSelect(a.chatIndexOf(tab))
			return
		}
	}
	a.chatSave()
	title := session.Title
	if title == "" {
		title = shortSessionID(session.SessionID)
	}
	state := blankChatState()
	state.session, state.status = session.SessionID, "加载中…"
	updated := session.UpdatedAt
	if updated == "" {
		updated = time.Now().Format(time.RFC3339)
	}
	tab := &chatTab{id: a.nextChatID(), title: title, updated: updated, state: state}
	a.chatTabs = append(a.chatTabs, tab)
	a.chatActivate(len(a.chatTabs) - 1)
	a.rightPanel, a.rightSection = true, "agent"
	a.loadAgentSession(session)
}
