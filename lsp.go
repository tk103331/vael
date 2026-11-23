package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/egoist/mygo/ui"
	"github.com/egoist/mygo/ui/editor"
	"vael/internal/core"
)

// Language servers: the editor speaks LSP to them, and the core runs them
// where the files are. This file is the editor's half: the servers it
// knows, the documents it keeps in step, and what the editor does with
// what they answer.

// lspServer is a language server Vael knows: what it serves, and how it is
// started.
type lspServer struct {
	Language   string
	Name       string
	Command    string
	Extensions []string
}

// lspRegistry are the servers the editor offers, as the servers of an IDE
// are offered: a project may change the command of one, or turn it off.
var lspRegistry = []lspServer{
	{Language: "go", Name: "gopls", Command: "gopls", Extensions: []string{".go"}},
	{Language: "typescript", Name: "typescript-language-server", Command: "typescript-language-server --stdio",
		Extensions: []string{".ts", ".tsx", ".js", ".jsx", ".mjs", ".cjs"}},
	{Language: "rust", Name: "rust-analyzer", Command: "rust-analyzer", Extensions: []string{".rs"}},
	{Language: "python", Name: "pyright", Command: "pyright-langserver --stdio",
		Extensions: []string{".py", ".pyi"}},
	{Language: "c", Name: "clangd", Command: "clangd",
		Extensions: []string{".c", ".h", ".cc", ".cpp", ".cxx", ".hpp"}},
	{Language: "lua", Name: "lua-language-server", Command: "lua-language-server", Extensions: []string{".lua"}},
}

// lspSetting is what a language does with its server: whether it starts
// one, and with which command.
type lspSetting struct {
	Enabled bool   `json:"enabled"`
	Command string `json:"command"`
}

// lspSettings are the language servers as the user set them.
type lspSettings struct {
	Languages map[string]lspSetting `json:"languages"`
}

// loadLSPSettings reads the settings of the language servers, filling in
// the servers Vael knows when there are none.
func (a *app) loadLSPSettings() {
	a.lspSet = lspSettings{Languages: map[string]lspSetting{}}
	a.lspOn, a.lspCmd = map[string]bool{}, map[string]*string{}
	for _, server := range lspRegistry {
		a.lspSet.Languages[server.Language] = lspSetting{Enabled: true, Command: server.Command}
	}
	a.bindLSPSettings()
	if a.lspPath == "" {
		return
	}
	data, err := os.ReadFile(a.lspPath)
	if err != nil {
		return
	}
	var saved lspSettings
	if json.Unmarshal(data, &saved) != nil {
		return
	}
	for language, setting := range saved.Languages {
		if _, known := a.lspSet.Languages[language]; known {
			a.lspSet.Languages[language] = setting
		}
	}
	a.bindLSPSettings()
}

// bindLSPSettings gives the settings panel something to hold: a switch of
// a language, and the command it starts.
func (a *app) bindLSPSettings() {
	a.lspOn, a.lspCmd = map[string]bool{}, map[string]*string{}
	for _, server := range lspRegistry {
		setting, ok := a.lspSet.Languages[server.Language]
		if !ok {
			setting = lspSetting{Enabled: true, Command: server.Command}
			a.lspSet.Languages[server.Language] = setting
		}
		a.lspOn[server.Language] = setting.Enabled
		command := setting.Command
		a.lspCmd[server.Language] = &command
	}
}

// applyLSPSettings writes what the panel holds, and stops the servers that
// were turned off.
func (a *app) applyLSPSettings() {
	if a.lspSet.Languages == nil {
		return
	}
	for language, setting := range a.lspSet.Languages {
		if on, ok := a.lspOn[language]; ok {
			setting.Enabled = on
		}
		if command, ok := a.lspCmd[language]; ok && command != nil && strings.TrimSpace(*command) != "" {
			setting.Command = strings.TrimSpace(*command)
		}
		a.lspSet.Languages[language] = setting
	}
	a.saveLSPSettings()
	for language, session := range a.lsps {
		if setting, ok := a.lspSet.Languages[language]; ok && !setting.Enabled {
			session.stop()
			delete(a.lsps, language)
		}
	}
}

// saveLSPSettings writes what the settings say.
func (a *app) saveLSPSettings() {
	if a.lspPath == "" {
		return
	}
	data, err := json.MarshalIndent(a.lspSet, "", "  ")
	if err != nil {
		return
	}
	a.report(os.WriteFile(a.lspPath, data, 0o644))
}

// lspServerFor is the server of a file: the one whose extensions match it,
// when the settings turned it on.
func (a *app) lspServerFor(path string) *lspServer {
	if a.lspSet.Languages == nil {
		// The settings were not read: no server runs, as in a test.
		return nil
	}
	ext := strings.ToLower(filepath.Ext(path))
	for i := range lspRegistry {
		server := &lspRegistry[i]
		for _, known := range server.Extensions {
			if ext != known {
				continue
			}
			setting, ok := a.lspSet.Languages[server.Language]
			if ok && !setting.Enabled {
				return nil
			}
			return server
		}
	}
	return nil
}

// lspCommand is how a server of a language starts, as the settings say.
func (a *app) lspCommand(language string) (string, []string) {
	setting := a.lspSet.Languages[language]
	line := strings.TrimSpace(setting.Command)
	if line == "" {
		for _, server := range lspRegistry {
			if server.Language == language {
				line = server.Command
			}
		}
	}
	return splitCommand(line)
}

// splitCommand reads a command line into the program and its arguments.
func splitCommand(line string) (string, []string) {
	fields := strings.Fields(line)
	if len(fields) == 0 {
		return "", nil
	}
	return fields[0], fields[1:]
}

