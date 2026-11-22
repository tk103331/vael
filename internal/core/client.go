package core

import (
	"context"
	"encoding/json"
	"errors"
	"net"
	"time"

	"vael/internal/workspace"
)

// Client is what the app talks to the core with: the same calls, whether
// the core runs on this machine or in the environment the editor is
// connected to.
type Client struct{ transport Transport }

// NewLocal makes a client of a core that runs on this machine. The core
// speaks the same protocol as a remote one, over a pair of pipes, so that
// both behave alike.
func NewLocal() *Client {
	server, client := net.Pipe()
	go Serve(context.Background(), server, server)
	return NewClient(NewStream(client, client, nil))
}

// NewClient makes a client of a transport.
func NewClient(t Transport) *Client { return &Client{t} }

func (c *Client) Close() error { return c.transport.Close() }

// Done is closed when the connection to the core ends.
func (c *Client) Done() <-chan struct{} {
	if state, ok := c.transport.(interface{ Done() <-chan struct{} }); ok {
		return state.Done()
	}
	return nil
}

// Err is why the connection ended, when it did.
func (c *Client) Err() error {
	if state, ok := c.transport.(interface{ Err() error }); ok {
		return state.Err()
	}
	return nil
}

// Info says what the core is and which protocol it speaks.
func (c *Client) Info(ctx context.Context) (Info, error) {
	var v Info
	err := c.transport.Call(ctx, "info", nil, &v)
	return v, err
}
func (c *Client) Stat(ctx context.Context, path string) (FileInfo, error) {
	var v FileInfo
	err := c.transport.Call(ctx, "stat", params{Path: path}, &v)
	return v, err
}
func (c *Client) List(ctx context.Context, path string) ([]workspace.Entry, error) {
	var v []workspace.Entry
	err := c.transport.Call(ctx, "list", params{Path: path}, &v)
	return v, err
}
func (c *Client) Read(ctx context.Context, path string) (string, error) {
	text, _, err := c.ReadFile(ctx, path)
	return text, err
}

// ReadFile reads a file and reports its stamp on disk, so that the editor
// knows whether the file changed since it was read.
func (c *Client) ReadFile(ctx context.Context, path string) (string, workspace.Stamp, error) {
	var v FileContent
	err := c.transport.Call(ctx, "read", params{Path: path}, &v)
	return v.Text, v.Stamp, err
}

// Save writes a file and reports its new stamp on disk.
func (c *Client) Save(ctx context.Context, path, original, text string) (workspace.Stamp, error) {
	var v workspace.Stamp
	err := c.transport.Call(ctx, "save", params{Path: path, Original: original, Text: text}, &v)
	return v, err
}
func (c *Client) Create(ctx context.Context, root, name string, directory bool) error {
	return c.transport.Call(ctx, "create", params{Root: root, Name: name, Directory: directory}, nil)
}
func (c *Client) Index(ctx context.Context, root string) (workspace.FileIndex, error) {
	var v workspace.FileIndex
	err := c.transport.Call(ctx, "index", params{Root: root}, &v)
	return v, err
}
func (c *Client) Search(ctx context.Context, root string, files []string, query string, matchCase bool) (workspace.SearchResults, error) {
	var v workspace.SearchResults
	err := c.transport.Call(ctx, "search", params{Root: root, Files: files, Query: query, MatchCase: matchCase}, &v)
	return v, err
}
func (c *Client) Git(ctx context.Context, root string, args ...string) (string, error) {
	var v string
	err := c.transport.Call(ctx, "git", params{Root: root, Args: args}, &v)
	return v, err
}

// Clone clones a repository into the folder name of parent, and returns
// the path of the clone.
func (c *Client) Clone(ctx context.Context, parent, url, name string) (string, error) {
	var v string
	err := c.transport.Call(ctx, "clone", params{Root: parent, URL: url, Name: name}, &v)
	return v, err
}

func (c *Client) Status(ctx context.Context, root string) (*workspace.Repository, error) {
	var v workspace.Repository
	err := c.transport.Call(ctx, "status", params{Root: root}, &v)
	if err != nil {
		return nil, err
	}
	return &v, nil
}
func (c *Client) Stage(ctx context.Context, root string, change workspace.Change) error {
	return c.transport.Call(ctx, "stage", params{Root: root, Change: change}, nil)
}
func (c *Client) Unstage(ctx context.Context, root string, change workspace.Change) error {
	return c.transport.Call(ctx, "unstage", params{Root: root, Change: change}, nil)
}

