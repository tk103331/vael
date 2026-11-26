package main

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"strconv"
	"strings"
	"time"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/transfer"
	"github.com/egoist/mygo/ui"
	"vael/internal/core"
	"vael/internal/workspace"
)

// agentLine is a line of the conversation with the agent: what the user
// asked, what the agent said or thought, a tool it ran, and what the
// panel says of itself.
type agentLine struct {
	Kind string // "you", "agent", "thought", "tool", "note", "error"
	Text string
	// ID is the call of a tool, which its updates find the line by.
	ID string
	// State is how a tool is doing: pending, in_progress, completed,
	// failed.
	State string
	// Detail is what the tool gave: the words of its output, or the
	// files its diff touches.
	Detail string
	// ToolKind is what kind of tool it is: read, edit, execute, search…
	ToolKind string
	// Collapsed says what the line hides until it is clicked: the words
	// of a thought, or the detail of a tool.
	Collapsed bool
}

// acpPending is a request of the agent that waits for the editor to say
// yes or no.
type acpPending struct {
	id  int
	ask acpPermission
}

// agentQueued is a message typed while the agent works: it waits in
// order, with the references that rode along when it was written, and
// goes as soon as the turn that runs ends.
type agentQueued struct {
	Text string
	Refs []agentRef
}

// loadAgents reads the agents the editor starts, and the one it starts by
// default.
func (a *app) loadAgents() {
	if a.agentsPath == "" {
		return
	}
	saved, err := workspace.LoadAgents(a.agentsPath)
	if err != nil {
		return
	}
	a.agents = saved
}

// saveAgents writes the agents down.
func (a *app) saveAgents() {
	if a.agentsPath == "" {
		return
	}
	a.report(workspace.SaveAgents(a.agentsPath, a.agents))
}

// startAgent runs the agent the panel chose, and makes a session of it in
// the project.
func (a *app) startAgent() {
	if a.agentConn != nil || a.agentStarting {
		return
	}
	chosen := a.agents.Chosen(a.agentChosen)
	if chosen == nil {
		a.agentStatus = "没有可用的 Agent"
		return
	}
	a.agentStarting = true
	a.ensureChats()
	tab := a.chatActive()
	a.chatSave()
	a.agentRun++
	run := a.agentRun
	ctx, cancel := context.WithCancel(context.Background())
	a.agentCancel = cancel
	a.agentConn = nil
	a.agentSession = ""
	a.resetAgentUsage()
	a.agentStatus = "启动中…"
	a.agentLines = append(a.agentLines,
		agentLine{Kind: "note", Text: "启动 " + chosen.Name + "…"})
	name, command, cwd := chosen.Name, chosen.Command, a.root
	go func() {
		conn, err := startACP(ctx, command, chosen.Env, func(message acpMessage) { a.incomingAgent(run, message) })
		if err != nil {
			a.update(func() {
				if run != a.agentRun {
					return
				}
				a.agentStarting = false
				a.agentStatus = "启动失败"
				a.agentCancel = nil
				a.agentLines = append(a.agentLines, agentLine{Kind: "error", Text: err.Error()})
				cancel()
			})
			return
		}
		var init acpInitialize
		err = conn.call(ctx, "initialize", map[string]any{
			"protocolVersion": acpProtocolVersion,
			"clientCapabilities": map[string]any{
				"fs": map[string]any{"readTextFile": false, "writeTextFile": false},
				// What the editor can show: the boolean options of a
				// session, its notices, the compaction of its context,
				// the plans it keeps, and the child sessions it opens.
				"session": map[string]any{
					"configOptions": map[string]any{"boolean": map[string]any{}},
					"notices":       map[string]any{},
					"compaction":    map[string]any{},
				},
				"subagents": map[string]any{},
				"plan":      map[string]any{},
			},
			"clientInfo": map[string]any{"name": "Vael", "version": core.Version},
		}, &init)
		var session acpSession
		if err == nil {
			err = conn.call(ctx, "session/new", map[string]any{
				"cwd": cwd, "mcpServers": []any{},
			}, &session)
		}
		a.update(func() {
			if run != a.agentRun {
				conn.close()
				return
			}
			if err != nil {
				a.agentStarting = false
				a.agentStatus = "连接失败"
				a.agentCancel = nil
				detail := err.Error()
				if words := conn.Errors(); words != "" {
					detail += "\n" + words
				}
				a.agentLines = append(a.agentLines, agentLine{Kind: "error", Text: detail})
				conn.close()
				cancel()
				return
			}
			a.chatWithTab(tab, func() {
				a.agentStarting = false
				a.agentConn = conn
				a.agentSession = session.SessionID
				a.agentCanLoad = init.AgentCapabilities.LoadSession
				a.agentCanList = init.canListSessions()
				a.agentCanFork = init.canForkSessions()
				a.agentSessions = nil
				a.agentSessionsLoaded = false
				a.agentConfig = session.ConfigOptions
				a.agentModes = session.Modes.AvailableModes
				a.agentModeID = session.Modes.CurrentModeID
				if session.AvailableCommands != nil {
					a.agentCommands = session.AvailableCommands
				}
				a.agentStatus = "已连接"
				a.agentSessionsLoaded = false
				a.agentLines = append(a.agentLines,
					agentLine{Kind: "note", Text: name + " 已就绪。在项目 " + shortPath(cwd) + " 中工作。"})
				if len(init.AuthMethods) > 0 {
					names := make([]string, 0, len(init.AuthMethods))
					for _, method := range init.AuthMethods {
						names = append(names, method.Name)
					}
					a.agentLines = append(a.agentLines, agentLine{
						Kind: "note",
						Text: "这个 Agent 需要先登录（" + strings.Join(names, "、") + "）：在终端运行一次它的登录命令即可。",
					})
				}
			})
		})
	}()
}

// ensureAgentStarted starts the agent chosen last, when its panel opens:
// the conversation is ready without a second click.
func (a *app) ensureAgentStarted() {
	if a.agentConn != nil || a.agentStarting {
		return
	}
	if a.agents.Chosen(a.agentChosen) == nil {
		return
	}
	a.startAgent()
}

// startAgentChoice opens a new conversation with an agent type: the
// running agent is kept when it is the same one, else it is replaced.
func (a *app) startAgentChoice(name string) {
	chosen := a.agents.Chosen(name)
	if chosen == nil {
		return
	}
	same := a.agentConn != nil && a.chosenAgentName() == chosen.Name
	a.agentChosen = name
	if same {
		a.chatNew()
		return
	}
	if a.agentConn != nil {
		a.stopAgent()
	}
	a.chatNew()
	a.startAgent()
}

// chosenAgentName is the name of the agent the panel is on.
func (a *app) chosenAgentName() string {
	if chosen := a.agents.Chosen(a.agentChosen); chosen != nil {
		return chosen.Name
	}
	return ""
}

// agentNewChatButton creates a conversation, or starts another agent for
// it: the floating panel lists the agent types the settings hold.
func (a *app) agentNewChatButton(c *ui.Context, p palette) {
	b := ui.ButtonBase(c).Size(22, 22).Radius(4).Center().Label("新对话 · 选择 Agent")
	if a.agentNewOpen {
		b.Background(p.selected)
	} else if b.Hovered() {
		b.Background(p.edge)
	}
	b.Children(func() { ui.Icon(c, plusIcon).Size(12, 12).TextColor(p.text) })
	b.Tooltip("新对话 · 选择 Agent 类型")
	if b.Clicked() {
		a.agentNewOpen = !a.agentNewOpen
	}
	ui.Popover(c, b, &a.agentNewOpen, func() { a.agentTypeMenu(c, p) })
}

