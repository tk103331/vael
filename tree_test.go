package main

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/egoist/mygo/ui"
	"vael/internal/core"
)

// TestTheFileTreeScrollsItsLongNames checks that a name wider than the
// panel is shown whole in a tree wider than the panel, which scrolls
// sideways, rather than cut to an ellipsis.
func TestTheFileTreeScrollsItsLongNames(t *testing.T) {
	a, queue := testDispatchApp(t)
	root := t.TempDir()
	long := "WikiTaskIsAutoEnumWithAVeryLongNameThatGoesOnAndOn.java"
	if err := os.WriteFile(filepath.Join(root, long), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	a.useProject(core.NewLocal(), nil, root)
	pumpUntil(t, queue, func() bool { return len(a.entries[a.root]) > 0 })
	a.sideWidth = 220
	a.sidebar = true
	tt := ui.NewTester(a.view, 1280, 840)
	tt.Frame()
	if a.treeScroll.MaxX <= 0 {
		t.Fatalf("the tree is not wider than the panel: %v", a.treeScroll)
	}
	tt.Scroll(120, 150, 200, 0)
	tt.Frame()
	if a.treeScroll.X == 0 {
		t.Fatal("the tree did not scroll sideways")
	}
	if a.treeScroll.X > a.treeScroll.MaxX {
		t.Fatalf("the scroll went past the tree: %v > %v", a.treeScroll.X, a.treeScroll.MaxX)
	}
	// The name is in the tree whole, whatever the panel shows.
	if !tt.HasText(long) {
		t.Fatalf("the name is not the tree's: %v", tt.Texts())
	}
}
