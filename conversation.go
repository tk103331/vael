package main

import (
	"strings"
	"time"

	"vael/internal/workspace"
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

// chatMetaOf is what the editor remembers of a session; a session it has
// not seen again gives the zero value.
func (a *app) chatMetaOf(session string) workspace.Chat {
	if session == "" {
		return workspace.Chat{}
	}
	return a.chats[session]
}

// setChatMeta remembers a name or a pin of a session, and writes them
// down.
func (a *app) setChatMeta(session string, meta workspace.Chat) {
	if session == "" {
		return
	}
	if a.chats == nil {
		a.chats = workspace.Chats{}
	}
	if meta.Title == "" && !meta.Pinned {
		delete(a.chats, session)
	} else {
		a.chats[session] = meta
	}
	if a.chatsPath != "" {
		a.report(workspace.SaveChats(a.chatsPath, a.chats))
	}
}

// chatTitleOfSession is the name the history shows: the user's, the
// agent's, or the short ID.
func (a *app) chatTitleOfSession(session acpSessionInfo) string {
	if title := strings.TrimSpace(a.chatMetaOf(session.SessionID).Title); title != "" {
		return title
	}
	if title := strings.TrimSpace(session.Title); title != "" {
		return title
	}
	return shortSessionID(session.SessionID)
}

// toggleChatPin pins a conversation above the others in the history, or
// takes the pin back.
func (a *app) toggleChatPin(session string) {
	meta := a.chatMetaOf(session)
	meta.Pinned = !meta.Pinned
	a.setChatMeta(session, meta)
	a.sortChatTabs()
}

// sortChatTabs puts the pinned conversations first, as the history does,
// and keeps the chosen tab chosen.
func (a *app) sortChatTabs() {
	active := a.chatActive()
	var pinned, rest []*chatTab
	for _, tab := range a.chatTabs {
		if a.chatMetaOf(tab.state.session).Pinned {
			pinned = append(pinned, tab)
		} else {
			rest = append(rest, tab)
		}
	}
	a.chatTabs = append(pinned, rest...)
	if i := a.chatIndexOf(active); i >= 0 {
		a.chatSel = i
	}
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
	titled := strings.TrimSpace(title) != ""
	meta := a.chatMetaOf(session.SessionID)
	if meta.Title != "" {
		title, titled = meta.Title, true
	}
	if title == "" {
		title = shortSessionID(session.SessionID)
	}
	state := blankChatState()
	state.session, state.status = session.SessionID, "加载中…"
	updated := session.UpdatedAt
	if updated == "" {
		updated = time.Now().Format(time.RFC3339)
	}
	tab := &chatTab{id: a.nextChatID(), title: title, titled: titled, renamed: meta.Title != "",
		updated: updated, state: state}
	a.chatTabs = append(a.chatTabs, tab)
	a.sortChatTabs()
	a.chatActivate(a.chatIndexOf(tab))
	a.rightPanel, a.rightSection = true, "agent"
	a.loadAgentSession(session)
}