// agentTypeMenu is the floating panel of the agent types.
func (a *app) agentTypeMenu(c *ui.Context, p palette) {
	ui.Column(c).Width(280).Padding(6).Gap(2).Children(func() {
		if len(a.agents.Items) == 0 {
			ui.Text(c, "还没有配置 Agent").FontSize(11).TextColor(p.muted).Padding(6, 8)
		}
		for _, agent := range a.agents.Items {
			agent := agent
			current := agent.Name == a.chosenAgentName()
			b := ui.ButtonBase(c).FillWidth().Height(36).Padding(0, 8).Gap(8).
				Radius(5).AlignItems(ui.Center).Label("新对话 · " + agent.Name)
			if b.Hovered() {
				b.Background(p.edge)
			}
			b.Children(func() {
				ui.Icon(c, a.agentSVG(agent)).Size(14, 14).TextColor(p.accent)
				ui.Column(c).Grow(1).MinWidth(0).Gap(1).Children(func() {
					ui.Text(c, agent.Name).FontSize(12).TextColor(p.text).SingleLine().Ellipsis("…")
					ui.Text(c, agent.Command).FontSize(9.5).TextColor(p.muted).
						Font("monospace").SingleLine().Ellipsis("…")
				})
				if current {
					ui.Icon(c, checkIcon).Size(11, 11).TextColor(p.accent)
				}
			})
			if b.Clicked() {
				a.agentNewOpen = false
				a.startAgentChoice(agent.Name)
			}
		}
		ui.Divider(c).Margin(4, 0)
		a.agentMenuItem(c, p, plusIcon, "在设置中添加…", func() {
			a.agentNewOpen = false
			a.settingsOpen, a.settingsSection = true, settingsAgent
		})
	})
}

// shortPath is the last part of a path, for the notes of the panel.
func shortPath(path string) string {
	if i := strings.LastIndexAny(path, "/\\"); i >= 0 && i+1 < len(path) {
		return path[i+1:]
	}
	return path
}

// stopAgent ends the agent, and the session with it.
func (a *app) stopAgent() {
	a.agentRun++
	a.agentStarting = false
	conn := a.agentConn
	session := a.agentSession
	a.agentConn, a.agentSession = nil, ""
	a.agentBusy = false
	a.agentPending = nil
	a.agentPlan = nil
	a.agentConfig = nil
	a.agentModes = nil
	a.agentModeID = ""
	a.agentOptionOpen = ""
	a.agentCommands = nil
	a.agentCommand = ""
	a.agentRefs = nil
	a.resetAgentUsage()
	a.agentMentionOpen = false
	a.agentSessions = nil
	a.agentSessionsLoaded = false
	a.agentHistoryOpen = false
	a.agentCanLoad = false
	a.agentCanList = false
	a.agentCanFork = false
	a.forkPending = ""
	a.forkUpdates = nil
	a.agentLoading = false
	a.agentStatus = "已停止"
	if a.agentCancel != nil {
		a.agentCancel()
		a.agentCancel = nil
	}
	if conn == nil {
		return
	}
	if session != "" {
		_ = conn.notify("session/cancel", map[string]any{"sessionId": session})
	}
	conn.close()
}

// incomingAgent takes what the agent says of its own accord, and what it
// asks of the editor.
func (a *app) incomingAgent(run int, message acpMessage) {
	switch message.Method {
	case "session/update":
		var update acpUpdate
		if err := json.Unmarshal(message.Params, &update); err != nil {
			return
		}
		a.update(func() {
			if run != a.agentRun {
				return
			}
			// While a fork is being made, the new session has no tab yet: keep
			// its early updates until the tab is there, and do not put
			// them in the source conversation.
			if a.forkPending != "" && update.SessionID != a.agentSession {
				a.forkUpdates = append(a.forkUpdates, update)
				return
			}
			a.chatWithSession(update.SessionID, func() { a.applyAgentUpdate(update) })
		})
	case "session/request_permission":
		var ask acpPermission
		if err := json.Unmarshal(message.Params, &ask); err != nil {
			return
		}
		if message.ID == nil {
			return
		}
		id := *message.ID
		a.update(func() {
			if run != a.agentRun {
				if a.agentConn != nil {
					_ = a.agentConn.reply(id, map[string]any{
						"outcome": map[string]any{"outcome": "cancelled"},
					})
				}
				return
			}
			a.chatWithSession(ask.SessionID, func() {
				a.queuePermission(acpPending{id: id, ask: ask})
			})
		})
	case "":
		// An answer, which nothing here waits for.
	default:
		// A request this editor does not do: tell the agent so, and let
		// it carry on.
		if message.ID == nil {
			return
		}
		id := *message.ID
		a.update(func() {
			if a.agentConn != nil {
				_ = a.agentConn.replyError(id, -32601, "Vael 不支持 "+message.Method)
			}
		})
	}
}

// applyAgentUpdate puts what the agent does into the conversation: the
// words it says, the tools it runs, the plan it keeps, and what it asks
// of the user.
func (a *app) applyAgentUpdate(update acpUpdate) {
	switch update.Update.SessionUpdate {
	case "agent_message_chunk":
		a.appendAgentLine("agent", update.Words())
	case "agent_thought_chunk":
		a.appendAgentLine("thought", update.Words())
	case "user_message_chunk":
		// The panel writes what the user said itself, except when a
		// previous session replays what they said.
		if a.agentLoading {
			a.appendAgentLine("you", update.Words())
		}
	case "tool_call":
		// A tool the agent takes up, which its updates then follow.
		for i := range a.agentLines {
			if a.agentLines[i].Kind == "tool" && a.agentLines[i].ID == update.Update.ToolCallID {
				a.agentLines[i].Text = toolTitle(update.Update.Title, update.Update.Kind)
				a.agentLines[i].State = update.Update.Status
				a.agentLines[i].ToolKind = update.Update.Kind
				return
			}
		}
		a.agentLines = append(a.agentLines, agentLine{
			Kind:     "tool",
			ID:       update.Update.ToolCallID,
			Text:     toolTitle(update.Update.Title, update.Update.Kind),
			State:    update.Update.Status,
			ToolKind: update.Update.Kind,
			Detail:   toolDetail(update.ToolParts(), update.Update.Locations),
		})
		a.followAgent()
	case "tool_call_update":
		for i := range a.agentLines {
			line := &a.agentLines[i]
			if line.Kind != "tool" || line.ID != update.Update.ToolCallID {
				continue
			}
			if update.Update.Title != "" || update.Update.Kind != "" {
				line.Text = toolTitle(update.Update.Title, update.Update.Kind)
			}
			if update.Update.Status != "" {
				line.State = update.Update.Status
			}
			if update.Update.Kind != "" {
				line.ToolKind = update.Update.Kind
			}
			if detail := toolDetail(update.ToolParts(), update.Update.Locations); detail != "" {
				line.Detail = detail
			}
			a.followAgent()
			return
		}
	case "plan":
		// The plan replaces the one before it: an agent replans as it
		// learns.
		a.agentPlan = update.Update.Entries
		if a.agentPlanOpen || len(update.Update.Entries) > 0 {
			a.agentPlanOpen = true
		}
	case "current_mode_update":
		if update.Update.CurrentModeID != "" {
			a.agentModeID = update.Update.CurrentModeID
		}
	case "config_option_update":
		// The complete list of the session's selectors, as the agent
		// changed them itself.
		a.agentConfig = update.Update.ConfigOptions
	case "available_commands_update":
		// The slash commands the agent offers. They are shown as
		// candidates as soon as "/" is typed in the composer.
		a.agentCommands = update.Update.AvailableCommands
	case "session_info_update":
		// The session names itself as the conversation moves on, unless
		// the user named it first.
		tab := a.chatForSession(update.SessionID)
		if tab == nil {
			if a.agentSession != "" {
				return
			}
			// The session is still being made: its name belongs to the
			// tab that asked for it.
			tab = a.chatActive()
		}
		if update.Update.UpdatedAt != "" {
			tab.updated = update.Update.UpdatedAt
		}
		if title := strings.TrimSpace(update.Update.Title); title != "" && !tab.renamed {
			tab.title, tab.titled = title, true
		}
	case "usage_update":
		// How much of the context window the session has filled, and what
		// it has spent so far.
		a.applyAgentUsage(update)
	case "notice":
		// Something the agent says to the user, outside the conversation.
		a.showAgentNotice(update)
	case "compaction_update":
		// The agent compressed the context of the session to keep going.
		a.applyAgentCompaction(update)
	case "plan_update":
		if plan := update.Update.Plan; plan != nil {
			a.agentPlan = plan.Entries
			if plan.Markdown != "" {
				a.agentPlan = []acpPlanEntry{{Content: plan.Markdown, Priority: "medium", Status: "in_progress"}}
			}
			a.agentPlanOpen = true
		}
	case "plan_removed":
		a.agentPlan = nil
	case "subagent_update":
		// The agent opened a child session of its own: it gets a tab of
		// its own, as a subagent of the conversation.
		a.openSubagentTab(update)
	case "session_message":
		// A message the agent sent to another session, or one it sent to
		// this one.
		a.showSessionMessage(update, false)
	case "session_message_chunk":
		a.showSessionMessage(update, true)
	}
}

