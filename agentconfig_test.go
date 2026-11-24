package main

import (
	"encoding/json"
	"os"
	"strings"
	"testing"

	"github.com/egoist/mygo/ui"
	"vael/internal/workspace"
)

// configOf finds a selector of the session by its ID.
func configOf(config []acpConfigOption, id string) *acpConfigOption {
	for i := range config {
		if config[i].ID == id {
			return &config[i]
		}
	}
	return nil
}

// startAgentHelper starts the fake agent the protocol tests answer with.
func startAgentHelper(t *testing.T) (*app, chan func()) {
	t.Helper()
	a, queue := testDispatchApp(t)
	a.root = t.TempDir()
	helper := "VAEL_ACP_AGENT=1 " + os.Args[0] + " -test.run=TestACPAgentHelper"
	if err := a.agents.Add(workspace.Agent{Name: "Helper", Command: helper}); err != nil {
		t.Fatal(err)
	}
	a.agentChosen = "Helper"
	a.startAgent()
	pumpUntil(t, queue, func() bool { return a.agentStatus == "已连接" })
	return a, queue
}

func TestTheAgentSelectorsComeWithTheSession(t *testing.T) {
	a, queue := startAgentHelper(t)
	defer a.stopAgent()
	if len(a.agentConfig) != 3 {
		t.Fatalf("the options: %+v", a.agentConfig)
	}
	model := configOf(a.agentConfig, "model")
	if model == nil || !model.IsModel() || model.CurrentName() != "Fast" {
		t.Fatalf("the model: %+v", model)
	}
	if mode := configOf(a.agentConfig, "mode"); mode == nil || mode.Category != "mode" {
		t.Fatalf("the mode: %+v", mode)
	}
	thinking := configOf(a.agentConfig, "thinking")
	if thinking == nil || !thinking.IsBoolean() || thinking.Bool() {
		t.Fatalf("the switch: %+v", thinking)
	}
	// A choice, and a switch, each ask the agent, whose answer holds the
	// whole list.
	a.setAgentConfig(*model, "smart")
	pumpUntil(t, queue, func() bool {
		current := configOf(a.agentConfig, "model")
		return current != nil && current.CurrentName() == "Smart"
	})
	if got := configOf(a.agentConfig, "model"); got.Value() != "smart" {
		t.Fatalf("the model is %+v", got)
	}
	a.setAgentConfig(*thinking, true)
	pumpUntil(t, queue, func() bool {
		current := configOf(a.agentConfig, "thinking")
		return current != nil && current.Bool()
	})
}

func TestTheAgentModeChanges(t *testing.T) {
	a, queue := startAgentHelper(t)
	defer a.stopAgent()
	if a.agentModeID != "ask" || len(a.agentModes) != 2 {
		t.Fatalf("the modes: %q %+v", a.agentModeID, a.agentModes)
	}
	a.setAgentMode("code")
	pumpUntil(t, queue, func() bool { return a.agentModeID == "code" })
	if a.agentModeName() != "Code" {
		t.Fatalf("the mode is %q", a.agentModeName())
	}
}

