package workspace

import (
	"context"
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func TestIndexRespectsGitIgnoreAndNestedProject(t *testing.T) {
	root := t.TempDir()
	if _, err := Git(root, "init"); err != nil {
		t.Skip(err)
	}
	for path, text := range map[string]string{".gitignore": "ignored/\n*.log\n", "src/main.go": "package main\n", "src/out.log": "ignored", "ignored/secret": "hidden", "outside.go": "outside"} {
		full := filepath.Join(root, path)
		if err := os.MkdirAll(filepath.Dir(full), 0755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(full, []byte(text), 0644); err != nil {
			t.Fatal(err)
		}
	}
	index, err := Index(context.Background(), filepath.Join(root, "src"))
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(index.Files, []string{"main.go"}) {
		t.Fatalf("nested index: %v", index.Files)
	}
}
func TestIndexFallbackSkipsGeneratedAndSymlinkedFiles(t *testing.T) {
	root := t.TempDir()
	for _, path := range []string{"src/a.go", "node_modules/pkg.js", ".git/config", "dist/app.js"} {
		full := filepath.Join(root, path)
		_ = os.MkdirAll(filepath.Dir(full), 0755)
		_ = os.WriteFile(full, []byte("text"), 0644)
	}
	_ = os.Symlink(filepath.Join(root, "src", "a.go"), filepath.Join(root, "link.go"))
	index, err := Index(context.Background(), root)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(index.Files, []string{"src/a.go"}) {
		t.Fatalf("fallback index: %v", index.Files)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := Index(ctx, root); err == nil {
		t.Fatal("index ignored cancellation")
	}
}
func TestFuzzyFiles(t *testing.T) {
	files := []string{"docs/main.md", "src/model.go", "main.go", "src/应用.go"}
	matches := MatchFiles(files, "main", 2)
	if !reflect.DeepEqual(matches, []string{"docs/main.md", "main.go"}) {
		t.Fatalf("%v", matches)
	}
	if got := MatchFiles(files, "smdl", 10); !reflect.DeepEqual(got, []string{"src/model.go"}) {
		t.Fatal(got)
	}
	if got := MatchFiles(files, "应用", 10); !reflect.DeepEqual(got, []string{"src/应用.go"}) {
		t.Fatal(got)
	}
}
func TestSearchUnicodePositionsAndBinary(t *testing.T) {
	root := t.TempDir()
	text := "你好 İSTANBUL\r\n第二行 istanbul\n"
	_ = os.WriteFile(filepath.Join(root, "a.txt"), []byte(text), 0644)
	_ = os.WriteFile(filepath.Join(root, "binary"), []byte{'a', 0}, 0644)
	result, err := Search(context.Background(), root, []string{"a.txt", "binary"}, "istanbul", false)
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Matches) != 2 || result.Skipped != 1 {
		t.Fatalf("%+v", result)
	}
	for i, match := range result.Matches {
		expected := []string{"İSTANBUL", "istanbul"}[i]
		if text[match.Start:match.End] != expected || match.Line != i+1 {
			t.Fatalf("%+v", match)
		}
	}
	exact, err := Search(context.Background(), root, []string{"a.txt"}, "istanbul", true)
	if err != nil || len(exact.Matches) != 1 || exact.Matches[0].Line != 2 {
		t.Fatalf("%+v %v", exact, err)
	}
}
func TestSearchLimitAndCancel(t *testing.T) {
	root := t.TempDir()
	text := ""
	for range 501 {
		text += "match\n"
	}
	_ = os.WriteFile(filepath.Join(root, "a.txt"), []byte(text), 0644)
	result, err := Search(context.Background(), root, []string{"a.txt"}, "match", true)
	if err != nil || !result.Limited || len(result.Matches) != 500 {
		t.Fatalf("len %d limited %v err %v", len(result.Matches), result.Limited, err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := Search(ctx, root, []string{"a.txt"}, "match", true); err == nil {
		t.Fatal("search ignored cancellation")
	}
}
func TestSessionRoundTrip(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config", "session.json")
	s := Session{Root: "/项目", Files: []OpenFile{{"/项目/a.go", 12}}, Active: "/项目/a.go", SideWidth: 300, EditorShare: 65, Sidebar: true, Dark: true, Section: "search"}
	if err := SaveSession(path, s); err != nil {
		t.Fatal(err)
	}
	actual, err := LoadSession(path)
	if err != nil || !reflect.DeepEqual(actual, s) {
		t.Fatalf("%+v %v", actual, err)
	}
	_ = os.WriteFile(path, []byte("invalid json"), 0600)
	if _, err := LoadSession(path); err == nil {
		t.Fatal("accepted invalid session")
	}
}