// showAgentNotice shows what the agent says to the user, as a notice
// above the window.
func (a *app) showAgentNotice(update acpUpdate) {
	title := strings.TrimSpace(update.Update.Title)
	if title == "" {
		return
	}
	text := title
	if description := strings.TrimSpace(update.Update.Description); description != "" {
		text += "：" + description
	}
	a.message = text
	switch strings.ToLower(update.Update.Severity) {
	case "warning", "warn":
		a.messageKind = "warning"
	case "error":
		a.messageKind = "error"
	default:
		a.messageKind = ""
	}
}

// applyAgentCompaction says that the agent compressed the context of the
// session, and what it kept of it.
func (a *app) applyAgentCompaction(update acpUpdate) {
	status := strings.TrimSpace(update.Update.Status)
	text := "上下文已压缩"
	switch strings.ToLower(status) {
	case "failed":
		text = "上下文压缩失败"
	case "in_progress", "pending":
		text = "正在压缩上下文"
	case "", "completed":
	default:
		text += "：" + status
	}
	if summary := compactionSummary(update.Update.Summary); summary != "" {
		text += "\n" + summary
	}
	a.appendAgentLine("note", text)
}

// compactionSummary reads what a compaction kept of the conversation.
func compactionSummary(summary []acpContent) string {
	var lines []string
	for _, block := range summary {
		if words := strings.TrimSpace(block.Reads()); words != "" {
			lines = append(lines, words)
		}
	}
	return strings.Join(lines, "\n")
}

// openSubagentTab opens the child session the agent made in a tab of its
// own, and keeps what the agent says about it there.
func (a *app) openSubagentTab(update acpUpdate) {
	session := strings.TrimSpace(update.Update.SessionID)
	if session == "" {
		return
	}
	title := strings.TrimSpace(update.Update.Title)
	if title == "" {
		title = "子 Agent"
	}
	state := strings.TrimSpace(update.Update.State)
	a.ensureChats()
	for _, tab := range a.chatTabs {
		if tab.state.session != session {
			continue
		}
		if title != "" {
			tab.title, tab.titled = title, true
		}
		tab.subState = state
		return
	}
	now := time.Now().Format(time.RFC3339)
	tab := &chatTab{id: a.nextChatID(), title: title, titled: true, updated: now,
		subagent: true, subState: state}
	tab.state = blankChatState()
	tab.state.session, tab.state.status = session, "子 Agent"
	a.chatTabs = append(a.chatTabs, tab)
	a.sortChatTabs()
}

// showSessionMessage shows a message the agent sent to another session,
// or one it sent to this one, in the conversation it belongs to.
func (a *app) showSessionMessage(update acpUpdate, chunk bool) {
	sender, recipient := update.Update.SenderSessionID, update.Update.RecipientSessionID
	text := ""
	if chunk {
		var block acpContent
		if len(update.Update.Content) > 0 {
			_ = json.Unmarshal(update.Update.Content, &block)
		}
		text = block.Reads()
	} else {
		var blocks []acpContent
		if len(update.Update.Content) > 0 {
			_ = json.Unmarshal(update.Update.Content, &blocks)
		}
		var lines []string
		for _, block := range blocks {
			if words := strings.TrimSpace(block.Reads()); words != "" {
				lines = append(lines, words)
			}
		}
		text = strings.Join(lines, "\n")
	}
	if strings.TrimSpace(text) == "" {
		return
	}
	// The message goes from one session to another: the panel says which
	// way it went.
	other := recipient
	arrow := "→"
	if sender != "" && sender != a.agentSession {
		other, arrow = sender, "←"
	}
	name := a.chatNameOfSession(other)
	if arrow == "→" {
		a.appendAgentLine("relay", "→ "+name+"："+text)
	} else {
		a.appendAgentLine("relay", "← "+name+"："+text)
	}
}

// chatNameOfSession is the name of a session, as the panel knows it.
func (a *app) chatNameOfSession(session string) string {
	if session == "" {
		return "另一个会话"
	}
	if tab := a.chatForSession(session); tab != nil && strings.TrimSpace(tab.title) != "" {
		return tab.title
	}
	return shortSessionID(session)
}

// applyAgentUsage takes the context window and the cost of the session,
// warns once as the window fills, and keeps the usage of the tab it
// belongs to.
func (a *app) applyAgentUsage(update acpUpdate) {
	if update.Update.Size <= 0 {
		return
	}
	a.agentUsed, a.agentSize = update.Update.Used, update.Update.Size
	if cost := update.Update.Cost; cost != nil {
		a.agentCost, a.agentCostCurrency = cost.Amount, cost.Currency
	}
	percent := a.agentUsagePercent()
	switch {
	case percent >= 90 && !a.agentUsageWarned:
		a.agentUsageWarned = true
		a.message = fmt.Sprintf("上下文已用 %d%%，建议新建会话或让 Agent 压缩。", percent)
		a.messageKind = "warning"
	case percent < 80:
		a.agentUsageWarned = false
	}
}

// toolDetail reads what a tool gave: the words of its output, and the
// files its diff touches.
func toolDetail(parts []acpToolContent, locations []acpLocation) string {
	var lines []string
	for _, part := range parts {
		switch part.Type {
		case "content":
			if part.Content != nil {
				if words := strings.TrimSpace(part.Content.Reads()); words != "" {
					lines = append(lines, firstLines(words, 3))
				}
			}
		case "diff":
			if part.Path != "" {
				lines = append(lines, "修改 "+shortPath(part.Path))
			}
		case "terminal":
			if part.Terminal != "" {
				lines = append(lines, "在终端中运行")
			}
		}
	}
	for _, at := range locations {
		if at.Path != "" {
			line := "涉及 " + shortPath(at.Path)
			if at.Line != nil {
				line += ":" + strconv.Itoa(*at.Line)
			}
			lines = append(lines, line)
		}
	}
	return strings.Join(dedupe(lines), "\n")
}

// firstLines keeps the first n lines of words.
func firstLines(words string, n int) string {
	parts := strings.Split(strings.TrimSpace(words), "\n")
	if len(parts) > n {
		parts = append(parts[:n], "…")
	}
	return strings.Join(parts, "\n")
}

// dedupe keeps the first of each line.
func dedupe(lines []string) []string {
	seen := map[string]bool{}
	kept := lines[:0]
	for _, line := range lines {
		if seen[line] {
			continue
		}
		seen[line] = true
		kept = append(kept, line)
	}
	return kept
}

// toolTitle is what a tool of the agent is called.
func toolTitle(title, kind string) string {
	if strings.TrimSpace(title) != "" {
		return title
	}
	if kind != "" {
		return kind
	}
	return "工具"
}

// toolStatus is a status of a tool of the agent in words.
func toolStatus(status string) string {
	switch status {
	case "completed":
		return "已完成"
	case "failed":
		return "失败"
	case "in_progress":
		return "进行中"
	case "pending":
		return "等待中"
	}
	return status
}

// appendAgentLine adds to the conversation, keeping the chunks of one
// answer together.
func (a *app) appendAgentLine(kind, text string) {
	if strings.TrimSpace(text) == "" {
		return
	}
	if n := len(a.agentLines); n > 0 && a.agentLines[n-1].Kind == kind {
		a.agentLines[n-1].Text += text
		a.followAgent()
		return
	}
	a.agentLines = append(a.agentLines, agentLine{
		Kind: kind, Text: text,
		// What the agent thought is folded away once it is done; the
		// line in progress shows, as the words come.
		Collapsed: kind == "thought",
	})
	a.followAgent()
}

// followAgent keeps the conversation at its end, unless the user scrolled
// away from it, as a chat does while the agent talks.
func (a *app) followAgent() {
	if a.agentScroll.MaxY <= 0 || a.agentScroll.Y >= a.agentScroll.MaxY-40 {
		a.agentScroll.Y = math.MaxFloat32
	}
}

// sendToAgent asks the agent what the box holds.
func (a *app) sendToAgent() {
	a.sendAgentText(a.agentInput)
}

