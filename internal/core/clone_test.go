package core

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

func TestCloneCopiesARepository(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git is not available")
	}
	dir := t.TempDir()
	source := filepath.Join(dir, "source")
	if err := os.Mkdir(source, 0o755); err != nil {
		t.Fatal(err)
	}
	for _, args := range [][]string{
		{"init"},
		{"config", "user.name", "Vael Test"},
		{"config", "user.email", "test@example.invalid"},
	} {
		cmd := exec.Command("git", append([]string{"-C", source}, args...)...)
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("%v: %s", args, out)
		}
	}
	if err := os.WriteFile(filepath.Join(source, "a.txt"), []byte("one"), 0o644); err != nil {
		t.Fatal(err)
	}
	if out, err := exec.Command("git", "-C", source, "add", "a.txt").CombinedOutput(); err != nil {
		t.Fatalf("%s", out)
	}
	if out, err := exec.Command("git", "-C", source, "commit", "-m", "first").CombinedOutput(); err != nil {
		t.Fatalf("%s", out)
	}
	parent := filepath.Join(dir, "parent")
	if err := os.Mkdir(parent, 0o755); err != nil {
		t.Fatal(err)
	}

	c := NewLocal()
	defer c.Close()
	path, err := c.Clone(context.Background(), parent, source, "copy")
	if err != nil {
		t.Fatal(err)
	}
	if path != filepath.Join(parent, "copy") {
		t.Fatalf("path %q", path)
	}
	if data, err := os.ReadFile(filepath.Join(path, "a.txt")); err != nil || string(data) != "one" {
		t.Fatalf("%q %v", data, err)
	}
	// A repository that is not there is an error, not a silent success.
	if _, err := c.Clone(context.Background(), parent, filepath.Join(dir, "missing"), "nope"); err == nil {
		t.Fatal("cloning a missing repository succeeded")
	}
}
