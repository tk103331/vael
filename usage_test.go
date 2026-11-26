package main

import (
	"testing"

	"github.com/egoist/mygo/ui"
)

// TestTheSessionReportsItsContext reads the usage an agent sends: the
// ring above the composer, the tokens, the cost, and the warning as the
// window fills.
func TestTheSessionReportsItsContext(t *testing.T) {
	a, queue := startAgentHelper(t)
	defer a.stopAgent()
	pumpUntil(t, queue, func() bool { return a.agentSize == 200000 })
	if a.agentUsed != 53000 || a.agentCost != 0.045 || a.agentCostCurrency != "USD" {
		t.Fatalf("the usage: used %d of %d, cost %v %q", a.agentUsed, a.agentSize, a.agentCost, a.agentCostCurrency)
	}
	if percent := a.agentUsagePercent(); percent != 27 {
		t.Fatalf("the percentage: %d", percent)
	}
	a.rightPanel, a.rightSection = true, "agent"
	tt := ui.NewTester(a.view, 1280, 840)
	tt.Frame()
	if !tt.HasText("27%") || !tt.HasText("53.0K/200.0K") {
		t.Fatalf("the usage is not there: %v", tt.Texts())
	}

	// A full window warns once, and says so in the notice.
	var update acpUpdate
	update.Update.Used, update.Update.Size = 190000, 200000
	a.applyAgentUsage(update)
	if a.messageKind != "warning" || a.message == "" {
		t.Fatalf("the warning: %q %q", a.message, a.messageKind)
	}
	a.message, a.messageKind = "", ""
	a.applyAgentUsage(update)
	if a.message != "" {
		t.Fatal("the warning repeated for the same window")
	}
}

// TestTheSessionNamesItsTab follows the title the agent gives a session,
// and keeps the one the user typed over it.
func TestTheSessionNamesItsTab(t *testing.T) {
	a, queue := startAgentHelper(t)
	defer a.stopAgent()
	pumpUntil(t, queue, func() bool { return a.chatActive().title == "助手会话" })
	if tab := a.chatActive(); !tab.titled {
		t.Fatal("the title of the agent did not stick")
	}

	a.chatRename = &chatRenamePrompt{tab: a.chatActive(), value: "我的对话"}
	a.renameChat()
	if tab := a.chatActive(); tab.title != "我的对话" || !tab.renamed {
		t.Fatalf("the renamed tab: %+v", tab)
	}

	var update acpUpdate
	update.SessionID = "s1"
	update.Update.Title = "Agent 的新标题"
	a.applyAgentUpdate(update)
	if title := a.chatActive().title; title != "我的对话" {
		t.Fatalf("the agent replaced the name: %q", title)
	}

	// A message does not replace a name either.
	a.agentInput = "嗨"
	a.sendToAgent()
	pumpUntil(t, queue, func() bool { return !a.agentBusy })
	if title := a.chatActive().title; title != "我的对话" {
		t.Fatalf("the message replaced the name: %q", title)
	}
}
