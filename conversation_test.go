package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"
	"github.com/egoist/mygo/ui/editor"
	"vael/internal/core"
	"vael/internal/workspace"
)

func TestTheAgentOffersItsCommands(t *testing.T) {
	a, queue := startAgentHelper(t)
	defer a.stopAgent()
	pumpUntil(t, queue, func() bool { return len(a.agentCommands) > 0 })
	if a.agentCommands[0].Name != "review" {
		t.Fatalf("commands: %+v", a.agentCommands)
	}
	a.agentInput, a.agentCaret, a.agentCaretPending = "/rev", 4, 4
	a.updateAgentToken()
	candidates := a.agentCandidates()
	if len(candidates) == 0 {
		t.Fatal("the command was not offered for /rev")
	}
	a.rightPanel, a.rightSection = true, "agent"
	tt := ui.NewTester(a.view, 1280, 840)
	tt.Frame()
	if err := tt.Click("插入 /review · 审查代码"); err != nil {
		t.Fatalf("the command chip is not there: %v (%v)", err, tt.Texts())
	}
	if !strings.HasPrefix(a.agentInput, "/review ") || a.agentCommand != "review" {
		t.Fatalf("the command was not inserted: %q %q", a.agentInput, a.agentCommand)
	}
	a.agentInput += "检查 main.go"
	a.sendToAgent()
	pumpUntil(t, queue, func() bool { return !a.agentBusy })
	if lines := a.agentLines; len(lines) == 0 || lines[len(lines)-1].Kind != "agent" {
		t.Fatalf("the agent did not answer: %+v", lines)
	}
	if a.agentRefs != nil || a.agentCommand != "" {
		t.Fatal("the command stayed after sending")
	}
}

func TestAnAtMentionCarriesTheFileToThePrompt(t *testing.T) {
	a := newApp()
	defer a.shutdown()
	a.root = "/project"
	a.index.Files = []string{"main.go", "internal/store/store.go"}
	a.agentInput, a.agentCaret = "看看 @store", 10
	a.updateAgentToken()
	candidates := a.agentCandidates()
	if len(candidates) == 0 || !strings.Contains(candidates[0].label, "store.go") {
		t.Fatalf("candidates: %+v", candidates)
	}
	a.chooseAgentCandidate(candidates[0])
	if !strings.Contains(a.agentInput, "@internal/store/store.go") {
		t.Fatalf("the path was not inserted: %q", a.agentInput)
	}
	if len(a.agentRefs) != 1 || a.agentRefs[0].Path != "/project/internal/store/store.go" {
		t.Fatalf("the reference: %+v", a.agentRefs)
	}
	blocks := a.agentPromptBlocks(a.agentInput)
	if len(blocks) < 2 {
		t.Fatalf("no content blocks: %+v", blocks)
	}
	link, ok := blocks[1].(map[string]any)
	if !ok || link["type"] != "resource_link" || link["name"] != "internal/store/store.go" {
		t.Fatalf("the file block: %+v", blocks)
	}
	if !strings.Contains(link["uri"].(string), "internal/store/store.go") {
		t.Fatalf("the uri: %v", link["uri"])
	}
	// A mention written by hand also travels with the prompt.
	a.agentRefs = nil
	blocks = a.agentPromptBlocks("解释 @main.go")
	if len(blocks) < 2 || blocks[1].(map[string]any)["name"] != "main.go" {
		t.Fatalf("a typed mention was not sent: %+v", blocks)
	}
}

