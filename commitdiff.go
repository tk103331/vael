package main

import (
	"context"
	"fmt"
	"strings"

	"github.com/egoist/mygo/ui"
	"github.com/egoist/mygo/ui/editor"
	"vael/internal/core"
)

// commitDiff keeps all the files of one immutable commit in a single tab.
type commitDiff struct {
	hash, parent, subject, author, date, body string
	files                                     []commitDiffFile
	loading                                   bool
	err                                       string
	scroll                                    ui.ScrollState
	cancel                                    context.CancelFunc
}

type commitDiffFile struct {
	code, path, original string
	diff                 *editor.DiffState
	folded               bool
	note                 string
}

func shortCommit(hash string) string { return hash[:min(8, len(hash))] }

func (a *app) openCommitDiff(hash, subject string) {
	if a.repo == nil || hash == "" || !a.requireProject() {
		return
	}
	// Both history lists may name the same commit with different hash lengths.
	for i, tab := range a.tabs {
		if d := tab.commitDiff; d != nil && (strings.HasPrefix(d.hash, hash) || strings.HasPrefix(hash, d.hash)) {
			a.focusTab(i)
			return
		}
	}
	ctx, cancel := context.WithCancel(a.projectCtx)
	d := &commitDiff{hash: hash, subject: subject, loading: true, cancel: cancel}
	tab := &fileTab{path: shortCommit(hash) + " · " + subject, commitDiff: d}
	a.tabs = append(a.tabs, tab)
	a.focusTab(len(a.tabs) - 1)
	backend, root, generation := a.backend, a.repo.Root, a.generation
	apply := func(fn func()) {
		a.update(func() {
			if generation != a.generation || ctx.Err() != nil {
				return
			}
			for _, open := range a.tabs {
				if open == tab {
					fn()
					return
				}
			}
		})
	}
	go func() {
		// Cancellation also belongs to the tab: closing it stops remote reads.
		meta, files, err := readCommitDiff(ctx, backend, root, hash)
		if err != nil {
			apply(func() {
				d.loading, d.err, d.cancel = false, err.Error(), nil
				cancel()
			})
			return
		}
		apply(func() {
			d.hash, d.parent, d.subject = meta.hash, meta.parent, meta.subject
			d.author, d.date, d.body = meta.author, meta.date, meta.body
			d.files = append([]commitDiffFile(nil), files...)
			tab.path = shortCommit(meta.hash) + " · " + meta.subject
			// A split pane identifies its tab by path.
			if a.splitPath == shortCommit(hash)+" · "+subject {
				a.splitPath = tab.path
			}
		})
		for i, file := range files {
			if ctx.Err() != nil {
				return
			}
			old, next, err := readCommitDiffFile(ctx, backend, root, meta, file)
			apply(func() {
				f := &d.files[i]
				if err != nil {
					f.note = "读取差异失败：" + err.Error()
					return
				}
				if strings.ContainsRune(old, '\x00') || strings.ContainsRune(next, '\x00') {
					f.note = "二进制文件"
					return
				}
				before, after := editor.NewBuffer(old), editor.NewBuffer(next)
				before.SetLanguage(editor.LanguageFor(file.original))
				after.SetLanguage(editor.LanguageFor(file.path))
				f.diff = &editor.DiffState{Old: before, New: after, Layout: editor.DiffSplit, FitContent: true}
			})
		}
		apply(func() {
			d.loading, d.cancel = false, nil
			cancel()
		})
	}()
}

// Compare merges with their first parent, as a commit review does. With no
// parent diff-tree --root compares against the empty tree.
func readCommitDiff(ctx context.Context, backend *core.Client, root, hash string) (*commitDiff, []commitDiffFile, error) {
	output, err := backend.Git(ctx, root, "show", "-s", "--date=iso-strict", "--format=%H%x00%P%x00%an%x00%ad%x00%s%x00%B", hash, "--")
	if err != nil {
		return nil, nil, err
	}
	fields := strings.SplitN(output, "\x00", 6)
	if len(fields) != 6 {
		return nil, nil, fmt.Errorf("无法读取提交 %s", hash)
	}
	d := &commitDiff{hash: fields[0], author: fields[2], date: fields[3], subject: fields[4], body: strings.TrimSpace(fields[5])}
	if parents := strings.Fields(fields[1]); len(parents) > 0 {
		d.parent = parents[0]
	}
	args := []string{"diff-tree", "-r", "--no-commit-id", "--name-status", "-z", "-M"}
	if d.parent == "" {
		args = append(args, "--root", d.hash)
	} else {
		args = append(args, d.parent, d.hash)
	}
	output, err = backend.Git(ctx, root, append(args, "--")...)
	if err != nil {
		return nil, nil, err
	}
	return d, parseCommitDiffFiles(output), nil
}

