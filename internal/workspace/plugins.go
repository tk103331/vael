package workspace

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"
)

// PluginAPI is the version of the plugin interface this editor speaks.
// A plugin that asks for another one is not loaded.
// https://github.com/dstrml/wu
const PluginAPI = 1

// PluginManifest is what a plugin says of itself in its plugin.json: what
// it is, which runtime it needs, what it may do, and the commands it
// brings to the command palette.
type PluginManifest struct {
	Name         string   `json:"name"`
	Version      string   `json:"version"`
	Description  string   `json:"description,omitempty"`
	API          int      `json:"api"`
	Runtime      string   `json:"runtime"`
	Main         string   `json:"main"`
	Capabilities []string `json:"capabilities,omitempty"`
	Contributes  struct {
		Commands   []PluginCommand   `json:"commands,omitempty"`
		Themes     []PluginTheme     `json:"themes,omitempty"`
		IconThemes []PluginIconTheme `json:"iconThemes,omitempty"`
	} `json:"contributes"`
}

// PluginTheme is a theme of the window a plugin brings: a family of dark
// and light palettes, and the colours of each part.
type PluginTheme struct {
	ID     string            `json:"id"`
	Name   string            `json:"name"`
	Family string            `json:"family,omitempty"`
	Dark   bool              `json:"dark"`
	Colors map[string]string `json:"colors"`
}

// PluginIconTheme is a way of marking files a plugin brings: by name or
// by one of the editor's icons, in a colour of the current theme.
type PluginIconTheme struct {
	ID    string                    `json:"id"`
	Name  string                    `json:"name"`
	Marks map[string]PluginIconMark `json:"marks,omitempty"`
}

// PluginIconMark is how one kind of file is marked.
type PluginIconMark struct {
	Text string `json:"text,omitempty"`
	Icon string `json:"icon,omitempty"`
	Tone string `json:"tone,omitempty"`
}

// PluginCommand is one command a plugin adds to the palette.
type PluginCommand struct {
	ID    string `json:"id"`
	Title string `json:"title"`
	Hint  string `json:"hint,omitempty"`
}

// Check says whether the editor can run this plugin: the interface
// version, the runtime, and the commands must all be ones it knows.
func (m PluginManifest) Check() error {
	if strings.TrimSpace(m.Name) == "" {
		return fmt.Errorf("插件清单缺少 name")
	}
	if m.API != PluginAPI {
		return fmt.Errorf("插件要求接口版本 %d，Vael 只支持 %d", m.API, PluginAPI)
	}
	if m.Runtime != "" && m.Runtime != "js" && m.Runtime != "wasm" {
		return fmt.Errorf("插件的运行时 %q 还不支持", m.Runtime)
	}
	// A plugin may be only data: a theme or an icon theme needs no program.
	hasContribution := len(m.Contributes.Themes) > 0 || len(m.Contributes.IconThemes) > 0
	if strings.TrimSpace(m.Main) == "" && !hasContribution {
		return fmt.Errorf("插件清单缺少 main")
	}
	for _, theme := range m.Contributes.Themes {
		if strings.TrimSpace(theme.ID) == "" || strings.TrimSpace(theme.Name) == "" {
			return fmt.Errorf("主题缺少 id 或 name")
		}
		if len(theme.Colors) == 0 {
			return fmt.Errorf("主题 %s 没有颜色", theme.ID)
		}
	}
	for _, icons := range m.Contributes.IconThemes {
		if strings.TrimSpace(icons.ID) == "" || strings.TrimSpace(icons.Name) == "" {
			return fmt.Errorf("图标主题缺少 id 或 name")
		}
	}
	for _, command := range m.Contributes.Commands {
		if strings.TrimSpace(command.ID) == "" || strings.TrimSpace(command.Title) == "" {
			return fmt.Errorf("插件的命令缺少 id 或 title")
		}
	}
	return nil
}

// IconTheme is the named part of an icon theme, for the settings.
func (m PluginManifest) IconThemes() []PluginIconTheme { return m.Contributes.IconThemes }

// Themes is the named part of a theme plugin, for the settings.
func (m PluginManifest) Themes() []PluginTheme { return m.Contributes.Themes }

// Has tells whether the plugin asked for a capability.
func (m PluginManifest) Has(capability string) bool {
	for _, name := range m.Capabilities {
		if strings.EqualFold(strings.TrimSpace(name), capability) {
			return true
		}
	}
	return false
}

// PluginStates are the plugins the user turned off, by name.
type PluginStates map[string]bool

func LoadPluginStates(file string) (PluginStates, error) {
	states := PluginStates{}
	data, err := os.ReadFile(file)
	if err != nil {
		return states, err
	}
	err = json.Unmarshal(data, &states)
	return states, err
}

func SavePluginStates(file string, states PluginStates) error { return saveJSON(file, states) }

// PluginStorage is what the plugins keep of their own, by plugin and key.
type PluginStorage map[string]map[string]string

func LoadPluginStorage(file string) (PluginStorage, error) {
	storage := PluginStorage{}
	data, err := os.ReadFile(file)
	if err != nil {
		return storage, err
	}
	err = json.Unmarshal(data, &storage)
	return storage, err
}

func SavePluginStorage(file string, storage PluginStorage) error { return saveJSON(file, storage) }
