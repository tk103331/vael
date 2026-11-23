package main

import (
	"os"
	"testing"

	"github.com/egoist/mygo/ui"
)

func TestTheProblemsListShowsWhatTheServersFound(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(root+"/main.go", []byte("package main\n\nfunc main() {\n}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	a, queue := lspTestApp(t, root)
	a.showBottom = true
	a.bottomTab = "problems"
	a.openFile(root + "/main.go")
	pumpUntil(t, queue, func() bool { return a.tabOf(root+"/main.go") != nil })
	tab := a.tabOf(root + "/main.go")
	pumpUntil(t, queue, func() bool { return len(tab.state.Diagnostics()) > 0 })

	if a.problems() != 1 || a.problemBadge() != "1" {
		t.Fatalf("problems: %d, badge %q", a.problems(), a.problemBadge())
	}
	tt := ui.NewTester(a.view, 1280, 840)
	tt.Frame()
	for _, text := range []string{"问题", "假的错误", "main.go", "fake · 1:1"} {
		found := false
		for _, shown := range tt.Texts() {
			if shown == text {
				found = true
			}
		}
		if !found {
			t.Fatalf("%q is not drawn: %v", text, tt.Texts())
		}
	}
	// A problem goes to its place in the file.
	tab.state.Select(2, 2)
	if err := tt.Click("假的错误"); err != nil {
		t.Fatal(err)
	}
	if selection := tab.state.Selection(); selection.Anchor != 0 || selection.Head != 7 {
		t.Fatalf("the problem did not go to its range: %+v", selection)
	}
}

func TestTheProblemsListSaysWhenThereAreNone(t *testing.T) {
	a := newApp()
	defer a.shutdown()
	a.root = t.TempDir()
	a.showBottom = true
	a.bottomTab = "problems"
	tt := ui.NewTester(a.view, 1280, 840)
	tt.Frame()
	if _, ok := tt.Find("没有发现问题"); !ok {
		t.Fatalf("the list does not say it is empty: %v", tt.Texts())
	}
	if a.problemBadge() != "" {
		t.Fatalf("the badge of no problems is %q", a.problemBadge())
	}
}
