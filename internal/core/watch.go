package core

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"vael/internal/workspace"
)

// errNoStream is what a transport that cannot hold subscriptions says.
var errNoStream = &Errorf{Code: CodeUnsupported, Err: errors.New("该连接不支持订阅")}

// startWatch begins to watch files for the client, and tells it the stream
// the changes will come on.
func (s *Server) startWatch(ctx context.Context, m Message) {
	var p struct {
		Paths  []string                   `json:"paths"`
		Before map[string]workspace.Stamp `json:"before"`
	}
	if err := json.Unmarshal(m.Params, &p); err != nil {
		s.write(Message{Kind: KindError, ID: m.ID, Error: &Error{Code: CodeInvalid, Message: err.Error()}})
		return
	}
	// The watch lasts as long as the connection, not as long as the
	// request that began it.
	watchCtx, cancel := context.WithCancel(s.ctx)
	s.mu.Lock()
	s.nextStream++
	id := s.nextStream
	stream := &serverStream{id: id, paths: p.Paths, stamps: map[string]workspace.Stamp{}, cancel: cancel}
	for path, stamp := range p.Before {
		stream.stamps[path] = stamp
	}
	for _, path := range p.Paths {
		// A path the client gave no stamp for is watched from now on: what
		// is on the disk as the watch begins is the baseline.
		if _, known := stream.stamps[path]; !known {
			if stamp, err := workspace.StampOf(path); err == nil {
				stream.stamps[path] = stamp
			}
		}
	}
	s.streams[id] = stream
	s.mu.Unlock()
	go s.watchFiles(watchCtx, stream)
	result, _ := json.Marshal(map[string]uint64{"stream": id})
	s.write(Message{Kind: KindResponse, ID: m.ID, Result: result})
}

// stopWatch ends a watch.
func (s *Server) stopWatch(m Message) {
	var p struct {
		Stream uint64 `json:"stream"`
	}
	json.Unmarshal(m.Params, &p)
	s.mu.Lock()
	stream := s.streams[p.Stream]
	delete(s.streams, p.Stream)
	delete(s.lsps, p.Stream)
	s.mu.Unlock()
	if stream != nil {
		stream.cancel()
	}
	s.write(Message{Kind: KindResponse, ID: m.ID, Result: json.RawMessage("{}")})
}

// watchFiles looks at the files of a watch, and pushes what changed. The
// core does the looking, so that the editor waits for nothing: the changes
// come when they happen, wherever the files are.
func (s *Server) watchFiles(ctx context.Context, stream *serverStream) {
	ticker := time.NewTicker(400 * time.Millisecond)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
		changes := []workspace.DiskChange{}
		for _, path := range stream.paths {
			after, err := workspace.StampOf(path)
			s.mu.Lock()
			before, known := stream.stamps[path]
			s.mu.Unlock()
			if err != nil {
				if known {
					changes = append(changes, workspace.DiskChange{Path: path, Before: before, Deleted: true})
					s.mu.Lock()
					delete(stream.stamps, path)
					s.mu.Unlock()
				}
				continue
			}
			if !known {
				continue
			}
			if before != after {
				changes = append(changes, workspace.DiskChange{Path: path, Before: before, After: after})
				s.mu.Lock()
				stream.stamps[path] = after
				s.mu.Unlock()
			}
		}
		if len(changes) == 0 {
			continue
		}
		payload, err := json.Marshal(map[string]any{"changes": changes})
		if err != nil {
			continue
		}
		s.notify(stream.id, "changed", json.RawMessage(payload))
	}
}
