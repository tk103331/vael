package workspace

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestWatchReportsChangesAndDeletions(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "a.txt")
	if err := os.WriteFile(path, []byte("one"), 0644); err != nil {
		t.Fatal(err)
	}
	stamp, err := StampOf(path)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	done := make(chan []DiskChange, 1)
	go func() {
		changes, err := Watch(ctx, []string{path}, map[string]Stamp{path: stamp}, 50*time.Millisecond)
		if err != nil {
			done <- nil
			return
		}
		done <- changes
	}()
	time.Sleep(200 * time.Millisecond)
	if err := os.WriteFile(path, []byte("two"), 0644); err != nil {
		t.Fatal(err)
	}
	select {
	case changes := <-done:
		if len(changes) != 1 || changes[0].Path != path || changes[0].Deleted {
			t.Fatalf("%+v", changes)
		}
		if changes[0].Before != stamp {
			t.Fatalf("before stamp: %+v", changes[0].Before)
		}
	case <-time.After(6 * time.Second):
		t.Fatal("watch did not report the change")
	}
	if err := os.Remove(path); err != nil {
		t.Fatal(err)
	}
	changes, err := Watch(ctx, []string{path}, map[string]Stamp{path: {Mod: stamp.Mod + 1, Size: 3}}, 50*time.Millisecond)
	if err != nil {
		t.Fatal(err)
	}
	if len(changes) != 1 || !changes[0].Deleted {
		t.Fatalf("%+v", changes)
	}
}

func TestWatchStopsWithItsContext(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := Watch(ctx, []string{"/tmp"}, nil, time.Millisecond); err == nil {
		t.Fatal("watch ignored its context")
	}
}

func TestRenameKeepsTheEntryInItsDirectory(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "old.go")
	if err := os.WriteFile(path, []byte("package main\n"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := Rename(root, path, "new.go"); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(root, "new.go")); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"", "  ", "..", "a/b", "sub/name"} {
		if err := Rename(root, filepath.Join(root, "new.go"), name); err == nil {
			t.Fatalf("accepted the name %q", name)
		}
	}
	if err := os.WriteFile(filepath.Join(root, "taken.go"), []byte("x"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := Rename(root, filepath.Join(root, "new.go"), "taken.go"); err == nil {
		t.Fatal("overwrote an existing file")
	}
	outside := filepath.Join(t.TempDir(), "outside.go")
	if err := os.WriteFile(outside, []byte("x"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := Rename(root, outside, "inside.go"); err == nil {
		t.Fatal("renamed a file outside the project")
	}
}

func TestRemoveDeletesFilesAndDirectories(t *testing.T) {
	root := t.TempDir()
	file := filepath.Join(root, "a.txt")
	if err := os.WriteFile(file, []byte("x"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := Remove(root, root); err == nil {
		t.Fatal("removed the project root")
	}
	if err := Remove(root, filepath.Join(t.TempDir(), "elsewhere")); err == nil {
		t.Fatal("removed a file outside the project")
	}
	if err := Remove(root, file); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(file); !os.IsNotExist(err) {
		t.Fatal("file still there")
	}
	dir := filepath.Join(root, "dir")
	if err := os.MkdirAll(filepath.Join(dir, "deep"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "deep", "f.txt"), []byte("x"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := Remove(root, dir); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(dir); !os.IsNotExist(err) {
		t.Fatal("directory still there")
	}
}

func TestDuplicateCopiesBesideTheOriginal(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "a.txt")
	if err := os.WriteFile(path, []byte("hello"), 0640); err != nil {
		t.Fatal(err)
	}
	copied, err := Duplicate(root, path)
	if err != nil {
		t.Fatal(err)
	}
	if filepath.Base(copied) != "a copy.txt" {
		t.Fatalf("copy name: %s", copied)
	}
	data, err := os.ReadFile(copied)
	if err != nil || string(data) != "hello" {
		t.Fatalf("%q %v", data, err)
	}
	info, err := os.Stat(copied)
	if err != nil || info.Mode().Perm() != 0640 {
		t.Fatalf("mode %v %v", info.Mode().Perm(), err)
	}
	again, err := Duplicate(root, path)
	if err != nil {
		t.Fatal(err)
	}
	if filepath.Base(again) != "a copy 2.txt" {
		t.Fatalf("second copy name: %s", again)
	}
	dir := filepath.Join(root, "d")
	if err := os.MkdirAll(filepath.Join(dir, "sub"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "sub", "f.txt"), []byte("x"), 0644); err != nil {
		t.Fatal(err)
	}
	dirCopy, err := Duplicate(root, dir)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(dirCopy, "sub", "f.txt")); err != nil {
		t.Fatal(err)
	}
}
