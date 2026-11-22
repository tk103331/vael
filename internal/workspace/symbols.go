package workspace

import (
	"context"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
)

// Symbol is a declaration a file holds: a function, a type, a class.
type Symbol struct {
	Path   string
	Name   string
	Kind   string
	Line   int // 1-based
	Column int // 1-based
	Offset int // bytes of the line's start
	Detail string
}

type symbolPattern struct {
	kind    string
	pattern *regexp.Regexp
}

// symbolPatterns are the declarations of a language, as they are written.
// The kinds are those of an outline: func, type, class, const, var, method.
var symbolPatterns = map[string][]symbolPattern{
	".go": {
		{"func", regexp.MustCompile(`^func\s+(?:\([^)]*\)\s*)?([A-Za-z_]\w*)`)},
		{"type", regexp.MustCompile(`^type\s+([A-Za-z_]\w*)`)},
		{"const", regexp.MustCompile(`^const\s+([A-Za-z_]\w*)`)},
		{"var", regexp.MustCompile(`^var\s+([A-Za-z_]\w*)`)},
	},
	".js":  jsPatterns(),
	".jsx": jsPatterns(),
	".mjs": jsPatterns(),
	".ts":  jsPatterns(),
	".tsx": jsPatterns(),
	".py": {
		{"func", regexp.MustCompile(`^\s*(?:async\s+)?def\s+([A-Za-z_]\w*)`)},
		{"class", regexp.MustCompile(`^\s*class\s+([A-Za-z_]\w*)`)},
	},
	".rs": {
		{"func", regexp.MustCompile(`^\s*(?:pub(?:\([^)]*\))?\s+)?(?:async\s+)?fn\s+([A-Za-z_]\w*)`)},
		{"type", regexp.MustCompile(`^\s*(?:pub(?:\([^)]*\))?\s+)?(?:struct|enum|trait|impl)\s+([A-Za-z_]\w*)`)},
		{"const", regexp.MustCompile(`^\s*(?:pub(?:\([^)]*\))?\s+)?(?:const|static)\s+([A-Za-z_]\w*)`)},
	},
	".java": {
		{"type", regexp.MustCompile(`^\s*(?:public\s+|private\s+|protected\s+|static\s+|final\s+|abstract\s+)*(?:class|interface|enum|record)\s+([A-Za-z_]\w*)`)},
		{"func", regexp.MustCompile(`^\s*(?:public\s+|private\s+|protected\s+|static\s+|final\s+|abstract\s+|synchronized\s+)*(?:[\w<>\[\],.]+\s+)?([A-Za-z_]\w*)\s*\([^;]*\)\s*\{`)},
	},
	".c": {
		{"func", regexp.MustCompile(`^[A-Za-z_][\w \t*]*\b([A-Za-z_]\w*)\s*\([^;]*\)\s*\{`)},
		{"type", regexp.MustCompile(`^\s*typedef\s+.*\b([A-Za-z_]\w*)\s*;`)},
	},
	".h": {
		{"func", regexp.MustCompile(`^[A-Za-z_][\w \t*]*\b([A-Za-z_]\w*)\s*\([^;]*\)\s*;`)},
	},
	".cpp": {
		{"func", regexp.MustCompile(`^[A-Za-z_][\w \t*:<>,]*\b([A-Za-z_]\w*)\s*\([^;]*\)\s*(?:const\s*)?\{`)},
		{"type", regexp.MustCompile(`^\s*(?:class|struct)\s+([A-Za-z_]\w*)`)},
	},
	".rb": {
		{"func", regexp.MustCompile(`^\s*def\s+([A-Za-z_]\w*[!?=]?)`)},
		{"class", regexp.MustCompile(`^\s*class\s+([A-Za-z_]\w*)`)},
	},
	".sh": {
		{"func", regexp.MustCompile(`^\s*(?:function\s+)?([A-Za-z_]\w*)\s*\(\s*\)\s*\{`)},
	},
	".lua": {
		{"func", regexp.MustCompile(`^\s*(?:local\s+)?function\s+([A-Za-z_][\w.:]*)`)},
	},
}

