package main

import (
	"math"
	"strings"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"
)

// TestTheConversationKeepsTheComposerInView checks that a long
// conversation scrolls in its own room, instead of pushing the box that
// asks the agent below the panel.
func TestTheConversationKeepsTheComposerInView(t *testing.T) {
	a := newApp()
	defer a.shutdown()
	a.root = t.TempDir()
	a.rightPanel = true
	a.rightSection = "agent"
	a.agentStatus = "已连接"
	for i := 0; i < 60; i++ {
		a.agentLines = append(a.agentLines, agentLine{Kind: "agent", Text: strings.Repeat("长消息 ", 20)})
	}
	tt := ui.NewTester(a.view, 1280, 840)
	tt.Frame()
	tt.Frame()
	send, ok := tt.Find("发送")
	if !ok || send.H == 0 || send.W == 0 {
		t.Fatalf("the send button is out of the panel: %v %+v", ok, send)
	}
	input, ok := tt.Find("Agent 输入")
	if !ok || input.H == 0 || input.Y+input.H > send.Y {
		t.Fatalf("the composer is out of the panel: %v %+v %+v", ok, input, send)
	}
	if a.agentScroll.MaxY <= 0 {
		t.Fatalf("the conversation does not scroll: %+v", a.agentScroll)
	}
}

func TestTheConversationGoesBackToItsEnd(t *testing.T) {
	a := newApp()
	defer a.shutdown()
	a.root = t.TempDir()
	a.rightPanel = true
	a.rightSection = "agent"
	a.agentStatus = "已连接"
	for i := 0; i < 60; i++ {
		a.agentLines = append(a.agentLines, agentLine{Kind: "agent", Text: strings.Repeat("长消息 ", 20)})
	}
	tt := ui.NewTester(a.view, 1280, 840)
	tt.Frame()
	tt.Frame()
	// At the end, the button is not there.
	a.agentScroll.Y = math.MaxFloat32
	tt.Frame()
	tt.Frame()
	if _, ok := tt.Find("回到底部"); ok {
		t.Fatal("the button shows at the end of the conversation")
	}
	a.agentScroll.Y = 0
	tt.Frame()
	tt.Frame()
	if _, ok := tt.Find("回到底部"); !ok {
		t.Fatalf("the button does not show above the conversation: %v", tt.Texts())
	}
	if err := tt.Click("回到底部"); err != nil {
		t.Fatal(err)
	}
	// The glide to the end moves the scroll over the frames that follow.
	for i := 0; i < 30 && a.agentScroll.Y < a.agentScroll.MaxY/2; i++ {
		tt.Advance(100 * time.Millisecond)
		tt.Frame()
	}
	if a.agentScroll.Y < a.agentScroll.MaxY/2 {
		t.Fatalf("the conversation did not go back: %v of %v", a.agentScroll.Y, a.agentScroll.MaxY)
	}
}