// lspSession is a language server of the project, and the documents of the
// editor it holds.
type lspSession struct {
	language string
	server   lspServer
	wire     *lspWire

	mu       sync.Mutex
	status   string // 启动中, 就绪, 错误, 已停止
	err      string
	docs     map[string]*lspDoc // by path
	encoding string
	problems int
}

// lspDoc is a document the server has, as the editor last sent it.
type lspDoc struct {
	path string
	uri  string
	// version is the document's version in LSP, which only grows, and
	// sent the editor's version it was sent at.
	version int
	sent    uint64
	text    string
}

// lspSessionFor is the session of a language, started when it is not yet.
func (a *app) lspSessionFor(language string) *lspSession {
	if session, ok := a.lsps[language]; ok {
		return session
	}
	if a.root == "" || a.backend == nil {
		return nil
	}
	if setting, ok := a.lspSet.Languages[language]; ok && !setting.Enabled {
		return nil
	}
	command, args := a.lspCommand(language)
	if command == "" {
		return nil
	}
	var server lspServer
	for _, known := range lspRegistry {
		if known.Language == language {
			server = known
		}
	}
	session := &lspSession{language: language, server: server, status: "启动中", docs: map[string]*lspDoc{}}
	a.lsps[language] = session
	root := a.root
	generation := a.generation
	backend := a.backend
	go func() {
		conn, err := backend.LSP(context.Background(), root, command, args)
		a.update(func() {
			if generation != a.generation {
				if conn != nil {
					_ = conn.Close()
				}
				return
			}
			if err != nil {
				session.setStatus("错误", err.Error())
				return
			}
			session.wire = newLSPWire(session, conn, a)
			session.initialize(root)
		})
	}()
	return session
}

// setStatus says how a server does.
func (s *lspSession) setStatus(status, problem string) {
	s.mu.Lock()
	s.status, s.err = status, problem
	s.mu.Unlock()
}

// state says how a server does.
func (s *lspSession) state() (string, string, int) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.status, s.err, s.problems
}

// initialize handshakes with the server as LSP asks, and tells it what the
// editor can do.
func (s *lspSession) initialize(root string) {
	rootURI := fileURI(root)
	s.wire.request("initialize", map[string]any{
		"processId":  nil,
		"clientInfo": map[string]any{"name": "Vael", "version": "0.5.0"},
		"rootUri":    rootURI,
		"workspaceFolders": []any{
			map[string]any{"uri": rootURI, "name": filepath.Base(root)},
		},
		"capabilities": map[string]any{
			"general": map[string]any{"positionEncodings": []string{"utf-8", "utf-16"}},
			"workspace": map[string]any{
				"workspaceFolders": true,
				"configuration":    true,
			},
			"textDocument": map[string]any{
				"synchronization": map[string]any{"didSave": true, "dynamicRegistration": false},
				"hover":           map[string]any{"contentFormat": []string{"markdown", "plaintext"}},
				"completion": map[string]any{
					"completionItem": map[string]any{
						"snippetSupport":      false,
						"documentationFormat": []string{"markdown", "plaintext"},
					},
				},
				"definition": map[string]any{},
				"formatting": map[string]any{},
				"publishDiagnostics": map[string]any{
					"versionSupport": true,
				},
			},
			"window": map[string]any{"workDoneProgress": false},
		},
	}, func(result json.RawMessage, err error) {
		if err != nil {
			s.setStatus("错误", err.Error())
			return
		}
		var answer struct {
			PositionEncoding string `json:"positionEncoding"`
		}
		_ = json.Unmarshal(result, &answer)
		s.mu.Lock()
		s.encoding = answer.PositionEncoding
		s.mu.Unlock()
		if s.encoding == "" {
			s.encoding = "utf-16"
		}
		s.wire.notify("initialized", map[string]any{})
		s.setStatus("就绪", "")
		// The files that are open were opened before the server was
		// ready: tell it of them now.
		a := s.wire.app
		a.update(func() { a.lspOpenFiles(s) })
	})
}

// didOpen tells the server of a document, as it is now.
func (s *lspSession) didOpen(path, text string, sent uint64) {
	s.mu.Lock()
	doc := &lspDoc{path: path, uri: fileURI(path), version: 1, sent: sent, text: text}
	s.docs[path] = doc
	s.mu.Unlock()
	s.wire.notify("textDocument/didOpen", map[string]any{
		"textDocument": map[string]any{
			"uri": doc.uri, "languageId": s.language, "version": 1, "text": text,
		},
	})
}

// didChange tells the server of the document as it is now. The whole text
// is sent: an editor of a few megabytes over a pipe can afford it, and it
// cannot drift.
func (s *lspSession) didChange(path, text string, sent uint64) {
	s.mu.Lock()
	doc := s.docs[path]
	if doc == nil {
		s.mu.Unlock()
		return
	}
	doc.version++
	doc.sent, doc.text = sent, text
	version := doc.version
	s.mu.Unlock()
	s.wire.notify("textDocument/didChange", map[string]any{
		"textDocument":   map[string]any{"uri": doc.uri, "version": version},
		"contentChanges": []any{map[string]any{"text": text}},
	})
}

// didSave tells the server that a document was saved.
func (s *lspSession) didSave(path, text string) {
	s.mu.Lock()
	doc := s.docs[path]
	s.mu.Unlock()
	if doc == nil {
		return
	}
	s.wire.notify("textDocument/didSave", map[string]any{
		"textDocument": map[string]any{"uri": doc.uri},
		"text":         text,
	})
}

// didClose tells the server that a document is gone.
func (s *lspSession) didClose(path string) {
	s.mu.Lock()
	doc := s.docs[path]
	delete(s.docs, path)
	s.mu.Unlock()
	if doc == nil {
		return
	}
	s.wire.notify("textDocument/didClose", map[string]any{
		"textDocument": map[string]any{"uri": doc.uri},
	})
}

