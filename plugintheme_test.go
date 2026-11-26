package main

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/egoist/mygo/ui"
)

// TestAPluginBringsATheme covers a plugin that is only data: its themes
// become part of the editor, as its commands do, and can be chosen and
// toggled like the built-in ones.
func TestAPluginBringsATheme(t *testing.T) {
	a := newApp()
	defer a.shutdown()
	dir := t.TempDir()
	pluginDir := filepath.Join(dir, "midnight")
	if err := os.MkdirAll(pluginDir, 0o755); err != nil {
		t.Fatal(err)
	}
	manifest := `{"name":"midnight","version":"0.1.0","description":"午夜配色","api":1,"runtime":"js",
		"contributes":{
		  "themes":[
		    {"id":"midnight-dark","name":"午夜 深色","family":"midnight","dark":true,
		     "colors":{"wash":"#0b1020","card":"#111a2e","text":"#e6ecff","accent":"#5b8cff",
		               "warn":"#ffb454","success":"#5ad18a","danger":"#ff6b6b"}},
		    {"id":"midnight-light","name":"午夜 浅色","family":"midnight","dark":false,
		     "colors":{"wash":"#e8edff","card":"#ffffff","text":"#162039","accent":"#2b5bd2"}}
		  ],
		  "iconThemes":[{"id":"midnight-icons","name":"午夜图标","marks":{
		    ".go":{"icon":"code","tone":"accent"},
		    ".md":{"text":"MD","tone":"warn"}
		  }}]
		}}`
	if err := os.WriteFile(filepath.Join(pluginDir, "plugin.json"), []byte(manifest), 0o644); err != nil {
		t.Fatal(err)
	}
	a.pluginsDir = dir
	a.loadPlugins()
	if len(a.plugins) != 1 || a.plugins[0].err != "" {
		t.Fatalf("the plugins: %+v", a.plugins)
	}
	if len(a.pluginThemes) != 2 || len(a.pluginIconThemes) != 1 {
		t.Fatalf("the contributions: %+v %+v", a.pluginThemes, a.pluginIconThemes)
	}
	// The theme is used the same way as a built-in one.
	a.setTheme("midnight-dark")
	if a.theme().Palette.accent != ui.Hex("#5b8cff") || !a.dark {
		t.Fatalf("the theme: %+v dark=%v", a.theme(), a.dark)
	}
	a.toggleDark()
	if a.themeID != "midnight-light" || a.dark {
		t.Fatalf("the other side: %q dark=%v", a.themeID, a.dark)
	}
	// The icon theme marks files by their kind.
	a.iconID = "midnight-icons"
	goMark := a.markFor("main.go")
	if goMark.icon != codeIcon || a.iconThemeFor() != "midnight-icons" {
		t.Fatalf("the mark of a Go file: %+v", goMark)
	}
	markdown := a.markFor("README.md")
	if markdown.text != "MD" || markdown.icon != nil {
		t.Fatalf("the mark of a Markdown file: %+v", markdown)
	}
	if _, ok := a.pluginMark("unknown.xyz"); ok {
		t.Fatal("the icon theme marked a kind it does not know")
	}
	// Both are offered in the settings.
	a.settingsOpen = true
	a.settingsSection = settingsLook
	tt := ui.NewTester(a.view, 1280, 840)
	tt.Frame()
	if !tt.HasText("午夜 深色") || !tt.HasText("午夜图标") {
		t.Fatalf("the settings do not offer the plugin: %v", tt.Texts())
	}
}

// TestAThemePluginThatIsTurnedOffIsNotChosen covers what happens when a
// plugin that gave the chosen theme goes away: the editor falls back to
// one of its own themes.
func TestAThemePluginThatIsTurnedOffIsNotChosen(t *testing.T) {
	a := newApp()
	defer a.shutdown()
	dir := t.TempDir()
	writePlugin(t, dir, "midnight",
		`{"name":"midnight","api":1,"runtime":"js","contributes":{"themes":[
		  {"id":"midnight-dark","name":"午夜","dark":true,"colors":{"accent":"#5b8cff"}}]}}`,
		``)
	a.pluginsDir = dir
	a.loadPlugins()
	a.setTheme("midnight-dark")
	if a.themeID != "midnight-dark" {
		t.Fatalf("the theme was not chosen: %q", a.themeID)
	}
	a.pluginStates["midnight"] = true
	a.loadPlugins()
	if a.pluginTheme(a.themeID) != nil || a.theme().ID != "vael-dark" {
		t.Fatalf("a theme of a plugin that is off is still chosen: %q", a.themeID)
	}
}

// TestAThemePluginOfAnotherVersionIsRefused covers a theme plugin whose
// colours are not what the editor reads.
func TestAThemePluginOfAnotherVersionIsRefused(t *testing.T) {
	a := newApp()
	defer a.shutdown()
	dir := t.TempDir()
	pluginDir := filepath.Join(dir, "broken")
	if err := os.MkdirAll(pluginDir, 0o755); err != nil {
		t.Fatal(err)
	}
	manifest := `{"name":"broken","api":1,"runtime":"js",
		"contributes":{"themes":[{"id":"broken","name":"坏的","dark":true,"colors":{"accent":"blue"}}]}}`
	if err := os.WriteFile(filepath.Join(pluginDir, "plugin.json"), []byte(manifest), 0o644); err != nil {
		t.Fatal(err)
	}
	a.pluginsDir = dir
	a.loadPlugins()
	if len(a.pluginThemes) != 0 {
		t.Fatalf("the theme was registered: %+v", a.pluginThemes)
	}
	if a.plugins[0].err == "" {
		t.Fatal("the plugin said nothing about the bad colour")
	}
}
