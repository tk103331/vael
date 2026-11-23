package main

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/egoist/mygo/ui"
	"vael/internal/workspace"
)

func TestTheSettingsAddAnACPAgent(t *testing.T) {
	a := newApp()
	defer a.shutdown()
	a.root = t.TempDir()
	a.agentsPath = filepath.Join(t.TempDir(), "agents.json")
	// The registry the panel offers, as the network would give it.
	a.registry = []acpRegistryAgent{{
		ID: "gemini", Name: "Gemini CLI", Version: "0.70.0",
		Distribution: acpDistribution{Npx: &acpPackage{Package: "@google/gemini-cli@0.70.0", Args: []string{"--acp"}}},
	}}
	a.registryHas["npx"] = true
	a.settingsOpen = true
	tt := ui.NewTester(a.view, 1280, 840)
	tt.Frame()
	if err := tt.Click("ACP Agent"); err != nil {
		t.Fatal(err)
	}
	if a.settingsSection != settingsAgent {
		t.Fatalf("section %q", a.settingsSection)
	}
	tt.Frame()
	if err := tt.Click("从 ACP Registry 添加…"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if err := tt.Click("Gemini CLI"); err != nil {
		t.Fatal(err)
	}
	if len(a.agents.Items) != 1 || a.agents.Items[0].Name != "Gemini CLI" {
		t.Fatalf("agents: %+v", a.agents)
	}
	if a.agents.Default != "Gemini CLI" {
		t.Fatalf("default: %+v", a.agents)
	}
	saved, err := workspace.LoadAgents(a.agentsPath)
	if err != nil || len(saved.Items) != 1 || saved.Items[0].Command != "npx -y @google/gemini-cli@0.70.0 --acp" {
		t.Fatalf("saved: %+v %v", saved, err)
	}
}

func TestTheAgentFormAddsAnAgent(t *testing.T) {
	a := newApp()
	defer a.shutdown()
	a.agentsPath = filepath.Join(t.TempDir(), "agents.json")
	a.agentAddOpen = true
	a.agentAddName = "我的 Agent"
	a.agentAddCommand = "my-agent --acp"
	a.addAgentFromForm()
	if len(a.agents.Items) != 1 || a.agents.Items[0].Command != "my-agent --acp" {
		t.Fatalf("agents: %+v", a.agents)
	}
	if a.agents.Default != "我的 Agent" || a.agentChosen != "我的 Agent" {
		t.Fatalf("chosen: %+v %q", a.agents, a.agentChosen)
	}
	if a.agentAddOpen {
		t.Fatal("the form did not close")
	}
	// A form without a command says what is missing, and adds nothing.
	a.agentAddOpen = true
	a.agentAddName = "另一个"
	a.agentAddCommand = ""
	a.addAgentFromForm()
	if len(a.agents.Items) != 1 || a.message == "" {
		t.Fatalf("agents: %+v %q", a.agents, a.message)
	}
}

func TestTheAgentPanelAsksForAnAgent(t *testing.T) {
	a := newApp()
	defer a.shutdown()
	a.agentInput = "给 store 包加一个计数器"
	a.sendToAgent()
	if len(a.agentLines) != 1 || a.agentLines[0].Kind != "note" {
		t.Fatalf("lines: %+v", a.agentLines)
	}
	if a.agentInput == "" {
		t.Fatal("the box was emptied without sending anything")
	}
}

// agentSays tells whether the conversation holds a line of a kind with
// the text.
func agentSays(lines []agentLine, kind, text string) bool {
	for _, line := range lines {
		if line.Kind == kind && line.Text == text {
			return true
		}
	}
	return false
}

func TestTheAgentPanelStartsAnAgent(t *testing.T) {
	a, queue := testDispatchApp(t)
	a.root = t.TempDir()
	helper := "VAEL_ACP_AGENT=1 " + os.Args[0] + " -test.run=TestACPAgentHelper"
	if err := a.agents.Add(workspace.Agent{Name: "Helper", Command: helper}); err != nil {
		t.Fatal(err)
	}
	a.agentChosen = "Helper"
	a.startAgent()
	pumpUntil(t, queue, func() bool { return a.agentStatus == "已连接" })
	if a.agentConn == nil || a.agentSession != "s1" {
		t.Fatalf("conn %v session %q", a.agentConn != nil, a.agentSession)
	}
	a.agentInput = "嗨"
	a.sendToAgent()
	pumpUntil(t, queue, func() bool { return !a.agentBusy && agentSays(a.agentLines, "agent", "你好") })
	if !agentSays(a.agentLines, "you", "嗨") {
		t.Fatalf("lines: %+v", a.agentLines)
	}
	a.stopAgent()
	if a.agentConn != nil || a.agentSession != "" || a.agentStatus != "已停止" {
		t.Fatalf("stopped: %v %q %q", a.agentConn != nil, a.agentSession, a.agentStatus)
	}
}

// agentLineOf finds the line of a kind, or nil.
func agentLineOf(lines []agentLine, kind string) *agentLine {
	for i := range lines {
		if lines[i].Kind == kind {
			return &lines[i]
		}
	}
	return nil
}

func TestTheAgentPanelShowsToolsPlansAndAsks(t *testing.T) {
	a, queue := testDispatchApp(t)
	a.root = t.TempDir()
	helper := "VAEL_ACP_AGENT=1 " + os.Args[0] + " -test.run=TestACPAgentHelper"
	if err := a.agents.Add(workspace.Agent{Name: "Helper", Command: helper}); err != nil {
		t.Fatal(err)
	}
	a.agentChosen = "Helper"
	a.startAgent()
	pumpUntil(t, queue, func() bool { return a.agentStatus == "已连接" })

	a.agentInput = "tool"
	a.sendToAgent()
	// The tool ran, the plan came, and the agent waits for the answer.
	pumpUntil(t, queue, func() bool { return len(a.agentPending) == 1 })
	tool := agentLineOf(a.agentLines, "tool")
	if tool == nil || tool.State != "completed" || tool.Text != "运行 go test" || tool.Detail != "ok" {
		t.Fatalf("tool: %+v", tool)
	}
	if len(a.agentPlan) != 3 || a.agentPlan[1].Status != "in_progress" {
		t.Fatalf("plan: %+v", a.agentPlan)
	}
	if !a.agentPlanOpen {
		t.Fatal("the plan is closed")
	}
	// Allowing it always answers the agent, and remembers the kind.
	ask := a.agentPending[0].ask
	if len(ask.Options) != 3 || ask.Options[1].Kind != "allow_always" {
		t.Fatalf("options: %+v", ask.Options)
	}
	a.answerAgent(ask.Options[1])
	if len(a.agentPending) != 0 {
		t.Fatalf("pending: %+v", a.agentPending)
	}
	if got := a.agentAllowed["edit"]; got != "always" {
		t.Fatalf("allowed: %+v", a.agentAllowed)
	}
	pumpUntil(t, queue, func() bool { return !a.agentBusy })
	if !agentSays(a.agentLines, "note", "已允许：写文件") {
		t.Fatalf("lines: %+v", a.agentLines)
	}

	// The next request of that kind needs no answer.
	a.agentInput = "tool"
	a.sendToAgent()
	pumpUntil(t, queue, func() bool { return !a.agentBusy })
	if len(a.agentPending) != 0 {
		t.Fatalf("the allowed kind asked again: %+v", a.agentPending)
	}
	if !agentSays(a.agentLines, "note", "已允许（此前选择了总是允许）：写文件") {
		t.Fatalf("lines: %+v", a.agentLines)
	}
	a.stopAgent()
}

func TestAnAgentThatNeedsTheUserSaysSo(t *testing.T) {
	a, queue := testDispatchApp(t)
	a.root = t.TempDir()
	helper := "VAEL_ACP_AGENT=2 " + os.Args[0] + " -test.run=TestACPAgentHelper"
	if err := a.agents.Add(workspace.Agent{Name: "Helper", Command: helper}); err != nil {
		t.Fatal(err)
	}
	a.agentChosen = "Helper"
	a.startAgent()
	pumpUntil(t, queue, func() bool { return a.agentStatus == "已连接" })
	a.agentInput = "问题"
	a.sendToAgent()
	pumpUntil(t, queue, func() bool { return !a.agentBusy })
	if !agentSays(a.agentLines, "note", "Agent 需要你的补充：直接回答即可。") {
		t.Fatalf("lines: %+v", a.agentLines)
	}
	a.stopAgent()
}
