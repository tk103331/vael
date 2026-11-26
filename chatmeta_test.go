package main

import (
	"path/filepath"
	"testing"

	"vael/internal/workspace"
)

// TestConversationsAreRenamedAndPinned covers what the editor keeps of a
// session beyond the agent's own title: the name the user gave it, and
// the pin that keeps it at the top of the history and of the tabs.
func TestConversationsAreRenamedAndPinned(t *testing.T) {
	a, queue := startAgentHelper(t)
	defer a.stopAgent()
	a.chatsPath = filepath.Join(t.TempDir(), "chats.json")
	a.loadAgentSessions()
	pumpUntil(t, queue, func() bool { return len(a.agentSessions) > 0 })
	var target acpSessionInfo
	for _, session := range a.agentSessions {
		if session.Title == "实现会话列表" {
			target = session
		}
	}
	if target.SessionID == "" {
		t.Fatalf("the sessions: %+v", a.agentSessions)
	}

	a.chatRename = &chatRenamePrompt{session: target.SessionID, value: "我的会话"}
	a.renameChat()
	saved, err := workspace.LoadChats(a.chatsPath)
	if err != nil || saved[target.SessionID].Title != "我的会话" {
		t.Fatalf("the saved name: %+v %v", saved, err)
	}

	a.toggleChatPin(target.SessionID)
	saved, err = workspace.LoadChats(a.chatsPath)
	if err != nil || !saved[target.SessionID].Pinned {
		t.Fatalf("the saved pin: %+v %v", saved, err)
	}

	// The session opens with the user's name, above the others because
	// it is pinned.
	a.openHistorySession(target)
	pumpUntil(t, queue, func() bool { return a.chatActive().state.session == target.SessionID })
	tab := a.chatActive()
	if tab.title != "我的会话" || !tab.renamed {
		t.Fatalf("the opened tab: %+v", tab)
	}
	if len(a.chatTabs) < 2 || a.chatTabs[0] != tab {
		t.Fatalf("the pinned tab is not first: %+v", a.chatTabs)
	}

	// Taking the pin back leaves the name, and the order is the old one.
	a.toggleChatPin(target.SessionID)
	saved, err = workspace.LoadChats(a.chatsPath)
	if err != nil || saved[target.SessionID].Pinned || saved[target.SessionID].Title != "我的会话" {
		t.Fatalf("the pin came back: %+v %v", saved, err)
	}
}