func TestSelectedCodeIsAddedToTheConversation(t *testing.T) {
	a := newApp()
	defer a.shutdown()
	a.root = t.TempDir()
	path := filepath.Join(a.root, "main.go")
	b := editor.NewBuffer("package main\n\nfunc main() {}\n")
	tab := &fileTab{path: path, state: &editor.State{Buffer: b}}
	tab.state.Select(14, 24)
	a.addSelectionToAgent(tab)
	if len(a.agentRefs) != 1 || a.agentRefs[0].Kind != "selection" {
		t.Fatalf("the reference: %+v", a.agentRefs)
	}
	if a.agentRefs[0].Line != "3" || !strings.Contains(a.agentRefs[0].Text, "func main") {
		t.Fatalf("the selected text: %+v", a.agentRefs[0])
	}
	if !a.rightPanel || a.rightSection != "agent" {
		t.Fatal("the conversation was not opened")
	}
	blocks := a.agentPromptBlocks("这段代码做什么？")
	if len(blocks) < 2 || !strings.Contains(blocks[1].(map[string]any)["text"].(string), "func main") {
		t.Fatalf("the selection did not travel: %+v", blocks)
	}
	// Adding the same selection twice keeps one chip.
	a.addSelectionToAgent(tab)
	if len(a.agentRefs) != 1 {
		t.Fatalf("the reference was duplicated: %+v", a.agentRefs)
	}
}

func TestFileTreeAddsAFileToTheConversation(t *testing.T) {
	a, queue := testDispatchApp(t)
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "main.go"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	a.useProject(core.NewLocal(), nil, root)
	pumpUntil(t, queue, func() bool { return len(a.entries[a.root]) > 0 })
	tt := ui.NewTester(a.view, 1280, 840)
	tt.Frame()
	if err := tt.RightClick("main.go"); err != nil {
		t.Fatal(err)
	}
	if err := tt.ChooseMenuItem("添加到对话"); err != nil {
		t.Fatal(err)
	}
	if len(a.agentRefs) != 1 || a.agentRefs[0].Path != filepath.Join(root, "main.go") {
		t.Fatalf("the tree file was not added: %+v", a.agentRefs)
	}
	tt.Frame()
	if _, ok := tt.Find("移除引用 main.go"); !ok {
		t.Fatalf("the chip is not shown: %v", tt.Texts())
	}
}

func TestTheAgentPanelKeepsConversationsAsItsOwnTabs(t *testing.T) {
	a := newApp()
	defer a.shutdown()
	a.root = t.TempDir()
	path := filepath.Join(a.root, "main.go")
	b := editor.NewBuffer("package main\n")
	a.tabs = []*fileTab{{path: path, state: &editor.State{Buffer: b}}}
	a.selected = 0
	a.rightPanel, a.rightSection = true, "agent"
	a.ensureChats()
	a.agentStatus = "已连接"
	a.agentLines = []agentLine{{Kind: "you", Text: "第一条对话"}}
	a.chatSave()
	a.chatNew()
	a.agentLines = []agentLine{{Kind: "you", Text: "第二条对话"}}
	a.chatSave()
	tt := ui.NewTester(a.view, 1280, 840)
	tt.Frame()
	for _, text := range []string{"第一条对话", "第二条对话", "Agent 输入"} {
		if _, ok := tt.Find(text); !ok {
			t.Fatalf("%q is missing: %v", text, tt.Texts())
		}
	}
	// The conversations never become editor tabs.
	if len(a.tabs) != 1 || a.tabs[0].path != path {
		t.Fatalf("the conversations entered the editor tabs: %+v", a.tabs)
	}
	if err := tt.Click("第一条对话"); err != nil {
		t.Fatal(err)
	}
	if len(a.agentLines) != 1 || a.agentLines[0].Text != "第一条对话" {
		t.Fatalf("the first tab did not come back: %+v", a.agentLines)
	}
	if err := tt.Click("第二条对话"); err != nil {
		t.Fatal(err)
	}
	if a.agentLines[0].Text != "第二条对话" {
		t.Fatalf("the second tab was lost: %+v", a.agentLines)
	}
	if err := tt.Click("关闭对话 第一条对话"); err != nil {
		t.Fatal(err)
	}
	if len(a.chatTabs) != 1 || a.chatTabs[0].title != "第二条对话" {
		t.Fatalf("the tab did not close: %+v", a.chatTabs)
	}
	if len(a.tabs) != 1 {
		t.Fatal("closing a conversation touched the editor's file tabs")
	}
}