// sendAgentText asks the agent what the words say, as the box and the
// text area do. While a turn runs the message waits in the queue instead,
// as the conversation keeps taking what the user types.
func (a *app) sendAgentText(text string) {
	text = strings.TrimSpace(text)
	refs := a.agentRefs
	if text == "" && len(refs) == 0 {
		return
	}
	if a.agentConn == nil || a.agentSession == "" {
		a.agentLines = append(a.agentLines, agentLine{
			Kind: "note", Text: "先启动一个 Agent，再向它提问。",
		})
		return
	}
	if a.agentBusy {
		a.agentQueue = append(a.agentQueue, agentQueued{Text: text, Refs: refs})
		a.clearAgentComposer()
		return
	}
	a.clearAgentComposer()
	a.sendAgentMessage(text, refs)
}

// clearAgentComposer empties the box once its message went, or joined
// the queue.
func (a *app) clearAgentComposer() {
	a.agentInput, a.agentRefs, a.agentCommand = "", nil, ""
	a.agentMentionOpen, a.agentMentionDismissed = false, ""
	a.agentCaret, a.agentCaretPending = 0, -1
}

// sendAgentMessage starts a turn of the session that runs with a message
// and the references it carries. The caller has taken them from the box.
func (a *app) sendAgentMessage(text string, refs []agentRef) {
	if a.agentConn == nil || a.agentSession == "" {
		return
	}
	prompt := a.agentPromptBlocksWith(text, refs)
	shown := text
	if shown == "" {
		shown = "（图片）"
	}
	a.agentLines = append(a.agentLines, agentLine{Kind: "you", Text: shown})
	a.agentBusy = true
	a.followAgent()
	a.chatSyncTitle()
	run, conn, session := a.agentRun, a.agentConn, a.agentSession
	ctx, cancel := context.WithCancel(context.Background())
	go func() {
		defer cancel()
		var result acpPromptResult
		err := conn.call(ctx, "session/prompt", map[string]any{
			"sessionId": session,
			"prompt":    prompt,
		}, &result)
		a.update(func() {
			if run != a.agentRun {
				return
			}
			a.chatWithSession(session, func() {
				a.agentBusy = false
				if err != nil {
					a.agentLines = append(a.agentLines, agentLine{Kind: "error", Text: err.Error()})
				} else {
					switch result.StopReason {
					case "cancelled":
						a.agentLines = append(a.agentLines, agentLine{Kind: "note", Text: "本轮已取消。"})
					case "input_required":
						a.agentLines = append(a.agentLines, agentLine{
							Kind: "note", Text: "Agent 需要你的补充：直接回答即可。",
						})
					case "refusal":
						a.agentLines = append(a.agentLines, agentLine{Kind: "note", Text: "Agent 拒绝了这个请求。"})
					}
				}
				// The messages that waited for this turn go now, in order.
				a.flushAgentQueue()
				if err == nil && result.StopReason != "cancelled" && !a.agentBusy {
					a.notifyAgentDone()
				}
			})
		})
	}()
}

// notifyAgentDone says the agent finished a turn, when the window is not
// in front, as a desktop notification; a click brings the window back.
func (a *app) notifyAgentDone() {
	if !a.notifyAgent || a.win == nil || a.win.IsFocused() {
		return
	}
	title := "Agent 完成了这一轮"
	if tab := a.chatActive(); tab != nil && strings.TrimSpace(tab.title) != "" {
		title = tab.title + " · 完成"
	}
	body := "回到 Vael 继续对话。"
	for i := len(a.agentLines) - 1; i >= 0; i-- {
		if a.agentLines[i].Kind == "agent" {
			if words := strings.TrimSpace(a.agentLines[i].Text); words != "" {
				body = firstLines(words, 2)
			}
			break
		}
	}
	note := mygo.NewNotification(mygo.NotificationOptions{
		ID:    "vael-agent-" + a.agentSession,
		Title: title,
		Body:  body,
		Group: "vael-agent",
	})
	go func() { _ = note.Show() }()
}

// flushAgentQueue starts the next message that waited while the agent
// worked, if one waits and no turn runs.
func (a *app) flushAgentQueue() {
	if a.agentBusy || len(a.agentQueue) == 0 {
		return
	}
	if a.agentConn == nil || a.agentSession == "" {
		return
	}
	item := a.agentQueue[0]
	a.agentQueue = a.agentQueue[1:]
	a.sendAgentMessage(item.Text, item.Refs)
}

// steerAgent says a waiting message now: it goes to the front of the
// queue, and the turn that runs is cancelled, so that the agent reads it
// as soon as it stops.
func (a *app) steerAgent(index int) {
	if index < 0 || index >= len(a.agentQueue) {
		return
	}
	item := a.agentQueue[index]
	a.agentQueue = append(a.agentQueue[:index], a.agentQueue[index+1:]...)
	a.agentQueue = append([]agentQueued{item}, a.agentQueue...)
	if !a.agentBusy {
		a.flushAgentQueue()
		return
	}
	a.cancelAgentPrompt()
}

// editQueuedAgent takes a waiting message back to the box, with its
// references, so that it can be changed before it goes.
func (a *app) editQueuedAgent(index int) {
	if index < 0 || index >= len(a.agentQueue) {
		return
	}
	item := a.agentQueue[index]
	a.agentQueue = append(a.agentQueue[:index], a.agentQueue[index+1:]...)
	switch {
	case item.Text == "":
		// Only the references were waiting.
	case a.agentInput == "":
		a.agentInput = item.Text
	default:
		a.agentInput = strings.TrimRight(a.agentInput, "\n") + "\n" + item.Text
	}
	a.agentRefs = append(a.agentRefs, item.Refs...)
	a.agentCaretPending = len([]rune(a.agentInput))
}

// queuePermission puts a request of the agent in line, or answers it at
// once when the user said this kind of tool is always allowed.
func (a *app) queuePermission(pending acpPending) {
	ask := pending.ask
	key := permissionKey(ask)
	if option, ok := a.agentAllowed[key]; ok {
		if a.agentConn != nil {
			_ = a.agentConn.reply(pending.id, map[string]any{
				"outcome": map[string]any{"outcome": "selected", "optionId": option},
			})
			a.agentLines = append(a.agentLines, agentLine{
				Kind: "note", Text: "已允许（此前选择了总是允许）：" + toolTitle(ask.ToolCall.Title, ask.ToolCall.Kind),
			})
			a.followAgent()
		}
		return
	}
	if option, ok := a.agentDenied[key]; ok {
		if a.agentConn != nil {
			_ = a.agentConn.reply(pending.id, map[string]any{
				"outcome": map[string]any{"outcome": "selected", "optionId": option},
			})
			a.agentLines = append(a.agentLines, agentLine{
				Kind: "note", Text: "已拒绝（此前选择了总是拒绝）：" + toolTitle(ask.ToolCall.Title, ask.ToolCall.Kind),
			})
			a.followAgent()
		}
		return
	}
	a.agentPending = append(a.agentPending, pending)
}

// permissionKey is what a request of the agent is remembered by when the
// user allows it always.
func permissionKey(ask acpPermission) string {
	if ask.ToolCall.Kind != "" {
		return ask.ToolCall.Kind
	}
	return ask.ToolCall.Title
}

// answerAgent says yes or no to what the agent asked, and lets the next
// question in line show.
func (a *app) answerAgent(option acpPermissionOption) {
	if len(a.agentPending) == 0 {
		return
	}
	pending := a.agentPending[0]
	a.agentPending = a.agentPending[1:]
	title := toolTitle(pending.ask.ToolCall.Title, pending.ask.ToolCall.Kind)
	if strings.HasPrefix(option.Kind, "allow") {
		if option.Kind == "allow_always" && a.agentConn != nil {
			if a.agentAllowed == nil {
				a.agentAllowed = map[string]string{}
			}
			a.agentAllowed[permissionKey(pending.ask)] = option.OptionID
		}
		a.agentLines = append(a.agentLines, agentLine{Kind: "note", Text: "已允许：" + title})
	} else {
		if option.Kind == "reject_always" && a.agentConn != nil {
			if a.agentDenied == nil {
				a.agentDenied = map[string]string{}
			}
			a.agentDenied[permissionKey(pending.ask)] = option.OptionID
		}
		a.agentLines = append(a.agentLines, agentLine{Kind: "note", Text: "已拒绝：" + title})
	}
	a.followAgent()
	if a.agentConn == nil {
		return
	}
	_ = a.agentConn.reply(pending.id, map[string]any{
		"outcome": map[string]any{"outcome": "selected", "optionId": option.OptionID},
	})
}

