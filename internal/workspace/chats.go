package workspace

import (
	"encoding/json"
	"os"
)

// Chat is what the editor remembers of a conversation of an agent: the
// name the user gave it, and whether it is pinned above the others.
// https://github.com/dstrml/wu
type Chat struct {
	Title  string `json:"title,omitempty"`
	Pinned bool   `json:"pinned,omitempty"`
}

// Chats are the conversations of the agent, by the session ID the agent
// gave them.
type Chats map[string]Chat

func LoadChats(file string) (Chats, error) {
	chats := Chats{}
	data, err := os.ReadFile(file)
	if err != nil {
		return chats, err
	}
	err = json.Unmarshal(data, &chats)
	if chats == nil {
		chats = Chats{}
	}
	return chats, err
}

func SaveChats(file string, chats Chats) error { return saveJSON(file, chats) }
