package main

import (
	"strings"
	"testing"

	"github.com/egoist/mygo/ui"
)

// TestAnAgentCanForkAConversation covers session/fork: the agent's
// capability is read, a new session opens in a tab of its own, and the
// source conversation is left alone.
func TestAnAgentCanForkAConversation(t *testing.T) {
	a, queue := startAgentHelper(t)
	defer a.stopAgent()
	if !a.agentCanFork {
		t.Fatal("the agent said it can fork, but Vael did not read it")
	}
	a.forkAgentSession("s1")
	pumpUntil(t, queue, func() bool {
		return len(a.chatTabs) > 1 && a.chatActive().state.session == "forked1"
	})
	tab := a.chatActive()
	// The title of the fork comes from the agent's own update.
	pumpUntil(t, queue, func() bool { return tab.title == "分叉会话" })
	if !strings.Contains(tab.state.lines[0].Text, "已从「") ||
		!strings.Contains(tab.state.lines[0].Text, "」分叉出新会话") {
		t.Fatalf("the note of the fork: %+v", tab.state.lines)
	}
	if tab.state.config == nil || tab.state.modeID == "" {
		t.Fatalf("the selectors of the fork: %+v", tab.state)
	}
	// The source conversation is still there, with its own session.
	if a.chatTabs[0].state.session != "s1" {
		t.Fatalf("the source session changed: %+v", a.chatTabs[0].state.session)
	}
	if a.forkPending != "" {
		t.Fatal("the editor still thinks a fork is in flight")
	}
	// The fork is offered in the menu of the conversation.
	a.rightPanel, a.rightSection = true, "agent"
	tt := ui.NewTester(a.view, 1280, 840)
	tt.Frame()
	if !tt.HasText("分叉会话") {
		t.Fatalf("the fork tab is not drawn: %v", tt.Texts())
	}
}

// TestAnAgentThatCannotForkSaysSo covers an agent that did not advertise
// the capability: Vael does not send a request it will not understand.
func TestAnAgentThatCannotForkSaysSo(t *testing.T) {
	a := newApp()
	defer func() {
		a.agentConn, a.agentSession = nil, ""
		a.shutdown()
	}()
	a.agentConn, a.agentSession = &acpConn{}, "s1"
	a.agentCanFork = false
	a.forkAgentSession("s1")
	if !strings.Contains(a.message, "不支持分叉") {
		t.Fatalf("the message: %q", a.message)
	}
}