// docSent is the editor's version of a document the server has, and
// whether it has it at all.
func (s *lspSession) docSent(path string) (uint64, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if doc := s.docs[path]; doc != nil {
		return doc.sent, true
	}
	return 0, false
}

// stop ends the session.
func (s *lspSession) stop() {
	if s.wire != nil {
		_ = s.wire.conn.Close()
	}
	s.setStatus("已停止", "")
}

// lspWire is the protocol over the pipes of the server: framed messages, the
// answers that come to the requests the editor sent, and what the server
// says on its own.
type lspWire struct {
	app  *app
	conn *core.LSP
	sess *lspSession

	mu      sync.Mutex
	nextID  int
	waiting map[int]func(json.RawMessage, error)
	buffer  []byte
}

func newLSPWire(session *lspSession, conn *core.LSP, app *app) *lspWire {
	wire := &lspWire{app: app, conn: conn, sess: session, waiting: map[int]func(json.RawMessage, error){}}
	go wire.read()
	return wire
}

// read reads the messages of the server, a frame at a time.
func (w *lspWire) read() {
	messages := w.conn.Messages()
	for message := range messages {
		switch message.Method {
		case "lsp/output":
			var payload struct {
				Data string `json:"data"`
			}
			if json.Unmarshal(message.Params, &payload) != nil {
				continue
			}
			for _, frame := range w.frames([]byte(payload.Data)) {
				w.dispatch(frame)
			}
		case "lsp/exit":
			var payload struct {
				Log string `json:"log"`
			}
			_ = json.Unmarshal(message.Params, &payload)
			w.app.update(func() {
				w.sess.setStatus("已停止", strings.TrimSpace(payload.Log))
			})
			return
		}
	}
	w.app.update(func() { w.sess.setStatus("已停止", "") })
}

// frames cuts the bytes the server wrote into the JSON messages of LSP,
// which are framed by a header holding their length.
func (w *lspWire) frames(data []byte) []json.RawMessage {
	w.mu.Lock()
	w.buffer = append(w.buffer, data...)
	var out []json.RawMessage
	for {
		end := -1
		header := 0
		for i := 0; i+3 < len(w.buffer); i++ {
			if w.buffer[i] == '\r' && w.buffer[i+1] == '\n' && w.buffer[i+2] == '\r' && w.buffer[i+3] == '\n' {
				header, end = i, i+4
				break
			}
		}
		if end < 0 {
			break
		}
		length := 0
		for _, line := range strings.Split(string(w.buffer[:header]), "\r\n") {
			key, value, ok := strings.Cut(line, ":")
			if ok && strings.EqualFold(strings.TrimSpace(key), "content-length") {
				length, _ = strconv.Atoi(strings.TrimSpace(value))
			}
		}
		if length <= 0 || len(w.buffer) < end+length {
			break
		}
		out = append(out, json.RawMessage(append([]byte(nil), w.buffer[end:end+length]...)))
		w.buffer = w.buffer[end+length:]
	}
	w.mu.Unlock()
	return out
}

// dispatch gives an answer to the request that waits for it, and everything
// else to the editor.
func (w *lspWire) dispatch(data json.RawMessage) {
	var message struct {
		ID     *int            `json:"id"`
		Method string          `json:"method"`
		Params json.RawMessage `json:"params"`
		Result json.RawMessage `json:"result"`
		Error  *struct {
			Message string `json:"message"`
		} `json:"error"`
	}
	if json.Unmarshal(data, &message) != nil {
		return
	}
	if message.Method == "" && message.ID != nil {
		w.mu.Lock()
		done := w.waiting[*message.ID]
		delete(w.waiting, *message.ID)
		w.mu.Unlock()
		if done == nil {
			return
		}
		if message.Error != nil {
			done(nil, fmt.Errorf("%s", message.Error.Message))
			return
		}
		done(message.Result, nil)
		return
	}
	if message.ID != nil {
		// A request of the server: the editor answers what it can, and
		// says so of what it cannot, so that the server does not wait.
		switch message.Method {
		case "workspace/configuration":
			w.reply(*message.ID, []any{})
		case "workspace/applyEdit":
			w.reply(*message.ID, map[string]any{"applied": false})
		case "window/workDoneProgress/create":
			w.reply(*message.ID, nil)
		case "client/registerCapability", "client/unregisterCapability":
			w.reply(*message.ID, nil)
		default:
			w.replyError(*message.ID, -32601, "Vael 不支持 "+message.Method)
		}
		return
	}
	w.app.update(func() {
		switch message.Method {
		case "textDocument/publishDiagnostics":
			w.diagnostics(message.Params)
		case "window/showMessage":
			var payload struct {
				Message string `json:"message"`
			}
			if json.Unmarshal(message.Params, &payload) == nil {
				w.app.message = "语言服务器：" + payload.Message
			}
		}
	})
}

// request sends a request and gives the answer to onAnswer, which runs on
// the editor's thread.
func (w *lspWire) request(method string, params any, onAnswer func(json.RawMessage, error)) {
	w.mu.Lock()
	w.nextID++
	id := w.nextID
	w.waiting[id] = func(result json.RawMessage, err error) {
		w.app.update(func() { onAnswer(result, err) })
	}
	w.mu.Unlock()
	message := map[string]any{"jsonrpc": "2.0", "id": id, "method": method}
	if params != nil {
		message["params"] = params
	}
	data, err := json.Marshal(message)
	if err != nil {
		return
	}
	go func() {
		if err := w.conn.Write(context.Background(), frame(data)); err != nil {
			w.app.update(func() {
				w.sess.setStatus("错误", err.Error())
			})
		}
	}()
}

