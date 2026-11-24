package main

import (
	"fmt"
	"strings"
	"time"

	"github.com/egoist/mygo/ui"
)

// Conversations live in tabs: the panel shows the chosen one, and the
// full-window conversation mode shows them in a strip like the editor's
// files. The fields on app hold the chosen tab's state, so the agent code
// keeps working unchanged; switching a tab saves and loads them together.

type chatTab struct {
	id      string
	title   string
	updated string
	state   chatState
}

type chatState struct {
	session, status             string
	lines                       []agentLine
	input                       string
	busy                        bool
	pending                     []acpPending
	plan                        []acpPlanEntry
	planOpen                    bool
	config                      []acpConfigOption
	modes                       []acpMode
	modeID                      string
	sessions                    []acpSessionInfo
	historyOpen, historyLoading bool
	historyErr                  string
	optionOpen                  string
	scroll                      ui.ScrollState
	allowed, denied             map[string]string
	loading                     bool
	commands                    []acpAvailableCommand
	refs                        []agentRef
	command                     string
	mentionOpen                 bool
	mentionQuery                string
	mentionDir                  bool
	dismissed                   string
	caret                       int
	caretPending                int
}

func (a *app) ensureChats() {
	if len(a.chatTabs) > 0 {
		if a.chatSel < 0 || a.chatSel >= len(a.chatTabs) {
			a.chatSel = len(a.chatTabs) - 1
		}
		return
	}
	tab := &chatTab{id: a.nextChatID(), title: "新对话",
		updated: time.Now().Format(time.RFC3339), state: a.chatState()}
	a.chatTabs = []*chatTab{tab}
	a.chatSel = 0
	a.chatSyncTitle()
}

func (a *app) nextChatID() string {
	a.chatSeq++
	return fmt.Sprintf("chat-%d", a.chatSeq)
}

func (a *app) chatState() chatState {
	return chatState{
		session: a.agentSession, status: a.agentStatus, lines: a.agentLines,
		input: a.agentInput, busy: a.agentBusy, pending: a.agentPending,
		plan: a.agentPlan, planOpen: a.agentPlanOpen, config: a.agentConfig,
		modes: a.agentModes, modeID: a.agentModeID, sessions: a.agentSessions,
		historyOpen: a.agentHistoryOpen, historyLoading: a.agentHistoryLoading,
		historyErr: a.agentHistoryErr, optionOpen: a.agentOptionOpen,
		scroll: a.agentScroll, allowed: a.agentAllowed, denied: a.agentDenied,
		loading: a.agentLoading, commands: a.agentCommands, refs: a.agentRefs,
		command: a.agentCommand, mentionOpen: a.agentMentionOpen,
		mentionQuery: a.agentMentionQuery, mentionDir: a.agentMentionDir,
		dismissed: a.agentMentionDismissed, caret: a.agentCaret,
		caretPending: a.agentCaretPending,
	}
}

func (a *app) setChatState(s chatState) {
	a.agentSession, a.agentStatus, a.agentLines = s.session, s.status, s.lines
	a.agentInput, a.agentBusy, a.agentPending = s.input, s.busy, s.pending
	a.agentPlan, a.agentPlanOpen, a.agentConfig = s.plan, s.planOpen, s.config
	a.agentModes, a.agentModeID = s.modes, s.modeID
	a.agentSessions, a.agentHistoryOpen, a.agentHistoryLoading = s.sessions, s.historyOpen, s.historyLoading
	a.agentHistoryErr, a.agentOptionOpen, a.agentScroll = s.historyErr, s.optionOpen, s.scroll
	a.agentAllowed, a.agentDenied, a.agentLoading = s.allowed, s.denied, s.loading
	a.agentCommands, a.agentRefs, a.agentCommand = s.commands, s.refs, s.command
	a.agentMentionOpen, a.agentMentionQuery, a.agentMentionDir = s.mentionOpen, s.mentionQuery, s.mentionDir
	a.agentMentionDismissed, a.agentCaret, a.agentCaretPending = s.dismissed, s.caret, s.caretPending
}

func (a *app) chatActive() *chatTab {
	a.ensureChats()
	return a.chatTabs[a.chatSel]
}

func (a *app) chatSave() {
	tab := a.chatActive()
	tab.state = a.chatState()
	a.chatSyncTitle()
}

func (a *app) chatTitleOf(tab *chatTab) string {
	if tab == nil {
		return "新对话"
	}
	return tab.title
}

func (a *app) chatSyncTitle() {
	tab := a.chatActive()
	for i := len(a.agentLines) - 1; i >= 0; i-- {
		if a.agentLines[i].Kind != "you" {
			continue
		}
		title := strings.Join(strings.Fields(a.agentLines[i].Text), " ")
		if len([]rune(title)) > 28 {
			title = string([]rune(title)[:28]) + "…"
		}
		tab.title, tab.updated = title, time.Now().Format(time.RFC3339)
		return
	}
	if tab.title == "" {
		tab.title = "新对话"
	}
	if tab.updated == "" {
		tab.updated = time.Now().Format(time.RFC3339)
	}
}

