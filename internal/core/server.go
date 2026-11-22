package core

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"io"
	"sync"

	"vael/internal/workspace"
)

// maxFrame is the largest message the core reads: a file of eight mebibytes
// is the largest thing it sends.
const maxFrame = 32 << 20

// Server is the core's side of the protocol. It answers the requests of one
// client, and sends it notifications of its own: the files that changed, the
// output of a task.
type Server struct {
	service    Service
	mu         sync.Mutex
	write      func(Message) error
	streams    map[uint64]*serverStream
	nextStream uint64
	closed     bool
	// ctx is the life of the server: what the streams it holds last as
	// long as, not as long as the request that began them.
	ctx context.Context
}

// serverStream is a subscription the client holds: the files it watches.
type serverStream struct {
	id    uint64
	paths []string
	// stamps are the files as the client last saw them.
	stamps map[string]workspace.Stamp
	cancel context.CancelFunc
}

// Serve answers the requests of a client over a stream, until the stream
// ends or the context is done.
func Serve(ctx context.Context, in io.Reader, out io.Writer) error {
	server := &Server{service: Service{}, streams: map[uint64]*serverStream{}, ctx: ctx}
	writer := bufio.NewWriter(out)
	server.write = func(m Message) error {
		server.mu.Lock()
		defer server.mu.Unlock()
		if server.closed {
			return io.ErrClosedPipe
		}
		data, err := encode(m)
		if err != nil {
			return err
		}
		if _, err = writer.Write(data); err != nil {
			return err
		}
		return writer.Flush()
	}
	defer server.close()
	return server.serve(ctx, bufio.NewReaderSize(in, 64<<10))
}

func (s *Server) close() {
	s.mu.Lock()
	s.closed = true
	streams := s.streams
	s.streams = map[uint64]*serverStream{}
	s.mu.Unlock()
	for _, stream := range streams {
		stream.cancel()
	}
}

// serve reads the requests of the client and answers them. Each request is
// answered on its own goroutine, so that a long one does not hold the
// others; a cancel message ends one.
func (s *Server) serve(ctx context.Context, reader *bufio.Reader) error {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	type request struct {
		id     uint64
		method string
		params json.RawMessage
		cancel context.CancelFunc
	}
	running := map[uint64]context.CancelFunc{}
	var mu sync.Mutex
	var wg sync.WaitGroup
	defer func() {
		cancel()
		mu.Lock()
		for _, stop := range running {
			stop()
		}
		mu.Unlock()
		wg.Wait()
	}()
	for {
		line, err := readLine(reader)
		if err != nil {
			if errors.Is(err, io.EOF) {
				return nil
			}
			return err
		}
		if len(line) == 0 {
			continue
		}
		message, err := decode(line)
		if err != nil {
			s.write(Message{Kind: KindError, Error: &Error{Code: CodeInvalid, Message: err.Error()}})
			continue
		}
		switch message.Kind {
		case KindCancel:
			mu.Lock()
			stop := running[message.ID]
			mu.Unlock()
			if stop != nil {
				stop()
			}
			continue
		case KindRequest:
			requestCtx, stop := context.WithCancel(ctx)
			mu.Lock()
			running[message.ID] = stop
			mu.Unlock()
			wg.Add(1)
			go func(m Message, requestCtx context.Context) {
				defer wg.Done()
				defer func() {
					mu.Lock()
					delete(running, m.ID)
					mu.Unlock()
					stop()
				}()
				s.answer(requestCtx, m)
			}(message, requestCtx)
		default:
			s.write(Message{Kind: KindError, Error: &Error{Code: CodeInvalid, Message: "未知的消息类型: " + string(message.Kind)}})
		}
	}
}

// answer runs one request and sends its result.
func (s *Server) answer(ctx context.Context, m Message) {
	switch m.Method {
	case "watch":
		s.startWatch(ctx, m)
		return
	case "unwatch":
		s.stopWatch(m)
		return
	case "run":
		s.startRun(ctx, m)
		return
	}
	result, err := s.service.Handle(ctx, m.Method, m.Params)
	if err != nil {
		s.write(Message{Kind: KindError, ID: m.ID, Error: asError(err)})
		return
	}
	data, err := json.Marshal(result)
	if err != nil {
		s.write(Message{Kind: KindError, ID: m.ID, Error: asError(err)})
		return
	}
	s.write(Message{Kind: KindResponse, ID: m.ID, Result: data})
}

// notify sends a message of the core's own, to the client that watches it.
func (s *Server) notify(streamID uint64, method string, params any) {
	data, err := json.Marshal(params)
	if err != nil {
		return
	}
	s.write(Message{Kind: KindNotification, Method: method, StreamID: streamID, Params: data})
}

// readLine reads one line of a message, up to the largest one.
func readLine(reader *bufio.Reader) ([]byte, error) {
	var line []byte
	for {
		chunk, err := reader.ReadSlice('\n')
		if err == nil {
			return append(line, chunk...), nil
		}
		if errors.Is(err, bufio.ErrBufferFull) {
			line = append(line, chunk...)
			if len(line) > maxFrame {
				return nil, errors.New("核心消息过大")
			}
			continue
		}
		if errors.Is(err, io.EOF) && len(line) > 0 {
			return append(line, '\n'), nil
		}
		return nil, err
	}
}