func jsPatterns() []symbolPattern {
	return []symbolPattern{
		{"func", regexp.MustCompile(`^\s*(?:export\s+)?(?:default\s+)?(?:async\s+)?function\s*\*?\s*([A-Za-z_$][\w$]*)`)},
		{"class", regexp.MustCompile(`^\s*(?:export\s+)?(?:default\s+)?(?:abstract\s+)?class\s+([A-Za-z_$][\w$]*)`)},
		{"type", regexp.MustCompile(`^\s*(?:export\s+)?(?:interface|type|enum)\s+([A-Za-z_$][\w$]*)`)},
		{"func", regexp.MustCompile(`^\s*(?:export\s+)?(?:const|let|var)\s+([A-Za-z_$][\w$]*)\s*(?::[^=]+)?=\s*(?:async\s*)?(?:\([^)]*\)\s*=>|function\b)`)},
		{"func", regexp.MustCompile(`^\s*(?:public\s+|private\s+|protected\s+|readonly\s+|static\s+|abstract\s+|async\s+)*([A-Za-z_$][\w$]*)\s*\([^)]*\)\s*\{`)},
	}
}

// SymbolsInFile are the declarations a file's text holds. It reads no file:
// the text is what the editor shows, so that an outline follows the edits.
func SymbolsInFile(path, text string) []Symbol {
	patterns, ok := symbolPatterns[strings.ToLower(filepath.Ext(path))]
	if !ok {
		return nil
	}
	symbols := []Symbol{}
	offset := 0
	for number, line := range strings.SplitAfter(text, "\n") {
		trimmed := strings.TrimRight(line, "\r\n")
		for _, p := range patterns {
			match := p.pattern.FindStringSubmatch(trimmed)
			if match == nil {
				continue
			}
			name := match[1]
			if name == "" {
				continue
			}
			symbols = append(symbols, Symbol{
				Path:   path,
				Name:   name,
				Kind:   p.kind,
				Line:   number + 1,
				Column: indexOf(trimmed, name) + 1,
				Offset: offset,
			})
			break
		}
		offset += len(line)
	}
	return symbols
}

func indexOf(text, name string) int {
	if i := strings.Index(text, name); i >= 0 {
		return i
	}
	return 0
}

// ProjectSymbols are the declarations of the files of a project. It reads
// the files that are text and skips the rest.
func ProjectSymbols(ctx context.Context, root string, files []string, limit int) ([]Symbol, error) {
	if limit <= 0 {
		limit = 2000
	}
	symbols := []Symbol{}
	for _, path := range files {
		if err := ctx.Err(); err != nil {
			return symbols, err
		}
		if _, ok := symbolPatterns[strings.ToLower(filepath.Ext(path))]; !ok {
			continue
		}
		full := filepath.Join(root, path)
		info, err := os.Lstat(full)
		if err != nil || !info.Mode().IsRegular() || info.Size() > MaxFileSize {
			continue
		}
		text, err := Read(full)
		if err != nil {
			continue
		}
		symbols = append(symbols, SymbolsInFile(path, text)...)
		if len(symbols) >= limit {
			symbols = symbols[:limit]
			break
		}
	}
	sort.SliceStable(symbols, func(i, j int) bool {
		if symbols[i].Path != symbols[j].Path {
			return symbols[i].Path < symbols[j].Path
		}
		return symbols[i].Line < symbols[j].Line
	})
	return symbols, nil
}

// MatchSymbols keeps the symbols whose name holds the query: the ones that
// are it first, then those that start with it, then those that hold it.
func MatchSymbols(symbols []Symbol, query string, limit int) []Symbol {
	query = strings.ToLower(strings.TrimSpace(query))
	type ranked struct {
		symbol Symbol
		rank   int
	}
	ranked2 := []ranked{}
	for _, s := range symbols {
		name := strings.ToLower(s.Name)
		rank := 3
		switch {
		case query == "":
		case name == query:
			rank = 0
		case strings.HasPrefix(name, query):
			rank = 1
		case strings.Contains(name, query):
			rank = 2
		default:
			continue
		}
		ranked2 = append(ranked2, ranked{s, rank})
	}
	sort.SliceStable(ranked2, func(i, j int) bool { return ranked2[i].rank < ranked2[j].rank })
	matches := []Symbol{}
	for _, r := range ranked2 {
		matches = append(matches, r.symbol)
		if limit > 0 && len(matches) >= limit {
			break
		}
	}
	return matches
}
