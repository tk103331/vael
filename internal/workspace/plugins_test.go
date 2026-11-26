package workspace

import (
	"path/filepath"
	"testing"
)

func TestAPluginManifestIsChecked(t *testing.T) {
	var manifest PluginManifest
	manifest.Name = "wordcount"
	manifest.API = PluginAPI
	manifest.Runtime = "js"
	manifest.Main = "plugin.js"
	if err := manifest.Check(); err != nil {
		t.Fatal(err)
	}
	manifest.Capabilities = []string{"editor"}
	if !manifest.Has("editor") || manifest.Has("notify") {
		t.Fatalf("the capabilities: %+v", manifest.Capabilities)
	}
	// Another interface version is not this editor's.
	manifest.API = PluginAPI + 1
	if err := manifest.Check(); err == nil {
		t.Fatal("a plugin of another version was accepted")
	}
	manifest.API = PluginAPI
	// Another runtime is not this editor's either.
	manifest.Runtime = "python"
	if err := manifest.Check(); err == nil {
		t.Fatal("a plugin of another runtime was accepted")
	}
	// The two runtimes this editor runs are both fine.
	for _, runtime := range []string{"js", "wasm"} {
		manifest.Runtime = runtime
		if err := manifest.Check(); err != nil {
			t.Fatalf("the runtime %q was refused: %v", runtime, err)
		}
	}
	manifest.Runtime = "js"
	// A command without a name cannot be shown.
	manifest.Contributes.Commands = []PluginCommand{{Title: "统计字数"}}
	if err := manifest.Check(); err == nil {
		t.Fatal("a command without an id was accepted")
	}
}

func TestAThemePluginNeedsNoProgram(t *testing.T) {
	var manifest PluginManifest
	manifest.Name = "midnight"
	manifest.API = PluginAPI
	manifest.Contributes.Themes = []PluginTheme{
		{ID: "midnight-dark", Name: "午夜 深色", Dark: true, Colors: map[string]string{"accent": "#5b8cff"}},
	}
	if err := manifest.Check(); err != nil {
		t.Fatalf("a theme of its own needs no main: %v", err)
	}
	manifest.Contributes.Themes[0].ID = ""
	if err := manifest.Check(); err == nil {
		t.Fatal("a theme without an id was accepted")
	}
}

func TestThePluginsKeepTheirStateAndStorage(t *testing.T) {
	dir := t.TempDir()
	states := PluginStates{"wordcount": true}
	if err := SavePluginStates(filepath.Join(dir, "plugins.json"), states); err != nil {
		t.Fatal(err)
	}
	saved, err := LoadPluginStates(filepath.Join(dir, "plugins.json"))
	if err != nil || !saved["wordcount"] {
		t.Fatalf("the states: %+v %v", saved, err)
	}
	storage := PluginStorage{"wordcount": {"last": "3 词"}}
	if err := SavePluginStorage(filepath.Join(dir, "plugin-storage.json"), storage); err != nil {
		t.Fatal(err)
	}
	kept, err := LoadPluginStorage(filepath.Join(dir, "plugin-storage.json"))
	if err != nil || kept["wordcount"]["last"] != "3 词" {
		t.Fatalf("the storage: %+v %v", kept, err)
	}
}
