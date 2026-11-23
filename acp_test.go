package main

import (
	"bufio"
	"context"
	"encoding/json"
	"os"
	"testing"
	"time"
)

// TestACPAgentHelper is the agent the test in acpConnSpeaks starts: a
// process that reads the protocol from its standard input, and answers
// it. It does nothing unless the test asked for it by name.
func TestACPAgentHelper(t *testing.T) {
	if os.Getenv("VAEL_ACP_AGENT") == "" {
		return
	}
	in := bufio.NewScanner(os.Stdin)
	out := os.Stdout
	const permissionID = 100
	promptID := 0
	write := func(message map[string]any) {
		data, _ := json.Marshal(message)
		out.Write(append(data, '\n'))
	}
	for in.Scan() {
		var message struct {
			ID     *int            `json:"id"`
			Method string          `json:"method"`
			Params json.RawMessage `json:"params"`
		}
		if json.Unmarshal(in.Bytes(), &message) != nil || message.ID == nil {
			continue
		}
		if message.Method == "" {
			// The answer to a request of its own: the turn that asked
			// for it ends now.
			if message.ID != nil && *message.ID == permissionID && promptID != 0 {
				write(map[string]any{
					"jsonrpc": "2.0", "id": promptID,
					"result": map[string]any{"stopReason": "end_turn"},
				})
			}
			continue
		}
		switch message.Method {
		case "initialize":
			write(map[string]any{
				"jsonrpc": "2.0", "id": *message.ID,
				"result": map[string]any{
					"protocolVersion":   acpProtocolVersion,
					"agentCapabilities": map[string]any{"loadSession": true},
					"agentInfo":         map[string]any{"name": "Helper", "version": "1"},
				},
			})
		case "session/new":
			write(map[string]any{
				"jsonrpc": "2.0", "id": *message.ID,
				"result": map[string]any{"sessionId": "s1"},
			})
		case "session/prompt":
			if promptWantsTool(message.Params) {
				// A tool, its output, a plan, and a request for what the
				// user allows.
				promptID = *message.ID
				write(map[string]any{
					"jsonrpc": "2.0", "method": "session/update",
					"params": map[string]any{
						"sessionId": "s1",
						"update": map[string]any{
							"sessionUpdate": "tool_call",
							"toolCallId":    "t1",
							"title":         "运行 go test",
							"kind":          "execute",
							"status":        "in_progress",
						},
					},
				})
				write(map[string]any{
					"jsonrpc": "2.0", "method": "session/update",
					"params": map[string]any{
						"sessionId": "s1",
						"update": map[string]any{
							"sessionUpdate": "tool_call_update",
							"toolCallId":    "t1",
							"status":        "completed",
							"content": []any{map[string]any{
								"type":    "content",
								"content": map[string]any{"type": "text", "text": "ok"},
							}},
						},
					},
				})
				write(map[string]any{
					"jsonrpc": "2.0", "method": "session/update",
					"params": map[string]any{
						"sessionId": "s1",
						"update": map[string]any{
							"sessionUpdate": "plan",
							"entries": []any{
								map[string]any{"content": "读代码", "priority": "high", "status": "completed"},
								map[string]any{"content": "改代码", "priority": "high", "status": "in_progress"},
								map[string]any{"content": "跑测试", "priority": "medium", "status": "pending"},
							},
						},
					},
				})
				write(map[string]any{
					"jsonrpc": "2.0", "id": permissionID,
					"method": "session/request_permission",
					"params": map[string]any{
						"sessionId": "s1",
						"toolCall": map[string]any{
							"toolCallId": "t2", "title": "写文件", "kind": "edit",
						},
						"options": []any{
							map[string]any{"optionId": "once", "name": "允许一次", "kind": "allow_once"},
							map[string]any{"optionId": "always", "name": "总是允许", "kind": "allow_always"},
							map[string]any{"optionId": "no", "name": "拒绝", "kind": "reject_once"},
						},
					},
				})
				continue
			}
			if promptWants(message.Params, "问题") {
				write(map[string]any{
					"jsonrpc": "2.0", "id": *message.ID,
					"result": map[string]any{"stopReason": "input_required"},
				})
				continue
			}
			write(map[string]any{
				"jsonrpc": "2.0", "method": "session/update",
				"params": map[string]any{
					"sessionId": "s1",
					"update": map[string]any{
						"sessionUpdate": "agent_message_chunk",
						"content":       map[string]any{"type": "text", "text": "你好"},
					},
				},
			})
			write(map[string]any{
				"jsonrpc": "2.0", "id": *message.ID,
				"result": map[string]any{"stopReason": "end_turn"},
			})
		default:
			write(map[string]any{
				"jsonrpc": "2.0", "id": *message.ID,
				"error": map[string]any{"code": -32601, "message": "no"},
			})
		}
	}
	os.Exit(0)
}

func TestACPConnSpeaks(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	updates := make(chan acpUpdate, 4)
	command := "VAEL_ACP_AGENT=1 " + os.Args[0] + " -test.run=TestACPAgentHelper"
	conn, err := startACP(ctx, command, nil, func(message acpMessage) {
		if message.Method != "session/update" {
			return
		}
		var update acpUpdate
		if json.Unmarshal(message.Params, &update) == nil {
			updates <- update
		}
	})
	if err != nil {
		t.Fatal(err)
	}
	defer conn.close()
	var init acpInitialize
	if err := conn.call(ctx, "initialize", map[string]any{"protocolVersion": acpProtocolVersion}, &init); err != nil {
		t.Fatal(err)
	}
	if init.ProtocolVersion != acpProtocolVersion || init.AgentInfo.Name != "Helper" {
		t.Fatalf("initialize: %+v", init)
	}
	var session acpSession
	if err := conn.call(ctx, "session/new", map[string]any{"cwd": "/tmp", "mcpServers": []any{}}, &session); err != nil {
		t.Fatal(err)
	}
	if session.SessionID != "s1" {
		t.Fatalf("session: %+v", session)
	}
	var result acpPromptResult
	if err := conn.call(ctx, "session/prompt", map[string]any{
		"sessionId": "s1",
		"prompt":    []any{map[string]any{"type": "text", "text": "嗨"}},
	}, &result); err != nil {
		t.Fatal(err)
	}
	if result.StopReason != "end_turn" {
		t.Fatalf("prompt: %+v", result)
	}
	select {
	case update := <-updates:
		if update.Update.SessionUpdate != "agent_message_chunk" || update.Words() != "你好" {
			t.Fatalf("update: %+v", update)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("the agent's update did not arrive")
	}
}

// promptWantsTool tells whether the words of a prompt ask for the tool.
func promptWantsTool(params json.RawMessage) bool {
	return promptWants(params, "tool")
}

// promptWants tells whether a prompt holds the words.
func promptWants(params json.RawMessage, words string) bool {
	var body struct {
		Prompt []struct {
			Text string `json:"text"`
		} `json:"prompt"`
	}
	if json.Unmarshal(params, &body) != nil {
		return false
	}
	for _, block := range body.Prompt {
		if block.Text == words {
			return true
		}
	}
	return false
}
