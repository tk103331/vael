package main

import (
	"os"
	"path/filepath"
	"strings"
)

// gitRel is the path of a file of the project, as Git names it. The disk
// may know a folder by two names (/var and /private/var on macOS), and Git
// answers with the one it resolved: both are resolved before comparing.
func (a *app) gitRel(repoRoot, path string) (string, error) {
	if a.connection == nil {
		if real, err := filepath.EvalSymlinks(repoRoot); err == nil {
			repoRoot = real
		}
		if real, err := filepath.EvalSymlinks(path); err == nil {
			path = real
		}
	}
	return a.relativePath(repoRoot, path)
}

// tabForPath is the editor of a file, when it is open.
func (a *app) tabForPath(path string) *fileTab {
	for _, f := range a.tabs {
		if f.path == path && f.diff == nil {
			return f
		}
	}
	return nil
}

// renameEntry renames a file or a directory of the project, and follows the
// editors of what it renamed.
func (a *app) renameEntry(path, name string) {
	if !a.requireProject() {
		return
	}
	name = strings.TrimSpace(name)
	if name == "" {
		return
	}
	backend := a.backend
	ctx := a.projectCtx
	root, generation := a.root, a.generation
	go func() {
		err := backend.Rename(ctx, root, path, name)
		a.update(func() {
			if generation != a.generation {
				return
			}
			a.report(err)
			if err != nil {
				return
			}
			dir := filepath.Dir(path)
			target := filepath.Join(dir, name)
			for _, f := range a.tabs {
				if f.path == path {
					f.path = target
				}
			}
			if stamp, ok := a.stamps[path]; ok {
				a.stamps[target] = stamp
				delete(a.stamps, path)
			}
			delete(a.stale, path)
			a.watchVersion++
			a.updateWatchTargets()
			a.loadDir(dir)
			a.refreshGit()
		})
	}()
}

// duplicateEntry copies a file or a directory next to itself.
func (a *app) duplicateEntry(path string) {
	if !a.requireProject() {
		return
	}
	backend := a.backend
	ctx := a.projectCtx
	root, generation := a.root, a.generation
	go func() {
		copied, err := backend.Duplicate(ctx, root, path)
		a.update(func() {
			if generation != a.generation {
				return
			}
			a.report(err)
			if err != nil {
				return
			}
			a.loadDir(filepath.Dir(path))
			a.refreshGit()
			if !strings.HasSuffix(copied, string(filepath.Separator)) {
				info, err := backend.Stat(ctx, copied)
				if err == nil && !info.Directory {
					a.openFile(copied)
				}
			}
		})
	}()
}

// deleteEntry removes a file or a directory, and closes the editors of what
// it removed.
func (a *app) deleteEntry(path string) {
	if !a.requireProject() {
		return
	}
	backend := a.backend
	ctx := a.projectCtx
	root, generation := a.root, a.generation
	go func() {
		err := backend.Remove(ctx, root, path)
		a.update(func() {
			if generation != a.generation {
				return
			}
			a.report(err)
			if err != nil {
				return
			}
			kept := a.tabs[:0]
			for _, f := range a.tabs {
				if f.path == path || strings.HasPrefix(f.path, path+string(filepath.Separator)) {
					delete(a.stamps, f.path)
					delete(a.stale, f.path)
					continue
				}
				kept = append(kept, f)
			}
			a.tabs = kept
			if a.selected >= len(a.tabs) {
				a.selected = len(a.tabs) - 1
			}
			a.watchVersion++
			a.updateWatchTargets()
			a.loadDir(filepath.Dir(path))
			a.refreshGit()
		})
	}()
}

// handleDrop opens what the user dropped on the window: a folder as the
// project, a file in the editor. What a remote connection holds cannot take
// a file of this machine.
func (a *app) handleDrop(paths []string) {
	if len(paths) == 0 {
		return
	}
	path := paths[0]
	if a.connection != nil {
		a.message = "已连接远程环境：拖入的是本机文件，无法在那里打开。请通过文件树或快速打开。"
		return
	}
	info, err := os.Stat(path)
	if err != nil {
		a.report(err)
		return
	}
	if info.IsDir() {
		a.openProject(path)
		return
	}
	a.openFile(path)
}

// renameActiveFile opens the dialog that renames the file that is open.
func (a *app) renameActiveFile() {
	f := a.active()
	if f == nil || f.diff != nil {
		return
	}
	a.renameTarget, a.renameValue = f.path, filepath.Base(f.path)
}

// revealInTree expands the folders of a file and scrolls the tree to it.
func (a *app) revealInTree(path string) {
	dir := filepath.Dir(path)
	for dir != "" && strings.HasPrefix(dir, a.root) {
		a.expanded[dir] = true
		a.loadDir(dir)
		if dir == a.root {
			break
		}
		dir = filepath.Dir(dir)
	}
}

// entryIsUnder tells whether a path is inside a folder of the project.
func entryIsUnder(path, folder string) bool {
	return strings.HasPrefix(path, folder+string(filepath.Separator))
}
