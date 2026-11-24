package main

import (
	"testing"

	"github.com/egoist/mygo/ui"
)

func TestTheAgentHistoryListsAndLoads(t *testing.T) {
	a, queue := startAgentHelper(t)
	defer a.stopAgent()
	if !a.agentCanList || !a.agentCanLoad {
		t.Fatalf("the capabilities: list=%v load=%v", a.agentCanList, a.agentCanLoad)
	}
	a.rightPanel = true
	a.rightSection = "agent"
	tt := ui.NewTester(a.view, 1280, 840)
	tt.Frame()
	if err := tt.Click("会话历史"); err != nil {
		t.Fatal(err)
	}
	pumpUntil(t, queue, func() bool { return len(a.agentSessions) > 0 })
	tt.Frame()
	if _, ok := tt.Find("打开 实现会话列表"); !ok {
		t.Fatalf("the history is not drawn: %v", tt.Texts())
	}
	if got := a.agentSessions[0].Title; got != "当前会话" {
		t.Fatalf("the newest session is %q", got)
	}
	if err := tt.Click("打开 实现会话列表"); err != nil {
		t.Fatal(err)
	}
	pumpUntil(t, queue, func() bool {
		return a.agentSession == "session-old-123456789" && agentSays(a.agentLines, "agent", "旧回答") && !a.agentLoading
	})
	if !agentSays(a.agentLines, "you", "旧问题") {
		t.Fatalf("the replay lost what the user said: %+v", a.agentLines)
	}
	if a.agentLoading {
		t.Fatal("the session is still loading")
	}
	if a.agentStatus != "已连接" {
		t.Fatalf("the status: %q", a.agentStatus)
	}
	if configOf(a.agentConfig, "model") == nil {
		t.Fatalf("the loaded options: %+v", a.agentConfig)
	}
}

func TestTheToolbarHasOneStop(t *testing.T) {
	a, queue := startAgentHelper(t)
	defer a.stopAgent()
	a.rightPanel = true
	a.rightSection = "agent"
	a.agentBusy = true
	tt := ui.NewTester(a.view, 1280, 840)
	tt.Frame()
	if _, ok := tt.Find("取消本轮"); ok {
		t.Fatalf("the toolbar still cancels the turn: %v", tt.Texts())
	}
	if _, ok := tt.Find("停止 Agent"); !ok {
		t.Fatalf("the agent can no longer be stopped: %v", tt.Texts())
	}
	if _, ok := tt.Find("停止"); !ok {
		t.Fatalf("the composer's stop is gone: %v", tt.Texts())
	}
	_ = queue
}

func TestTheHistoryWithoutTheListCapability(t *testing.T) {
	a, queue := startAgentHelper(t)
	defer a.stopAgent()
	a.agentCanList = false
	a.rightPanel = true
	a.rightSection = "agent"
	tt := ui.NewTester(a.view, 1280, 840)
	tt.Frame()
	if err := tt.Click("会话历史"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if _, ok := tt.Find("这个 Agent 不支持列出会话（session/list）。"); !ok {
		t.Fatalf("the note is not drawn: %v", tt.Texts())
	}
	_ = queue
}

func TestASessionIsNamedInShort(t *testing.T) {
	if got := shortSessionID("session-old-123456789"); got != "session-…6789" {
		t.Fatalf("the short ID: %q", got)
	}
	if got := shortSessionID("s1"); got != "s1" {
		t.Fatalf("a short ID was cut: %q", got)
	}
	if got := sessionWhen(""); got != "" {
		t.Fatalf("no date became %q", got)
	}
}
