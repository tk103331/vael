package main

import (
	"strings"
	"testing"

	"github.com/egoist/mygo/ui"
)

// TestTheAgentOpensAChildSession covers what an agent says about a child
// session of its own: the child gets a tab of its own, the panel shows
// what it is doing, and the messages between them are shown in the
// conversation they belong to.
func TestTheAgentOpensAChildSession(t *testing.T) {
	a, queue := startAgentHelper(t)
	defer a.stopAgent()

	a.agentInput = "subagent"
	a.sendToAgent()
	pumpUntil(t, queue, func() bool { return !a.agentBusy && len(a.chatTabs) > 1 })

	var child *chatTab
	for _, tab := range a.chatTabs {
		if tab.subagent {
			child = tab
		}
	}
	if child == nil {
		t.Fatalf("the child session did not get a tab: %+v", a.chatTabs)
	}
	if child.title != "子任务" || child.subState != "running" {
		t.Fatalf("the child tab: %+v", child)
	}
	if child.state.session != "child1" {
		t.Fatalf("the child session: %+v", child.state.session)
	}

	// The message the child sent to the parent shows in the conversation
	// of the parent, with the way it went.
	if !agentSays(a.agentLines, "relay", "← 子任务：子任务做完了") {
		t.Fatalf("the message between the sessions: %+v", a.agentLines)
	}

	// The child session has no box of its own: the main agent drives it.
	a.chatSelect(a.chatIndexOf(child))
	a.rightPanel, a.rightSection = true, "agent"
	tt := ui.NewTester(a.view, 1280, 840)
	tt.Frame()
	if !tt.HasText("子 Agent 会话") {
		t.Fatalf("the child panel: %v", tt.Texts())
	}
	if tt.HasText("Agent 输入") {
		t.Fatalf("the child session has a box: %v", tt.Texts())
	}
}

// TestTheAgentNoticesAndCompaction covers the two things an agent says
// outside the conversation: a notice, and the compaction of its context.
func TestTheAgentNoticesAndCompaction(t *testing.T) {
	a, queue := startAgentHelper(t)
	defer a.stopAgent()

	a.agentInput = "notice"
	a.sendToAgent()
	pumpUntil(t, queue, func() bool { return !a.agentBusy && a.message != "" })
	if !strings.Contains(a.message, "注意") || a.messageKind != "warning" {
		t.Fatalf("the notice: %q %q", a.message, a.messageKind)
	}

	a.agentInput = "compact"
	a.sendToAgent()
	pumpUntil(t, queue, func() bool { return !a.agentBusy && agentSaysContains(a.agentLines, "note", "上下文已压缩") })
	if !agentSaysContains(a.agentLines, "note", "留下了摘要") {
		t.Fatalf("the compaction: %+v", a.agentLines)
	}
}

// agentSaysContains tells whether a line of a kind holds the words.
func agentSaysContains(lines []agentLine, kind, words string) bool {
	for _, line := range lines {
		if line.Kind == kind && strings.Contains(line.Text, words) {
			return true
		}
	}
	return false
}

// TestTheAgentKeepsItsPlan covers the plan the agent sends in its new
// form, with entries of its own.
func TestTheAgentKeepsItsPlan(t *testing.T) {
	a, queue := startAgentHelper(t)
	defer a.stopAgent()
	a.agentInput = "plan"
	a.sendToAgent()
	pumpUntil(t, queue, func() bool { return !a.agentBusy && len(a.agentPlan) == 2 })
	if a.agentPlan[1].Content != "改代码" || a.agentPlan[1].Status != "in_progress" {
		t.Fatalf("the plan: %+v", a.agentPlan)
	}
	if !a.agentPlanOpen {
		t.Fatal("the plan is closed")
	}
}
