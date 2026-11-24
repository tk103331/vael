package main

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/egoist/mygo/ui"
	"github.com/egoist/mygo/ui/editor"
	"vael/internal/core"
)

func commitDiffGit(t *testing.T, root string, args ...string) string {
	t.Helper()
	out, err := exec.Command("git", append([]string{"-C", root}, args...)...).CombinedOutput()
	if err != nil {
		t.Fatalf("git %v: %s (%v)", args, out, err)
	}
	return strings.TrimSpace(string(out))
}

func commitDiffWrite(t *testing.T, root, name, content string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(root, name), []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestCommitDiffReadsHistoricalFiles(t *testing.T) {
	a, _ := testDispatchApp(t)
	root := gitRepository(t)
	for name, content := range map[string]string{"edit.txt": "before\n", "delete.txt": "deleted\n", "旧 文件.txt": "renamed\n"} {
		commitDiffWrite(t, root, name, content)
	}
	commitDiffGit(t, root, "add", ".")
	commitDiffGit(t, root, "commit", "-m", "initial")
	initial := commitDiffGit(t, root, "rev-parse", "HEAD")
	meta, files, err := readCommitDiff(context.Background(), a.backend, root, initial)
	if err != nil || len(files) != 3 || meta.parent != "" {
		t.Fatalf("initial: %+v %+v %v", meta, files, err)
	}
	for _, f := range files {
		old, next, err := readCommitDiffFile(context.Background(), a.backend, root, meta, f)
		if err != nil || old != "" || next == "" || f.code != "A" {
			t.Fatalf("initial file %+v: %q -> %q: %v", f, old, next, err)
		}
	}
	commitDiffWrite(t, root, "edit.txt", "after\n")
	commitDiffWrite(t, root, "added.txt", "added\n")
	commitDiffGit(t, root, "rm", "delete.txt")
	commitDiffGit(t, root, "mv", "旧 文件.txt", "新\t文件.txt")
	commitDiffGit(t, root, "add", ".")
	commitDiffGit(t, root, "commit", "-m", "update files")
	meta, files, err = readCommitDiff(context.Background(), a.backend, root, "HEAD")
	if err != nil || len(files) != 4 || meta.parent != initial {
		t.Fatalf("update: %+v %+v %v", meta, files, err)
	}
	// The review must read historical blobs even when the working tree differs.
	commitDiffWrite(t, root, "edit.txt", "uncommitted\n")
	want := map[string][3]string{
		"edit.txt": {"M", "before\n", "after\n"}, "added.txt": {"A", "", "added\n"},
		"delete.txt": {"D", "deleted\n", ""}, "新\t文件.txt": {"R100", "renamed\n", "renamed\n"},
	}
	for _, f := range files {
		old, next, err := readCommitDiffFile(context.Background(), a.backend, root, meta, f)
		if expected, ok := want[f.path]; !ok || err != nil || [3]string{f.code, old, next} != expected {
			t.Fatalf("file %+v: %q -> %q: %v", f, old, next, err)
		}
	}
}

func TestCommitDiffMergeUsesFirstParent(t *testing.T) {
	a, _ := testDispatchApp(t)
	root := gitRepository(t)
	commitDiffWrite(t, root, "base.txt", "base\n")
	commitDiffGit(t, root, "add", ".")
	commitDiffGit(t, root, "commit", "-m", "base")
	branch := commitDiffGit(t, root, "branch", "--show-current")
	commitDiffGit(t, root, "checkout", "-b", "topic")
	commitDiffWrite(t, root, "topic.txt", "topic\n")
	commitDiffGit(t, root, "add", ".")
	commitDiffGit(t, root, "commit", "-m", "topic")
	commitDiffGit(t, root, "checkout", branch)
	commitDiffWrite(t, root, "main.txt", "main\n")
	commitDiffGit(t, root, "add", ".")
	commitDiffGit(t, root, "commit", "-m", "main")
	parent := commitDiffGit(t, root, "rev-parse", "HEAD")
	commitDiffGit(t, root, "merge", "--no-ff", "topic", "-m", "merge topic")
	meta, files, err := readCommitDiff(context.Background(), a.backend, root, "HEAD")
	if err != nil || meta.parent != parent || len(files) != 1 || files[0].path != "topic.txt" {
		t.Fatalf("merge: %+v %+v %v", meta, files, err)
	}
}

func TestBothHistoryListsOpenCommitDiffOnDoubleClick(t *testing.T) {
	for _, panel := range []string{"commit", "git"} {
		t.Run(panel, func(t *testing.T) {
			a, queue := testDispatchApp(t)
			root := gitRepository(t)
			commitDiffWrite(t, root, "one.txt", "one\n")
			commitDiffWrite(t, root, "two.txt", "two\n")
			commitDiffWrite(t, root, "binary.dat", "a\x00b")
			commitDiffGit(t, root, "add", ".")
			commitDiffGit(t, root, "commit", "-m", "review all files")
			hash := commitDiffGit(t, root, "rev-parse", "HEAD")
			a.useProject(core.NewLocal(), nil, root)
			pumpUntil(t, queue, func() bool { return a.repo != nil })
			label := "review all files"
			if panel == "commit" {
				a.section = "commit"
				a.loadCommits()
				pumpUntil(t, queue, func() bool { return !a.commitsLoading })
				label = "提交：" + a.commits[0]
			} else {
				a.showLog()
				pumpUntil(t, queue, func() bool { return a.logLoaded && !a.logLoading })
			}
			tt := ui.NewTester(a.view, 1280, 840)
			tt.Frame()
			if err := tt.Click(label); err != nil {
				t.Fatal(err)
			}
			if len(a.tabs) != 0 {
				t.Fatal("single click opened a tab")
			}
			if err := tt.Click(label); err != nil {
				t.Fatal(err)
			}
			if len(a.tabs) != 1 || a.active().commitDiff == nil {
				t.Fatalf("double click did not open the commit: %+v", a.tabs)
			}
			d := a.active().commitDiff
			pumpUntil(t, queue, func() bool { return !d.loading })
			if d.err != "" || len(d.files) != 3 {
				t.Fatalf("files: %+v, error: %s", d.files, d.err)
			}
			for _, f := range d.files {
				if f.path == "binary.dat" {
					if f.note != "二进制文件" || f.diff != nil {
						t.Fatalf("binary: %+v", f)
					}
				} else if f.diff == nil || f.diff.Editable || !f.diff.FitContent {
					t.Fatalf("text: %+v", f)
				}
			}
			tt.Frame()
			tt.Frame()
			for _, name := range []string{"one.txt", "two.txt"} {
				if _, ok := tt.Find("折叠差异：" + name); !ok {
					t.Fatalf("file %q not rendered: %v", name, tt.Texts())
				}
			}
			if err := tt.Click("折叠差异：one.txt"); err != nil {
				t.Fatal(err)
			}
			if !d.files[1].folded {
				t.Fatal("file did not fold")
			}
			a.openCommitDiff(hash, "review all files")
			a.openCommitDiff(hash[:7], "review all files")
			if len(a.tabs) != 1 {
				t.Fatal("the two hash lengths duplicated a tab")
			}
			a.closeTab(0)
			if len(a.tabs) != 0 {
				t.Fatal("commit tab did not close")
			}
		})
	}
}

func TestCommitDiffScrollsAcrossFiles(t *testing.T) {
	a := newApp()
	defer a.shutdown()
	a.root = t.TempDir()
	d := &commitDiff{hash: "1234567890", subject: "多文件差异"}
	for i, name := range []string{"first.go", "last.go"} {
		d.files = append(d.files, commitDiffFile{code: "A", path: name, original: name,
			diff: &editor.DiffState{Old: editor.NewBuffer(""), New: editor.NewBuffer(strings.Repeat("package main\n", 80-i*78)), FitContent: true}})
	}
	a.tabs = []*fileTab{{path: "12345678 · 多文件差异", commitDiff: d}}
	a.selected = 0
	tt := ui.NewTester(a.view, 1280, 840)
	tt.Frame()
	tt.Frame()
	if d.scroll.MaxY < 500 {
		t.Fatalf("diff files do not scroll together: %+v", d.scroll)
	}
	r, ok := tt.Find("折叠差异：first.go")
	if !ok {
		t.Fatal("the first file is missing")
	}
	tt.Scroll(r.X+100, r.Y+r.H+40, 0, 400)
	tt.Frame()
	if d.scroll.Y <= 0 {
		t.Fatal("scrolling over a diff did not move the shared scroll")
	}
	d.scroll.Y = d.scroll.MaxY
	tt.Frame()
	tt.Frame()
	if r, ok := tt.Find("折叠差异：last.go"); !ok || r.H <= 0 || r.W <= 0 {
		t.Fatalf("last file is unreachable: %v %+v", ok, r)
	}
	if d.scroll.Y <= 0 {
		t.Fatal("shared scroll stayed at the top")
	}
}
