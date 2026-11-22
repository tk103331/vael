package main

import (
	"testing"

	"vael/internal/workspace"
)

func TestPaletteOrdersByUse(t *testing.T) {
	a := newApp()
	defer a.shutdown()
	commands := a.commands()
	chosen := commands[2].title
	a.useCommand(chosen)
	a.useCommand(chosen)
	ordered := matchCommands(commands, "", a.commandUses)
	if ordered[0].title != chosen {
		t.Fatalf("the used command did not come first: %q", ordered[0].title)
	}
	if got := matchCommands(commands, "", nil); got[0].title == chosen {
		t.Fatal("without uses the order changed")
	}
}

func TestTreeColorsSayWhatGitKnows(t *testing.T) {
	a := newApp()
	defer a.shutdown()
	p := a.colors()
	a.repo = &workspace.Repository{
		Root:   "/p",
		Branch: "main",
		Changes: []workspace.Change{
			{Code: " M", Path: "changed.go"},
			{Code: "??", Path: "new.go"},
			{Code: "M ", Path: "staged.go"},
		},
	}
	if color, tip := a.treeColor("/p/changed.go", p); color != p.warn || tip != "Git 已修改" {
		t.Fatalf("modified: %v %q", color, tip)
	}
	if color, tip := a.treeColor("/p/new.go", p); color != p.success || tip != "Git 未跟踪" {
		t.Fatalf("untracked: %v %q", color, tip)
	}
	if color, tip := a.treeColor("/p/staged.go", p); color != p.text || tip != "" {
		t.Fatalf("staged: %v %q", color, tip)
	}
	if color, tip := a.treeColor("/p/plain.go", p); color != p.text || tip != "" {
		t.Fatalf("plain: %v %q", color, tip)
	}
}
