package workspace

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestReplaceAllRewritesOnlyTheFilesThatHoldTheQuery(t *testing.T) {
	root := t.TempDir()
	write := func(name, text string) {
		if err := os.WriteFile(filepath.Join(root, name), []byte(text), 0640); err != nil {
			t.Fatal(err)
		}
	}
	write("a.go", "func A() {}\nfunc a2() {}\n")
	write("b.md", "The Cat sat.\n")
	write("c.txt", "nothing here\n")

	result, err := ReplaceAll(context.Background(), root, []string{"a.go", "b.md", "c.txt"}, "func A", "func B", true)
	if err != nil {
		t.Fatal(err)
	}
	if result.Files != 1 || result.Occurrences != 1 {
		t.Fatalf("%+v", result)
	}
	a, _ := os.ReadFile(filepath.Join(root, "a.go"))
	if string(a) != "func B() {}\nfunc a2() {}\n" {
		t.Fatalf("%q", a)
	}
	info, _ := os.Stat(filepath.Join(root, "a.go"))
	if info.Mode().Perm() != 0640 {
		t.Fatal("the file's permissions were lost")
	}
	b, _ := os.ReadFile(filepath.Join(root, "b.md"))
	if string(b) != "The Cat sat.\n" {
		t.Fatalf("a file without the query was rewritten: %q", b)
	}
}

func TestReplaceAllIgnoresCaseWhenAsked(t *testing.T) {
	root := t.TempDir()
	text := "İstanbul İstanbul\nsmall istanbul\n"
	if err := os.WriteFile(filepath.Join(root, "a.txt"), []byte(text), 0644); err != nil {
		t.Fatal(err)
	}
	result, err := ReplaceAll(context.Background(), root, []string{"a.txt"}, "istanbul", "X", false)
	if err != nil {
		t.Fatal(err)
	}
	if result.Occurrences != 3 {
		t.Fatalf("%+v", result)
	}
	got, _ := os.ReadFile(filepath.Join(root, "a.txt"))
	// The replacement keeps the text around each match as it was.
	if string(got) != "X X\nsmall X\n" {
		t.Fatalf("%q", got)
	}
}

func TestReplaceAllOverlapsAndEmptyQuery(t *testing.T) {
	// "aa" in "aaa" matches twice, not overlapping.
	if got, n := replaceAll("aaa", "aa", "b", true); got != "ba" || n != 1 {
		t.Fatalf("%q %d", got, n)
	}
	if _, err := ReplaceAll(context.Background(), t.TempDir(), nil, "  ", "x", true); err == nil {
		t.Fatal("an empty query was accepted")
	}
	if got, n := replaceAll("abc", "zzz", "x", true); got != "abc" || n != 0 {
		t.Fatalf("%q %d", got, n)
	}
}
