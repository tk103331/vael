package workspace

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"unicode/utf8"
)

const MaxFileSize = 8 << 20

type Entry struct {
	Name, Path string
	Dir        bool
	Symlink    bool
}

func List(dir string) ([]Entry, error) {
	items, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}
	result := make([]Entry, 0, len(items))
	for _, item := range items {
		if item.Name() == ".git" {
			continue
		}
		// Do not recurse through symbolic links, which may lead outside the project.
		result = append(result, Entry{item.Name(), filepath.Join(dir, item.Name()), item.IsDir(), item.Type()&os.ModeSymlink != 0})
	}
	sort.Slice(result, func(i, j int) bool {
		if result[i].Dir != result[j].Dir {
			return result[i].Dir
		}
		return result[i].Name < result[j].Name
	})
	return result, nil
}

// ReadWithStamp reads a file and says what its stamp on disk is, so that a
// caller can tell later whether the file changed.
func ReadWithStamp(path string) (string, Stamp, error) {
	text, err := Read(path)
	if err != nil {
		return "", Stamp{}, err
	}
	stamp, err := StampOf(path)
	if err != nil {
		return text, Stamp{}, err
	}
	return text, stamp, nil
}

func Read(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()
	info, err := f.Stat()
	if err != nil {
		return "", err
	}
	if !info.Mode().IsRegular() {
		return "", errors.New("只能打开普通文本文件")
	}
	data, err := io.ReadAll(io.LimitReader(f, MaxFileSize+1))
	if err != nil {
		return "", err
	}
	if len(data) > MaxFileSize {
		return "", errors.New("文件超过 8 MiB，暂不支持编辑")
	}
	if bytes.IndexByte(data, 0) >= 0 || !utf8.Valid(data) {
		return "", errors.New("该文件不是 UTF-8 文本")
	}
	return string(data), nil
}

// Save checks the on-disk version before writing, preserving permissions and
// avoiding silent replacement of edits made by another program.
func Save(path, original, text string) error {
	current, err := Read(path)
	if err != nil {
		return err
	}
	if current != original {
		return errors.New("文件已被其他程序修改，请重新打开并合并修改后保存")
	}
	info, err := os.Stat(path)
	if err != nil {
		return err
	}
	resolved, err := filepath.EvalSymlinks(path)
	if err != nil {
		return err
	}
	f, err := os.CreateTemp(filepath.Dir(resolved), ".vael-save-*")
	if err != nil {
		return err
	}
	defer os.Remove(f.Name())
	if err = f.Chmod(info.Mode().Perm()); err == nil {
		_, err = f.WriteString(text)
	}
	if err == nil {
		err = f.Sync()
	}
	closeErr := f.Close()
	if err == nil {
		err = closeErr
	}
	if err != nil {
		return err
	}
	return os.Rename(f.Name(), resolved)
}

func Create(root, name string, directory bool) error {
	if name == "" || filepath.IsAbs(name) || filepath.Base(name) != name || name == "." || name == ".." {
		return fmt.Errorf("请输入不含路径分隔符的名称")
	}
	path := filepath.Join(root, name)
	if directory {
		return os.Mkdir(path, 0755)
	}
	f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0644)
	if err != nil {
		return err
	}
	return f.Close()
}
