package main

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os/exec"
	"runtime"
	"strings"
	"sync"

	"vael/internal/proc"
)

// The Agent Client Protocol, as Zed's ACP tells it: the editor starts an
// agent, and the two speak JSON-RPC, one message a line, over the
// agent's standard input and output.

const acpProtocolVersion = 1

// acpMessage is one message of the protocol: a request, the answer to
// one, or a notification.
type acpMessage struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      *int            `json:"id,omitempty"`
	Method  string          `json:"method,omitempty"`
	Params  json.RawMessage `json:"params,omitempty"`
	Result  json.RawMessage `json:"result,omitempty"`
	Error   *acpError       `json:"error,omitempty"`
}

// acpError is what an agent answers a request it could not do with.
type acpError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

// acpInitialize is what an agent says of itself.
type acpInitialize struct {
	ProtocolVersion   int `json:"protocolVersion"`
	AgentCapabilities struct {
		LoadSession bool `json:"loadSession"`
	} `json:"agentCapabilities"`
	AgentInfo struct {
		Name    string `json:"name"`
		Version string `json:"version"`
	} `json:"agentInfo"`
	AuthMethods []struct {
		ID          string `json:"id"`
		Name        string `json:"name"`
		Description string `json:"description"`
	} `json:"authMethods"`
}

// acpSession is the answer to a new session.
type acpSession struct {
	SessionID string `json:"sessionId"`
	// Modes are the ways the agent can be told to work, as the modes of
	// an agent that edits files of its own accord.
	Modes struct {
		CurrentModeID  string `json:"currentModeId"`
		AvailableModes []struct {
			ID   string `json:"id"`
			Name string `json:"name"`
		} `json:"availableModes"`
	} `json:"modes"`
}

// acpPromptResult is how a turn of the conversation ended.
type acpPromptResult struct {
	StopReason string `json:"stopReason"`
}

// acpContent is a block of what an agent says: its words, and the other
// kinds a session sends, which the panel names.
type acpContent struct {
	Type     string `json:"type"`
	Text     string `json:"text"`
	Name     string `json:"name"`
	URI      string `json:"uri"`
	MimeType string `json:"mimeType"`
}

// Reads says what a block of content reads as: its words, or what kind
// of thing it is.
func (c acpContent) Reads() string {
	switch c.Type {
	case "text":
		return c.Text
	case "image":
		return "[图片]"
	case "audio":
		return "[音频]"
	case "resource_link":
		if c.Name != "" {
			return "[" + c.Name + "]"
		}
		return "[" + c.URI + "]"
	case "resource":
		return "[资源 " + c.URI + "]"
	}
	if c.Text != "" {
		return c.Text
	}
	return ""
}

// acpToolContent is a part of what a tool of the agent gives: its words,
// a diff of a file, or a terminal.
type acpToolContent struct {
	Type     string      `json:"type"`
	Content  *acpContent `json:"content"`
	Path     string      `json:"path"`
	OldText  *string     `json:"oldText"`
	NewText  *string     `json:"newText"`
	Terminal string      `json:"terminalId"`
}

// acpPlanEntry is one line of the plan an agent keeps for its work.
type acpPlanEntry struct {
	Content  string `json:"content"`
	Priority string `json:"priority"`
	Status   string `json:"status"`
}

// acpLocation is a place of the project a tool works in.
type acpLocation struct {
	Path string `json:"path"`
	Line *int   `json:"line"`
}

// acpUpdate is a notification of what the agent does in a session.
type acpUpdate struct {
	SessionID string `json:"sessionId"`
	Update    struct {
		SessionUpdate string `json:"sessionUpdate"`
		// Content is a block for what the agent says, and parts of a
		// tool for what it does: what it is, the panel reads anew.
		Content json.RawMessage `json:"content"`
		// Entries are the plan's lines, when the update is a plan.
		Entries []acpPlanEntry `json:"entries"`
		// The tool a tool call is about.
		ToolCallID string        `json:"toolCallId"`
		Title      string        `json:"title"`
		Kind       string        `json:"kind"`
		Status     string        `json:"status"`
		Locations  []acpLocation `json:"locations"`
		RawInput   any           `json:"rawInput"`
		// The mode the agent works in, when the update says it.
		CurrentModeID string `json:"currentModeId"`
	} `json:"update"`
}

// Words reads the block of a message update.
func (u acpUpdate) Words() string {
	var block acpContent
	if len(u.Update.Content) == 0 {
		return ""
	}
	if err := json.Unmarshal(u.Update.Content, &block); err != nil {
		return ""
	}
	return block.Reads()
}

// ToolParts reads what a tool call gives.
func (u acpUpdate) ToolParts() []acpToolContent {
	var parts []acpToolContent
	if len(u.Update.Content) == 0 {
		return nil
	}
	if err := json.Unmarshal(u.Update.Content, &parts); err != nil {
		return nil
	}
	return parts
}

// acpPermission is what an agent asks before it does something of its
// own, as running a command.
type acpPermission struct {
	SessionID string `json:"sessionId"`
	ToolCall  struct {
		ToolCallID string `json:"toolCallId"`
		Title      string `json:"title"`
		Kind       string `json:"kind"`
	} `json:"toolCall"`
	Options []acpPermissionOption `json:"options"`
}

// acpPermissionOption is a choice an agent offers for one of its
// requests: to allow the tool once or always, or to reject it.
type acpPermissionOption struct {
	OptionID string `json:"optionId"`
	Name     string `json:"name"`
	Kind     string `json:"kind"`
}

// acpConn is a running agent, and the messages the editor and it go on
// with.
type acpConn struct {
	cmd   *exec.Cmd
	stdin io.WriteCloser

	mu     sync.Mutex
	nextID int
	wait   map[int]chan acpMessage
	closed bool

	// stderr keeps the last words of the agent's errors, for when it
	// will not start.
	stderrMu sync.Mutex
	stderr   []string

	// handler gets what the agent says on its own, and what it asks.
	handler func(acpMessage)
}

