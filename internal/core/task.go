package core

import (
	"context"
	"encoding/json"
	"strings"
	"time"

	"vael/internal/workspace"
)

// startRun runs a command in the project and streams its output to the
// client, as the changes of a watch are pushed. The stream lasts as long as
// the command does: the client is told the stream first, the output as it
// is printed, and how the command ended last.
func (s *Server) startRun(ctx context.Context, m Message) {
	var p struct {
		Root     string `json:"root"`
		Text     string `json:"text"`
		Interval int    `json:"interval"`
	}
	if err := json.Unmarshal(m.Params, &p); err != nil {
		s.write(Message{Kind: KindError, ID: m.ID, Error: &Error{Code: CodeInvalid, Message: err.Error()}})
		return
	}
	if strings.TrimSpace(p.Text) == "" {
		s.write(Message{Kind: KindError, ID: m.ID, Error: &Error{Code: CodeInvalid, Message: "请输入要运行的命令"}})
		return
	}
	// The task lasts as long as the connection, not as long as the request
	// that began it, and is stopped when the client lets the stream go.
	runCtx, cancel := context.WithCancel(s.ctx)
	s.mu.Lock()
	s.nextStream++
	id := s.nextStream
	s.streams[id] = &serverStream{id: id, cancel: cancel}
	s.mu.Unlock()
	result, _ := json.Marshal(map[string]uint64{"stream": id})
	s.write(Message{Kind: KindResponse, ID: m.ID, Result: result})
	go func() {
		task, err := workspace.RunStream(runCtx, p.Root, p.Text, time.Duration(p.Interval)*time.Millisecond, func(chunk []byte) {
			payload, _ := json.Marshal(map[string]string{"text": string(chunk)})
			s.notify(id, "output", json.RawMessage(payload))
		})
		if err != nil {
			// The client gave the task up, or the connection ended:
			// there is no one to tell.
			return
		}
		data, _ := json.Marshal(task)
		s.notify(id, "done", json.RawMessage(data))
	}()
}