// notify sends a notification, which no answer comes for.
func (w *lspWire) notify(method string, params any) {
	message := map[string]any{"jsonrpc": "2.0", "method": method}
	if params != nil {
		message["params"] = params
	}
	data, err := json.Marshal(message)
	if err != nil {
		return
	}
	go func() { _ = w.conn.Write(context.Background(), frame(data)) }()
}

// reply answers a request of the server.
func (w *lspWire) reply(id int, result any) {
	data, err := json.Marshal(map[string]any{"jsonrpc": "2.0", "id": id, "result": result})
	if err != nil {
		return
	}
	go func() { _ = w.conn.Write(context.Background(), frame(data)) }()
}

// replyError answers a request of the server that the editor cannot do.
func (w *lspWire) replyError(id, code int, message string) {
	data, err := json.Marshal(map[string]any{
		"jsonrpc": "2.0", "id": id,
		"error": map[string]any{"code": code, "message": message},
	})
	if err != nil {
		return
	}
	go func() { _ = w.conn.Write(context.Background(), frame(data)) }()
}

// frame wraps a message in the header LSP reads its length from.
func frame(data []byte) string {
	return "Content-Length: " + strconv.Itoa(len(data)) + "\r\n\r\n" + string(data)
}

// fileURI is the URI of a file, as LSP names it.
func fileURI(path string) string {
	path = filepath.ToSlash(filepath.Clean(path))
	if !strings.HasPrefix(path, "/") {
		path = "/" + path
	}
	return "file://" + percentEncode(path)
}

// filePath is the path a URI names.
func filePath(uri string) string {
	if !strings.HasPrefix(uri, "file://") {
		return ""
	}
	path := strings.TrimPrefix(uri, "file://")
	return percentDecode(path)
}

// percentEncode keeps the characters a URI may hold, and escapes the rest.
func percentEncode(s string) string {
	var out strings.Builder
	for _, b := range []byte(s) {
		switch {
		case 'a' <= b && b <= 'z', 'A' <= b && b <= 'Z', '0' <= b && b <= '9',
			b == '/', b == '-', b == '_', b == '.', b == '~':
			out.WriteByte(b)
		default:
			fmt.Fprintf(&out, "%%%02X", b)
		}
	}
	return out.String()
}

// percentDecode undoes percentEncode.
func percentDecode(s string) string {
	var out strings.Builder
	for i := 0; i < len(s); i++ {
		if s[i] == '%' && i+2 < len(s) {
			if value, err := strconv.ParseUint(s[i+1:i+3], 16, 8); err == nil {
				out.WriteByte(byte(value))
				i += 2
				continue
			}
		}
		out.WriteByte(s[i])
	}
	return out.String()
}

// normalizeURI is the URI of a file, without what the editors of two
// machines write differently.
func normalizeURI(uri string) string {
	return fileURI(filePath(uri))
}

// languageOf is the language of a file, for a server.
func languageOf(path string) string {
	ext := strings.ToLower(filepath.Ext(path))
	for _, server := range lspRegistry {
		for _, known := range server.Extensions {
			if ext == known {
				return server.Language
			}
		}
	}
	return ""
}

// lspPosition is a place in a document, as LSP names it: a line and a
// character of it, from 0.
type lspPosition struct {
	Line      int `json:"line"`
	Character int `json:"character"`
}

// lspRange is a span of a document.
type lspRange struct {
	Start lspPosition `json:"start"`
	End   lspPosition `json:"end"`
}

// offsetIn is the offset of a position in a text, in the encoding the
// server chose: UTF-16 code units, UTF-8 bytes, or runes.
func offsetIn(text string, at lspPosition, encoding string) int {
	line := 0
	i := 0
	for line < at.Line && i < len(text) {
		if text[i] == '\n' {
			line++
		}
		i++
	}
	units := 0
	for i < len(text) && text[i] != '\n' {
		if units >= at.Character {
			break
		}
		r, size := decodeRune(text[i:])
		switch encoding {
		case "utf-8":
			units += size
		case "utf-32":
			units++
		default:
			if r > 0xFFFF {
				units += 2
			} else {
				units++
			}
		}
		i += size
	}
	return i
}

// positionIn is the position of an offset, in the server's encoding.
func positionIn(text string, offset int, encoding string) lspPosition {
	if offset > len(text) {
		offset = len(text)
	}
	line, start := 0, 0
	for i := 0; i < offset; i++ {
		if text[i] == '\n' {
			line, start = line+1, i+1
		}
	}
	units := 0
	for i := start; i < offset; {
		r, size := decodeRune(text[i:])
		switch encoding {
		case "utf-8":
			units += size
		case "utf-32":
			units++
		default:
			if r > 0xFFFF {
				units += 2
			} else {
				units++
			}
		}
		i += size
	}
	return lspPosition{Line: line, Character: units}
}

// lspDiagnostic is a problem a server found.
type lspDiagnostic struct {
	Range struct {
		Start lspPosition `json:"start"`
		End   lspPosition `json:"end"`
	} `json:"range"`
	Severity int    `json:"severity"`
	Message  string `json:"message"`
	Source   string `json:"source"`
}