// startACP runs an agent, and starts reading what it says.
func startACP(ctx context.Context, command string, handler func(acpMessage)) (*acpConn, error) {
	var cmd *exec.Cmd
	if runtime.GOOS == "windows" {
		cmd = exec.CommandContext(ctx, "cmd", "/c", command)
	} else {
		cmd = exec.CommandContext(ctx, "sh", "-c", command)
	}
	stdin, err := cmd.StdinPipe()
	if err != nil {
		return nil, err
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, err
	}
	stderr, err := cmd.StderrPipe()
	if err != nil {
		return nil, err
	}
	// The agent and what it starts go away with the editor.
	proc.SetGroup(cmd)
	if err := cmd.Start(); err != nil {
		return nil, err
	}
	conn := &acpConn{cmd: cmd, stdin: stdin, wait: map[int]chan acpMessage{}, handler: handler}
	go conn.read(stdout)
	go conn.readErrors(stderr)
	go func() { _ = cmd.Wait() }()
	return conn, nil
}

// read reads the messages of the agent, one a line.
func (c *acpConn) read(r io.Reader) {
	scanner := bufio.NewScanner(r)
	scanner.Buffer(make([]byte, 0, 64*1024), 8*1024*1024)
	for scanner.Scan() {
		var message acpMessage
		if err := json.Unmarshal(scanner.Bytes(), &message); err != nil {
			continue
		}
		c.dispatch(message)
	}
	c.close()
}

// readErrors keeps what the agent wrote to its errors, which says why it
// died when it does.
func (c *acpConn) readErrors(r io.Reader) {
	scanner := bufio.NewScanner(r)
	scanner.Buffer(make([]byte, 0, 16*1024), 256*1024)
	for scanner.Scan() {
		c.stderrMu.Lock()
		c.stderr = append(c.stderr, scanner.Text())
		if len(c.stderr) > 20 {
			c.stderr = c.stderr[1:]
		}
		c.stderrMu.Unlock()
	}
}

// Errors is what the agent said on its errors, as one line.
func (c *acpConn) Errors() string {
	c.stderrMu.Lock()
	defer c.stderrMu.Unlock()
	return strings.Join(c.stderr, "\n")
}

// dispatch gives an answer to the call that waits for it, and everything
// else to the handler.
func (c *acpConn) dispatch(message acpMessage) {
	if message.ID != nil && message.Method == "" {
		c.mu.Lock()
		wait, ok := c.wait[*message.ID]
		delete(c.wait, *message.ID)
		c.mu.Unlock()
		if ok {
			wait <- message
		}
		return
	}
	if c.handler != nil {
		c.handler(message)
	}
}

// call sends a request, and waits for its answer.
func (c *acpConn) call(ctx context.Context, method string, params any, result any) error {
	_, wait, err := c.send(method, params)
	if err != nil {
		return err
	}
	select {
	case <-ctx.Done():
		return ctx.Err()
	case message := <-wait:
		if message.Error != nil {
			return fmt.Errorf("%s", message.Error.Message)
		}
		if result != nil && len(message.Result) > 0 {
			return json.Unmarshal(message.Result, result)
		}
		return nil
	}
}

// send writes a request, and returns what its answer will come through.
func (c *acpConn) send(method string, params any) (int, chan acpMessage, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.closed {
		return 0, nil, fmt.Errorf("Agent 已断开")
	}
	c.nextID++
	id := c.nextID
	wait := make(chan acpMessage, 1)
	c.wait[id] = wait
	message := map[string]any{"jsonrpc": "2.0", "id": id, "method": method}
	if params != nil {
		message["params"] = params
	}
	data, err := json.Marshal(message)
	if err != nil {
		return 0, nil, err
	}
	if _, err := c.stdin.Write(append(data, '\n')); err != nil {
		return 0, nil, err
	}
	return id, wait, nil
}

// reply answers a request of the agent.
func (c *acpConn) reply(id int, result any) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.closed {
		return nil
	}
	message := map[string]any{"jsonrpc": "2.0", "id": id, "result": result}
	data, err := json.Marshal(message)
	if err != nil {
		return err
	}
	_, err = c.stdin.Write(append(data, '\n'))
	return err
}

// replyError answers a request of the agent that the editor cannot do.
func (c *acpConn) replyError(id int, code int, message string) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.closed {
		return nil
	}
	data, err := json.Marshal(map[string]any{
		"jsonrpc": "2.0", "id": id,
		"error": map[string]any{"code": code, "message": message},
	})
	if err != nil {
		return err
	}
	_, err = c.stdin.Write(append(data, '\n'))
	return err
}

// notify sends a notification, which no answer comes for.
func (c *acpConn) notify(method string, params any) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.closed {
		return nil
	}
	message := map[string]any{"jsonrpc": "2.0", "method": method}
	if params != nil {
		message["params"] = params
	}
	data, err := json.Marshal(message)
	if err != nil {
		return err
	}
	_, err = c.stdin.Write(append(data, '\n'))
	return err
}

// close ends the agent.
func (c *acpConn) close() {
	c.mu.Lock()
	if c.closed {
		c.mu.Unlock()
		return
	}
	c.closed = true
	for id, wait := range c.wait {
		delete(c.wait, id)
		close(wait)
	}
	stdin := c.stdin
	c.mu.Unlock()
	if stdin != nil {
		_ = stdin.Close()
	}
	if c.cmd != nil {
		proc.KillGroup(c.cmd)
	}
}

// Closed tells whether the agent went away.
func (c *acpConn) Closed() bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.closed
}
