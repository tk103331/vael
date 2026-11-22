package workspace

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"unicode"
)

// ReplaceResult is what a replacement across a project changed.
type ReplaceResult struct {
	Files       int
	Occurrences int
}

// ReplaceAll replaces every occurrence of a query in the files of a
// project, and says how much it changed. Each file is written only when its
// text on disk is still what was read, as Save does.
func ReplaceAll(ctx context.Context, root string, files []string, query, replacement string, matchCase bool) (ReplaceResult, error) {
	result := ReplaceResult{}
	query = strings.TrimSpace(query)
	if query == "" {
		return result, fmt.Errorf("请输入要替换的文本")
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
		if err != nil || !info.Mode().IsRegular() || info.Size() > MaxFileSize {
			continue
		}
		text, err := Read(full)
		if err != nil {
			continue
		}
		next, count := replaceAll(text, query, replacement, matchCase)
		if count == 0 {
			continue
		}
		if err := Save(full, text, next); err != nil {
			return result, fmt.Errorf("%s: %w", path, err)
		}
		result.Files++
		result.Occurrences += count
	}
	return result, nil
}

// replaceAll replaces every occurrence of a query in a text, without
// overlapping, and says how many there were.
func replaceAll(text, query, replacement string, matchCase bool) (string, int) {
	ranges := findAll(text, query, matchCase)
	if len(ranges) == 0 {
		return text, 0
	}
	builder := &strings.Builder{}
	last := 0
	for _, r := range ranges {
		builder.WriteString(text[last:r[0]])
		builder.WriteString(replacement)
		last = r[1]
	}
	builder.WriteString(text[last:])
	return builder.String(), len(ranges)
}

// findAll holds the byte ranges of every occurrence of a query in a text,
// in order, none overlapping.
func findAll(text, query string, matchCase bool) [][2]int {
	if query == "" || len(text) == 0 {
		return nil
	}
	if matchCase {
		var ranges [][2]int
		at := 0
		for {
			i := strings.Index(text[at:], query)
			if i < 0 {
				break
			}
			ranges = append(ranges, [2]int{at + i, at + i + len(query)})
			at += i + len(query)
		}
		return ranges
	}
	// The query matches case insensitively: the text and the query are
	// lowered a rune at a time, and the byte of each rune of the text is
	// kept, so that what is found points into the original.
	needle := []rune(query)
	positions := make([]int, 0, len(text)+1)
	for pos := range text {
		positions = append(positions, pos)
	}
	positions = append(positions, len(text))
	runes := make([]rune, 0, len(positions))
	for _, r := range text {
		runes = append(runes, toLower(r))
	}
	var ranges [][2]int
	for i := 0; i+len(needle) <= len(runes); i++ {
		found := true
		for j, r := range needle {
			if runes[i+j] != toLower(r) {
				found = false
				break
			}
		}
		if !found {
			continue
		}
		ranges = append(ranges, [2]int{positions[i], positions[i+len(needle)]})
		i += len(needle) - 1
	}
	return ranges
}

// toLower lowers one rune, the way a search that ignores case does.
func toLower(r rune) rune { return unicode.ToLower(r) }
