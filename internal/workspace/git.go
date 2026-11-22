package workspace

import (
	"context"
	"fmt"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

type Change struct{ Code, Path, Original string }

func (c Change) Staged() bool   { return c.Code != "??" && c.Code[0] != ' ' }
func (c Change) Unstaged() bool { return c.Code == "??" || c.Code[1] != ' ' }

type Repository struct {
	Root, Branch string
	Changes      []Change
}

// Clone clones url into the folder name of parent, and returns the path of
// the clone. Cloning may take a while, so its timeout is longer than the
// other Git commands'.
func Clone(ctx context.Context, parent, url, name string) (string, error) {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Minute)
	defer cancel()
	cmd := exec.CommandContext(ctx, "git", "-C", parent, "clone", "--", url, name)
	data, err := cmd.CombinedOutput()
	if err != nil {
		if ctx.Err() != nil {
			return "", ctx.Err()
		}
		return "", fmt.Errorf("git: %s", strings.TrimSpace(string(data)))
	}
	return filepath.Join(parent, name), nil
}

func Git(dir string, args ...string) (string, error) {
	return GitContext(context.Background(), dir, args...)
}
func GitContext(parent context.Context, dir string, args ...string) (string, error) {
	ctx, cancel := context.WithTimeout(parent, 15*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, "git", append([]string{"-C", dir}, args...)...)
	data, err := cmd.Output()
	if err != nil {
		if e, ok := err.(*exec.ExitError); ok {
			return "", fmt.Errorf("git: %s", strings.TrimSpace(string(e.Stderr)))
		}
		return "", err
	}
	return string(data), nil
}

func Status(dir string) (*Repository, error) {
	return StatusContext(context.Background(), dir)
}
func StatusContext(ctx context.Context, dir string) (*Repository, error) {
	root, err := GitContext(ctx, dir, "rev-parse", "--show-toplevel")
	if err != nil {
		return nil, err
	}
	root = strings.TrimSpace(root)
	branch, err := GitContext(ctx, root, "symbolic-ref", "--short", "-q", "HEAD")
	if err != nil {
		branch, _ = GitContext(ctx, root, "rev-parse", "--short", "HEAD")
	}
	raw, err := GitContext(ctx, root, "status", "--porcelain=v1", "-z", "--untracked-files=all")
	if err != nil {
		return nil, err
	}
	changes, err := ParseStatus(raw)
	if err != nil {
		return nil, err
	}
	return &Repository{root, strings.TrimSpace(branch), changes}, nil
}

func ParseStatus(raw string) ([]Change, error) {
	var result []Change
	records := strings.Split(raw, "\x00")
	for i := 0; i < len(records); i++ {
		row := records[i]
		if row == "" {
			continue
		}
		if len(row) < 4 || row[2] != ' ' {
			return nil, fmt.Errorf("无效的 Git 状态记录")
		}
		c := Change{Code: row[:2], Path: row[3:]}
		if strings.ContainsAny(c.Code, "RC") {
			i++
			if i >= len(records) || records[i] == "" {
				return nil, fmt.Errorf("缺少重命名源路径")
			}
			c.Original = records[i]
		}
		result = append(result, c)
	}
	return result, nil
}

func Stage(root string, c Change) error {
	return StageContext(context.Background(), root, c)
}
func StageContext(ctx context.Context, root string, c Change) error {
	args := []string{"add", "--", c.Path}
	if c.Original != "" {
		args = append(args, c.Original)
	}
	_, err := GitContext(ctx, root, args...)
	return err
}

func Unstage(root string, c Change) error {
	return UnstageContext(context.Background(), root, c)
}
func UnstageContext(ctx context.Context, root string, c Change) error {
	args := []string{"reset", "-q", "HEAD", "--", c.Path}
	if _, err := GitContext(ctx, root, "rev-parse", "--verify", "HEAD"); err != nil {
		args = []string{"rm", "--cached", "--", c.Path}
	}
	if c.Original != "" {
		args = append(args, c.Original)
	}
	_, err := GitContext(ctx, root, args...)
	return err
}
