package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/egoist/mygo/ui"
	"vael/internal/core"
)

func TestCommandPaletteFindsCommands(t *testing.T) {
	a := newApp()
	defer a.shutdown()
	commands := a.commands()
	if len(commands) == 0 {
		t.Fatal("no commands")
	}
	matches := matchCommands(commands, "快速打开", nil)
	if len(matches) == 0 || !strings.Contains(matches[0].title, "快速打开") {
		t.Fatalf("%+v", matches)
	}
	if got := matchCommands(commands, "", nil); len(got) != len(commands) {
		t.Fatalf("empty query filtered commands: %d", len(got))
	}
	if got := matchCommands(commands, "zzzz-not-a-command", nil); len(got) != 0 {
		t.Fatalf("unknown query matched: %+v", got)
	}
	// A query whose characters appear in order matches too.
	if got := matchCommands(commands, "快速打", nil); len(got) == 0 {
		t.Fatal("fuzzy match found nothing")
	}
}

func TestPaletteOpensAndRunsACommand(t *testing.T) {
	a := newApp()
	defer a.shutdown()
	a.paletteOpen = true
	a.paletteQuery = "切换终端"
	tt := ui.NewTester(a.view, 1280, 840)
	tt.Frame()
	if !a.paletteOpen {
		t.Fatal("palette did not open")
	}
	matches := matchCommands(a.commands(), a.paletteQuery, nil)
	if len(matches) == 0 {
		t.Fatal("no command matched")
	}
	a.runPaletteCommand(matches, 0)
	if a.paletteOpen {
		t.Fatal("palette stayed open")
	}
	if !a.showBottom {
		t.Fatal("command did not run")
	}
}

func TestWatcherReloadsACleanFileAndKeepsADirtyOne(t *testing.T) {
	a, queue := testDispatchApp(t)
	root := t.TempDir()
	path := filepath.Join(root, "a.txt")
	if err := os.WriteFile(path, []byte("one"), 0644); err != nil {
		t.Fatal(err)
	}
	a.useProject(core.NewLocal(), nil, root)
	a.openFile(path)
	pumpUntil(t, queue, func() bool { return a.tabForPath(path) != nil })

	// A file that changed on disk is read again, and the caret stays.
	if err := os.WriteFile(path, []byte("two"), 0644); err != nil {
		t.Fatal(err)
	}
	pumpUntil(t, queue, func() bool {
		tab := a.tabForPath(path)
		return tab != nil && tab.state.Buffer.String() == "two"
	})
	if tab := a.tabForPath(path); tab.original != "two" || tab.state.Buffer.Modified() {
		t.Fatal("reload did not settle the file")
	}

	// A file with changes of its own keeps them, and says what happened.
	tab := a.tabForPath(path)
	tab.state.Buffer.Set("mine")
	if err := os.WriteFile(path, []byte("theirs"), 0644); err != nil {
		t.Fatal(err)
	}
	pumpUntil(t, queue, func() bool { return a.stale[path] != "" })
	if tab.state.Buffer.String() != "mine" {
		t.Fatalf("the editor's own text was lost: %q", tab.state.Buffer.String())
	}
	if !strings.Contains(a.stale[path], "磁盘") {
		t.Fatalf("the change was not reported: %q", a.stale[path])
	}
	// Saving now reports the conflict instead of overwriting the other text.
	a.save()
	pumpUntil(t, queue, func() bool { return a.message != "" })
	actual, _ := os.ReadFile(path)
	if string(actual) != "theirs" {
		t.Fatalf("the other program's text was overwritten: %q", actual)
	}
}

func TestRenameAndDeleteFollowOpenEditors(t *testing.T) {
	a, queue := testDispatchApp(t)
	root := t.TempDir()
	path := filepath.Join(root, "old.txt")
	if err := os.WriteFile(path, []byte("hello"), 0644); err != nil {
		t.Fatal(err)
	}
	a.useProject(core.NewLocal(), nil, root)
	a.openFile(path)
	pumpUntil(t, queue, func() bool { return a.tabForPath(path) != nil })

	a.renameEntry(path, "new.txt")
	pumpUntil(t, queue, func() bool {
		_, err := os.Stat(filepath.Join(root, "new.txt"))
		return err == nil
	})
	renamed := filepath.Join(root, "new.txt")
	pumpUntil(t, queue, func() bool { return a.tabForPath(renamed) != nil })
	if a.tabForPath(path) != nil {
		t.Fatal("the editor still points at the old name")
	}

	a.duplicateEntry(renamed)
	pumpUntil(t, queue, func() bool {
		_, err := os.Stat(filepath.Join(root, "new copy.txt"))
		return err == nil
	})

	a.deleteEntry(renamed)
	pumpUntil(t, queue, func() bool { return a.tabForPath(renamed) == nil })
	if _, err := os.Stat(renamed); !os.IsNotExist(err) {
		t.Fatal("the file was not deleted")
	}
}

func TestRevealInTreeExpandsTheFoldersOfAFile(t *testing.T) {
	a := newApp()
	defer a.shutdown()
	root := t.TempDir()
	deep := filepath.Join(root, "one", "two")
	if err := os.MkdirAll(deep, 0755); err != nil {
		t.Fatal(err)
	}
	a.root = root
	a.revealInTree(filepath.Join(deep, "file.txt"))
	if !a.expanded[filepath.Join(root, "one")] || !a.expanded[deep] {
		t.Fatalf("folders were not expanded: %+v", a.expanded)
	}
}
