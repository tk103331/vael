package main

import (
	"context"
	"time"

	"vael/internal/core"
	"vael/internal/workspace"
)

// watchFiles watches the files that are open in the editor and keeps the
// editor in step with the disk: a file that changed is reloaded, unless the
// editor has changes of its own, which are kept and reported instead.
//
// The watcher runs on its own goroutine and never touches the editor's state
// directly: what it watches is a snapshot the editor hands it, and what it
// learns it hands back to the editor's thread.
func (a *app) watchFiles(ctx context.Context, backend *core.Client) {
	var watch *core.Watch
	var watched []string
	defer func() {
		if watch != nil {
			_ = watch.Close()
		}
	}()
	for ctx.Err() == nil {
		paths, before := a.watchSnapshot()
		if watch == nil || !samePaths(paths, watched) {
			// The files that are open changed: the core is told to watch
			// the new ones.
			if watch != nil {
				_ = watch.Close()
				watch = nil
			}
			watched = paths
			if len(paths) == 0 {
				select {
				case <-ctx.Done():
					return
				case <-time.After(400 * time.Millisecond):
				}
				continue
			}
			started, err := backend.Watch(ctx, paths, before)
			if err != nil {
				if ctx.Err() != nil {
					return
				}
				select {
				case <-ctx.Done():
					return
				case <-time.After(time.Second):
				}
				continue
			}
			watch = started
		}
		select {
		case <-ctx.Done():
			return
		case change, ok := <-watch.Changes():
			if !ok {
				watch = nil
				continue
			}
			a.update(func() { a.applyDiskChanges([]workspace.DiskChange{change}) })
		}
	}
}

// samePaths tells whether two sets of files are the same.
func samePaths(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

// updateWatchTargets hands the watcher the files that are open, and what the
// disk looked like when they were last read.
func (a *app) updateWatchTargets() {
	paths := []string{}
	before := map[string]workspace.Stamp{}
	for _, f := range a.tabs {
		if f.state == nil || f.diff != nil {
			continue
		}
		stamp, ok := a.stamps[f.path]
		if !ok || stamp.Mod == 0 {
			continue
		}
		paths = append(paths, f.path)
		before[f.path] = stamp
	}
	a.watchMu.Lock()
	a.watchPaths, a.watchBefore = paths, before
	a.watchMu.Unlock()
}

func (a *app) watchSnapshot() ([]string, map[string]workspace.Stamp) {
	a.watchMu.Lock()
	defer a.watchMu.Unlock()
	before := make(map[string]workspace.Stamp, len(a.watchBefore))
	for path, stamp := range a.watchBefore {
		before[path] = stamp
	}
	return append([]string(nil), a.watchPaths...), before
}

func (a *app) applyDiskChanges(changes []workspace.DiskChange) {
	for _, change := range changes {
		tab := a.tabForPath(change.Path)
		if tab == nil {
			continue
		}
		if change.Deleted {
			a.stale[change.Path] = "文件已从磁盘删除"
			continue
		}
		a.stamps[change.Path] = change.After
		if tab.state.Buffer.Modified() {
			a.stale[change.Path] = "文件已在磁盘上更改，你的修改尚未保存"
			continue
		}
		a.reloadFile(tab)
	}
	a.updateWatchTargets()
}

// reloadFile reads a file anew and puts it in its editor, keeping the caret
// where it was.
func (a *app) reloadFile(tab *fileTab) {
	generation := a.generation
	backend := a.backend
	ctx := a.projectCtx
	path := tab.path
	go func() {
		text, stamp, err := backend.ReadFile(ctx, path)
		a.update(func() {
			if generation != a.generation {
				return
			}
			if err != nil {
				a.report(err)
				return
			}
			if tab.state == nil {
				return
			}
			if tab.state.Buffer.Modified() {
				// The file was read while the user was typing: their text
				// is kept, and the change is reported instead.
				a.stamps[path] = stamp
				a.stale[path] = "文件已在磁盘上更改，你的修改尚未保存"
				a.updateWatchTargets()
				return
			}
			cursor := tab.state.Cursor()
			tab.state.Buffer.Reset(text)
			tab.state.Select(cursor, cursor)
			tab.original = text
			a.stamps[path] = stamp
			delete(a.stale, path)
			a.updateWatchTargets()
		})
	}()
}

// forgetFileStamps drops what is known about the disk state of files, so
// that they are watched anew.
func (a *app) forgetFileStamps() {
	a.stamps = map[string]workspace.Stamp{}
	a.stale = map[string]string{}
	a.watchVersion++
	a.updateWatchTargets()
}