// diagnostics puts what a server found in the editor of the file, and
// counts them.
func (w *lspWire) diagnostics(params json.RawMessage) {
	var payload struct {
		URI         string          `json:"uri"`
		Version     *int            `json:"version"`
		Diagnostics []lspDiagnostic `json:"diagnostics"`
	}
	if json.Unmarshal(params, &payload) != nil {
		return
	}
	path := filePath(payload.URI)
	tab := w.app.tabOf(path)
	if tab == nil || tab.state == nil {
		return
	}
	text := tab.state.Buffer.String()
	encoding := w.sess.encoding
	var found []editor.Diagnostic
	for _, diagnostic := range payload.Diagnostics {
		start := offsetIn(text, diagnostic.Range.Start, encoding)
		end := offsetIn(text, diagnostic.Range.End, encoding)
		if end < start {
			start, end = end, start
		}
		severity := editor.SeverityError
		switch diagnostic.Severity {
		case 2:
			severity = editor.SeverityWarning
		case 3:
			severity = editor.SeverityInfo
		case 4:
			severity = editor.SeverityHint
		}
		found = append(found, editor.Diagnostic{
			Start: start, End: end, Severity: severity,
			Message: diagnostic.Message, Source: diagnostic.Source,
		})
	}
	tab.state.SetDiagnostics(tab.state.Buffer.Version(), found)
	w.sess.mu.Lock()
	w.sess.problems = len(found)
	w.sess.mu.Unlock()
}

// tabOf is the tab of a path that is open.
func (a *app) tabOf(path string) *fileTab {
	clean := filepath.Clean(path)
	for _, tab := range a.tabs {
		if filepath.Clean(tab.path) == clean && tab.state != nil {
			return tab
		}
	}
	return nil
}

// lspOpenFiles tells a server of every file that is open.
func (a *app) lspOpenFiles(session *lspSession) {
	for _, tab := range a.tabs {
		if tab.state == nil || tab.diff != nil {
			continue
		}
		if a.lspServerFor(tab.path) == nil || languageOf(tab.path) != session.language {
			continue
		}
		session.didOpen(tab.path, tab.state.Buffer.String(), tab.state.Buffer.Version())
	}
}

// lspSync keeps the server in step with the buffer of a file, a moment
// after the text stopped changing.
func (a *app) lspSync(tab *fileTab) {
	if tab == nil || tab.state == nil || tab.diff != nil {
		return
	}
	server := a.lspServerFor(tab.path)
	if server == nil {
		return
	}
	session := a.lsps[server.Language]
	if session == nil {
		return
	}
	if _, ok := session.docSent(tab.path); !ok {
		return
	}
	stamp := tab.state.Buffer.Version()
	if sent, _ := session.docSent(tab.path); sent >= stamp {
		return
	}
	path := tab.path
	time.AfterFunc(350*time.Millisecond, func() {
		a.update(func() {
			if tab.state == nil {
				return
			}
			if tab.state.Buffer.Version() != stamp {
				// It changed again: whatever is synced now is stale,
				// and the change that came will sync it.
				return
			}
			session.didChange(path, tab.state.Buffer.String(), stamp)
		})
	})
}

// lspComplete asks the server for completions where the caret is, and
// shows what it answers.
func (a *app) lspComplete(tab *fileTab, request editor.CompletionRequest) bool {
	if tab == nil || tab.state == nil {
		return false
	}
	server := a.lspServerFor(tab.path)
	if server == nil {
		return false
	}
	session := a.lsps[server.Language]
	if session == nil || session.wire == nil {
		return false
	}
	text := tab.state.Buffer.String()
	at := positionIn(text, request.Offset, session.encoding)
	path, version, state := tab.path, request.Version, tab.state
	session.wire.request("textDocument/completion", map[string]any{
		"textDocument": map[string]any{"uri": fileURI(path)},
		"position":     at,
	}, func(result json.RawMessage, err error) {
		if err != nil {
			return
		}
		items := lspCompletions(result, session.encoding, text, request.Offset)
		if len(items) == 0 || state.Buffer.Version() != version {
			return
		}
		state.ShowCompletions(request.Offset, items)
	})
	return true
}

// lspCompletions reads what a server answered a completion request with.
func lspCompletions(result json.RawMessage, encoding, text string, offset int) []editor.Completion {
	var items struct {
		IsIncomplete bool `json:"isIncomplete"`
		Items        []struct {
			Label         string `json:"label"`
			Kind          int    `json:"kind"`
			Detail        string `json:"detail"`
			InsertText    string `json:"insertText"`
			Documentation any    `json:"documentation"`
			TextEdit      *struct {
				NewText string `json:"newText"`
			} `json:"textEdit"`
		} `json:"items"`
	}
	if json.Unmarshal(result, &items) != nil {
		// A server may answer the list itself.
		var plain []struct {
			Label string `json:"label"`
		}
		if json.Unmarshal(result, &plain) != nil {
			return nil
		}
		items.Items = nil
	}
	var out []editor.Completion
	for _, item := range items.Items {
		insert := item.InsertText
		if item.TextEdit != nil && item.TextEdit.NewText != "" {
			insert = item.TextEdit.NewText
		}
		if insert == "" {
			insert = item.Label
		}
		out = append(out, editor.Completion{
			Label:         item.Label,
			Detail:        item.Detail,
			Kind:          completionKind(item.Kind),
			Insert:        insert,
			Documentation: markupText(item.Documentation),
		})
	}
	if len(out) > 200 {
		out = out[:200]
	}
	return out
}

// completionKind is what an LSP kind of completion is in the editor.
func completionKind(kind int) editor.CompletionKind {
	switch kind {
	case 3:
		return editor.CompletionFunction
	case 2, 4, 5, 6:
		return editor.CompletionMethod
	case 7:
		return editor.CompletionType
	case 8, 9:
		return editor.CompletionModule
	case 12:
		return editor.CompletionConstant
	case 14:
		return editor.CompletionKeyword
	case 20:
		return editor.CompletionVariable
	case 15:
		return editor.CompletionSnippet
	}
	return editor.CompletionText
}

// markupText reads what a server wrote for people: a string, or markdown.
func markupText(value any) string {
	switch typed := value.(type) {
	case string:
		return typed
	case map[string]any:
		if text, ok := typed["value"].(string); ok {
			return text
		}
	}
	return ""
}

