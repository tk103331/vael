package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/egoist/mygo/ui/editor"
)

// wasmPluginOf writes a plugin built to wasm, using the one the
// experiments built, and returns the directory of the plugins.
func wasmPluginOf(t *testing.T, name string) string {
	t.Helper()
	code, err := os.ReadFile(filepath.Join("experiments", "plugins", "plugin", "wordcount.wasm"))
	if err != nil {
		t.Skip("编译好的 wasm 插件不在：", err)
	}
	dir := t.TempDir()
	pluginDir := filepath.Join(dir, name)
	if err := os.MkdirAll(pluginDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(pluginDir, "plugin.wasm"), code, 0o644); err != nil {
		t.Fatal(err)
	}
	manifest := `{"name":"` + name + `","version":"0.1.0","api":1,"runtime":"wasm",
		"main":"plugin.wasm","capabilities":["notify"],
		"contributes":{"commands":[{"id":"wordcount","title":"统计字数"}]}}`
	if err := os.WriteFile(filepath.Join(pluginDir, "plugin.json"), []byte(manifest), 0o644); err != nil {
		t.Fatal(err)
	}
	return dir
}

// TestAWasmPluginRuns covers a plugin built to wasm: the same ABI as the
// JavaScript one, in a sandbox of its own.
func TestAWasmPluginRuns(t *testing.T) {
	a, queue := testDispatchApp(t)
	a.pluginsDir = wasmPluginOf(t, "wordcount")
	a.loadPlugins()
	if len(a.plugins) != 1 {
		t.Fatalf("the plugins: %+v", a.plugins)
	}
	if a.plugins[0].err != "" {
		t.Fatalf("the plugin did not load: %s", a.plugins[0].err)
	}
	commands := a.pluginCommands()
	if len(commands) != 1 {
		t.Fatalf("the commands of the plugin: %+v", commands)
	}
	buffer := editor.NewBuffer("你好 世界")
	a.tabs = []*fileTab{{path: "/p/main.go", state: &editor.State{Buffer: buffer}}}
	a.selected = 0
	commands[0].run()
	pumpUntil(t, queue, func() bool { return a.message != "" })
	if !strings.Contains(a.message, "词") {
		t.Fatalf("the answer of the plugin: %q", a.message)
	}
}

// TestAWasmPluginThatLoopsIsStopped covers the timeout of a plugin built
// to wasm: the sandbox is closed and the editor goes on.
func TestAWasmPluginThatLoopsIsStopped(t *testing.T) {
	a, queue := testDispatchApp(t)
	old := pluginTimeout
	pluginTimeout = 300 * time.Millisecond
	defer func() { pluginTimeout = old }()
	a.pluginsDir = wasmPluginOf(t, "looper")
	a.loadPlugins()
	commands := a.pluginCommands()
	if len(commands) != 1 {
		t.Fatalf("the commands of the plugin: %+v", commands)
	}
	// The plugin loops when the command says so.
	a.plugins[0].manifest.Contributes.Commands[0].ID = "loop"
	commands = a.pluginCommands()
	commands[0].run()
	pumpUntil(t, queue, func() bool { return a.message != "" })
	if !strings.Contains(a.message, "looper") {
		t.Fatalf("the stop of the plugin: %q", a.message)
	}
}