// NUL-delimited paths preserve spaces, tabs and non-ASCII names. Rename
// records carry both names so each side reads the matching historical blob.
func parseCommitDiffFiles(output string) []commitDiffFile {
	fields := strings.Split(strings.TrimSuffix(output, "\x00"), "\x00")
	var files []commitDiffFile
	for i := 0; i+1 < len(fields); {
		f := commitDiffFile{code: fields[i], path: fields[i+1], original: fields[i+1]}
		i += 2
		if strings.HasPrefix(f.code, "R") || strings.HasPrefix(f.code, "C") {
			if i >= len(fields) {
				break
			}
			f.path = fields[i]
			i++
		}
		files = append(files, f)
	}
	return files
}

func readCommitDiffFile(ctx context.Context, backend *core.Client, root string, d *commitDiff, f commitDiffFile) (old, next string, err error) {
	if d.parent != "" && !strings.HasPrefix(f.code, "A") {
		old, err = backend.Git(ctx, root, "show", d.parent+":"+f.original)
	}
	if err == nil && !strings.HasPrefix(f.code, "D") {
		next, err = backend.Git(ctx, root, "show", d.hash+":"+f.path)
	}
	return
}

func (a *app) commitDiffView(c *ui.Context, p palette, d *commitDiff, pane int) {
	ui.Column(c).FillWidth().Grow(1).MinHeight(0).Children(func() {
		ui.Row(c).FillWidth().Height(32).Shrink(0).Padding(0, 10).Gap(8).AlignItems(ui.Center).Children(func() {
			ui.Icon(c, commitIcon).Size(13, 13).TextColor(p.accent)
			ui.Text(c, shortCommit(d.hash)).Font("monospace").FontSize(11).TextColor(p.accent)
			ui.Text(c, d.subject).FontSize(12).TextColor(p.text).Grow(1).MinWidth(0).SingleLine().Ellipsis("…").Tooltip(d.body)
			ui.Textf(c, "%d 个文件", len(d.files)).FontSize(10.5).TextColor(p.muted)
			a.diffLayoutControls(c, p)
		})
		ui.Divider(c)
		ui.Scroll(c).Key("commit diff " + d.hash).FillWidth().Grow(1).MinHeight(0).TrackScroll(&d.scroll).Children(func() {
			ui.Column(c).FillWidth().Children(func() {
				if d.author != "" {
					ui.Text(c, d.author+" · "+d.date).FontSize(11).TextColor(p.muted).Padding(8, 10)
				}
				if d.err != "" {
					ui.Text(c, d.err).FontSize(12).TextColor(p.danger).Padding(10)
				}
				for i := range d.files {
					f := &d.files[i]
					ui.Column(c).Key(f.path).FillWidth().Shrink(0).Children(func() {
						b := ui.ButtonBase(c).FillWidth().Height(30).Shrink(0).Padding(0, 10).Gap(7).
							Background(p.wash).AlignItems(ui.Center).Label("折叠差异：" + f.path)
						if b.Hovered() {
							b.Background(p.edge)
						}
						b.Children(func() {
							icon := chevronDownIcon
							if f.folded {
								icon = chevronRightIcon
							}
							ui.Icon(c, icon).Size(10, 10).TextColor(p.muted)
							ui.Text(c, f.code[:1]).FontSize(11).FontWeight(650).TextColor(logCodeColor(f.code, p))
							name := f.path
							if f.original != f.path {
								name = f.original + " → " + f.path
							}
							ui.Text(c, name).FontSize(11.5).TextColor(p.text).Grow(1).MinWidth(0).SingleLine().Ellipsis("…").Tooltip(name)
						})
						if b.Clicked() {
							a.focusPaneOn(pane)
							f.folded = !f.folded
						}
						if !f.folded {
							switch {
							case f.diff != nil:
								f.diff.Layout = a.diffLayout
								f.diff.Font, f.diff.LineHeight, f.diff.TabSize = a.editorFont(), a.editorLineHeight(), a.tabSize
								editor.DiffView(c, f.diff).FillWidth().Shrink(0)
							case f.note != "":
								ui.Text(c, f.note).FontSize(11).TextColor(p.muted).Padding(10)
							default:
								ui.Text(c, "正在读取差异…").FontSize(11).TextColor(p.muted).Padding(10)
							}
						}
						ui.Divider(c)
					})
				}
				if d.loading {
					ui.Text(c, "正在读取提交…").FontSize(11).TextColor(p.muted).Padding(10)
				} else if d.err == "" && len(d.files) == 0 {
					ui.Text(c, "没有文件变更").FontSize(12).TextColor(p.muted).Padding(10)
				}
			})
		})
	})
}