// chatActivate loads a tab without saving the current one first.
func (a *app) chatActivate(i int) {
	if i < 0 || i >= len(a.chatTabs) {
		return
	}
	a.chatSel = i
	a.setChatState(a.chatTabs[i].state)
}

func (a *app) chatSelect(i int) {
	if i < 0 || i >= len(a.chatTabs) || i == a.chatSel {
		return
	}
	a.chatSave()
	a.chatActivate(i)
}

// agentChatTabs is the strip of conversations inside the agent panel:
// each session is a tab of its own, and the panel never borrows a tab
// from the editor's files.
func (a *app) agentChatTabs(c *ui.Context, p palette) {
	a.ensureChats()
	ui.Row(c).FillWidth().Height(30).Shrink(0).Padding(0, 4, 0, 4).Gap(2).
		AlignItems(ui.Center).Children(func() {
		ui.Row(c).Height(30).Grow(1).Shrink(1).MinWidth(0).Gap(2).
			AlignItems(ui.Center).Children(func() {
			for i, tab := range a.chatTabs {
				i, tab := i, tab
				active := i == a.chatSel
				b := ui.ButtonBase(c).Grow(1).MinWidth(28).MaxWidth(180).Height(24).
					Padding(0, 5).Gap(4).Radius(5).AlignItems(ui.Center).Label(tab.title)
				if active {
					b.Background(p.selected)
				} else if b.Hovered() {
					b.Background(p.edge)
				}
				b.Children(func() {
					ui.Icon(c, a.agentLogo()).Size(11, 11).Shrink(0).TextColor(p.accent)
					ui.Text(c, tab.title).FontSize(10.5).TextColor(p.text).Grow(1).
						MinWidth(0).SingleLine().Ellipsis("…")
					close := ui.ButtonBase(c).Size(14, 14).Shrink(0).Radius(3).Center().
						Label("关闭对话 " + tab.title)
					if close.Hovered() {
						close.Background(p.edge)
					}
					close.Children(func() { ui.Icon(c, xIcon).Size(8, 8).TextColor(p.muted) })
					if close.Clicked() {
						a.chatClose(i)
					}
				})
				b.Tooltip(tab.title)
				if b.Clicked() {
					a.chatSelect(i)
				}
			}
		})
	})
}

func (a *app) chatNew() {
	a.ensureChats()
	a.chatSave()
	tab := &chatTab{id: a.nextChatID(), title: "新对话", updated: time.Now().Format(time.RFC3339)}
	tab.state = blankChatState()
	a.chatTabs = append(a.chatTabs, tab)
	a.chatActivate(len(a.chatTabs) - 1)
	a.restartAgentSession()
}

func (a *app) chatIndexOf(chat *chatTab) int {
	for i, tab := range a.chatTabs {
		if tab == chat {
			return i
		}
	}
	return -1
}

func blankChatState() chatState {
	return chatState{status: "新会话", caretPending: -1}
}

func (a *app) chatClose(i int) {
	a.ensureChats()
	if len(a.chatTabs) <= 1 {
		// Closing the last one leaves a fresh conversation, as the
		// editor's last tab does.
		a.chatTabs[0].state = blankChatState()
		a.chatActivate(0)
		return
	}
	a.chatSave()
	a.chatTabs = append(a.chatTabs[:i], a.chatTabs[i+1:]...)
	if a.chatSel >= len(a.chatTabs) {
		a.chatSel = len(a.chatTabs) - 1
	}
	if i <= a.chatSel {
		a.chatActivate(a.chatSel)
	} else {
		a.chatSave()
	}
}

// chatWithSession runs an answer for a session in the tab it belongs to,
// so an in-flight answer never lands in the conversation the user is
// looking at now.
func (a *app) chatWithSession(session string, fn func()) {
	if session == "" {
		fn()
		return
	}
	a.ensureChats()
	from := a.chatSel
	target := -1
	for i, tab := range a.chatTabs {
		if tab.state.session == session {
			target = i
			break
		}
	}
	if target < 0 || target == from {
		fn()
		return
	}
	a.chatSave()
	a.chatActivate(target)
	fn()
	a.chatSave()
	a.chatActivate(from)
}

// chatWithTab runs an answer on the tab it started in.
func (a *app) chatWithTab(tab *chatTab, fn func()) {
	if tab == nil {
		fn()
		return
	}
	a.ensureChats()
	from := a.chatSel
	target := -1
	for i, t := range a.chatTabs {
		if t == tab {
			target = i
			break
		}
	}
	if target < 0 || target == from {
		fn()
		return
	}
	a.chatSave()
	a.chatActivate(target)
	fn()
	a.chatSave()
	a.chatActivate(from)
}

// chatForSession finds the tab an answer belongs to, when a caller needs
// more than a callback around it.
func (a *app) chatForSession(session string) *chatTab {
	a.ensureChats()
	for _, tab := range a.chatTabs {
		if tab.state.session == session {
			return tab
		}
	}
	return nil
}
