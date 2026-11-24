package main

import (
	"fmt"
	"math"
	"testing"

	"github.com/egoist/mygo/ui"
	"github.com/egoist/mygo/ui/editor"
	"vael/internal/workspace"
)

func TestGitChangeStatusUsesTheSectionColumn(t *testing.T) {
	for _, test := range []struct {
		raw, staged, worktree string
	}{
		{"MM", "M", "M"}, {"AM", "A", "M"}, {"RM", "R", "M"},
		{"MD", "M", "D"}, {"A ", "A", "?"}, {" D", "?", "D"},
		{"??", "?", "?"}, {"T ", "T", "?"}, {"C ", "C", "?"},
		{"UU", "U", "U"}, {"AA", "U", "U"}, {"DU", "U", "U"},
	} {
		for _, staged := range []bool{true, false} {
			want := test.worktree
			if staged {
				want = test.staged
			}
			got, description := gitChangeStatus(workspace.Change{Code: test.raw}, staged)
			if got != want || description == "" {
				t.Errorf("status %q, staged %v: %q (%s), want %q", test.raw, staged, got, description, want)
			}
		}
	}
}

func TestCommitPanelHasCompactHistoryAndDraggableSplit(t *testing.T) {
	a := newApp()
	defer a.shutdown()
	a.root, a.section = t.TempDir(), "commit"
	a.repo = &workspace.Repository{Root: a.root, Branch: "main"}
	for i := 0; i < 40; i++ {
		a.repo.Changes = append(a.repo.Changes, workspace.Change{Code: " M", Path: fmt.Sprintf("file-%02d.go", i)})
		a.commits = append(a.commits, fmt.Sprintf("%07d 提交 %02d", i, i))
	}
	tt := ui.NewTester(a.view, 1280, 840)
	tt.Frame()
	row, ok := tt.Find("file-00.go")
	if !ok || row.H <= 0 {
		t.Fatal("no change row")
	}
	status, ok := tt.Find("修改")
	if !ok || status.X > row.X+8 {
		t.Fatalf("status is not left aligned: row %+v, status %+v", row, status)
	}
	first, _ := tt.Find("提交：" + a.commits[0])
	second, _ := tt.Find("提交：" + a.commits[1])
	if first.H != 22 || second.Y-first.Y != 22 {
		t.Fatalf("history rows are not compact: %+v %+v", first, second)
	}
	// Scrolling the upper list must leave the history in place.
	tt.Scroll(row.X+30, row.Y+10, 0, 240)
	if after, _ := tt.Find("提交：" + a.commits[0]); after.Y != first.Y {
		t.Fatalf("scrolling changes moved history: %+v -> %+v", first, after)
	}
	area, ok := tt.Find("Commit 面板分隔条")
	if !ok {
		t.Fatal("no split")
	}
	x, y := area.X+area.W/2, area.Y+(area.H-1)*a.commitShare/100+0.5
	before := a.commitShare
	tt.Press(x, y)
	tt.Move(x, y-80)
	tt.Release(x, y-80)
	if a.commitShare >= before-5 {
		t.Fatalf("dragging did not resize panels: %v -> %v, split %+v", before, a.commitShare, area)
	}
	if after, _ := tt.Find("提交：" + a.commits[0]); after.Y >= first.Y-40 {
		t.Fatalf("history did not move with split: %+v -> %+v", first, after)
	}
}

func TestDiffLayoutSwitchesSingleAndCommitFiles(t *testing.T) {
	for _, multi := range []bool{false, true} {
		t.Run(fmt.Sprintf("multi=%v", multi), func(t *testing.T) {
			a := newApp()
			defer a.shutdown()
			a.root = t.TempDir()
			diff := &editor.DiffState{Old: editor.NewBuffer("before\n"), New: editor.NewBuffer("after\n")}
			tab := &fileTab{path: "main.go · 工作区", diff: diff}
			if multi {
				diff.FitContent = true
				tab = &fileTab{path: "abc12345 · 提交", commitDiff: &commitDiff{hash: "abc12345", files: []commitDiffFile{
					{code: "M", path: "main.go", diff: diff},
					{code: "M", path: "other.go", diff: &editor.DiffState{Old: editor.NewBuffer("one"), New: editor.NewBuffer("two"), FitContent: true}},
				}}}
			}
			a.tabs, a.selected = []*fileTab{tab}, 0
			tt := ui.NewTester(a.view, 1280, 840)
			tt.Frame()
			old, next := diff.Old, diff.New
			for _, layout := range []struct {
				label string
				want  editor.DiffLayout
			}{{"行内对比", editor.DiffUnified}, {"分栏对比", editor.DiffSplit}} {
				if err := tt.Click(layout.label); err != nil {
					t.Fatal(err)
				}
				tt.Frame()
				if diff.Layout != layout.want || diff.Old != old || diff.New != next {
					t.Fatalf("layout %s: %v, buffers changed: %v", layout.label, diff.Layout, diff.Old != old || diff.New != next)
				}
				if multi && tab.commitDiff.files[1].diff.Layout != layout.want {
					t.Fatal("second file did not follow layout")
				}
			}
		})
	}
}

func TestSessionKeepsCommitSplitAndDiffLayout(t *testing.T) {
	a := newApp()
	defer a.shutdown()
	a.commitShare, a.diffLayout = 42, editor.DiffUnified
	restored := newApp()
	defer restored.shutdown()
	restored.applyLayout(a.snapshot())
	if math.Abs(float64(restored.commitShare-42)) > .01 || restored.diffLayout != editor.DiffUnified {
		t.Fatalf("restored split %v, diff layout %v", restored.commitShare, restored.diffLayout)
	}
}
