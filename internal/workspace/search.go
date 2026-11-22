package workspace

import (
	"context"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"unicode"
)

const MaxIndexedFiles = 100000

type FileIndex struct {
	Files   []string
	Limited bool
}

// Index honors Git ignore rules when available. The fallback skips common
// generated directories and never follows symlinks.
func Index(ctx context.Context, root string) (FileIndex, error) {
	cmd := exec.CommandContext(ctx, "git", "-C", root, "ls-files", "-z", "--cached", "--others", "--exclude-standard", "--", ".")
	if data, err := cmd.Output(); err == nil {
		seen := map[string]bool{}
		result := FileIndex{}
		for _, path := range strings.Split(string(data), "\x00") {
			if err := ctx.Err(); err != nil {
				return FileIndex{}, err
			}
			if path == "" || seen[path] || !filepath.IsLocal(path) {
				continue
			}
			seen[path] = true
			info, err := os.Lstat(filepath.Join(root, path))
			if err != nil || !info.Mode().IsRegular() {
				continue
			}
			if len(result.Files) == MaxIndexedFiles {
				result.Limited = true
				break
			}
			result.Files = append(result.Files, path)
		}
		sort.Strings(result.Files)
		return result, nil
	}
	result := FileIndex{}
	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if e := ctx.Err(); e != nil {
			return e
		}
		if err != nil {
			if path == root {
				return err
			}
			return nil
		}
		if d.IsDir() {
			if path != root {
				switch d.Name() {
				case ".git", "node_modules", "vendor", "target", "dist", ".mygo":
					return filepath.SkipDir
				}
			}
			return nil
		}
		if !d.Type().IsRegular() {
			return nil
		}
		if len(result.Files) == MaxIndexedFiles {
			result.Limited = true
			return fs.SkipAll
		}
		rel, err := filepath.Rel(root, path)
		if err == nil {
			result.Files = append(result.Files, rel)
		}
		return err
	})
	sort.Strings(result.Files)
	return result, err
}

// MatchFiles ranks filename matches ahead of path subsequences.
func MatchFiles(files []string, query string, limit int) []string {
	type match struct {
		path  string
		score int
	}
	query = strings.ToLower(strings.TrimSpace(query))
	var matches []match
	for _, path := range files {
		lower := strings.ToLower(path)
		score := 0
		if query != "" {
			if pos := strings.Index(strings.ToLower(filepath.Base(path)), query); pos >= 0 {
				score = 10000 - pos
			} else if pos := strings.Index(lower, query); pos >= 0 {
				score = 5000 - pos
			} else {
				needle := []rune(query)
				next, last, gaps := 0, -1, 0
				for pos, r := range []rune(lower) {
					if next < len(needle) && r == needle[next] {
						if last >= 0 {
							gaps += pos - last - 1
						}
						last = pos
						next++
					}
				}
				if next != len(needle) {
					continue
				}
				score = 1000 - gaps
			}
		}
		matches = append(matches, match{path, score})
	}
	sort.Slice(matches, func(i, j int) bool {
		if matches[i].score != matches[j].score {
			return matches[i].score > matches[j].score
		}
		return matches[i].path < matches[j].path
	})
	if limit <= 0 {
		return nil
	}
	if len(matches) > limit {
		matches = matches[:limit]
	}
	result := make([]string, len(matches))
	for i, m := range matches {
		result[i] = m.path
	}
	return result
}

type SearchMatch struct {
	Path, Text       string
	Line, Start, End int
}
type SearchResults struct {
	Matches []SearchMatch
	Limited bool
	Skipped int
}

// Search scans the disk version of UTF-8 text files. Offsets refer to the
// original text even for case-insensitive Unicode matches.
func Search(ctx context.Context, root string, files []string, query string, matchCase bool) (SearchResults, error) {
	result := SearchResults{}
	if query == "" {
		return result, nil
	}
	const budget = 64 << 20
	total := int64(0)
	needle := []rune(query)
	if !matchCase {
		needle = []rune(strings.Map(unicode.ToLower, query))
	}
	for _, path := range files {
		if err := ctx.Err(); err != nil {
			return result, err
		}
		if !filepath.IsLocal(path) {
			continue
		}
		full := filepath.Join(root, path)
		info, err := os.Lstat(full)
		if err != nil || !info.Mode().IsRegular() {
			result.Skipped++
			continue
		}
		if info.Size() > MaxFileSize {
			result.Skipped++
			continue
		}
		if total+info.Size() > budget {
			result.Limited = true
			break
		}
		total += info.Size()
		text, err := Read(full)
		if err != nil {
			result.Skipped++
			continue
		}
		offset := 0
		for lineNumber, line := range strings.SplitAfter(text, "\n") {
			if err := ctx.Err(); err != nil {
				return result, err
			}
			runes := []rune(line)
			positions := make([]int, 0, len(runes)+1)
			for pos := range line {
				positions = append(positions, pos)
			}
			positions = append(positions, len(line))
			if !matchCase {
				for i, r := range runes {
					runes[i] = unicode.ToLower(r)
				}
			}
			for i := 0; i+len(needle) <= len(runes); i++ {
				found := true
				for j, r := range needle {
					if runes[i+j] != r {
						found = false
						break
					}
				}
				if !found {
					continue
				}
				if len(result.Matches) == 500 {
					result.Limited = true
					return result, nil
				}
				snippet := strings.TrimSpace(line)
				if len([]rune(snippet)) > 200 {
					snippet = string([]rune(snippet)[:200]) + "…"
				}
				result.Matches = append(result.Matches, SearchMatch{path, snippet, lineNumber + 1, offset + positions[i], offset + positions[i+len(needle)]})
				i += len(needle) - 1
			}
			offset += len(line)
		}
	}
	return result, nil
}
