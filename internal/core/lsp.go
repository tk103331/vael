package core

import (
	"bufio"
	"context"
	"encoding/json"
	"io"
	"os/exec"
	"runtime"
	"strings"
	"sync"

	"vael/internal/proc"
)

// A language server is a program a project brings, speaking LSP on its
// standard input and output. The core does not speak the protocol: it runs
// the program where the files are, and hands the editor its pipes, so that
// a server of a remote project runs on the remote machine, as the files
// do, and the editor speaks to it over the same connection as everything
// else.

// lspProcess is a language server the core runs, and the pipes of it.
type lspProcess struct {
	cmd   *exec.Cmd
	stdin io.WriteCloser
	// output is what the server prints, a chunk at a time.
	output chan []byte

	mu     sync.Mutex
	stderr []string
	closed bool
}

// startLSP runs a language server in a directory, and starts reading what
// it prints. The command is run by the shell, so that it may set the
// environment of the server, as a task's command may.
func startLSP(ctx context.Context, dir, command string, args []string) (*lspProcess, error) {
	line := strings.Join(append([]string{command}, args...), " ")
	var cmd *exec.Cmd
	if runtime.GOOS == "windows" {
		cmd = exec.CommandContext(ctx, "cmd", "/c", line)
	} else {
		cmd = exec.CommandContext(ctx, "sh", "-c", line)
	}
	cmd.Dir = dir
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
	// The server and what it starts go away with the core.
	proc.SetGroup(cmd)
	if err := cmd.Start(); err != nil {
		return nil, err
	}
	server := &lspProcess{cmd: cmd, stdin: stdin, output: make(chan []byte, 32)}
	go server.read(stdout)
	go server.readErrors(stderr)
	return server, nil
}

// read reads what the server prints, and passes it on as it comes.
func (p *lspProcess) read(r io.Reader) {
	defer close(p.output)
	buffer := make([]byte, 32<<10)
	for {
		n, err := r.Read(buffer)
		if n > 0 {
			chunk := make([]byte, n)
			copy(chunk, buffer[:n])
			p.output <- chunk
		}
		if err != nil {
			return
		}
	}
}

// readErrors keeps what the server said on its errors, for when it dies.
func (p *lspProcess) readErrors(r io.Reader) {
	scanner := bufio.NewScanner(r)
	scanner.Buffer(make([]byte, 0, 16<<10), 256<<10)
	for scanner.Scan() {
		p.mu.Lock()
		p.stderr = append(p.stderr, scanner.Text())
		if len(p.stderr) > 40 {
			p.stderr = p.stderr[1:]
		}
		p.mu.Unlock()
	}
}

// write sends the editor's words to the server.
func (p *lspProcess) write(data string) error {
	p.mu.Lock()
	closed := p.closed
	p.mu.Unlock()
	if closed {
		return io.ErrClosedPipe
	}
	_, err := io.WriteString(p.stdin, data)
	return err
}

// errors is what the server said on its errors, in one line.
func (p *lspProcess) errors() string {
	p.mu.Lock()
	defer p.mu.Unlock()
	return strings.Join(p.stderr, "\n")
}

// stop ends the server.
func (p *lspProcess) stop() {
	p.mu.Lock()
	if p.closed {
		p.mu.Unlock()
		return
	}
	p.closed = true
	p.mu.Unlock()
	_ = p.stdin.Close()
	proc.KillGroup(p.cmd)
}

// startLSPStream runs a language server for the client, and tells it the
// stream its output comes on, as a task's output does.
func (s *Server) startLSPStream(ctx context.Context, m Message) {
	var p struct {
		Root string   `json:"root"`
		Text string   `json:"text"`
		Args []string `json:"args"`
	}
	if err := json.Unmarshal(m.Params, &p); err != nil {
		s.write(Message{Kind: KindError, ID: m.ID, Error: &Error{Code: CodeInvalid, Message: err.Error()}})
		return
	}
	if strings.TrimSpace(p.Text) == "" {
		s.write(Message{Kind: KindError, ID: m.ID, Error: &Error{Code: CodeInvalid, Message: "请输入语言服务器的命令"}})
		return
	}
	// The server lasts as long as the client keeps the stream, not as
	// long as the request that began it.
	serverCtx, cancel := context.WithCancel(s.ctx)
	server, err := startLSP(serverCtx, p.Root, p.Text, p.Args)
	if err != nil {
		cancel()
		s.write(Message{Kind: KindError, ID: m.ID, Error: &Error{Code: CodeInternal, Message: err.Error()}})
		return
	}
	s.mu.Lock()
	s.nextStream++
	id := s.nextStream
	s.streams[id] = &serverStream{id: id, cancel: func() { server.stop(); cancel() }}
	if s.lsps == nil {
		s.lsps = map[uint64]*lspProcess{}
	}
	s.lsps[id] = server
	s.mu.Unlock()
	result, _ := json.Marshal(map[string]uint64{"stream": id})
	s.write(Message{Kind: KindResponse, ID: m.ID, Result: result})
	go func() {
		defer func() {
			s.mu.Lock()
			delete(s.lsps, id)
			s.mu.Unlock()
		}()
		for chunk := range server.output {
			payload, _ := json.Marshal(map[string]string{"data": string(chunk)})
			s.notify(id, "lsp/output", json.RawMessage(payload))
		}
		payload, _ := json.Marshal(map[string]string{"log": server.errors()})
		s.notify(id, "lsp/exit", json.RawMessage(payload))
	}()
}

// writeLSP passes what the editor wrote to a language server.
func (s *Server) writeLSP(m Message) {
	var p struct {
		Stream uint64 `json:"stream"`
		Text   string `json:"text"`
	}
	if err := json.Unmarshal(m.Params, &p); err != nil {
		s.write(Message{Kind: KindError, ID: m.ID, Error: &Error{Code: CodeInvalid, Message: err.Error()}})
		return
	}
	s.mu.Lock()
	server := s.lsps[p.Stream]
	s.mu.Unlock()
	if server == nil {
		s.write(Message{Kind: KindError, ID: m.ID, Error: &Error{Code: CodeNotFound, Message: "语言服务器已结束"}})
		return
	}
	if err := server.write(p.Text); err != nil {
		s.write(Message{Kind: KindError, ID: m.ID, Error: &Error{Code: CodeInternal, Message: err.Error()}})
		return
	}
	s.write(Message{Kind: KindResponse, ID: m.ID, Result: json.RawMessage("{}")})
}
