package workspace

import (
	"path/filepath"
	"testing"
)

func TestAgentsKeepTheirNamesAndDefault(t *testing.T) {
	var agents Agents
	if err := agents.Add(Agent{Name: " A ", Command: " a --acp "}); err != nil {
		t.Fatal(err)
	}
	if agents.Items[0].Name != "A" || agents.Items[0].Command != "a --acp" {
		t.Fatalf("trimmed: %+v", agents.Items[0])
	}
	if err := agents.Add(Agent{Name: "", Command: "x"}); err == nil {
		t.Fatal("an agent without a name was added")
	}
	if err := agents.Add(Agent{Name: "B", Command: ""}); err == nil {
		t.Fatal("an agent without a command was added")
	}
	// The same name replaces the agent, and not a new one.
	if err := agents.Add(Agent{Name: "A", Command: "a2"}); err != nil {
		t.Fatal(err)
	}
	if len(agents.Items) != 1 || agents.Items[0].Command != "a2" {
		t.Fatalf("replaced: %+v", agents.Items)
	}
	if err := agents.Add(Agent{Name: "B", Command: "b --acp"}); err != nil {
		t.Fatal(err)
	}
	// The default is the one named, or the first.
	if chosen := agents.Chosen("B"); chosen == nil || chosen.Name != "B" {
		t.Fatalf("chosen: %+v", chosen)
	}
	agents.Default = "B"
	if chosen := agents.Chosen(""); chosen == nil || chosen.Name != "B" {
		t.Fatalf("default: %+v", chosen)
	}
	agents.Remove("B")
	if len(agents.Items) != 1 || agents.Default != "" {
		t.Fatalf("removed: %+v", agents)
	}
	if chosen := agents.Chosen(""); chosen == nil || chosen.Name != "A" {
		t.Fatalf("fallback: %+v", chosen)
	}
}

func TestAgentsAreSaved(t *testing.T) {
	path := filepath.Join(t.TempDir(), "agents.json")
	var agents Agents
	if err := agents.Add(Agent{Name: "Gemini", Command: "gemini --experimental-acp"}); err != nil {
		t.Fatal(err)
	}
	agents.Default = "Gemini"
	if err := SaveAgents(path, agents); err != nil {
		t.Fatal(err)
	}
	saved, err := LoadAgents(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(saved.Items) != 1 || saved.Items[0].Name != "Gemini" || saved.Default != "Gemini" {
		t.Fatalf("saved: %+v", saved)
	}
}