func TestOpeningAFileLeavesTheAgentTabsAlone(t *testing.T) {
	a := newApp()
	defer a.shutdown()
	a.root = t.TempDir()
	path := filepath.Join(a.root, "main.go")
	b := editor.NewBuffer("package main\n")
	a.tabs = []*fileTab{{path: path, state: &editor.State{Buffer: b}}}
	a.selected = 0
	a.rightPanel, a.rightSection = true, "agent"
	a.ensureChats()
	a.agentLines = []agentLine{{Kind: "you", Text: "第一条对话"}}
	a.chatSave()
	a.chatNew()
	a.agentLines = []agentLine{{Kind: "you", Text: "第二条对话"}}
	a.chatSave()
	tt := ui.NewTester(a.view, 1280, 840)
	tt.Frame()
	before := a.chatSel
	a.focusTab(0)
	tt.Frame()
	if a.chatSel != before {
		t.Fatal("opening a file changed the chosen conversation")
	}
	for _, text := range []string{"第一条对话", "第二条对话"} {
		if _, ok := tt.Find(text); !ok {
			t.Fatalf("the agent tab %q went away", text)
		}
	}
}

func TestAHistorySessionOpensInANewPanelTab(t *testing.T) {
	a, queue := startAgentHelper(t)
	defer a.stopAgent()
	a.root = t.TempDir()
	a.rightPanel, a.rightSection = true, "agent"
	a.ensureChats()
	before := len(a.chatTabs)
	now := time.Now()
	a.agentSessions = []acpSessionInfo{
		{SessionID: "session-today-123456", Title: "今天的会话", UpdatedAt: now.Add(-time.Hour).Format(time.RFC3339)},
		{SessionID: "session-old-123456789", Title: "昨天的会话", UpdatedAt: now.Add(-25 * time.Hour).Format(time.RFC3339)},
		{SessionID: "session-week-1234567", Title: "上周的会话", UpdatedAt: now.Add(-4 * 24 * time.Hour).Format(time.RFC3339)},
		{SessionID: "session-month-123456", Title: "上月的会话", UpdatedAt: now.Add(-20 * 24 * time.Hour).Format(time.RFC3339)},
	}
	a.agentSessionsLoaded = true
	a.agentHistoryOpen = true
	tt := ui.NewTester(a.view, 1280, 840)
	tt.Frame()
	for _, period := range []string{"今天", "昨天", "最近 7 天", "最近 30 天"} {
		if _, ok := tt.Find(period); !ok {
			t.Fatalf("the group %q is missing: %v", period, tt.Texts())
		}
	}
	if err := tt.Click("打开 昨天的会话"); err != nil {
		t.Fatalf("the history session is not listed: %v", err)
	}
	pumpUntil(t, queue, func() bool { return agentSays(a.agentLines, "agent", "旧回答") })
	if len(a.chatTabs) != before+1 {
		t.Fatalf("the session did not get a tab: %+v", a.chatTabs)
	}
	if a.chatTabs[len(a.chatTabs)-1].state.session != "session-old-123456789" {
		t.Fatalf("the wrong tab was opened: %+v", a.chatTabs)
	}
	if len(a.tabs) != 0 {
		t.Fatal("the session opened an editor tab")
	}
}

func TestChatPeriodPutsTheDaysInGroups(t *testing.T) {
	now := time.Now()
	when := func(d time.Duration) string { return now.Add(-d).Format(time.RFC3339) }
	for _, test := range []struct {
		at   string
		want string
	}{
		{when(time.Hour), "今天"},
		{when(28 * time.Hour), "昨天"},
		{when(4 * 24 * time.Hour), "最近 7 天"},
		{when(20 * 24 * time.Hour), "最近 30 天"},
		{when(200 * 24 * time.Hour), "更早"},
	} {
		if got := chatPeriod(test.at, now); got != test.want {
			t.Errorf("%s became %q, not %q", test.at, got, test.want)
		}
	}
}

