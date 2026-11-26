package main

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/egoist/mygo/ui/editor"
)

// TestAPreviewIsReplacedAndPinned covers the tabs a single click opens: a
// second single click replaces the preview, opening the file for good
// pins it, and a preview the user edited is pinned rather than replaced.
func TestAPreviewIsReplacedAndPinned(t *testing.T) {
	a, queue := testDispatchApp(t)
	root := t.TempDir()
	first, second, third := filepath.Join(root, "first.go"), filepath.Join(root, "second.go"), filepath.Join(root, "third.go")
	for _, path := range []string{first, second, third} {
		if err := os.WriteFile(path, []byte("package demo\n"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	a.root = root

	a.openPreview(first)
	pumpUntil(t, queue, func() bool { return len(a.tabs) == 1 && a.tabs[0].path == first })
	if !a.tabs[0].preview {
		t.Fatal("the first file did not open as a preview")
	}

	// The next preview takes its place instead of adding a tab.
	a.openPreview(second)
	pumpUntil(t, queue, func() bool { return len(a.tabs) == 1 && a.tabs[0].path == second })
	if !a.tabs[0].preview {
		t.Fatal("the second file is not a preview")
	}
	if _, err := os.Stat(first); err != nil {
		t.Fatal(err)
	}

	// Opening it for good pins it, and the next preview gets a tab of its
	// own.
	a.openFile(second)
	if a.tabs[0].preview {
		t.Fatal("opening the preview for good did not pin it")
	}
	a.openPreview(third)
	pumpUntil(t, queue, func() bool { return len(a.tabs) == 2 && a.tabs[1].path == third })
	if !a.tabs[1].preview || a.tabs[0].preview {
		t.Fatalf("the tabs: %+v", a.tabs)
	}

	// An edited preview is pinned, so that its text cannot be replaced.
	a.openPreview(first)
	pumpUntil(t, queue, func() bool { return len(a.tabs) == 2 && a.tabs[1].path == first })
	a.tabs[1].state.Buffer.Edit(editor.Edit{Start: 0, End: 0, Text: "// edited\n"})
	if !a.tabs[1].state.Buffer.Modified() {
		t.Fatal("the edit did not change the buffer")
	}
	a.openPreview(third)
	pumpUntil(t, queue, func() bool { return len(a.tabs) == 3 && a.tabs[2].path == third })
	if a.tabs[1].preview || a.tabs[1].path != first {
		t.Fatalf("the edited preview was replaced: %+v", a.tabs[1])
	}
	if !a.tabs[2].preview || a.tabs[2].path != third {
		t.Fatalf("the new preview: %+v", a.tabs[2])
	}
}
