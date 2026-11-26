package workspace

import (
	"path/filepath"
	"testing"
	"time"
)

func TestThePluginStoreKeepsItsIndex(t *testing.T) {
	file := filepath.Join(t.TempDir(), "plugin-store.json")
	store := PluginStore{
		URL:       "https://example.com/plugins.json",
		UpdatedAt: time.Now().Format(time.RFC3339),
		Plugins:   []PluginStoreEntry{{ID: "wordcount", Name: "统计字数", Version: "0.2.0"}},
	}
	if err := SavePluginStore(file, store); err != nil {
		t.Fatal(err)
	}
	saved, err := LoadPluginStore(file)
	if err != nil || len(saved.Plugins) != 1 || saved.Plugins[0].Label() != "统计字数" {
		t.Fatalf("the saved store: %+v %v", saved, err)
	}
	if !saved.Fresh() {
		t.Fatal("a store just read is already old")
	}
	saved.UpdatedAt = time.Now().Add(-7 * time.Hour).Format(time.RFC3339)
	if saved.Fresh() {
		t.Fatal("a store from a week ago is still fresh")
	}
}

func TestTheStoreTellsWhichVersionIsNewer(t *testing.T) {
	entry := PluginStoreEntry{ID: "wordcount", Version: "0.2.0"}
	if !entry.Newer("0.1.0") || !entry.Newer("") {
		t.Fatal("the new version is not offered")
	}
	if entry.Newer("0.2.0") || entry.Newer("0.3.0") {
		t.Fatal("an old or same version is offered")
	}
	entry.Version = "1.0.0"
	if !entry.Newer("0.9.9") {
		t.Fatal("a major update is not offered")
	}
}
