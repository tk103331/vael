package workspace

import (
	"path/filepath"
	"testing"
)

func TestChatsSaveTheirNameAndPin(t *testing.T) {
	path := filepath.Join(t.TempDir(), "chats.json")
	chats := Chats{"s1": {Title: "登录超时", Pinned: true}, "s2": {}}
	if err := SaveChats(path, chats); err != nil {
		t.Fatal(err)
	}
	saved, err := LoadChats(path)
	if err != nil {
		t.Fatal(err)
	}
	if saved["s1"].Title != "登录超时" || !saved["s1"].Pinned {
		t.Fatalf("the saved chats: %+v", saved)
	}
	if saved["s2"].Pinned || saved["s2"].Title != "" {
		t.Fatalf("an empty chat: %+v", saved["s2"])
	}
	// A file that is not there leaves an empty map to fill.
	missing, err := LoadChats(filepath.Join(t.TempDir(), "none.json"))
	if err == nil || missing == nil {
		t.Fatalf("missing: %+v %v", missing, err)
	}
}
