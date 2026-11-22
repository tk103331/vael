package workspace

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestSymbolsInFileForSeveralLanguages(t *testing.T) {
	goSource := `package main

// Server serves.
type Server struct {
	port int
}

func (s *Server) Start() error {
	return nil
}

func NewServer(port int) *Server {
	return &Server{port: port}
}

const DefaultPort = 8080
`
	symbols := SymbolsInFile("main.go", goSource)
	names := map[string]string{}
	for _, s := range symbols {
		names[s.Name] = s.Kind
	}
	if names["Server"] != "type" || names["Start"] != "func" || names["NewServer"] != "func" || names["DefaultPort"] != "const" {
		t.Fatalf("%+v", names)
	}
	for _, s := range symbols {
		if s.Name == "NewServer" {
			if s.Line != 12 {
				t.Fatalf("line: %d", s.Line)
			}
			if goSource[s.Offset:s.Offset+3] != "fun" {
				t.Fatalf("offset: %d", s.Offset)
			}
		}
	}
	js := `export class Widget {
  render() {}
}
export function build(name) {
  return name;
}
const helper = () => 1;
interface Shape { }
`
	names = map[string]string{}
	for _, s := range SymbolsInFile("app.ts", js) {
		names[s.Name] = s.Kind
	}
	if names["Widget"] != "class" || names["build"] != "func" || names["helper"] != "func" || names["Shape"] != "type" {
		t.Fatalf("%+v", names)
	}
	python := `class Model:
    pass

def train(model):
    return model
`
	names = map[string]string{}
	for _, s := range SymbolsInFile("model.py", python) {
		names[s.Name] = s.Kind
	}
	if names["Model"] != "class" || names["train"] != "func" {
		t.Fatalf("%+v", names)
	}
	if got := SymbolsInFile("notes.txt", "hello world"); len(got) != 0 {
		t.Fatalf("plain text has symbols: %+v", got)
	}
}

func TestProjectSymbolsReadTheFilesOfTheProject(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "a.go"), []byte("package a\n\nfunc One() {}\n"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(root, "sub"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "sub", "b.py"), []byte("def two():\n    pass\n"), 0644); err != nil {
		t.Fatal(err)
	}
	symbols, err := ProjectSymbols(context.Background(), root, []string{"a.go", "sub/b.py", "missing.go"}, 100)
	if err != nil {
		t.Fatal(err)
	}
	if len(symbols) != 2 {
		t.Fatalf("%+v", symbols)
	}
	if symbols[0].Path != "a.go" || symbols[0].Name != "One" || symbols[1].Name != "two" {
		t.Fatalf("%+v", symbols)
	}
	limited, err := ProjectSymbols(context.Background(), root, []string{"a.go", "sub/b.py"}, 1)
	if err != nil || len(limited) != 1 {
		t.Fatalf("%+v %v", limited, err)
	}
}

func TestMatchSymbolsPrefersPrefixes(t *testing.T) {
	symbols := []Symbol{
		{Name: "handleRequest"},
		{Name: "builder"},
		{Name: "handle"},
		{Name: "other"},
	}
	matches := MatchSymbols(symbols, "handle", 10)
	if len(matches) != 2 {
		t.Fatalf("%+v", matches)
	}
	if matches[0].Name != "handle" {
		t.Fatalf("the prefix did not come first: %+v", matches)
	}
	if got := MatchSymbols(symbols, "nothing", 10); len(got) != 0 {
		t.Fatalf("%+v", got)
	}
	if got := MatchSymbols(symbols, "", 2); len(got) != 2 {
		t.Fatalf("%+v", got)
	}
}