func TestTheTabStripShowsByDefault(t *testing.T) {
	a := newApp()
	defer a.shutdown()
	a.root = t.TempDir()
	a.rightPanel, a.rightSection = true, "agent"
	a.ensureChats()
	a.agentStatus = "已连接"
	a.agentLines = []agentLine{{Kind: "you", Text: "唯一对话"}}
	a.chatSave()
	tt := ui.NewTester(a.view, 1280, 840)
	tt.Frame()
	if _, ok := tt.Find("关闭对话 唯一对话"); !ok {
		t.Fatalf("the strip is missing for one conversation: %v", tt.Texts())
	}
	a.chatNew()
	a.agentLines = []agentLine{{Kind: "you", Text: "第二对话"}}
	a.chatSave()
	tt.Frame()
	if _, ok := tt.Find("关闭对话 第二对话"); !ok {
		t.Fatalf("the second tab is missing: %v", tt.Texts())
	}
}

func TestOpeningTheAgentPanelStartsTheChosenAgent(t *testing.T) {
	a, queue := testDispatchApp(t)
	defer a.shutdown()
	a.root = t.TempDir()
	helper := "VAEL_ACP_AGENT=1 " + os.Args[0] + " -test.run=TestACPAgentHelper"
	if err := a.agents.Add(workspace.Agent{Name: "Helper", Command: helper}); err != nil {
		t.Fatal(err)
	}
	a.agentChosen = "Helper"
	a.rightPanel, a.rightSection = true, "agent"
	tt := ui.NewTester(a.view, 1280, 840)
	tt.Frame()
	pumpUntil(t, queue, func() bool { return a.agentStatus == "已连接" })
	if a.agentConn == nil || a.agentSession == "" {
		t.Fatalf("the agent did not start: %v %q", a.agentConn != nil, a.agentSession)
	}
}

func TestTheNewChatButtonListsTheAgentTypes(t *testing.T) {
	a, queue := startAgentHelper(t)
	defer a.stopAgent()
	a.rightPanel, a.rightSection = true, "agent"
	if err := a.agents.Add(workspace.Agent{Name: "Other", Command: "other --acp"}); err != nil {
		t.Fatal(err)
	}
	tt := ui.NewTester(a.view, 1280, 840)
	tt.Frame()
	if err := tt.Click("新对话 · 选择 Agent"); err != nil {
		t.Fatalf("the new chat button is missing: %v", err)
	}
	tt.Frame()
	for _, want := range []string{"新对话 · Helper", "新对话 · Other", "在设置中添加…"} {
		if _, ok := tt.Find(want); !ok {
			t.Fatalf("%q is not in the floating panel: %v", want, tt.Texts())
		}
	}
	// The panel's title no longer closes it; the bar on the right does.
	if _, ok := tt.Find("关闭面板"); ok {
		t.Fatal("the title still has a close button")
	}
	pumpUntil(t, queue, func() bool { return a.agentConn != nil })
}

func TestTheAgentToolbarHasNewChatAndHistoryOrNotConnected(t *testing.T) {
	a := newApp()
	defer a.shutdown()
	a.root = t.TempDir()
	a.rightPanel, a.rightSection = true, "agent"
	tt := ui.NewTester(a.view, 1280, 840)
	tt.Frame()
	for _, label := range []string{"新对话 · 选择 Agent", "会话历史"} {
		if _, ok := tt.Find(label); !ok {
			t.Fatalf("%q is missing before an agent starts: %v", label, tt.Texts())
		}
	}
	if err := tt.Click("会话历史"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if _, ok := tt.Find("先启动 Agent 再查看会话历史。"); !ok {
		t.Fatalf("the history panel says nothing when idle: %v", tt.Texts())
	}
}

func TestANewConversationTabStartsEmpty(t *testing.T) {
	a := newApp()
	defer a.shutdown()
	a.root = t.TempDir()
	a.ensureChats()
	a.agentLines = []agentLine{{Kind: "you", Text: "第一条对话"}}
	a.chatSave()
	a.chatNew()
	if len(a.agentLines) != 0 {
		t.Fatalf("the new conversation is not empty: %+v", a.agentLines)
	}
	if len(a.chatTabs) != 2 {
		t.Fatalf("tabs: %d", len(a.chatTabs))
	}
	a.chatSelect(0)
	if a.agentLines[0].Text != "第一条对话" {
		t.Fatalf("the first tab was lost: %+v", a.agentLines)
	}
}
