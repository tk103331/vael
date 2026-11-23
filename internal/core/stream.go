package core

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"io"
	"sync"
)

// Transport is what the client talks to the core through.
type Transport interface {
	Call(ctx context.Context, method string, input, output any) error
	Close() error
}

// Streaming is a transport that can hold subscriptions: the core pushes the
// messages of a subscription, as the files of a project change.
type Streaming interface {
	Transport
	Subscribe(ctx context.Context, method string, input any) (Subscription, error)
	Done() <-chan struct{}
	Err() error
}

// Subscription is a stream of messages the core pushes.
type Subscription interface {
	Messages() <-chan Message
	// Stream is the number the core knows the stream by, which requests
	// about it carry.
	Stream() uint64
	Close() error
}

// Stream is the client side of the protocol over a pair of pipes.
type Stream struct {
	input   io.WriteCloser
	output  io.ReadCloser
	stop    func() error
	writeMu sync.Mutex
	mu      sync.Mutex
	next    uint64
	pending map[uint64]chan Message
	subs    map[uint64]*subscription
	closed  error
	done    chan struct{}
	once    sync.Once
}

type subscription struct {
	mu       sync.Mutex
	items    []Message
	wait     chan struct{}
	closed   bool
	streamID uint64
	owner    *Stream
}

// NewStream makes the client side of the protocol over a pair of pipes.
func NewStream(input io.WriteCloser, output io.ReadCloser, stop func() error) *Stream {
	s := &Stream{
		input: input, output: output, stop: stop,
		pending: map[uint64]chan Message{},
		subs:    map[uint64]*subscription{},
		done:    make(chan struct{}),
	}
	go s.read()
	return s
}

func (s *Stream) Done() <-chan struct{} { return s.done }
func (s *Stream) Err() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.closed
}

// fail ends the stream: every request that waits for an answer is told the
// connection is gone.
func (s *Stream) fail(err error) {
	s.mu.Lock()
	if s.closed == nil {
		s.closed = err
		close(s.done)
	}
	pending := s.pending
	s.pending = map[uint64]chan Message{}
	subs := s.subs
	s.subs = map[uint64]*subscription{}
	s.mu.Unlock()
	for id, ch := range pending {
		ch <- Message{Kind: KindError, ID: id, Error: &Error{Code: CodeInternal, Message: err.Error()}}
	}
	for _, sub := range subs {
		sub.close()
	}
}

// read reads the messages of the core and gives them to those who wait.
func (s *Stream) read() {
	reader := bufio.NewReaderSize(s.output, 64<<10)
	for {
		line, err := readLine(reader)
		if err != nil {
			s.fail(err)
			_ = s.Close()
			return
		}
		if len(line) == 0 {
			continue
		}
		message, err := decode(line)
		if err != nil {
			continue
		}
		s.dispatch(message)
	}
}

func (s *Stream) dispatch(m Message) {
	switch m.Kind {
	case KindNotification:
		s.mu.Lock()
		sub := s.subs[m.StreamID]
		s.mu.Unlock()
		if sub != nil {
			sub.push(m)
		}
	case KindResponse, KindError:
		s.mu.Lock()
		ch := s.pending[m.ID]
		delete(s.pending, m.ID)
		s.mu.Unlock()
		if ch != nil {
			ch <- m
		}
	}
}

// write sends a message to the core.
func (s *Stream) write(m Message) error {
	data, err := encode(m)
	if err != nil {
		return err
	}
	s.writeMu.Lock()
	defer s.writeMu.Unlock()
	if _, err = s.input.Write(data); err != nil {
		s.fail(err)
		return err
	}
	return nil
}

