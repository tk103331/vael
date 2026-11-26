package main

import (
	"bytes"
	"encoding/base64"
	"image"
	"image/png"
	"strings"
	"testing"

	"github.com/egoist/mygo/ui"
)

// TestTheComposerShowsTheQueueAndPictures draws the panel with a waiting
// message and an attached picture, so that the chips of both hold.
func TestTheComposerShowsTheQueueAndPictures(t *testing.T) {
	a := newApp()
	defer func() {
		a.agentConn, a.agentSession = nil, ""
		a.shutdown()
	}()
	a.root = t.TempDir()
	a.rightPanel, a.rightSection = true, "agent"
	a.agentConn, a.agentSession, a.agentStatus = &acpConn{}, "s1", "已连接"
	a.agentBusy = true
	a.agentQueue = []agentQueued{{Text: "排队的问题"}}

	var data bytes.Buffer
	if err := png.Encode(&data, image.NewRGBA(image.Rect(0, 0, 4, 3))); err != nil {
		t.Fatal(err)
	}
	a.addAgentImage(data.Bytes(), "shot.png")

	tt := ui.NewTester(a.view, 1280, 840)
	tt.Frame()
	if !tt.HasText("待发送 1 条") || !tt.HasText("排队的问题") {
		t.Fatalf("the queue is not there: %v", tt.Texts())
	}
	if !tt.HasText("shot.png · 4×3") {
		t.Fatalf("the picture is not there: %v", tt.Texts())
	}
}

// TestMessagesWaitWhileTheAgentWorks covers the queue of the composer:
// what is typed during a turn waits, with its references, and goes back
// to the box when the user wants to edit it.
func TestMessagesWaitWhileTheAgentWorks(t *testing.T) {
	a := newApp()
	defer a.shutdown()
	// A connection that is there, and a turn that runs: nothing is sent,
	// so no agent process is needed.
	a.agentConn = &acpConn{}
	a.agentSession = "s1"
	a.agentBusy = true

	a.agentInput = "第二条"
	a.sendToAgent()
	if len(a.agentQueue) != 1 || a.agentQueue[0].Text != "第二条" {
		t.Fatalf("the queue: %+v", a.agentQueue)
	}
	if a.agentInput != "" {
		t.Fatalf("the box still holds %q", a.agentInput)
	}
	if agentSays(a.agentLines, "you", "第二条") {
		t.Fatal("a message of the queue was shown as sent")
	}

	a.agentInput = "第三条"
	a.agentRefs = []agentRef{{Kind: "file", Path: "/p/main.go", Rel: "main.go"}}
	a.sendToAgent()
	if len(a.agentQueue) != 2 || len(a.agentQueue[1].Refs) != 1 {
		t.Fatalf("the second message: %+v", a.agentQueue)
	}
	if a.agentRefs != nil {
		t.Fatal("the references stayed in the box after queueing")
	}

	// A click takes the first message back, with its references.
	a.editQueuedAgent(0)
	if len(a.agentQueue) != 1 || a.agentQueue[0].Text != "第三条" {
		t.Fatalf("the queue after editing: %+v", a.agentQueue)
	}
	if a.agentInput != "第二条" {
		t.Fatalf("the box: %q", a.agentInput)
	}

	// The queue of a conversation goes with its tab.
	a.agentConn, a.agentSession, a.agentBusy = nil, "", false
	a.ensureChats()
	a.chatSave()
	a.chatNew()
	if len(a.agentQueue) != 0 {
		t.Fatalf("the new conversation inherited the queue: %+v", a.agentQueue)
	}
	a.chatSelect(0)
	if len(a.agentQueue) != 1 || a.agentInput != "第二条" {
		t.Fatalf("the old conversation lost its queue: %+v %q", a.agentQueue, a.agentInput)
	}
}

// TestTheQueueGoesWhenTheTurnEnds sends a message that waited, once the
// turn of the agent is free again.
func TestTheQueueGoesWhenTheTurnEnds(t *testing.T) {
	a, queue := startAgentHelper(t)
	defer a.stopAgent()
	a.agentQueue = []agentQueued{{Text: "排队的问题"}}
	a.flushAgentQueue()
	pumpUntil(t, queue, func() bool {
		return !a.agentBusy && agentSays(a.agentLines, "you", "排队的问题") &&
			agentSays(a.agentLines, "agent", "你好")
	})
	if len(a.agentQueue) != 0 {
		t.Fatalf("the queue: %+v", a.agentQueue)
	}
}

// TestAPictureTravelsWithThePrompt attaches an image the way a paste
// does, and reads the content block the agent gets.
func TestAPictureTravelsWithThePrompt(t *testing.T) {
	a := newApp()
	defer a.shutdown()
	a.root = t.TempDir()

	var data bytes.Buffer
	if err := png.Encode(&data, image.NewRGBA(image.Rect(0, 0, 4, 3))); err != nil {
		t.Fatal(err)
	}
	a.addAgentImage(data.Bytes(), "shot.png")
	if len(a.agentRefs) != 1 {
		t.Fatalf("the references: %+v", a.agentRefs)
	}
	ref := a.agentRefs[0]
	if ref.Kind != "image" || ref.Mime != "image/png" || ref.Src == nil {
		t.Fatalf("the picture: %+v", ref)
	}
	if !strings.Contains(ref.Rel, "shot.png") || !strings.Contains(ref.Rel, "4×3") {
		t.Fatalf("the label: %q", ref.Rel)
	}

	blocks := a.agentPromptBlocks("看这张图")
	if len(blocks) != 2 {
		t.Fatalf("the blocks: %+v", blocks)
	}
	picture, ok := blocks[1].(map[string]any)
	if !ok || picture["type"] != "image" || picture["mimeType"] != "image/png" {
		t.Fatalf("the image block: %+v", blocks[1])
	}
	sent, err := base64.StdEncoding.DecodeString(picture["data"].(string))
	if err != nil || !bytes.Equal(sent, data.Bytes()) {
		t.Fatalf("the picture bytes: %v %d", err, len(sent))
	}

	// A picture on its own needs no words, and gets no empty text block.
	alone := a.agentPromptBlocksWith("", a.agentRefs)
	if len(alone) != 1 || alone[0].(map[string]any)["type"] != "image" {
		t.Fatalf("a picture alone: %+v", alone)
	}

	// The same picture is not attached twice.
	a.addAgentImage(data.Bytes(), "shot.png")
	if len(a.agentRefs) != 1 {
		t.Fatalf("the picture was duplicated: %+v", a.agentRefs)
	}
}
