package workspace

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

func TestSaveProtectsExternalChangesAndMode(t *testing.T) {
	path := filepath.Join(t.TempDir(), "script.sh")
	if err := os.WriteFile(path, []byte("original\n"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := Save(path, "original\n", "edited\n"); err != nil {
		t.Fatal(err)
	}
	info, _ := os.Stat(path)
	if info.Mode().Perm() != 0755 {
		t.Fatal("executable mode lost")
	}
	if err := os.WriteFile(path, []byte("external\n"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := Save(path, "edited\n", "replacement\n"); err == nil {
		t.Fatal("external change overwritten")
	}
	actual, _ := os.ReadFile(path)
	if string(actual) != "external\n" {
		t.Fatal(string(actual))
	}
}
func TestCreateAndRead(t *testing.T) {
	dir := t.TempDir()
	for _, name := range []string{"../escape", "/absolute", "nested/file", ".", ""} {
		if Create(dir, name, false) == nil {
			t.Fatalf("accepted %q", name)
		}
	}
	if err := Create(dir, "file.go", false); err != nil {
		t.Fatal(err)
	}
	if Create(dir, "file.go", false) == nil {
		t.Fatal("overwrote existing file")
	}
	path := filepath.Join(dir, "binary")
	_ = os.WriteFile(path, []byte{'a', 0, 'b'}, 0644)
	if _, err := Read(path); err == nil {
		t.Fatal("accepted binary")
	}
}
func TestParseRenameAndUnusualNames(t *testing.T) {
	changes, err := ParseStatus("R  new name.go\x00old\nname.go\x00?? -option.go\x00 M line\nbreak.go\x00")
	if err != nil {
		t.Fatal(err)
	}
	if len(changes) != 3 || changes[0].Original != "old\nname.go" || !changes[0].Staged() || changes[0].Unstaged() || !changes[1].Unstaged() {
		t.Fatalf("%+v", changes)
	}
	if _, err := ParseStatus("R  missing-source\x00"); err == nil {
		t.Fatal("accepted malformed rename")
	}
}
func TestGitStageUnstageIncludingUnbornBranch(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git unavailable")
	}
	root := t.TempDir()
	for _, args := range [][]string{{"init"}, {"config", "user.name", "Vael Test"}, {"config", "user.email", "test@example.invalid"}} {
		if _, err := Git(root, args...); err != nil {
			t.Fatal(err)
		}
	}
	path := filepath.Join(root, "a file.go")
	_ = os.WriteFile(path, []byte("package main\n"), 0644)
	repo, err := Status(root)
	if err != nil || len(repo.Changes) != 1 {
		t.Fatalf("%+v %v", repo, err)
	}
	if err := Stage(root, repo.Changes[0]); err != nil {
		t.Fatal(err)
	}
	repo, _ = Status(root)
	if !repo.Changes[0].Staged() {
		t.Fatal("not staged")
	}
	if err := Unstage(root, repo.Changes[0]); err != nil {
		t.Fatal(err)
	}
	repo, _ = Status(root)
	if repo.Changes[0].Code != "??" {
		t.Fatal("not unstaged")
	}
	if err := Stage(root, repo.Changes[0]); err != nil {
		t.Fatal(err)
	}
	if _, err := Git(root, "commit", "-m", "initial"); err != nil {
		t.Fatal(err)
	}
	_ = os.WriteFile(path, []byte("package changed\n"), 0644)
	repo, _ = Status(root)
	if err := Stage(root, repo.Changes[0]); err != nil {
		t.Fatal(err)
	}
	repo, _ = Status(root)
	if err := Unstage(root, repo.Changes[0]); err != nil {
		t.Fatal(err)
	}
	repo, _ = Status(root)
	if repo.Changes[0].Code != " M" {
		t.Fatalf("%+v", repo.Changes)
	}
}
