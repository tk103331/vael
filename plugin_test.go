package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"
	"github.com/egoist/mygo/ui/editor"
)

// writePlugin puts a plugin into a directory of its own, and returns the
// directory of all of them.
func writePlugin(t *testing.T, dir, name, manifest, program string) {
	t.Helper()
	pluginDir := filepath.Join(dir, name)
	if err := os.MkdirAll(pluginDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(pluginDir, "plugin.json"), []byte(manifest), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(pluginDir, "plugin.js"), []byte(program), 0o644); err != nil {
		t.Fatal(err)
	}
}

// TestAPluginBringsItsCommands covers a plugin of the user's own: the
// editor reads its manifest, opens its program, and its command shows in
// the palette and runs.
func TestAPluginBringsItsCommands(t *testing.T) {
	a, queue := testDispatchApp(t)
	dir := t.TempDir()
	writePlugin(t, dir, "wordcount",
		`{"name":"wordcount","version":"0.1.0","description":"数一数","api":1,"runtime":"js",
		  "main":"plugin.js","capabilities":["editor","notify"],
		  "contributes":{"commands":[{"id":"wordcount","title":"统计字数","hint":"数一数"}]}}`,
		`export function run(request) {
		  const { text } = JSON.parse(request);
		  const words = text.trim().split(/\s+/).filter(Boolean).length;
		  return JSON.stringify({ notify: words + " 词" });
		}`)
	a.pluginsDir = dir
	a.loadPlugins()
	if len(a.plugins) != 1 {
		t.Fatalf("the plugins: %+v", a.plugins)
	}
	if a.plugins[0].err != "" {
		t.Fatalf("the plugin did not load: %s", a.plugins[0].err)
	}
	commands := a.pluginCommands()
	if len(commands) != 1 || commands[0].title != "插件：统计字数" {
		t.Fatalf("the commands of the plugin: %+v", commands)
	}

	// The command runs on the file that is open.
	buffer := editor.NewBuffer("你好 世界")
	a.tabs = []*fileTab{{path: "/p/main.go", state: &editor.State{Buffer: buffer}}}
	a.selected = 0
	commands[0].run()
	pumpUntil(t, queue, func() bool { return a.message != "" })
	if !strings.Contains(a.message, "2 词") {
		t.Fatalf("the answer of the plugin: %q", a.message)
	}
}

// TestAPluginNeedsTheCapabilityItDeclared covers the capabilities: a
// plugin that says nothing about notifications may not send one.
func TestAPluginNeedsTheCapabilityItDeclared(t *testing.T) {
	a, queue := testDispatchApp(t)
	dir := t.TempDir()
	writePlugin(t, dir, "talker",
		`{"name":"talker","version":"0.1.0","api":1,"runtime":"js","main":"plugin.js",
		  "capabilities":[],"contributes":{"commands":[{"id":"talk","title":"说话"}]}}`,
		`export function run(request) {
		  vael.notify("我不该说话");
		  return JSON.stringify({});
		}`)
	a.pluginsDir = dir
	a.loadPlugins()
	if len(a.plugins) != 1 || a.plugins[0].err != "" {
		t.Fatalf("the plugins: %+v", a.plugins)
	}
	commands := a.pluginCommands()
	if len(commands) != 1 {
		t.Fatalf("the commands of the plugin: %+v", commands)
	}
	commands[0].run()
	pumpUntil(t, queue, func() bool { return a.message != "" })
	if !strings.Contains(a.message, "notify") {
		t.Fatalf("the refusal of the editor: %q", a.message)
	}
}

// TestAPluginThatLoopsIsStopped covers the timeout: a plugin that never
// ends does not hold the editor.
func TestAPluginThatLoopsIsStopped(t *testing.T) {
	a, queue := testDispatchApp(t)
	old := pluginTimeout
	pluginTimeout = 200 * time.Millisecond
	defer func() { pluginTimeout = old }()
	dir := t.TempDir()
	writePlugin(t, dir, "looper",
		`{"name":"looper","version":"0.1.0","api":1,"runtime":"js","main":"plugin.js",
		  "capabilities":[],"contributes":{"commands":[{"id":"loop","title":"转圈"}]}}`,
		`export function run(request) { for (;;) {} }`)
	a.pluginsDir = dir
	a.loadPlugins()
	commands := a.pluginCommands()
	if len(commands) != 1 {
		t.Fatalf("the commands of the plugin: %+v", commands)
	}
	commands[0].run()
	pumpUntil(t, queue, func() bool { return a.message != "" })
	if !strings.Contains(a.message, "looper") {
		t.Fatalf("the stop of the plugin: %q", a.message)
	}
}

