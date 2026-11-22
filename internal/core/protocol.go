// Package core is the UI-independent editor service and its transport contract.
// It has no dependency on MyGo or a desktop runtime.
package core

import (
	"encoding/json"
	"fmt"
)

// ProtocolVersion is the version of the wire format. A client and a core
// that do not agree on it cannot talk: the handshake says so.
const ProtocolVersion = 2

// Kind is what a message is: a request, its answer, an error, a
// notification the core sends on its own, or the end of a request.
type Kind string

const (
	KindRequest      Kind = "request"
	KindResponse     Kind = "response"
	KindError        Kind = "error"
	KindNotification Kind = "notification"
	KindCancel       Kind = "cancel"
)

// Message is one frame of the protocol. It is written as one line of JSON,
// as the proxy of Lapce does: a request carries its number, an answer
// carries the number of the request it answers, and a notification carries
// none, as it is not an answer to anything.
type Message struct {
	Kind     Kind            `json:"kind"`
	ID       uint64          `json:"id,omitempty"`
	Method   string          `json:"method,omitempty"`
	Params   json.RawMessage `json:"params,omitempty"`
	Result   json.RawMessage `json:"result,omitempty"`
	Error    *Error          `json:"error,omitempty"`
	StreamID uint64          `json:"stream,omitempty"`
}

// Error is what went wrong, with a code the app can act on: a missing file
// is not the same as a conflict with another program's edit.
type Error struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

func (e *Error) Error() string { return e.Message }

// The features a core reports, and the ones the UI needs. A core without a
// feature is older than the app talking to it: the handshake says so, and a
// reconnect uploads the new core.
const (
	FeatureWatch     = "watch"      // subscriptions that push file changes
	FeatureStreamRun = "stream.run" // tasks whose output streams
	FeatureReplace   = "replace"    // replacement across the project
	FeatureSymbols   = "symbols"    // the declarations of the project
	FeatureClone     = "clone"      // cloning a repository into a folder
)

// RequiredFeatures are what the editor needs of a core.
var RequiredFeatures = []string{FeatureWatch, FeatureStreamRun, FeatureReplace, FeatureSymbols, FeatureClone}

// The codes of the errors the core reports.
const (
	CodeUnknown     = 0
	CodeInvalid     = 400 // the request was not understood
	CodeNotFound    = 404 // the file or the folder is not there
	CodeConflict    = 409 // the file changed under the editor's feet
	CodeTimeout     = 408 // the operation took too long
	CodeCanceled    = 499 // the caller gave it up
	CodeTooLarge    = 413 // the file is too large to open
	CodeNotText     = 415 // the file is not text
	CodePermission  = 403 // the core may not do that
	CodeInternal    = 500 // something went wrong in the core
	CodeUnsupported = 501 // the core does not know that operation
)

// Errorf is an error with a code, which the core reports as it is.
type Errorf struct {
	Code int
	Err  error
}

func (e *Errorf) Error() string { return e.Err.Error() }
func (e *Errorf) Unwrap() error { return e.Err }

// ErrorCode is the code an error carries, or CodeUnknown for one that has
// none.
func ErrorCode(err error) int {
	for err != nil {
		if e, ok := err.(*Errorf); ok {
			return e.Code
		}
		if e, ok := err.(*Error); ok {
			return e.Code
		}
		break
	}
	return CodeUnknown
}

// asError turns any error into the one the protocol carries.
func asError(err error) *Error {
	if err == nil {
		return nil
	}
	if e, ok := err.(*Error); ok {
		return e
	}
	code := ErrorCode(err)
	if code == CodeUnknown {
		code = CodeInternal
	}
	return &Error{Code: code, Message: err.Error()}
}

// encode writes a message as one line of JSON.
func encode(m Message) ([]byte, error) {
	data, err := json.Marshal(m)
	if err != nil {
		return nil, err
	}
	return append(data, '\n'), nil
}

// decode reads one message from a line of JSON.
func decode(line []byte) (Message, error) {
	var m Message
	if err := json.Unmarshal(line, &m); err != nil {
		return m, fmt.Errorf("无法解析核心消息: %w", err)
	}
	return m, nil
}
