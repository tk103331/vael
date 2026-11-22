package workspace

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// Stamp is what a file's identity on disk is at a moment: when it changed
// and how large it is. It is what the watcher compares.
type Stamp struct {
	Mod  int64
	Size int64
}

func StampOf(path string) (Stamp, error) {
	info, err := os.Stat(path)
	if err != nil {
		return Stamp{}, err
	}
	return Stamp{Mod: info.ModTime().UnixNano(), Size: info.Size()}, nil
}

type DiskChange struct {
	Path          string
	Before, After Stamp
	Deleted       bool
}

// Watch waits until one of the paths changes on disk, and reports how it
// changed. It returns when the context ends, so a caller watches again.
func Watch(ctx context.Context, paths []string, before map[string]Stamp, interval time.Duration) ([]DiskChange, error) {
	if interval <= 0 {
		interval = 300 * time.Millisecond
	}
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		changes := []DiskChange{}
		for _, path := range paths {
			after, err := StampOf(path)
			if err != nil {
				if os.IsNotExist(err) {
					changes = append(changes, DiskChange{Path: path, Before: before[path], Deleted: true})
				}
				continue
			}
			old, known := before[path]
			if !known {
				// The disk state of this path is not known yet: learning it
				// is not a change.
				continue
			}
			if old != after {
				changes = append(changes, DiskChange{Path: path, Before: old, After: after})
			}
		}
		if len(changes) > 0 {
			return changes, nil
		}
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-ticker.C:
		}
	}
}

// Rename renames an entry of the project, keeping it in its directory.
func Rename(root, from, to string) error {
	name := strings.TrimSpace(to)
	if name == "" || name == "." || name == ".." || strings.ContainsAny(name, "/\x00") {
		return errors.New("请输入不含路径分隔符的名称")
	}
	if from == "" || !strings.HasPrefix(from, root) {
		return errors.New("只能重命名项目内的文件")
	}
	dir, _ := filepath.Split(from)
	dir = filepath.Clean(dir)
	target := filepath.Join(dir, name)
	if target == from {
		return nil
	}
	if _, err := os.Lstat(target); err == nil {
		return fmt.Errorf("%s 已存在", name)
	}
	return os.Rename(from, target)
}

// Remove deletes a file, or a directory with everything in it.
func Remove(root, path string) error {
	if path == "" || path == root || !strings.HasPrefix(path, root) {
		return errors.New("只能删除项目内的文件")
	}
	info, err := os.Lstat(path)
	if err != nil {
		return err
	}
	if info.IsDir() {
		return os.RemoveAll(path)
	}
	return os.Remove(path)
}

// Duplicate copies an entry next to itself, with a name that does not exist.
func Duplicate(root, path string) (string, error) {
	if path == "" || !strings.HasPrefix(path, root) {
		return "", errors.New("只能复制项目内的文件")
	}
	info, err := os.Lstat(path)
	if err != nil {
		return "", err
	}
	dir, base := filepath.Split(path)
	ext := filepath.Ext(base)
	stem := strings.TrimSuffix(base, ext)
	candidate := filepath.Join(dir, stem+" copy"+ext)
	for i := 2; ; i++ {
		if _, err := os.Lstat(candidate); err != nil {
			break
		}
		candidate = filepath.Join(dir, fmt.Sprintf("%s copy %d%s", stem, i, ext))
	}
	if info.IsDir() {
		return candidate, copyDir(path, candidate)
	}
	return candidate, copyFile(path, candidate, info.Mode().Perm())
}

func copyFile(source, target string, mode os.FileMode) error {
	in, err := os.Open(source)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.OpenFile(target, os.O_WRONLY|os.O_CREATE|os.O_EXCL, mode)
	if err != nil {
		return err
	}
	_, err = io.Copy(out, in)
	if err == nil {
		err = out.Sync()
	}
	closeErr := out.Close()
	if err == nil {
		err = closeErr
	}
	return err
}

func copyDir(source, target string) error {
	return filepath.Walk(source, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(source, path)
		if err != nil {
			return err
		}
		destination := filepath.Join(target, rel)
		if info.IsDir() {
			return os.MkdirAll(destination, info.Mode().Perm())
		}
		if info.Mode()&os.ModeSymlink != 0 {
			return nil
		}
		return copyFile(path, destination, info.Mode().Perm())
	})
}
