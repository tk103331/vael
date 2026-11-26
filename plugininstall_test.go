package main

import (
	"archive/tar"
	"archive/zip"
	"bytes"
	"compress/gzip"
	"os"
	"path/filepath"
	"testing"
)

// zipPackage builds a package of a plugin, as a zip.
func zipPackage(t *testing.T, files map[string]string) []byte {
	t.Helper()
	var buf bytes.Buffer
	writer := zip.NewWriter(&buf)
	for name, body := range files {
		file, err := writer.Create(name)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := file.Write([]byte(body)); err != nil {
			t.Fatal(err)
		}
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

// tarPackage builds a package of a plugin, as a tar.gz.
func tarPackage(t *testing.T, files map[string]string) []byte {
	t.Helper()
	var buf bytes.Buffer
	gz := gzip.NewWriter(&buf)
	writer := tar.NewWriter(gz)
	for name, body := range files {
		header := &tar.Header{Name: name, Mode: 0o644, Size: int64(len(body))}
		if err := writer.WriteHeader(header); err != nil {
			t.Fatal(err)
		}
		if _, err := writer.Write([]byte(body)); err != nil {
			t.Fatal(err)
		}
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	if err := gz.Close(); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

const wordcountManifest = `{"name":"wordcount","version":"0.1.0","api":1,"runtime":"js",
	"main":"plugin.js","capabilities":["notify"],
	"contributes":{"commands":[{"id":"wc","title":"统计字数"}]}}`

const wordcountProgram = `export function run(request) {
	return JSON.stringify({ notify: "装好了" });
}`

// TestAPluginPackageIsInstalled covers the install of a plugin from a
// package: it lands in the plugin directory, the editor loads it, and
// its command runs.
func TestAPluginPackageIsInstalled(t *testing.T) {
	a, queue := testDispatchApp(t)
	a.pluginsDir = filepath.Join(t.TempDir(), "plugins")
	package_ := zipPackage(t, map[string]string{
		"wordcount/plugin.json": wordcountManifest,
		"wordcount/plugin.js":   wordcountProgram,
	})
	if err := a.installPluginPackage(package_, "wordcount.zip"); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(a.pluginsDir, "wordcount", "plugin.js")); err != nil {
		t.Fatalf("the plugin is not in its place: %v", err)
	}
	if len(a.plugins) != 1 || a.plugins[0].err != "" {
		t.Fatalf("the plugins: %+v", a.plugins)
	}
	commands := a.pluginCommands()
	if len(commands) != 1 {
		t.Fatalf("the commands of the plugin: %+v", commands)
	}
	commands[0].run()
	pumpUntil(t, queue, func() bool { return a.message != "" })
	if a.message != "装好了" {
		t.Fatalf("the answer of the plugin: %q", a.message)
	}
}

// TestAPluginPackageOfTarIsInstalled covers the other package format.
func TestAPluginPackageOfTarIsInstalled(t *testing.T) {
	a := newApp()
	defer a.shutdown()
	a.pluginsDir = filepath.Join(t.TempDir(), "plugins")
	package_ := tarPackage(t, map[string]string{
		"wordcount/plugin.json": wordcountManifest,
		"wordcount/plugin.js":   wordcountProgram,
	})
	if err := a.installPluginPackage(package_, "wordcount.tar.gz"); err != nil {
		t.Fatal(err)
	}
	if len(a.plugins) != 1 || a.plugins[0].err != "" {
		t.Fatalf("the plugins: %+v", a.plugins)
	}
}

// TestAPackageThatDoesNotFitIsRefused covers the checks of the install:
// a manifest of another version, a package without a manifest, and a
// package that tries to write outside of its folder.
func TestAPackageThatDoesNotFitIsRefused(t *testing.T) {
	a := newApp()
	defer a.shutdown()
	a.pluginsDir = filepath.Join(t.TempDir(), "plugins")
	cases := []struct {
		name     string
		package_ []byte
	}{
		{"另一个版本", zipPackage(t, map[string]string{
			"plugin.json": `{"name":"future","api":99,"runtime":"js","main":"plugin.js"}`,
			"plugin.js":   wordcountProgram,
		})},
		{"没有清单", zipPackage(t, map[string]string{"readme.md": "没有 plugin.json"})},
		{"写到外面去", zipPackage(t, map[string]string{
			"plugin.json": wordcountManifest,
			"../evil.js":  wordcountProgram,
		})},
	}
	for _, item := range cases {
		if err := a.installPluginPackage(item.package_, item.name); err == nil {
			t.Fatalf("the package %q was installed", item.name)
		}
	}
	if entries, err := os.ReadDir(a.pluginsDir); err == nil && len(entries) > 0 {
		t.Fatalf("the refused packages left something behind: %v", entries)
	}
}

// TestAPluginIsUninstalled covers the removal of a plugin: its folder
// goes, and the editor forgets it.
func TestAPluginIsUninstalled(t *testing.T) {
	a := newApp()
	defer a.shutdown()
	a.pluginsDir = filepath.Join(t.TempDir(), "plugins")
	package_ := zipPackage(t, map[string]string{
		"wordcount/plugin.json": wordcountManifest,
		"wordcount/plugin.js":   wordcountProgram,
	})
	if err := a.installPluginPackage(package_, "wordcount.zip"); err != nil {
		t.Fatal(err)
	}
	if err := a.uninstallPlugin("wordcount"); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(a.pluginsDir, "wordcount")); !os.IsNotExist(err) {
		t.Fatalf("the plugin is still there: %v", err)
	}
	if len(a.plugins) != 0 {
		t.Fatalf("the plugins: %+v", a.plugins)
	}
}