// agentView is the panel of the right that holds the conversation with
// the agent, and the box to ask it something.
func (a *app) agentView(c *ui.Context, p palette) {
	// The panel's title takes the top of the column around this; what is
	// left is the conversation, and the box under it.
	ui.Column(c).FillWidth().Grow(1).MinHeight(0).Children(func() {
		a.agentChatTabs(c, p)
		ui.Divider(c)
		// The conversation takes the room the things below it leave, and
		// scrolls what is longer, instead of pushing them out.
		ui.Box(c).FillWidth().Grow(1).MinHeight(0).Children(func() {
			ui.Scroll(c).FillWidth().FillHeight().TrackScroll(&a.agentScroll).Children(func() {
				ui.Column(c).FillWidth().Padding(10).Gap(10).Children(func() {
					if len(a.agentLines) == 0 {
						a.agentEmptyView(c, p)
						return
					}
					// One Agent mark at the top of what it says, not one on
					// every thought, tool and answer of the same turn.
					wasAgent := false
					for i := range a.agentLines {
						line := &a.agentLines[i]
						isAgent := line.Kind == "thought" || line.Kind == "tool" || line.Kind == "agent"
						if isAgent && !wasAgent {
							a.agentSender(c, p)
						}
						a.agentLineView(c, p, line, i)
						wasAgent = isAgent
					}
					if a.agentBusy {
						a.agentBusyView(c, p)
					}
				})
			})
			a.agentScrollButton(c, p)
		})
		a.agentPlanView(c, p)
		a.agentPermissionView(c, p)
		ui.Divider(c)
		a.agentComposer(c, p)
	})
}

// agentToolbar is the bar of the panel: the agent it starts, and whether
// one runs.
func (a *app) agentToolbar(c *ui.Context, p palette) {
	name := "选择 Agent"
	if chosen := a.agents.Chosen(a.agentChosen); chosen != nil {
		name = chosen.Name
	}
	ui.Row(c).FillWidth().Height(32).Padding(0, 8).Gap(6).AlignItems(ui.Center).Children(func() {
		selector := ui.ButtonBase(c).Height(24).Padding(0, 6).Radius(5).Gap(4).
			AlignItems(ui.Center).Label("Agent：" + name)
		if a.agentMenuOpen {
			selector.Background(p.selected)
		} else if selector.Hovered() {
			selector.Background(p.edge)
		}
		selector.Children(func() {
			ui.Icon(c, a.agentLogo()).Size(13, 13).TextColor(p.muted)
			ui.Text(c, name).FontSize(11.5).TextColor(p.text).SingleLine().Ellipsis("…")
			ui.Icon(c, chevronDownIcon).Size(9, 9).TextColor(p.muted)
		})
		if selector.Clicked() {
			a.agentMenuOpen = !a.agentMenuOpen
		}
		ui.Popover(c, selector, &a.agentMenuOpen, func() { a.agentMenu(c, p) })
		a.agentNewChatButton(c, p)
		a.agentHistoryButton(c, p)
		ui.Box(c).Grow(1)
		// The turn is stopped by the one button in the composer: a second
		// stop here would be the same action twice.
		if a.agentStatus != "" {
			ui.Text(c, a.agentStatus).FontSize(10).TextColor(p.muted)
		}
		if a.agentConn == nil {
			if smallIconButton(c, playIcon, "启动 Agent", p) {
				a.startAgent()
			}
		} else {
			if smallIconButton(c, xCircleIcon, "停止 Agent", p) {
				a.stopAgent()
			}
		}
	})
}

// agentMenu is the list of the agents the panel can start.
func (a *app) agentMenu(c *ui.Context, p palette) {
	ui.Column(c).Width(260).Padding(6).Gap(2).Children(func() {
		if len(a.agents.Items) == 0 {
			ui.Text(c, "还没有配置 Agent").FontSize(11).TextColor(p.muted).Padding(6, 8)
		}
		for _, agent := range a.agents.Items {
			agent := agent
			b := ui.ButtonBase(c).FillWidth().Height(34).Padding(0, 8).Gap(8).
				Radius(5).AlignItems(ui.Center).Label(agent.Name)
			if b.Hovered() {
				b.Background(p.edge)
			}
			b.Children(func() {
				if svg := a.agentSVG(agent); svg != nil {
					ui.Icon(c, svg).Size(14, 14).TextColor(p.muted)
				}
				ui.Column(c).Grow(1).MinWidth(0).Gap(1).Children(func() {
					ui.Text(c, agent.Name).FontSize(12).TextColor(p.text).SingleLine().Ellipsis("…")
					ui.Text(c, agent.Command).FontSize(9.5).TextColor(p.muted).
						Font("monospace").SingleLine().Ellipsis("…")
				})
				if agent.Name == a.agentChosen || (a.agentChosen == "" && agent.Name == a.agents.Default) {
					ui.Icon(c, checkIcon).Size(11, 11).TextColor(p.accent)
				}
			})
			if b.Clicked() {
				a.agentMenuOpen = false
				if a.agentConn != nil {
					a.stopAgent()
				}
				a.agentChosen = agent.Name
				a.agentRun++
				a.agentLines = nil
				a.agentConfig = nil
				a.agentModes = nil
				a.agentModeID = ""
				a.agentOptionOpen = ""
				a.agentSessions = nil
				a.agentHistoryOpen = false
				a.agentStatus = ""
				a.startAgent()
			}
		}
		ui.Divider(c).Margin(4, 0)
		a.agentMenuItem(c, p, plusIcon, "在设置中添加…", func() {
			a.agentMenuOpen = false
			a.settingsOpen = true
		})
	})
}

