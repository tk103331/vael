package core

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"vael/internal/workspace"
)

type Info struct {
	Protocol int
	Home     string
	Features []string
}
type FileInfo struct {
	Directory bool
	Path      string
	Stamp     workspace.Stamp
}

// FileContent is a file's text and what its stamp on disk is.
type FileContent struct {
	Text  string
	Stamp workspace.Stamp
}

type params struct {
	Path      string                     `json:"path,omitempty"`
	URL       string                     `json:"url,omitempty"`
	Root      string                     `json:"root,omitempty"`
	Original  string                     `json:"original,omitempty"`
	Text      string                     `json:"text,omitempty"`
	Directory bool                       `json:"directory,omitempty"`
	Name      string                     `json:"name,omitempty"`
	Args      []string                   `json:"args,omitempty"`
	Files     []string                   `json:"files,omitempty"`
	Query     string                     `json:"query,omitempty"`
	MatchCase bool                       `json:"match_case,omitempty"`
	Change    workspace.Change           `json:"change,omitempty"`
	Paths     []string                   `json:"paths,omitempty"`
	Before    map[string]workspace.Stamp `json:"before,omitempty"`
	Interval  int                        `json:"interval,omitempty"`
	Limit     int                        `json:"limit,omitempty"`
	Stream    uint64                     `json:"stream,omitempty"`
}

// Service runs the operations of the editor, with the permissions of the
// user of the machine the core runs on.
type Service struct{}

// Handle runs one operation. The errors it returns carry the code that says
// what went wrong: a file that is not there, a file that changed, a file
// that is too large.
func (Service) Handle(ctx context.Context, method string, raw json.RawMessage) (any, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	var p params
	if len(raw) > 0 {
		if err := json.Unmarshal(raw, &p); err != nil {
			return nil, &Errorf{Code: CodeInvalid, Err: err}
		}
	}
	switch method {
	case "info":
		home, err := os.UserHomeDir()
		return Info{ProtocolVersion, home, []string{
			FeatureWatch, FeatureStreamRun, FeatureReplace, FeatureSymbols, FeatureClone, FeatureLSP,
		}}, err
	case "stat":
		info, err := os.Stat(p.Path)
		if err != nil {
			return nil, fileError(err)
		}
		path, err := filepath.Abs(p.Path)
		return FileInfo{info.IsDir(), path, workspace.Stamp{Mod: info.ModTime().UnixNano(), Size: info.Size()}}, err
	case "list":
		entries, err := workspace.List(p.Path)
		return entries, fileError(err)
	case "read":
		text, stamp, err := workspace.ReadWithStamp(p.Path)
		if err != nil {
			return nil, fileError(err)
		}
		return FileContent{text, stamp}, nil
	case "save":
		if err := workspace.Save(p.Path, p.Original, p.Text); err != nil {
			return nil, saveError(err)
		}
		stamp, err := workspace.StampOf(p.Path)
		if err != nil {
			return nil, fileError(err)
		}
		return stamp, nil
	case "create":
		return nil, workspace.Create(p.Root, p.Name, p.Directory)
	case "index":
		return workspace.Index(ctx, p.Root)
	case "search":
		return workspace.Search(ctx, p.Root, p.Files, p.Query, p.MatchCase)
	case "git":
		return workspace.GitContext(ctx, p.Root, p.Args...)
	case "clone":
		return workspace.Clone(ctx, p.Root, p.URL, p.Name)
	case "status":
		return workspace.StatusContext(ctx, p.Root)
	case "stage":
		return nil, workspace.StageContext(ctx, p.Root, p.Change)
	case "unstage":
		return nil, workspace.UnstageContext(ctx, p.Root, p.Change)
	case "symbols":
		return workspace.ProjectSymbols(ctx, p.Root, p.Files, p.Limit)
	case "run":
		return workspace.Run(ctx, p.Root, p.Text, time.Duration(p.Interval)*time.Millisecond)
	case "replace":
		return workspace.ReplaceAll(ctx, p.Root, p.Files, p.Query, p.Text, p.MatchCase)
	case "rename":
		return nil, workspace.Rename(p.Root, p.Path, p.Text)
	case "remove":
		return nil, workspace.Remove(p.Root, p.Path)
	case "duplicate":
		return workspace.Duplicate(p.Root, p.Path)
	default:
		return nil, &Errorf{Code: CodeUnsupported, Err: fmt.Errorf("核心不支持该操作: %s", method)}
	}
}

// fileError gives an error of the file system the code that says what it is.
func fileError(err error) error {
	if err == nil {
		return nil
	}
	if os.IsNotExist(err) {
		return &Errorf{Code: CodeNotFound, Err: err}
	}
	if os.IsPermission(err) {
		return &Errorf{Code: CodePermission, Err: err}
	}
	return &Errorf{Code: CodeInternal, Err: err}
}

// saveError tells a conflict with another program's edit from other errors.
func saveError(err error) error {
	if err == nil {
		return nil
	}
	if os.IsNotExist(err) {
		return &Errorf{Code: CodeNotFound, Err: err}
	}
	if os.IsPermission(err) {
		return &Errorf{Code: CodePermission, Err: err}
	}
	return &Errorf{Code: CodeConflict, Err: err}
}