// lspHover asks the server what the text at a place is.
func (a *app) lspHover(tab *fileTab, request editor.HoverRequest) bool {
	if tab == nil || tab.state == nil {
		return false
	}
	server := a.lspServerFor(tab.path)
	if server == nil {
		return false
	}
	session := a.lsps[server.Language]
	if session == nil || session.wire == nil {
		return false
	}
	text := tab.state.Buffer.String()
	at := positionIn(text, request.Offset, session.encoding)
	path, version, state := tab.path, request.Version, tab.state
	start := request.Offset
	word := wordRangeAt(text, request.Offset)
	session.wire.request("textDocument/hover", map[string]any{
		"textDocument": map[string]any{"uri": fileURI(path)},
		"position":     at,
	}, func(result json.RawMessage, err error) {
		if err != nil || len(result) == 0 || string(result) == "null" {
			return
		}
		var answer struct {
			Contents any `json:"contents"`
		}
		if json.Unmarshal(result, &answer) != nil {
			return
		}
		body := markupText(answer.Contents)
		if body == "" {
			if list, ok := answer.Contents.([]any); ok {
				var parts []string
				for _, part := range list {
					if text := markupText(part); text != "" {
						parts = append(parts, text)
					}
				}
				body = strings.Join(parts, "\n\n")
			}
		}
		if body == "" || state.Buffer.Version() != version {
			return
		}
		state.ShowHover(word[0], word[1], body, nil)
	})
	_ = start
	return true
}

// wordRangeAt is the span of the word around an offset.
func wordRangeAt(text string, offset int) [2]int {
	if offset > len(text) {
		offset = len(text)
	}
	start, end := offset, offset
	for start > 0 && isWordByte(text[start-1]) {
		start--
	}
	for end < len(text) && isWordByte(text[end]) {
		end++
	}
	return [2]int{start, end}
}

// isWordByte tells whether a byte is part of a word, as far as the editor
// cares.
func isWordByte(b byte) bool {
	return b == '_' || b >= 0x80 || 'a' <= b && b <= 'z' || 'A' <= b && b <= 'Z' || '0' <= b && b <= '9'
}

// lspDefinition asks the server where a place is defined, and opens it.
func (a *app) lspDefinition(tab *fileTab, offset int) bool {
	if tab == nil || tab.state == nil {
		return false
	}
	server := a.lspServerFor(tab.path)
	if server == nil {
		return false
	}
	session := a.lsps[server.Language]
	if session == nil || session.wire == nil {
		return false
	}
	text := tab.state.Buffer.String()
	at := positionIn(text, offset, session.encoding)
	session.wire.request("textDocument/definition", map[string]any{
		"textDocument": map[string]any{"uri": fileURI(tab.path)},
		"position":     at,
	}, func(result json.RawMessage, err error) {
		if err != nil {
			return
		}
		where, ok := lspLocation(result)
		if !ok {
			a.message = "没有找到定义"
			return
		}
		a.pendingCaret[where.path] = editor.Pos{Line: where.line, Col: where.character}
		a.openFile(where.path)
	})
	return true
}

// lspLocation is a place a definition is at.
type lspPlace struct {
	path      string
	line      int
	character int
}

// lspLocation reads what a definition request answered: a place, several of
// them, or a link to one.
func lspLocation(result json.RawMessage) (lspPlace, bool) {
	var one struct {
		URI   string `json:"uri"`
		Range struct {
			Start lspPosition `json:"start"`
		} `json:"range"`
	}
	if json.Unmarshal(result, &one) == nil && one.URI != "" {
		return lspPlace{filePath(one.URI), one.Range.Start.Line, one.Range.Start.Character}, true
	}
	var many []struct {
		URI   string `json:"uri"`
		Range struct {
			Start lspPosition `json:"start"`
		} `json:"range"`
		TargetURI   string `json:"targetUri"`
		TargetRange struct {
			Start lspPosition `json:"start"`
		} `json:"targetRange"`
	}
	if json.Unmarshal(result, &many) == nil && len(many) > 0 {
		first := many[0]
		uri, start := first.URI, first.Range.Start
		if uri == "" {
			uri, start = first.TargetURI, first.TargetRange.Start
		}
		if uri != "" {
			return lspPlace{filePath(uri), start.Line, start.Character}, true
		}
	}
	return lspPlace{}, false
}

// lspFormat asks the server to format the file that is open.
func (a *app) lspFormat() {
	tab := a.active()
	if tab == nil || tab.state == nil || tab.diff != nil {
		return
	}
	if !a.formatDocument(tab, nil) {
		if a.lspServerFor(tab.path) == nil {
			a.message = "这个文件没有语言服务器"
		} else {
			a.message = "语言服务器还没就绪"
		}
	}
}

// formatDocument asks the server of a file to format it, and calls done once
// the buffer has what it answered. It is false when there is no server to
// ask.
func (a *app) formatDocument(tab *fileTab, done func()) bool {
	if tab == nil || tab.state == nil || tab.diff != nil {
		return false
	}
	server := a.lspServerFor(tab.path)
	if server == nil {
		return false
	}
	session := a.lsps[server.Language]
	if session == nil || session.wire == nil {
		return false
	}
	text := tab.state.Buffer.String()
	state := tab.state
	session.wire.request("textDocument/formatting", map[string]any{
		"textDocument": map[string]any{"uri": fileURI(tab.path)},
		"options":      map[string]any{"tabSize": 4, "insertSpaces": true},
	}, func(result json.RawMessage, err error) {
		if err != nil {
			a.message = "格式化失败：" + err.Error()
			return
		}
		var edits []struct {
			Range   lspRange `json:"range"`
			NewText string   `json:"newText"`
		}
		if json.Unmarshal(result, &edits) != nil || len(edits) == 0 {
			if done != nil {
				done()
			}
			return
		}
		var changes []editor.Edit
		for _, edit := range edits {
			start := offsetIn(text, edit.Range.Start, session.encoding)
			end := offsetIn(text, edit.Range.End, session.encoding)
			changes = append(changes, editor.Edit{Start: start, End: end, Text: edit.NewText})
		}
		state.Buffer.Edit(changes...)
		if done != nil {
			done()
		}
	})
	return true
}

