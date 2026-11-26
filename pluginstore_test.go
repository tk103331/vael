package main

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"

	"vael/internal/workspace"
)

// TestThePluginStoreInstallsFromAnIndex covers the store: the editor
// reads the index, checks the sum of a package, and installs it.
func TestThePluginStoreInstallsFromAnIndex(t *testing.T) {
	a, queue := testDispatchApp(t)
	a.pluginsDir = filepath.Join(t.TempDir(), "plugins")
	a.pluginStorePath = filepath.Join(t.TempDir(), "plugin-store.json")
	package_ := zipPackage(t, map[string]string{
		"wordcount/plugin.json": `{"name":"wordcount","version":"0.2.0","api":1,"runtime":"js",
			"main":"plugin.js","capabilities":["notify"],
			"contributes":{"commands":[{"id":"wc","title":"统计字数"}]}}`,
		"wordcount/plugin.js": wordcountProgram,
	})
	sum := sha256.Sum256(package_)
	var server *httptest.Server
	server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/plugins.json":
			fmt.Fprintf(w, `{"plugins":[{"id":"wordcount","name":"统计字数","version":"0.2.0",`+
				`"description":"数一数","url":%q,"sha256":%q}]}`, server.URL+"/plugin.zip", hex.EncodeToString(sum[:]))
		case "/plugin.zip":
			_, _ = w.Write(package_)
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	a.pluginStore.URL = server.URL + "/plugins.json"
	a.loadPluginStore(true)
	pumpUntil(t, queue, func() bool { return len(a.pluginStore.Plugins) == 1 })
	entry := a.pluginStore.Plugins[0]
	if entry.Label() != "统计字数" || !entry.Newer("0.1.0") {
		t.Fatalf("the index: %+v", entry)
	}
	// The index was kept, so a new run does not have to read it again.
	if !a.pluginStore.Fresh() {
		t.Fatalf("the index was not kept: %+v", a.pluginStore)
	}
	if err := a.installPluginFromStore(entry); err != nil {
		t.Fatal(err)
	}
	if len(a.plugins) != 1 || a.installedVersion("wordcount") != "0.2.0" {
		t.Fatalf("the installed plugin: %+v", a.plugins)
	}
	if len(a.pluginCommands()) != 1 {
		t.Fatalf("the commands of the plugin: %+v", a.pluginCommands())
	}
}

// TestThePluginStoreRefusesAnotherVersion covers a package that does not
// hold the version its index entry says.
func TestThePluginStoreRefusesAnotherVersion(t *testing.T) {
	a := newApp()
	defer a.shutdown()
	a.pluginsDir = filepath.Join(t.TempDir(), "plugins")
	package_ := zipPackage(t, map[string]string{
		"wordcount/plugin.json": wordcountManifest,
		"wordcount/plugin.js":   wordcountProgram,
	})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write(package_)
	}))
	defer server.Close()
	entry := workspace.PluginStoreEntry{ID: "wordcount", URL: server.URL, Version: "9.9.9"}
	if err := a.installPluginFromStore(entry); err == nil {
		t.Fatal("a package of another version was installed")
	}
}

// TestThePluginStoreRefusesABadPackage covers the SHA-256 of a store
// package: the wrong sum means no install.
func TestThePluginStoreRefusesABadPackage(t *testing.T) {
	a := newApp()
	defer a.shutdown()
	a.pluginsDir = filepath.Join(t.TempDir(), "plugins")
	package_ := zipPackage(t, map[string]string{
		"wordcount/plugin.json": wordcountManifest,
		"wordcount/plugin.js":   wordcountProgram,
	})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write(package_)
	}))
	defer server.Close()
	entry := workspace.PluginStoreEntry{ID: "wordcount", URL: server.URL, SHA256: "deadbeef"}
	if err := a.installPluginFromStore(entry); err == nil {
		t.Fatal("a package with the wrong sum was installed")
	}
	if len(a.plugins) != 0 {
		t.Fatalf("the plugins: %+v", a.plugins)
	}
}