// Rename renames a file or a directory of the project.
func (c *Client) Rename(ctx context.Context, root, from, to string) error {
	return c.transport.Call(ctx, "rename", params{Root: root, Path: from, Text: to}, nil)
}

// Remove deletes a file, or a directory with everything in it.
func (c *Client) Remove(ctx context.Context, root, path string) error {
	return c.transport.Call(ctx, "remove", params{Root: root, Path: path}, nil)
}

// Duplicate copies a file or a directory next to itself.
func (c *Client) Duplicate(ctx context.Context, root, path string) (string, error) {
	var v string
	err := c.transport.Call(ctx, "duplicate", params{Root: root, Path: path}, &v)
	return v, err
}

// Symbols are the declarations of the files of a project.
func (c *Client) Symbols(ctx context.Context, root string, files []string, limit int) ([]workspace.Symbol, error) {
	var v []workspace.Symbol
	err := c.transport.Call(ctx, "symbols", params{Root: root, Files: files, Limit: limit}, &v)
	return v, err
}

// ReplaceAll replaces every occurrence of a query in the files of a
// project, and says what changed.
func (c *Client) ReplaceAll(ctx context.Context, root string, files []string, query, replacement string, matchCase bool) (workspace.ReplaceResult, error) {
	var v workspace.ReplaceResult
	err := c.transport.Call(ctx, "replace", params{Root: root, Files: files, Query: query, Text: replacement, MatchCase: matchCase}, &v)
	return v, err
}

// Run runs a command in a project directory and waits for it.
func (c *Client) Run(ctx context.Context, root, command string, timeout time.Duration) (workspace.RunResult, error) {
	return c.RunStream(ctx, root, command, timeout, nil)
}

// RunStream runs a command in a project directory and hands its output to
// onOutput as the command prints it, so that a build is shown as it runs.
// Giving the context up stops the command on the core's side.
func (c *Client) RunStream(ctx context.Context, root, command string, timeout time.Duration, onOutput func(string)) (workspace.RunResult, error) {
	streaming, ok := c.transport.(Streaming)
	if !ok {
		return workspace.RunResult{}, &Errorf{Code: CodeUnsupported, Err: errNoStream}
	}
	subscription, err := streaming.Subscribe(ctx, "run", params{Root: root, Text: command, Interval: int(timeout / time.Millisecond)})
	if err != nil {
		return workspace.RunResult{}, err
	}
	defer subscription.Close()
	messages := subscription.Messages()
	var result workspace.RunResult
	for {
		select {
		case m, ok := <-messages:
			if !ok {
				// The stream of messages ended without a result: the
				// connection to the core did.
				if err := c.Err(); err != nil {
					return result, err
				}
				return result, &Errorf{Code: CodeInternal, Err: errors.New("任务未完成：核心连接已结束")}
			}
			switch m.Method {
			case "output":
				var payload struct {
					Text string `json:"text"`
				}
				if json.Unmarshal(m.Params, &payload) == nil && onOutput != nil {
					onOutput(payload.Text)
				}
			case "done":
				if json.Unmarshal(m.Params, &result) == nil {
					return result, nil
				}
				return result, &Errorf{Code: CodeInternal, Err: errors.New("核心返回了无法解析的任务结果")}
			}
		case <-ctx.Done():
			return result, ctx.Err()
		}
	}
}

// Watch holds the files that the core watches, and tells of the changes it
// sees. The core pushes them: the editor does not ask again and again.
func (c *Client) Watch(ctx context.Context, paths []string, before map[string]workspace.Stamp) (*Watch, error) {
	streaming, ok := c.transport.(Streaming)
	if !ok {
		return nil, &Errorf{Code: CodeUnsupported, Err: errNoStream}
	}
	subscription, err := streaming.Subscribe(ctx, "watch", params{Paths: paths, Before: before})
	if err != nil {
		return nil, err
	}
	watch := &Watch{subscription: subscription, changes: make(chan workspace.DiskChange, 64)}
	go watch.read()
	return watch, nil
}

// Watch is what the core tells of the files it watches.
type Watch struct {
	subscription Subscription
	changes      chan workspace.DiskChange
}

func (w *Watch) read() {
	for m := range w.subscription.Messages() {
		var payload struct {
			Changes []workspace.DiskChange `json:"changes"`
		}
		if err := json.Unmarshal(m.Params, &payload); err != nil {
			continue
		}
		for _, change := range payload.Changes {
			w.changes <- change
		}
	}
}

// Changes are the changes the core has seen.
func (w *Watch) Changes() <-chan workspace.DiskChange { return w.changes }

// Close ends the watch: the core stops looking at the files.
func (w *Watch) Close() error { return w.subscription.Close() }