// lspStatus is what the status bar says of the language servers.
func (a *app) lspStatus() string {
	session := a.lspCurrent()
	if session == nil {
		return ""
	}
	status, err, problems := session.state()
	text := session.server.Name
	switch status {
	case "就绪":
		if problems > 0 {
			text += fmt.Sprintf(" · %d 问题", problems)
		}
	case "错误":
		if err != "" {
			text += " · 错误"
		}
	case "启动中":
		text += " · 启动中"
	case "已停止":
		text += " · 已停止"
	}
	return text
}

// lspCurrent is the session of the file that is open.
func (a *app) lspCurrent() *lspSession {
	tab := a.active()
	if tab == nil {
		return nil
	}
	server := a.lspServerFor(tab.path)
	if server == nil {
		return nil
	}
	return a.lsps[server.Language]
}

// lspProblemText is what the status bar says on hover of the indicator: the
// servers that run, and what the one of this file said.
func (a *app) lspProblemText() string {
	session := a.lspCurrent()
	if session == nil {
		return ""
	}
	status, err, problems := session.state()
	text := session.server.Name + "：" + status
	if problems > 0 {
		text += fmt.Sprintf("，%d 个问题", problems)
	}
	if err != "" {
		text += "\n" + err
	}
	return text
}

// stopLSP ends every language server of the project.
func (a *app) stopLSP() {
	for _, session := range a.lsps {
		session.stop()
	}
	a.lsps = nil
}

var _ = normalizeURI

// lspDidOpen tells the language server of a file that it is open, and
// starts the server the first time a file of its language is.
func (a *app) lspDidOpen(tab *fileTab) {
	if tab == nil || tab.state == nil || tab.diff != nil {
		return
	}
	server := a.lspServerFor(tab.path)
	if server == nil {
		return
	}
	session := a.lspSessionFor(server.Language)
	if session == nil {
		return
	}
	if session.wire == nil {
		// The server is still starting: its handshake tells it of the
		// files that are open.
		return
	}
	session.didOpen(tab.path, tab.state.Buffer.String(), tab.state.Buffer.Version())
}

// lspDidSave tells the language server of a file that it was saved.
func (a *app) lspDidSave(tab *fileTab, text string) {
	if tab == nil || tab.state == nil {
		return
	}
	server := a.lspServerFor(tab.path)
	if server == nil {
		return
	}
	if session := a.lsps[server.Language]; session != nil && session.wire != nil {
		if _, ok := session.docSent(tab.path); !ok {
			session.didOpen(tab.path, text, tab.state.Buffer.Version())
		}
		session.didSave(tab.path, text)
	}
}

// lspDidClose tells the language server that a file is gone.
func (a *app) lspDidClose(tab *fileTab) {
	if tab == nil {
		return
	}
	server := a.lspServerFor(tab.path)
	if server == nil {
		return
	}
	if session := a.lsps[server.Language]; session != nil && session.wire != nil {
		session.didClose(tab.path)
	}
}

// defineAt goes to what the word at an offset is defined by: the language
// server of the file, or the symbols of the project when it has none.
func (a *app) defineAt(tab *fileTab, offset int) {
	if a.lspDefinition(tab, offset) {
		return
	}
	a.goToDefinition(tab, offset)
}

// lspEditor gives the editor of a file what the language server knows: the
// completions it offers, the hovers it writes, and how the document stays
// in step with it.
func (a *app) lspEditor(tab *fileTab) {
	if tab == nil || tab.state == nil {
		return
	}
	state := tab.state
	state.OnCompletion = func(request editor.CompletionRequest) {
		a.lspComplete(tab, request)
	}
	state.OnHover = func(request editor.HoverRequest) {
		a.lspHover(tab, request)
	}
	state.OnDefinition = func(offset int) { a.defineAt(tab, offset) }
	a.lspSync(tab)
}

// lspProblems is how many problems the server of the file found.
func (a *app) lspProblems() int {
	session := a.lspCurrent()
	if session == nil {
		return 0
	}
	_, _, problems := session.state()
	return problems
}

// lspSettingsView is the panel of the language servers: those Vael knows,
// which of them starts, and the command it starts with.
func (a *app) lspSettingsView(c *ui.Context, p palette) {
	if a.lspOn == nil {
		a.bindLSPSettings()
	}
	ui.Column(c).FillWidth().Gap(10).Children(func() {
		ui.Text(c, "语言服务器用 LSP 提供补全、悬停、转到定义与格式化。"+
			"它在项目所在的机器上运行：远端项目就在远端启动。").
			FontSize(10.5).TextColor(p.muted)
		for _, server := range lspRegistry {
			server := server
			on := a.lspOn[server.Language]
			ui.Column(c).FillWidth().Gap(4).Children(func() {
				ui.Row(c).FillWidth().Gap(8).AlignItems(ui.Center).Children(func() {
					enabled := on
					ui.Switch(c, &enabled).Label(server.Name + "开关")
					if enabled != on {
						a.lspOn[server.Language] = enabled
					}
					ui.Column(c).Grow(1).MinWidth(0).Gap(1).Children(func() {
						ui.Text(c, server.Name).FontSize(12).TextColor(p.text)
						ui.Text(c, server.Language).FontSize(10).TextColor(p.muted)
					})
					if status := a.lspServerState(server.Language); status != "" {
						ui.Text(c, status).FontSize(10).TextColor(p.muted)
					}
				})
				if buffer, ok := a.lspCmd[server.Language]; ok && buffer != nil {
					ui.TextInput(c, buffer).FillWidth().Height(26).Padding(2, 6).
						Placeholder("启动命令").Label(server.Name + " 启动命令")
				}
			})
		}
		ui.Text(c, "改动在关闭设置时生效；命令在项目目录中执行。").
			FontSize(10).TextColor(p.muted)
	})
}