func TestTheAgentComposerSendsWithEnter(t *testing.T) {
	a, queue := testDispatchApp(t)
	a.root = t.TempDir()
	helper := "VAEL_ACP_AGENT=1 " + os.Args[0] + " -test.run=TestACPAgentHelper"
	if err := a.agents.Add(workspace.Agent{Name: "Helper", Command: helper}); err != nil {
		t.Fatal(err)
	}
	a.agentChosen = "Helper"
	a.rightPanel = true
	a.rightSection = "agent"
	a.startAgent()
	pumpUntil(t, queue, func() bool { return a.agentStatus == "已连接" })
	defer a.stopAgent()

	tt := ui.NewTester(a.view, 1280, 840)
	tt.Frame()
	if _, ok := tt.Find("模型：Fast"); !ok {
		t.Fatalf("the model chip is not drawn: %v", tt.Texts())
	}
	if _, ok := tt.Find("模式：Ask"); !ok {
		t.Fatalf("the mode chip is not drawn: %v", tt.Texts())
	}
	if _, ok := tt.Find("思考：关"); !ok {
		t.Fatalf("the switch chip is not drawn: %v", tt.Texts())
	}
	if err := tt.Click("Agent 输入"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	tt.Type("嗨")
	tt.Frame()
	tt.Key(0, ui.KeyEnter)
	pumpUntil(t, queue, func() bool { return !a.agentBusy && agentSays(a.agentLines, "agent", "你好") })
	if !agentSays(a.agentLines, "you", "嗨") {
		t.Fatalf("the prompt was not sent: %+v", a.agentLines)
	}
	if strings.TrimSpace(a.agentInput) != "" {
		t.Fatalf("the composer kept %q", a.agentInput)
	}
}

func TestTheAgentModelChipChooses(t *testing.T) {
	a, queue := testDispatchApp(t)
	a.root = t.TempDir()
	helper := "VAEL_ACP_AGENT=1 " + os.Args[0] + " -test.run=TestACPAgentHelper"
	if err := a.agents.Add(workspace.Agent{Name: "Helper", Command: helper}); err != nil {
		t.Fatal(err)
	}
	a.agentChosen = "Helper"
	a.rightPanel = true
	a.rightSection = "agent"
	a.startAgent()
	pumpUntil(t, queue, func() bool { return a.agentStatus == "已连接" })
	defer a.stopAgent()

	tt := ui.NewTester(a.view, 1280, 840)
	tt.Frame()
	if err := tt.Click("模型：Fast"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if _, ok := tt.Find("选择 Smart"); !ok {
		t.Fatalf("the choices are not drawn: %v", tt.Texts())
	}
	if err := tt.Click("选择 Smart"); err != nil {
		t.Fatal(err)
	}
	pumpUntil(t, queue, func() bool { return configOf(a.agentConfig, "model").CurrentName() == "Smart" })
	tt.Frame()
	if _, ok := tt.Find("模型：Smart"); !ok {
		t.Fatalf("the chip did not change: %v", tt.Texts())
	}
}

func TestTheAgentPanelDrawsThoughtsToolsAndCode(t *testing.T) {
	a := newApp()
	defer a.shutdown()
	a.root = t.TempDir()
	a.rightPanel = true
	a.rightSection = "agent"
	a.agentLines = []agentLine{
		{Kind: "thought", Text: "先看看文件。\n再决定怎么改。", Collapsed: true},
		{Kind: "tool", Text: "读取 main.go", ToolKind: "read", State: "completed", Detail: "main.go: 3 行"},
		{Kind: "agent", Text: "改好了：\n```go\nfunc main() {}\n```\n可以跑一下。"},
	}
	tt := ui.NewTester(a.view, 1280, 840)
	tt.Frame()
	if _, ok := tt.Find("读取 main.go"); !ok {
		t.Fatalf("the tool is not drawn: %v", tt.Texts())
	}
	if _, ok := tt.Find("func main() {}"); !ok {
		t.Fatalf("the code block is not drawn: %v", tt.Texts())
	}
	if _, ok := tt.Find("main.go: 3 行"); !ok {
		t.Fatalf("the tool's detail is not drawn: %v", tt.Texts())
	}
	// A thought that is done is folded; its header opens it.
	if _, ok := tt.Find("先看看文件。\n再决定怎么改。"); ok {
		t.Fatalf("the thought is open before it is clicked: %v", tt.Texts())
	}
	if err := tt.Click("思考过程"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if _, ok := tt.Find("先看看文件。\n再决定怎么改。"); !ok {
		t.Fatalf("the thought did not open: %v", tt.Texts())
	}
	// The tool's detail folds too.
	if err := tt.Click("读取 main.go"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if _, ok := tt.Find("main.go: 3 行"); ok {
		t.Fatalf("the tool's detail did not fold: %v", tt.Texts())
	}
}

func TestThePermissionShowsWhatItAsks(t *testing.T) {
	var ask acpPermission
	ask.ToolCall.ToolCallID = "t2"
	ask.ToolCall.Title = "运行 go test"
	ask.ToolCall.Kind = "execute"
	ask.ToolCall.RawInput = json.RawMessage(`{"command":"go test ./..."}`)
	ask.Options = []acpPermissionOption{
		{OptionID: "once", Name: "允许一次", Kind: "allow_once"},
		{OptionID: "always", Name: "总是允许", Kind: "allow_always"},
		{OptionID: "no", Name: "拒绝", Kind: "reject_once"},
	}
	a := newApp()
	defer a.shutdown()
	a.root = t.TempDir()
	a.rightPanel = true
	a.rightSection = "agent"
	a.agentPending = []acpPending{{id: 7, ask: ask}}
	tt := ui.NewTester(a.view, 1280, 840)
	tt.Frame()
	for _, text := range []string{"Agent 请求允许：运行 go test", "go test ./...", "允许一次", "总是允许", "拒绝"} {
		if _, ok := tt.Find(text); !ok {
			t.Fatalf("%q is not drawn: %v", text, tt.Texts())
		}
	}
	if err := tt.Click("拒绝"); err != nil {
		t.Fatal(err)
	}
	if len(a.agentPending) != 0 {
		t.Fatalf("the request still waits: %+v", a.agentPending)
	}
	if !agentSays(a.agentLines, "note", "已拒绝：运行 go test") {
		t.Fatalf("lines: %+v", a.agentLines)
	}
}

func TestAPermissionCommandIsRead(t *testing.T) {
	if got := permissionCommand(json.RawMessage(`{"command":"go test ./..."}`)); got != "go test ./..." {
		t.Fatalf("the command: %q", got)
	}
	if got := permissionCommand(json.RawMessage(`"rm -rf build"`)); got != "rm -rf build" {
		t.Fatalf("the words: %q", got)
	}
	if got := permissionCommand(nil); got != "" {
		t.Fatalf("nothing: %q", got)
	}
}

func TestTheAgentMarkdownIsSplit(t *testing.T) {
	blocks := splitAgentText("先说：\n```go\nfunc main() {}\n```\n再说。")
	if len(blocks) != 3 || blocks[1].code != true || blocks[1].lang != "go" ||
		blocks[1].text != "func main() {}" {
		t.Fatalf("the blocks: %+v", blocks)
	}
	spans := agentSpans("用 `go test` 或**直接跑**", 11.5, ui.Hex("#ffffff"), palette{})
	foundCode, foundBold := false, false
	for _, span := range spans {
		if span.Text == "go test" && span.Font == "monospace" {
			foundCode = true
		}
		if span.Text == "直接跑" && span.Weight == 650 {
			foundBold = true
		}
	}
	if !foundCode || !foundBold {
		t.Fatalf("the spans: %+v", spans)
	}
}

// TestTheAgentConfigBarPutsTheModelFirstAndShowsThinkingOnce checks the
// row of selectors: the model first, the thinking option named once, and
// no mode chip beside it when the agent offers config options.
func TestTheAgentConfigBarPutsTheModelFirstAndShowsThinkingOnce(t *testing.T) {
	a := newApp()
	defer a.shutdown()
	a.root = t.TempDir()
	a.rightPanel = true
	a.rightSection = "agent"
	a.agentConfig = []acpConfigOption{
		{ID: "mode", Name: "模式", Category: "mode", Type: "select",
			CurrentValue: json.RawMessage(`"code"`),
			Options:      []acpConfigChoice{{Value: "ask", Name: "Ask"}, {Value: "code", Name: "Code"}}},
		{ID: "thinking", Name: "Thinking", Category: "thought_level", Type: "select",
			CurrentValue: json.RawMessage(`"medium"`),
			Options:      []acpConfigChoice{{Value: "low", Name: "Thinking: low"}, {Value: "medium", Name: "Thinking: medium"}}},
		{ID: "model", Name: "Model", Category: "model", Type: "select",
			CurrentValue: json.RawMessage(`"gpt"`),
			Options:      []acpConfigChoice{{Value: "gpt", Name: "magpie/GPT-6.1-Sol"}}},
	}
	// An agent that names its modes too, as the one of the report did.
	a.agentModes = []acpMode{{ID: "m", Name: "Thinking: medium"}}
	a.agentModeID = "m"
	tt := ui.NewTester(a.view, 1280, 840)
	tt.Frame()
	texts := strings.Join(tt.Texts(), "\n")
	model := strings.Index(texts, "Model：magpie/GPT-6.1-Sol")
	thinking := strings.Index(texts, "Thinking: medium")
	configMode := strings.Index(texts, "模式：Code")
	if model < 0 || thinking < 0 || configMode < 0 {
		t.Fatalf("the chips are not drawn: %v", tt.Texts())
	}
	if model > configMode || model > thinking {
		t.Fatalf("the model is not first: model %d, mode %d, thinking %d", model, configMode, thinking)
	}
	if strings.Contains(texts, "Thinking：Thinking: medium") {
		t.Fatalf("the thought option names its value twice: %v", tt.Texts())
	}
	if strings.Contains(texts, "模式：Thinking: medium") {
		t.Fatalf("the mode chip doubles the thought option: %v", tt.Texts())
	}
}

func TestTheAgentConfigOrderKeepsTheRest(t *testing.T) {
	options := []acpConfigOption{
		{ID: "mode"},
		{ID: "thinking", Category: "thought_level"},
		{ID: "model", Category: "model"},
		{ID: "context", Category: "model_config"},
	}
	ordered := agentConfigOrder(options)
	got := []string{ordered[0].ID, ordered[1].ID, ordered[2].ID, ordered[3].ID}
	want := []string{"model", "mode", "thinking", "context"}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("the order: %v, not %v", got, want)
		}
	}
}

func TestALegacyModeIsShownWithoutConfigOptions(t *testing.T) {
	a := newApp()
	defer a.shutdown()
	a.root = t.TempDir()
	a.rightPanel = true
	a.rightSection = "agent"
	a.agentModes = []acpMode{{ID: "ask", Name: "Ask"}, {ID: "code", Name: "Code"}}
	a.agentModeID = "code"
	tt := ui.NewTester(a.view, 1280, 840)
	tt.Frame()
	if _, ok := tt.Find("模式：Code"); !ok {
		t.Fatalf("the mode chip is not drawn: %v", tt.Texts())
	}
}

func TestAnUnknownConfigTypeIsLeftAlone(t *testing.T) {
	shown := agentConfigShown(acpConfigOption{ID: "x", Type: ""})
	if !shown {
		t.Fatal("a select of no type is hidden")
	}
	if !agentConfigShown(acpConfigOption{ID: "x", Type: "boolean"}) {
		t.Fatal("a switch is hidden")
	}
	if agentConfigShown(acpConfigOption{ID: "x", Type: "_custom"}) {
		t.Fatal("an unknown type is shown")
	}
	if agentConfigShown(acpConfigOption{}) {
		t.Fatal("an option without an ID is shown")
	}
}

// TestTheAgentContentIsAllHis checks that one Agent mark heads the
// thought, tools and answer of a turn, instead of naming each of them.
func TestTheAgentContentIsAllHis(t *testing.T) {
	a := newApp()
	defer a.shutdown()
	a.root = t.TempDir()
	a.rightPanel = true
	a.rightSection = "agent"
	a.agentLines = []agentLine{
		{Kind: "you", Text: "嗨"},
		{Kind: "thought", Text: "写点什么。", Collapsed: true},
		{Kind: "tool", Text: "读取 main.go", ToolKind: "read", State: "completed", Detail: "main.go"},
		{Kind: "agent", Text: "你好。"},
		{Kind: "you", Text: "再来一次"},
		{Kind: "agent", Text: "好的。"},
	}
	tt := ui.NewTester(a.view, 1280, 840)
	tt.Frame()
	agents := 0
	for _, text := range tt.Texts() {
		if text == "Agent" {
			agents++
		}
	}
	// The panel's title is one more "Agent" than the two marks.
	if agents != 3 {
		t.Fatalf("the Agent marks: %d, %v", agents, tt.Texts())
	}
	for _, text := range tt.Texts() {
		if text == "· 思考" || text == "· 工具" {
			t.Fatalf("a mark names its kind again: %v", tt.Texts())
		}
	}
	if _, ok := tt.Find("你"); !ok {
		t.Fatalf("the user is not named: %v", tt.Texts())
	}
}