// TestAPluginGivesAPanel covers a plugin that asks for a panel of its
// own: the editor draws the tree it gave, with its own parts.
func TestAPluginGivesAPanel(t *testing.T) {
	a, queue := testDispatchApp(t)
	dir := t.TempDir()
	writePlugin(t, dir, "panel",
		`{"name":"panel","version":"0.1.0","api":1,"runtime":"js","main":"plugin.js","capabilities":[],
		  "contributes":{"commands":[{"id":"show","title":"打开面板"}]}}`,
		`export function run(request) {
		  return JSON.stringify({ view: [
		    { type: "column", children: [
		      { type: "text", text: "提交摘要", role: "title" },
		      { type: "text", text: "工作区有 1 个文件改动" }
		    ] }
		  ] });
		}`)
	a.pluginsDir = dir
	a.loadPlugins()
	commands := a.pluginCommands()
	if len(commands) != 1 {
		t.Fatalf("the commands of the plugin: %+v", commands)
	}
	commands[0].run()
	pumpUntil(t, queue, func() bool { return a.rightSection == "plugin" })
	if len(a.pluginViews["panel"]) == 0 {
		t.Fatalf("the panel of the plugin: %+v", a.pluginViews)
	}
	tt := ui.NewTester(a.view, 1280, 840)
	tt.Frame()
	if !tt.HasText("提交摘要") || !tt.HasText("工作区有 1 个文件改动") {
		t.Fatalf("the panel is not drawn: %v", tt.Texts())
	}
}

// TestAPluginCannotRunAFileOutsideItsFolder covers the manifest: main may
// not point out of the plugin's own directory.
func TestAPluginCannotRunAFileOutsideItsFolder(t *testing.T) {
	a := newApp()
	defer a.shutdown()
	dir := t.TempDir()
	writePlugin(t, dir, "escape",
		`{"name":"escape","api":1,"runtime":"js","main":"../../outside.js",
		  "contributes":{"commands":[{"id":"go","title":"跑"}]}}`,
		`export function run(r) { return "{}" }`)
	a.pluginsDir = dir
	a.loadPlugins()
	if len(a.plugins) != 1 || a.plugins[0].err == "" {
		t.Fatalf("the plugin was loaded: %+v", a.plugins)
	}
	if len(a.pluginCommands()) != 0 {
		t.Fatal("the plugin of another folder brought commands")
	}
}

// TestTheSettingsShowThePlugins covers the list of the plugins in the
// settings: what they are, and what went wrong with them.
func TestTheSettingsShowThePlugins(t *testing.T) {
	a := newApp()
	defer a.shutdown()
	dir := t.TempDir()
	writePlugin(t, dir, "wordcount",
		`{"name":"wordcount","version":"0.1.0","description":"数一数","api":1,"runtime":"js",
		  "main":"plugin.js","capabilities":["editor","notify"],
		  "contributes":{"commands":[{"id":"wordcount","title":"统计字数"}]}}`,
		`export function run(request) { return JSON.stringify({}); }`)
	writePlugin(t, dir, "broken", `{"name":"broken"}`, `export function run(r) { return "{}" }`)
	a.pluginsDir = dir
	a.loadPlugins()
	a.settingsOpen = true
	a.settingsSection = settingsPlugin
	tt := ui.NewTester(a.view, 1280, 840)
	tt.Frame()
	if !tt.HasText("wordcount") || !tt.HasText("统计字数") {
		t.Fatalf("the plugin is not in the settings: %v", tt.Texts())
	}
	if !tt.HasText("插件要求接口版本") {
		t.Fatalf("the broken plugin says nothing: %v", tt.Texts())
	}
}

// TestAPluginThatDoesNotFitIsNotLoaded covers the manifest: a plugin of
// another interface version, or of another runtime, is left alone.
func TestAPluginThatDoesNotFitIsNotLoaded(t *testing.T) {
	a := newApp()
	defer a.shutdown()
	dir := t.TempDir()
	writePlugin(t, dir, "future",
		`{"name":"future","version":"1.0.0","api":99,"runtime":"js","main":"plugin.js"}`,
		`export function run(request) { return "{}" }`)
	writePlugin(t, dir, "foreign",
		`{"name":"foreign","version":"1.0.0","api":1,"runtime":"wasm","main":"plugin.wasm"}`,
		`export function run(request) { return "{}" }`)
	a.pluginsDir = dir
	a.loadPlugins()
	if len(a.plugins) != 2 {
		t.Fatalf("the plugins: %+v", a.plugins)
	}
	for _, item := range a.plugins {
		if item.err == "" {
			t.Fatalf("the plugin %s was loaded", item.manifest.Name)
		}
	}
	if len(a.pluginCommands()) != 0 {
		t.Fatal("a plugin that did not load brought commands")
	}
}
