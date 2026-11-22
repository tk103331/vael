package main

import (
	"context"
	"strconv"
	"strings"

	"github.com/egoist/mygo/ui"
	"github.com/egoist/mygo/ui/editor"
	"vael/internal/workspace"
)

// loadBranches reads the branches of the repository, so that another one can
// be chosen.
func (a *app) loadBranches() {
	if a.repo == nil || a.branchesLoading {
		return
	}
	a.branchesLoading = true
	ctx, cancel := context.WithCancel(a.projectCtx)
	a.branchCancel = cancel
	generation, root := a.generation, a.repo.Root
	backend := a.backend
	go func() {
		output, err := backend.Git(ctx, root, "branch", "--format=%(refname:short)")
		remote, _ := backend.Git(ctx, root, "branch", "-r", "--format=%(refname:short)")
		upstream, _ := backend.Git(ctx, root, "rev-parse", "--abbrev-ref", "--symbolic-full-name", "@{upstream}")
		counts, _ := backend.Git(ctx, root, "rev-list", "--left-right", "--count", "HEAD...@{upstream}")
		a.update(func() {
			if generation != a.generation {
				return
			}
			a.branchesLoading = false
			a.branchCancel = nil
			if ctx.Err() != nil {
				return
			}
			a.report(err)
			if err != nil {
				return
			}
			a.branches = nil
			for _, line := range strings.Split(output, "\n") {
				line = strings.TrimSpace(line)
				if line != "" {
					a.branches = append(a.branches, line)
				}
			}
			a.remoteBranches = nil
			for _, line := range strings.Split(remote, "\n") {
				line = strings.TrimSpace(line)
				if line == "" || strings.HasSuffix(line, "/HEAD") {
					continue
				}
				a.remoteBranches = append(a.remoteBranches, line)
			}
			a.upstream = strings.TrimSpace(upstream)
			a.branchAhead, a.branchBehind = 0, 0
			if fields := strings.Fields(counts); len(fields) == 2 {
				a.branchAhead, _ = strconv.Atoi(fields[0])
				a.branchBehind, _ = strconv.Atoi(fields[1])
			}
		})
	}()
}

// loadCommits reads the commits of the repository, the newest first.
func (a *app) loadCommits() {
	if a.repo == nil || a.commitsLoading {
		return
	}
	a.commitsLoading = true
	ctx, cancel := context.WithCancel(a.projectCtx)
	a.commitCancel = cancel
	generation, root := a.generation, a.repo.Root
	backend := a.backend
	go func() {
		output, err := backend.Git(ctx, root, "log", "--oneline", "--no-decorate", "-20")
		a.update(func() {
			if generation != a.generation {
				return
			}
			a.commitsLoading = false
			a.commitCancel = nil
			if ctx.Err() != nil {
				return
			}
			a.report(err)
			if err != nil {
				return
			}
			a.commits = nil
			for _, line := range strings.Split(strings.TrimRight(output, "\n"), "\n") {
				if line != "" {
					a.commits = append(a.commits, line)
				}
			}
		})
	}()
}

// noteRecentBranch puts a branch first in the ones used lately.
func (a *app) noteRecentBranch(name string) {
	kept := make([]string, 0, len(a.recentBranches)+1)
	for _, b := range a.recentBranches {
		if b != name {
			kept = append(kept, b)
		}
	}
	a.recentBranches = append([]string{name}, kept...)
	if len(a.recentBranches) > 5 {
		a.recentBranches = a.recentBranches[:5]
	}
}

// switchBranch checks out another branch of the repository.
func (a *app) switchBranch(name string) {
	if a.repo == nil || strings.TrimSpace(name) == "" {
		return
	}
	name = strings.TrimSpace(name)
	root := a.repo.Root
	a.noteRecentBranch(name)
	a.gitAction(func() error {
		_, err := a.backend.Git(a.projectCtx, root, "checkout", name)
		return err
	})
}

// checkoutRemote checks out a branch of a remote, as a local branch that
// tracks it.
func (a *app) checkoutRemote(remote string) {
	if a.repo == nil || strings.TrimSpace(remote) == "" {
		return
	}
	remote = strings.TrimSpace(remote)
	local := remote
	if i := strings.Index(local, "/"); i >= 0 {
		local = local[i+1:]
	}
	root := a.repo.Root
	a.noteRecentBranch(local)
	a.gitAction(func() error {
		if _, err := a.backend.Git(a.projectCtx, root, "checkout", local); err == nil {
			return nil
		}
		_, err := a.backend.Git(a.projectCtx, root, "checkout", "--track", remote)
		return err
	})
}