// Call runs one request and waits for its answer.
func (s *Stream) Call(ctx context.Context, method string, input, output any) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	data, err := json.Marshal(input)
	if err != nil {
		return err
	}
	s.mu.Lock()
	if s.closed != nil {
		err := s.closed
		s.mu.Unlock()
		return err
	}
	s.next++
	id := s.next
	ch := make(chan Message, 1)
	s.pending[id] = ch
	s.mu.Unlock()
	if err = s.write(Message{Kind: KindRequest, ID: id, Method: method, Params: data}); err != nil {
		return err
	}
	select {
	case m := <-ch:
		if m.Kind == KindError {
			return errorOf(m.Error)
		}
		if output == nil {
			return nil
		}
		return json.Unmarshal(m.Result, output)
	case <-ctx.Done():
		s.mu.Lock()
		delete(s.pending, id)
		s.mu.Unlock()
		// The core is told to stop: the work it does for this request ends.
		go func() { _ = s.write(Message{Kind: KindCancel, ID: id}) }()
		return ctx.Err()
	}
}

// Subscribe holds a stream of messages the core pushes.
func (s *Stream) Subscribe(ctx context.Context, method string, input any) (Subscription, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	data, err := json.Marshal(input)
	if err != nil {
		return nil, err
	}
	s.mu.Lock()
	if s.closed != nil {
		err := s.closed
		s.mu.Unlock()
		return nil, err
	}
	s.next++
	id := s.next
	ch := make(chan Message, 1)
	s.pending[id] = ch
	s.mu.Unlock()
	if err = s.write(Message{Kind: KindRequest, ID: id, Method: method, Params: data}); err != nil {
		return nil, err
	}
	var answer Message
	select {
	case answer = <-ch:
	case <-ctx.Done():
		s.mu.Lock()
		delete(s.pending, id)
		s.mu.Unlock()
		go func() { _ = s.write(Message{Kind: KindCancel, ID: id}) }()
		return nil, ctx.Err()
	}
	if answer.Kind == KindError {
		return nil, errorOf(answer.Error)
	}
	var opened struct {
		Stream uint64 `json:"stream"`
	}
	if err = json.Unmarshal(answer.Result, &opened); err != nil {
		return nil, err
	}
	sub := &subscription{wait: make(chan struct{}), streamID: opened.Stream, owner: s}
	s.mu.Lock()
	s.subs[opened.Stream] = sub
	s.mu.Unlock()
	return sub, nil
}

// Close ends the stream and the process behind it.
func (s *Stream) Close() error {
	var err error
	s.once.Do(func() {
		_ = s.input.Close()
		_ = s.output.Close()
		if s.stop != nil {
			err = s.stop()
		}
		s.fail(errors.New("核心连接已关闭"))
	})
	return err
}

// subscription is the client's side of a stream the core pushes.
func (sub *subscription) Messages() <-chan Message {
	out := make(chan Message, 64)
	go func() {
		defer close(out)
		for {
			sub.mu.Lock()
			for len(sub.items) > 0 {
				m := sub.items[0]
				sub.items = sub.items[1:]
				sub.mu.Unlock()
				out <- m
				sub.mu.Lock()
			}
			if sub.closed {
				sub.mu.Unlock()
				return
			}
			wait := sub.wait
			sub.mu.Unlock()
			select {
			case <-wait:
			case <-sub.owner.done:
				return
			}
		}
	}()
	return out
}

func (sub *subscription) push(m Message) {
	sub.mu.Lock()
	sub.items = append(sub.items, m)
	close(sub.wait)
	sub.wait = make(chan struct{})
	sub.mu.Unlock()
}

func (sub *subscription) close() {
	sub.mu.Lock()
	if !sub.closed {
		sub.closed = true
		close(sub.wait)
		sub.wait = make(chan struct{})
	}
	sub.mu.Unlock()
}

// Stream is the number the core knows the stream by.
func (sub *subscription) Stream() uint64 { return sub.streamID }

// Close ends the subscription: the core stops sending, and the work it does
// for it ends.
func (sub *subscription) Close() error {
	sub.owner.mu.Lock()
	delete(sub.owner.subs, sub.streamID)
	sub.owner.mu.Unlock()
	sub.close()
	ctx, cancel := context.WithTimeout(context.Background(), 5_000_000_000)
	defer cancel()
	return sub.owner.Call(ctx, "unwatch", map[string]uint64{"stream": sub.streamID}, nil)
}

// errorOf turns the error of a message into one the app can read.
func errorOf(e *Error) error {
	if e == nil {
		return errors.New("核心返回了空的错误")
	}
	return e
}