// lspServerState is how the server of a language does, when one runs.
func (a *app) lspServerState(language string) string {
	session := a.lsps[language]
	if session == nil {
		return ""
	}
	status, err, problems := session.state()
	if err != "" && status == "错误" {
		return "错误：" + firstLine(err, 60)
	}
	if problems > 0 {
		return status + " · " + strconv.Itoa(problems) + " 问题"
	}
	return status
}

// firstLine keeps the first line of a text, cut to a length.
func firstLine(text string, limit int) string {
	line, _, _ := strings.Cut(text, "\n")
	if len([]rune(line)) > limit {
		return string([]rune(line)[:limit]) + "…"
	}
	return line
}

// takeCaret puts the caret where a definition named, once the file of it
// is open.
func (a *app) takeCaret(path string, tab *fileTab) {
	caret, ok := a.pendingCaret[path]
	if !ok || tab == nil || tab.state == nil {
		return
	}
	delete(a.pendingCaret, path)
	tab.state.GoTo(caret)
}

// lspTotals is how many language servers run, and how many problems they
// found in all.
func (a *app) lspTotals() (int, int) {
	running, problems := 0, 0
	for _, session := range a.lsps {
		status, _, found := session.state()
		if status == "就绪" || status == "启动中" {
			running++
		}
		problems += found
	}
	return running, problems
}

// lspTooltip is what the button of the language servers says on hover.
func (a *app) lspTooltip(running, problems int) string {
	switch {
	case running == 0:
		return "语言服务器：未启动"
	case problems > 0:
		return fmt.Sprintf("语言服务器：%d 个在运行，%d 个问题", running, problems)
	}
	return fmt.Sprintf("语言服务器：%d 个在运行", running)
}

// lspMenu is the panel the button of the language servers opens: what each
// server of a language does.
func (a *app) lspMenu(c *ui.Context, p palette) {
	ui.Column(c).Width(320).Padding(6).Gap(2).Children(func() {
		ui.Row(c).FillWidth().Height(26).Padding(0, 6).Gap(6).AlignItems(ui.Center).Children(func() {
			ui.Text(c, "语言服务器").FontSize(11.5).FontWeight(650).TextColor(p.text).Grow(1)
			if running, problems := a.lspTotals(); running > 0 {
				detail := strconv.Itoa(running) + " 个在运行"
				if problems > 0 {
					detail += " · " + strconv.Itoa(problems) + " 问题"
				}
				ui.Text(c, detail).FontSize(10).TextColor(p.muted)
			}
		})
		if len(a.lspSet.Languages) == 0 {
			ui.Text(c, "设置没有读入：语言服务器不可用。").FontSize(10.5).
				TextColor(p.muted).Padding(4, 6)
		}
		for _, server := range lspRegistry {
			server := server
			setting, known := a.lspSet.Languages[server.Language]
			if !known {
				continue
			}
			session := a.lsps[server.Language]
			status, problem, problems := "未启动", "", 0
			if session != nil {
				status, problem, problems = session.state()
			}
			row := ui.ButtonBase(c).FillWidth().Height(34).Padding(0, 6).Gap(8).
				Radius(5).AlignItems(ui.Center).Label(server.Name)
			if row.Hovered() {
				row.Background(p.edge)
			}
			row.Children(func() {
				a.lspMark(c, p, status, setting.Enabled)
				ui.Column(c).Grow(1).MinWidth(0).Gap(1).Children(func() {
					ui.Text(c, server.Name).FontSize(11.5).TextColor(p.text).SingleLine().Ellipsis("…")
					detail := server.Language + " · " + status
					if !setting.Enabled {
						detail = server.Language + " · 已关闭"
					}
					if problems > 0 {
						detail += fmt.Sprintf(" · %d 问题", problems)
					}
					if problem != "" {
						detail = server.Language + " · " + firstLine(problem, 60)
					}
					ui.Text(c, detail).FontSize(10).TextColor(p.muted).SingleLine().Ellipsis("…")
				})
			})
			if row.Clicked() {
				a.lspMenuOpen = false
				a.settingsOpen = true
				a.settingsSection = settingsLSP
			}
		}
		ui.Divider(c).Margin(4, 0)
		a.agentMenuItem(c, p, gearIcon, "语言服务器设置…", func() {
			a.lspMenuOpen = false
			a.settingsOpen = true
			a.settingsSection = settingsLSP
		})
	})
}

// lspMark is how a server does: a tick when it is ready, a dot while it
// starts, a cross when it failed, and a hollow dot when it does not run.
func (a *app) lspMark(c *ui.Context, p palette, status string, enabled bool) {
	switch {
	case !enabled:
		ui.Box(c).Size(7, 7).Radius(3.5).Border(1, p.muted)
	case status == "就绪":
		ui.Icon(c, checkIcon).Size(11, 11).TextColor(p.success)
	case status == "启动中":
		ui.Box(c).Size(7, 7).Radius(3.5).Background(p.accent)
	case status == "错误":
		ui.Icon(c, xIcon).Size(11, 11).TextColor(p.danger)
	default:
		ui.Box(c).Size(7, 7).Radius(3.5).Border(1, p.muted)
	}
}
