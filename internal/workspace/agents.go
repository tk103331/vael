package workspace

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"
)

// Agent is an ACP agent the editor starts: a name, and the command that
// runs it, which speaks the Agent Client Protocol over its standard input
// and output, with the environment the registry asks it to run with.
type Agent struct {
	Name    string            `json:"name"`
	Command string            `json:"command"`
	Env     map[string]string `json:"env,omitempty"`
}

// Agents are the ACP agents the editor may start, and the one it starts
// by default.
type Agents struct {
	Items   []Agent `json:"items"`
	Default string  `json:"default,omitempty"`
}

// Add puts an agent among the others, replacing one of the same name.
func (a *Agents) Add(e Agent) error {
	e.Name = strings.TrimSpace(e.Name)
	e.Command = strings.TrimSpace(e.Command)
	if e.Name == "" {
		return fmt.Errorf("请输入 Agent 名称")
	}
	if e.Command == "" {
		return fmt.Errorf("请输入启动命令")
	}
	for i, old := range a.Items {
		if old.Name == e.Name {
			a.Items[i] = e
			return nil
		}
	}
	a.Items = append(a.Items, e)
	return nil
}

// Remove takes an agent away, and the default with it when it was that.
func (a *Agents) Remove(name string) {
	for i, old := range a.Items {
		if old.Name == name {
			a.Items = append(a.Items[:i], a.Items[i+1:]...)
			break
		}
	}
	if a.Default == name {
		a.Default = ""
	}
}

// Has reports whether an agent of a name is among the others.
func (a *Agents) Has(name string) bool {
	for _, old := range a.Items {
		if old.Name == name {
			return true
		}
	}
	return false
}

// Chosen is the agent to start when one is asked for: the default, one
// the editor knows, or none.
func (a *Agents) Chosen(name string) *Agent {
	for i := range a.Items {
		if a.Items[i].Name == name {
			return &a.Items[i]
		}
	}
	if a.Default != "" {
		for i := range a.Items {
			if a.Items[i].Name == a.Default {
				return &a.Items[i]
			}
		}
	}
	if len(a.Items) > 0 {
		return &a.Items[0]
	}
	return nil
}

func LoadAgents(file string) (Agents, error) {
	var s Agents
	data, err := os.ReadFile(file)
	if err != nil {
		return s, err
	}
	err = json.Unmarshal(data, &s)
	return s, err
}

func SaveAgents(file string, s Agents) error { return saveJSON(file, s) }