// createBranch creates a branch and checks it out.
func (a *app) createBranch(name string) {
	name = strings.TrimSpace(name)
	if a.repo == nil {
		return
	}
	if name == "" {
		a.message = "请输入分支名称。"
		return
	}
	if strings.ContainsAny(name, " ~^:?*[\\") || strings.HasPrefix(name, "-") {
		a.message = "分支名称包含无效字符。"
		return
	}
	root := a.repo.Root
	a.noteRecentBranch(name)
	a.gitAction(func() error {
		_, err := a.backend.Git(a.projectCtx, root, "checkout", "-b", name)
		return err
	})
}

// createBranchFrom makes a branch at a commit, without checking it out.
func (a *app) createBranchFrom(name, hash string) {
	name = strings.TrimSpace(name)
	if a.repo == nil {
		return
	}
	if name == "" {
		a.message = "请输入分支名称。"
		return
	}
	if strings.ContainsAny(name, " ~^:?*[\\") || strings.HasPrefix(name, "-") {
		a.message = "分支名称包含无效字符。"
		return
	}
	root := a.repo.Root
	a.gitAction(func() error {
		_, err := a.backend.Git(a.projectCtx, root, "branch", name, hash)
		return err
	})
}

// checkoutRevision checks out a tag or a revision, as a detached HEAD.
func (a *app) checkoutRevision(revision string) {
	revision = strings.TrimSpace(revision)
	if a.repo == nil {
		return
	}
	if revision == "" {
		a.message = "请输入标签或修订。"
		return
	}
	root := a.repo.Root
	a.gitAction(func() error {
		_, err := a.backend.Git(a.projectCtx, root, "checkout", revision)
		return err
	})
}

// discardChange throws away the change of a file in the working tree.
func (a *app) discardChange(change workspace.Change) {
	if a.repo == nil {
		return
	}
	root := a.repo.Root
	a.gitAction(func() error {
		if change.Code == "??" {
			_, err := a.backend.Git(a.projectCtx, root, "clean", "-f", "--", change.Path)
			return err
		}
		_, err := a.backend.Git(a.projectCtx, root, "restore", "--worktree", "--", change.Path)
		if err == nil {
			return nil
		}
		// Git before 2.23 has no restore.
		_, err = a.backend.Git(a.projectCtx, root, "checkout", "--", change.Path)
		return err
	})
}

// treeColor is the color of a file of the tree by what Git says of it, and
// what the color says: modified in amber, new in green.
func (a *app) treeColor(path string, p palette) (ui.Color, string) {
	if a.repo == nil {
		return p.text, ""
	}
	rel, err := a.gitRel(a.repo.Root, path)
	if err != nil {
		return p.text, ""
	}
	for _, change := range a.repo.Changes {
		if change.Path != rel {
			continue
		}
		switch {
		case change.Code == "??":
			return p.success, "Git 未跟踪"
		case !change.Staged() && change.Unstaged():
			return p.warn, "Git 已修改"
		}
	}
	return p.text, ""
}

// refreshDiffBase tells an editor what the file's text is in Git, so that
// its gutter marks the lines that changed since the last commit.
func (a *app) refreshDiffBase(tab *fileTab) {
	if tab == nil || tab.state == nil || tab.diff != nil || a.repo == nil {
		return
	}
	rel, err := a.gitRel(a.repo.Root, tab.path)
	if err != nil {
		return
	}
	backend, ctx, generation, root := a.backend, a.projectCtx, a.generation, a.repo.Root
	go func() {
		head, err := backend.Git(ctx, root, "show", "HEAD:"+rel)
		a.update(func() {
			if generation != a.generation {
				return
			}
			// A file Git does not track has no base: the gutter marks
			// nothing.
			if err != nil || tab.state == nil || a.tabForPath(tab.path) != tab {
				return
			}
			tab.state.DiffBase = editor.NewBuffer(head)
		})
	}()
}

// refreshDiffBases tells the editors of the files that are open what Git
// holds, as after a commit changed it.
func (a *app) refreshDiffBases() {
	for _, tab := range a.tabs {
		a.refreshDiffBase(tab)
	}
}

// gitRemote runs a command that talks to the remote: fetch, pull or push.
func (a *app) gitRemote(args ...string) {
	if a.repo == nil {
		return
	}
	root := a.repo.Root
	a.gitAction(func() error {
		_, err := a.backend.Git(a.projectCtx, root, args...)
		return err
	})
}