// agentMenuItem is a choice of the agent menu.
func (a *app) agentMenuItem(c *ui.Context, p palette, icon *ui.SVG, text string, fn func()) {
	b := ui.ButtonBase(c).FillWidth().Height(28).Padding(0, 8).Gap(8).Radius(5).
		AlignItems(ui.Center).Label(text)
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

// restartAgentSession makes a new session of the agent that runs, so that
// it starts the project afresh.
func (a *app) restartAgentSession() {
	if a.agentConn == nil {
		return
	}
	a.ensureChats()
	tab := a.chatActive()
	a.chatSave()
	a.agentRun++
	conn, cwd := a.agentConn, a.root
	run := a.agentRun
	a.agentSession = ""
	a.resetAgentUsage()
	a.agentStatus = "新建会话中…"
	go func() {
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()
		var session acpSession
		err := conn.call(ctx, "session/new", map[string]any{"cwd": cwd, "mcpServers": []any{}}, &session)
		a.update(func() {
			if run != a.agentRun || a.agentConn != conn {
				return
			}
			if err != nil {
				a.agentStatus = "新建会话失败"
				a.agentLines = append(a.agentLines, agentLine{Kind: "error", Text: err.Error()})
				return
			}
			a.chatWithTab(tab, func() {
				a.agentSession = session.SessionID
				a.agentConfig = session.ConfigOptions
				a.agentModes = session.Modes.AvailableModes
				a.agentModeID = session.Modes.CurrentModeID
				if session.AvailableCommands != nil {
					a.agentCommands = session.AvailableCommands
				}
				a.agentStatus = "已连接"
				a.agentSessionsLoaded = false
				a.agentLines = append(a.agentLines, agentLine{Kind: "note", Text: "已开始新的会话。"})
			})
		})
	}()
}

// forkAgentSession makes a new session from the one the user chose, and
// opens it in a tab of its own: the conversation before the fork stays as
// it was.
func (a *app) forkAgentSession(source string) {
	if a.agentConn == nil || source == "" {
		a.message = "先启动 Agent，再分叉会话。"
		return
	}
	if !a.agentCanFork {
		a.message = "这个 Agent 不支持分叉会话（session/fork）。"
		return
	}
	if a.forkPending != "" {
		return
	}
	a.chatSave()
	name := "新会话"
	if tab := a.chatForSession(source); tab != nil && strings.TrimSpace(tab.title) != "" {
		name = tab.title
	}
	a.forkPending = source
	run, conn := a.agentRun, a.agentConn
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	go func() {
		var session acpSession
		err := conn.call(ctx, "session/fork", map[string]any{
			"sessionId": source, "cwd": a.root, "mcpServers": []any{},
		}, &session)
		a.update(func() {
			defer cancel()
			a.forkPending = ""
			if run != a.agentRun || a.agentConn != conn {
				a.forkUpdates = nil
				return
			}
			if err != nil {
				a.forkUpdates = nil
				a.message, a.messageKind = "分叉失败："+err.Error(), "error"
				return
			}
			a.openForkedChat(source, name, session)
		})
	}()
}

// openForkedChat is the tab of a session the agent forked from another
// one: it holds the new session, its selectors, and a note of where it
// came from.
func (a *app) openForkedChat(source, name string, session acpSession) {
	a.chatSave()
	state := blankChatState()
	state.session, state.status = session.SessionID, "已连接"
	state.config = session.ConfigOptions
	state.modes = session.Modes.AvailableModes
	state.modeID = session.Modes.CurrentModeID
	if session.AvailableCommands != nil {
		state.commands = session.AvailableCommands
	}
	state.lines = append(state.lines, agentLine{Kind: "note", Text: "已从「" + name + "」分叉出新会话。"})
	tab := &chatTab{id: a.nextChatID(), title: "分叉：" + name,
		updated: time.Now().Format(time.RFC3339), state: state}
	a.chatTabs = append(a.chatTabs, tab)
	a.chatActivate(len(a.chatTabs) - 1)
	// What the agent said about the fork before it had a tab goes here.
	for _, update := range a.forkUpdates {
		a.applyAgentUpdate(update)
	}
	a.forkUpdates = nil
	a.agentSessionsLoaded = false
	a.followAgent()
}

// cancelAgentPrompt ends the turn that runs.
func (a *app) cancelAgentPrompt() {
	if a.agentConn == nil || a.agentSession == "" {
		return
	}
	_ = a.agentConn.notify("session/cancel", map[string]any{"sessionId": a.agentSession})
	// The requests the agent asked are answered as cancelled, so that it
	// does not wait on them.
	for _, pending := range a.agentPending {
		_ = a.agentConn.reply(pending.id, map[string]any{
			"outcome": map[string]any{"outcome": "cancelled"},
		})
	}
	a.agentPending = nil
}

// agentEmptyView says what the panel does, before anything runs.
func (a *app) agentEmptyView(c *ui.Context, p palette) {
	ui.Row(c).Gap(8).AlignItems(ui.Center).Children(func() {
		ui.Icon(c, a.agentLogo()).Size(26, 26).TextColor(p.accent)
		ui.Text(c, "让 Agent 读取项目、修改代码、运行命令。").FontSize(12).TextColor(p.text)
	})
	ui.Text(c, "Agent 通过 ACP（Agent Client Protocol）启动：选一个 Agent，点 ▶ 启动，"+
		"它会在此项目里开会话。").FontSize(11).TextColor(p.muted)
	ui.Text(c, "在设置中可以接入 ACP Registry 里的 Agent，或用自己的启动命令。").FontSize(11).TextColor(p.muted)
	if chosen := a.agents.Chosen(a.agentChosen); chosen != nil {
		ui.Text(c, "当前："+chosen.Name+" · "+chosen.Command).FontSize(10.5).
			TextColor(p.muted).Font("monospace")
	}
}

// agentLineView is one line of the conversation: a bubble of the user,
// the words of the agent, what it thought, or the tool it ran.
func (a *app) agentLineView(c *ui.Context, p palette, line *agentLine, index int) {
	switch line.Kind {
	case "note", "error":
		color := p.muted
		if line.Kind == "error" {
			color = p.danger
		}
		ui.Text(c, line.Text).FontSize(10.5).TextColor(color)
	case "relay":
		// A message that went from this session to another one, or came
		// from it.
		ui.Row(c).FillWidth().Gap(6).AlignItems(ui.Start).Children(func() {
			ui.Box(c).Width(2).FillHeight().Background(p.edge).Shrink(0)
			ui.Text(c, line.Text).FontSize(10.5).TextColor(p.muted).Grow(1).MinWidth(0)
		})
	case "tool":
		a.agentToolView(c, p, line)
	case "thought":
		a.agentThoughtView(c, p, line, index)
	case "you":
		a.agentMessageView(c, p, line.Text, true)
	default:
		a.agentMessageView(c, p, line.Text, false)
	}
}

// agentSender is the mark at the top of what the agent says, before its
// thought, tools and words. The user's words are on the other side.
func (a *app) agentSender(c *ui.Context, p palette) {
	ui.Row(c).FillWidth().Gap(5).AlignItems(ui.Center).Children(func() {
		ui.Icon(c, a.agentLogo()).Size(13, 13).TextColor(p.muted)
		ui.Text(c, "Agent").FontSize(10).FontWeight(650).TextColor(p.muted)
	})
}

// agentThoughtView is what the agent thought before it answered: a folded
// line, which shows as it comes, until it is done or clicked.
func (a *app) agentThoughtView(c *ui.Context, p palette, line *agentLine, index int) {
	// The thought in progress shows while the agent is busy; once it is
	// done, it folds away unless the user opened it.
	open := !line.Collapsed
	if a.agentBusy && index == len(a.agentLines)-1 {
		open = true
	}
	ui.Column(c).FillWidth().BorderWidth(0, 0, 0, 2).BorderColor(p.edge).
		Padding(2, 8).Gap(3).Children(func() {
		header := ui.ButtonBase(c).FillWidth().Height(20).Gap(4).AlignItems(ui.Center).Label("思考过程")
		if header.Hovered() {
			header.Background(p.edge)
		}
		header.Children(func() {
			icon := chevronDownIcon
			if !open {
				icon = chevronRightIcon
			}
			ui.Icon(c, icon).Size(8, 8).TextColor(p.muted)
			ui.Text(c, "过程").FontSize(10).TextColor(p.muted)
			ui.Textf(c, "%d 字", len([]rune(line.Text))).FontSize(9).TextColor(p.muted)
			if !open {
				ui.Text(c, firstLine(line.Text, 80)).FontSize(9.5).TextColor(p.muted).Grow(1).
					SingleLine().Ellipsis("…")
			}
		})
		if header.Clicked() {
			line.Collapsed = !line.Collapsed
		}
		if open {
			ui.Text(c, line.Text).FontSize(10.5).TextColor(p.muted).LineHeight(1.45)
		}
	})
}

// agentToolView is a tool the agent ran: what it was, how it did, and what
// it gave.
func (a *app) agentToolView(c *ui.Context, p palette, line *agentLine) {
	hasDetail := strings.TrimSpace(line.Detail) != ""
	ui.Column(c).FillWidth().Background(p.wash).Radius(5).Padding(5, 8).Gap(2).Children(func() {
		header := ui.ButtonBase(c).FillWidth().Height(22).Gap(5).AlignItems(ui.Center).Label(line.Text)
		if header.Hovered() {
			header.Background(p.edge)
		}
		header.Children(func() {
			ui.Icon(c, toolKindIcon(line.ToolKind)).Size(11, 11).TextColor(p.muted)
			ui.Text(c, line.Text).FontSize(11).TextColor(p.text).Grow(1).SingleLine().Ellipsis("…")
			if status := toolStatus(line.State); status != "" {
				color := p.muted
				switch line.State {
				case "completed":
					color = p.success
				case "failed":
					color = p.danger
				case "in_progress":
					color = p.accent
				}
				ui.Text(c, status).FontSize(9).TextColor(color)
			}
			if hasDetail {
				icon := chevronDownIcon
				if line.Collapsed {
					icon = chevronRightIcon
				}
				ui.Icon(c, icon).Size(8, 8).TextColor(p.muted)
			}
		})
		if header.Clicked() && hasDetail {
			line.Collapsed = !line.Collapsed
		}
		if hasDetail && !line.Collapsed {
			for _, row := range strings.Split(line.Detail, "\n") {
				ui.Text(c, row).FontSize(9.5).TextColor(p.muted).Font("monospace").
					LineHeight(1.4).Padding(0, 0, 0, 16)
			}
		}
	})
}

// toolKindIcon is the mark of a kind of tool, as the protocol names it.
func toolKindIcon(kind string) *ui.SVG {
	switch kind {
	case "read":
		return fileIcon
	case "edit":
		return pencilIcon
	case "delete":
		return trashIcon
	case "move":
		return filesIcon
	case "search":
		return searchIcon
	case "execute":
		return terminalIcon
	case "think":
		return agentIcon
	case "fetch":
		return globeIcon
	case "switch_mode":
		return repeatIcon
	}
	return taskIcon
}

// agentMessageView is what the user or the agent said, the user's in a
// bubble at the right, the agent's as running text.
func (a *app) agentMessageView(c *ui.Context, p palette, text string, mine bool) {
	if !mine {
		a.agentMarkdown(c, p, text, 11.5, p.text)
		return
	}
	ui.Column(c).FillWidth().Gap(4).Children(func() {
		ui.Row(c).FillWidth().Children(func() {
			ui.Box(c).Grow(1)
			ui.Text(c, "你").FontSize(10).FontWeight(650).TextColor(p.muted)
		})
		ui.Row(c).FillWidth().Children(func() {
			ui.Box(c).Grow(1)
			ui.Box(c).MaxWidthPercent(88).Padding(8, 10).Radius(6).
				Background(p.edge).Children(func() {
				a.agentMarkdown(c, p, text, 11.5, p.text)
			})
		})
	})
}

// agentMarkdown draws what the agent says: its prose, its lists, and its
// code, which stands apart in a block of its own.
func (a *app) agentMarkdown(c *ui.Context, p palette, text string, size float32, color ui.Color) {
	for _, block := range splitAgentText(text) {
		block := block
		if block.code {
			ui.Column(c).FillWidth().Background(p.wash).Radius(5).Padding(6, 8).Gap(2).Children(func() {
				if block.lang != "" {
					ui.Text(c, block.lang).FontSize(9).TextColor(p.muted)
				}
				ui.Text(c, block.text).Font("monospace").FontSize(size - 1).
					TextColor(p.text).LineHeight(1.4)
			})
			continue
		}
		if strings.TrimSpace(block.text) == "" {
			continue
		}
		ui.RichText(c, agentSpans(block.text, size, color, p)...).FillWidth()
	}
}

// agentBlock is a part of what the agent says: its words, or code.
type agentBlock struct {
	code bool
	lang string
	text string
}

// splitAgentText cuts what the agent says into prose and fenced code.
func splitAgentText(text string) []agentBlock {
	lines := strings.Split(strings.ReplaceAll(text, "\r\n", "\n"), "\n")
	var blocks []agentBlock
	var prose []string
	flush := func() {
		if len(prose) > 0 {
			blocks = append(blocks, agentBlock{text: strings.Join(prose, "\n")})
			prose = nil
		}
	}
	for i := 0; i < len(lines); i++ {
		if fence := strings.TrimSpace(lines[i]); strings.HasPrefix(fence, "```") {
			flush()
			lang := strings.TrimSpace(strings.TrimPrefix(fence, "```"))
			var code []string
			for i++; i < len(lines); i++ {
				if strings.HasPrefix(strings.TrimSpace(lines[i]), "```") {
					break
				}
				code = append(code, lines[i])
			}
			blocks = append(blocks, agentBlock{code: true, lang: lang, text: strings.Join(code, "\n")})
			continue
		}
		prose = append(prose, lines[i])
	}
	flush()
	return blocks
}

// agentSpans reads the words of a message: `code` in a monospace chip, and
// **bold** heavy.
func agentSpans(text string, size float32, color ui.Color, p palette) []ui.Span {
	var spans []ui.Span
	plain := func(part string) {
		for len(part) > 0 {
			open := strings.Index(part, "**")
			if open < 0 {
				spans = append(spans, ui.Span{Text: part, Size: size, Color: color})
				return
			}
			close := strings.Index(part[open+2:], "**")
			if close < 0 {
				spans = append(spans, ui.Span{Text: part, Size: size, Color: color})
				return
			}
			if open > 0 {
				spans = append(spans, ui.Span{Text: part[:open], Size: size, Color: color})
			}
			spans = append(spans, ui.Span{
				Text: part[open+2 : open+2+close], Size: size, Color: color, Weight: 650,
			})
			part = part[open+2+close+2:]
		}
	}
	for len(text) > 0 {
		at := strings.IndexByte(text, '`')
		if at < 0 {
			plain(text)
			break
		}
		if at > 0 {
			plain(text[:at])
		}
		rest := text[at+1:]
		close := strings.IndexByte(rest, '`')
		if close < 0 {
			plain(text[at:])
			break
		}
		if code := rest[:close]; code != "" {
			spans = append(spans, ui.Span{
				Text: code, Size: size - 0.5, Font: "monospace", Color: color,
				Background: p.wash, Radius: 3, PaddingX: 3,
			})
		}
		text = rest[close+1:]
	}
	return spans
}

// agentScrollButton goes back to the end of the conversation, floating
// over it when the user has scrolled away.
func (a *app) agentScrollButton(c *ui.Context, p palette) {
	if a.agentScroll.MaxY <= 0 || a.agentScroll.Y >= a.agentScroll.MaxY-24 {
		return
	}
	b := ui.ButtonBase(c).Absolute().Right(14).Bottom(10).Size(28, 28).Radius(14).
		Center().Label("回到底部").Background(p.card).Border(1, p.edge)
	if b.Hovered() {
		b.Background(p.edge)
	}
	b.Children(func() {
		ui.Icon(c, arrowDownIcon).Size(13, 13).TextColor(p.text)
	})
	b.Tooltip("回到底部")
	if b.Clicked() {
		a.agentScroll.ScrollTo(a.agentScroll.X, math.MaxFloat32, ui.Smooth)
	}
}

// agentBusyView says what the agent is doing, after the last line.
func (a *app) agentBusyView(c *ui.Context, p palette) {
	label := "正在回答…"
	if n := len(a.agentLines); n > 0 {
		last := a.agentLines[n-1]
		switch {
		case last.Kind == "thought":
			label = "正在思考…"
		case last.Kind == "tool" && (last.State == "pending" || last.State == "in_progress"):
			label = "正在运行工具…"
		}
	}
	ui.Row(c).FillWidth().Gap(6).AlignItems(ui.Center).Children(func() {
		ui.Box(c).Size(6, 6).Radius(3).Background(p.accent)
		ui.Text(c, label).FontSize(10).TextColor(p.muted)
	})
}

// agentMark is how a step of the agent's work does: a dot, a tick, or a
// cross.
func (a *app) agentMark(c *ui.Context, p palette, state string) {
	switch state {
	case "completed":
		ui.Icon(c, checkIcon).Size(11, 11).TextColor(p.success)
	case "failed":
		ui.Icon(c, xIcon).Size(11, 11).TextColor(p.danger)
	case "in_progress":
		ui.Box(c).Size(7, 7).Radius(3.5).Background(p.accent)
	default:
		ui.Box(c).Size(7, 7).Radius(3.5).Border(1, p.muted)
	}
}

// agentPlanView is the plan the agent keeps for its work, above the box
// that asks it something.
func (a *app) agentPlanView(c *ui.Context, p palette) {
	if len(a.agentPlan) == 0 {
		return
	}
	done := 0
	for _, entry := range a.agentPlan {
		if entry.Status == "completed" {
			done++
		}
	}
	ui.Column(c).FillWidth().Padding(0, 10).Gap(3).Children(func() {
		header := ui.ButtonBase(c).FillWidth().Height(24).Gap(5).AlignItems(ui.Center).Radius(4).Label("任务")
		if header.Hovered() {
			header.Background(p.edge)
		}
		header.Children(func() {
			icon := chevronDownIcon
			if !a.agentPlanOpen {
				icon = chevronRightIcon
			}
			ui.Icon(c, icon).Size(9, 9).TextColor(p.muted)
			ui.Text(c, "任务").FontSize(11).FontWeight(650).TextColor(p.text)
			ui.Textf(c, "%d/%d", done, len(a.agentPlan)).FontSize(9.5).
				TextColor(p.muted).FontFeatures("tnum").Grow(1)
		})
		if header.Clicked() {
			a.agentPlanOpen = !a.agentPlanOpen
		}
		if !a.agentPlanOpen {
			return
		}
		ui.Scroll(c).FillWidth().MaxHeight(160).Children(func() {
			ui.Column(c).FillWidth().Gap(2).Children(func() {
				for _, entry := range a.agentPlan {
					entry := entry
					ui.Row(c).FillWidth().Gap(6).AlignItems(ui.Center).Children(func() {
						a.agentMark(c, p, entry.Status)
						color := p.text
						if entry.Status == "completed" {
							color = p.muted
						}
						ui.Text(c, entry.Content).FontSize(11).TextColor(color).
							Grow(1).MinWidth(0).SingleLine().Ellipsis("…")
					})
				}
			})
		})
	})
}

// agentPermissionView asks what the agent asked to do, and waits for the
// answer the agent offered.
func (a *app) agentPermissionView(c *ui.Context, p palette) {
	if len(a.agentPending) == 0 {
		return
	}
	ask := a.agentPending[0].ask
	ui.Column(c).FillWidth().Padding(8, 10).Gap(5).Background(p.warnSoft).Children(func() {
		ui.Row(c).FillWidth().Gap(6).AlignItems(ui.Center).Children(func() {
			ui.Icon(c, toolKindIcon(ask.ToolCall.Kind)).Size(12, 12).TextColor(p.warn)
			ui.Text(c, "Agent 请求允许："+toolTitle(ask.ToolCall.Title, ask.ToolCall.Kind)).
				FontSize(11).TextColor(p.text).Grow(1).SingleLine().Ellipsis("…")
		})
		if command := permissionCommand(ask.ToolCall.RawInput); command != "" {
			ui.Text(c, firstLines(command, 3)).FontSize(9.5).TextColor(p.muted).
				Font("monospace").Padding(0, 0, 0, 16)
		}
		ui.Row(c).FillWidth().Wrap().Gap(6).Children(func() {
			for _, option := range ask.Options {
				option := option
				switch {
				case option.Kind == "allow_once":
					if ui.PrimaryButton(c, option.Name).Clicked() {
						a.answerAgent(option)
					}
				case strings.HasPrefix(option.Kind, "allow"):
					if ui.Button(c, option.Name).Clicked() {
						a.answerAgent(option)
					}
				default:
					if quiet(c, option.Name, false, p) {
						a.answerAgent(option)
					}
				}
			}
		})
		if len(a.agentPending) > 1 {
			ui.Textf(c, "Agent 还在等你回答另外 %d 个请求。", len(a.agentPending)-1).
				FontSize(9.5).TextColor(p.muted)
		} else {
			ui.Text(c, "Agent 在这一步等你回答。").
				FontSize(9.5).TextColor(p.muted)
		}
	})
}

// permissionCommand is the command a permission asks for, when it holds
// one: agents put it in rawInput, as words or a field.
func permissionCommand(raw json.RawMessage) string {
	if len(raw) == 0 {
		return ""
	}
	var words string
	if json.Unmarshal(raw, &words) == nil {
		return strings.TrimSpace(words)
	}
	var body struct {
		Command string `json:"command"`
		Cmd     string `json:"cmd"`
	}
	if json.Unmarshal(raw, &body) != nil {
		return ""
	}
	if body.Command != "" {
		return body.Command
	}
	return body.Cmd
}

// agentComposer is the box that asks the agent something: its session's
// selectors, the words, and the send that ends them. The send and the stop
// are one button inside the box, whose mark says which it is.
func (a *app) agentComposer(c *ui.Context, p palette) {
	// A child session the agent opened is its own: the panel shows what
	// happens there, and the user does not write in it.
	if tab := a.chatActive(); tab != nil && tab.subagent {
		a.agentSubagentComposer(c, p, tab)
		return
	}
	ui.Column(c).FillWidth().Padding(8).Gap(4).Children(func() {
		if len(a.agentConfig) > 0 || len(a.agentModes) > 0 {
			a.agentConfigBar(c, p)
		}
		a.agentUsageView(c, p)
		a.agentQueueChips(c, p)
		a.agentRefChips(c, p)
		// The box sits in a box of its own, so that the button can float
		// in its corner: two lines to write in, up to six as the words
		// grow, with room on the right for the button. Pictures pasted
		// into it, or dropped on it, ride with the message.
		var input ui.Element
		box := ui.Box(c).FillWidth()
		box.Children(func() {
			input = ui.TextArea(c, &a.agentInput).FillWidth().Lines(2, 6).MinHeight(46).
				Padding(6, 40, 6, 8).Placeholder("询问 Agent… @ 引用文件 / 使用命令").Label("Agent 输入")
			input.OnPaste(func(text string) bool {
				data := c.Services().ReadClipboardData(transfer.PNG)
				if len(data.Items()) == 0 {
					return false
				}
				png, err := data.Read(transfer.PNG)
				if err != nil || len(png) == 0 {
					return false
				}
				a.update(func() { a.addAgentImage(png, "剪贴板图片") })
				return true
			})
			input.OnTextKey(func(k ui.TextKey) bool {
				// Enter sends, or joins the queue while the agent works;
				// Shift+Enter breaks the line.
				if k.Key != ui.KeyEnter || k.Mods != 0 {
					return false
				}
				text := k.Text()
				a.sendAgentText(text)
				if (strings.TrimSpace(text) != "" || len(a.agentRefs) > 0) && a.agentConn != nil {
					k.SetText("")
				}
				return true
			})
			a.agentSendButton(c, p)
		})
		if files := box.DroppedFiles(); len(files) > 0 {
			a.attachAgentImages(files)
		}
		if box.FileDragOver() {
			box.Border(1, p.accent)
		}
		_, caret := input.TextSelection()
		a.agentCaret = caret
		if a.agentCaretPending >= 0 {
			input.SetTextSelection(a.agentCaretPending, a.agentCaretPending)
			a.agentCaret = a.agentCaretPending
			a.agentCaretPending = -1
		}
		a.updateAgentToken()
		a.agentCompletion(c, p)
		hint := "Enter 发送 · Shift+Enter 换行 · @ 引用文件 · / 命令 · 粘贴图片"
		if a.agentConn == nil {
			hint = "先启动 Agent 才能发送"
		} else if a.agentBusy {
			hint = "正在工作… Enter 加入队列 · 点右下角停止"
			if len(a.agentQueue) > 0 {
				hint = "正在工作… 队列里的消息在完成后依次发送"
			}
		}
		ui.Text(c, hint).FontSize(9.5).TextColor(p.muted).SingleLine().Ellipsis("…")
	})
}

// agentSubagentComposer is what stands in for the box of a child session:
// the child works on what the main agent asked it, and the panel only
// shows how it goes.
func (a *app) agentSubagentComposer(c *ui.Context, p palette, tab *chatTab) {
	state := "空闲"
	switch strings.ToLower(tab.subState) {
	case "running":
		state = "工作中"
	case "requires_action":
		state = "等待主 Agent"
	case "idle":
		state = "空闲"
	}
	ui.Column(c).FillWidth().Padding(10, 8).Gap(4).Background(p.wash).Radius(6).Children(func() {
		ui.Row(c).FillWidth().Gap(6).AlignItems(ui.Center).Children(func() {
			ui.Icon(c, branchIcon).Size(12, 12).TextColor(p.warn)
			ui.Text(c, "子 Agent 会话").FontSize(10.5).FontWeight(650).TextColor(p.text)
			ui.Box(c).Grow(1)
			ui.Text(c, state).FontSize(9.5).TextColor(p.muted)
		})
		ui.Text(c, "这个会话由主 Agent 驱动，这里只看它做了什么。").FontSize(9.5).TextColor(p.muted)
	})
}

// agentSendButton is the send of the composer, and the stop of the turn
// that runs: one button, whose mark says which it is.
func (a *app) agentSendButton(c *ui.Context, p palette) {
	busy := a.agentBusy
	ready := a.agentConn != nil && (strings.TrimSpace(a.agentInput) != "" || len(a.agentRefs) > 0)
	enabled := busy || ready
	icon, tip := arrowUpIcon, "发送"
	if busy {
		icon, tip = stopIcon, "停止"
	}
	b := ui.ButtonBase(c).Absolute().Right(6).Bottom(6).Size(26, 26).Radius(13).
		Center().Label(tip)
	theme := c.Theme()
	switch {
	case !enabled:
		b.Background(p.edge)
	case b.Hovered():
		b.Background(theme.AccentHover)
	default:
		b.Background(theme.Accent)
	}
	b.Children(func() {
		color := p.muted
		if enabled {
			color = theme.AccentText
		}
		ui.Icon(c, icon).Size(13, 13).TextColor(color)
	})
	b.Tooltip(tip)
	if enabled && b.Clicked() {
		if busy {
			a.cancelAgentPrompt()
			return
		}
		a.sendToAgent()
	}
}
