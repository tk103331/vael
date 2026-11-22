// Package plugins is the exploration of how Vael could run plugins: one
// ABI, a JSON request answered with JSON, run either by a wasm module
// (wazero) or by a JavaScript plugin (moejs).
//
// The ABI is one call: the editor writes a request such as
//
//	{"command":"wordcount","text":"你好 世界"}
//
// and the plugin answers with what the editor should do about it:
//
//	{"notify":"2 词 · 1 行 · 5 字符"}
//
// so that a plugin needs no UI of its own, and the editor keeps drawing
// it: the answer names actions, and the host applies them.
package plugins

import (
	"context"
	"encoding/json"
	"fmt"
	"os"

	"github.com/tetratelabs/wazero"
	"github.com/tetratelabs/wazero/api"
)

// request is what a call of a plugin is about.
type request struct {
	Command string `json:"command"`
	Text    string `json:"text"`
}

// answer is what a plugin asks the editor to do: say something, or
// replace the text it was given.
type answer struct {
	Notify  string  `json:"notify,omitempty"`
	Replace *string `json:"replace,omitempty"`
}

// WasmPlugin is a plugin built to wasm, run by wazero.
type WasmPlugin struct {
	runtime wazero.Runtime
	module  api.Module
	alloc   api.Function
	run     api.Function
	memory  api.Memory
}

// LoadWasm opens a wasm plugin: the module exports `vael_alloc` to give
// the host room for a request, and `vael_run` to answer it.
func LoadWasm(ctx context.Context, path string) (*WasmPlugin, error) {
	code, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	// CloseOnContextDone stops a plugin whose call is cancelled, even
	// one in an endless loop: what an editor needs of a plugin.
	runtime := wazero.NewRuntimeWithConfig(ctx, wazero.NewRuntimeConfig().
		WithCloseOnContextDone(true))
	module, err := runtime.InstantiateWithConfig(ctx, code, wazero.NewModuleConfig().
		WithStartFunctions())
	if err != nil {
		_ = runtime.Close(ctx)
		return nil, err
	}
	plugin := &WasmPlugin{
		runtime: runtime,
		module:  module,
		alloc:   module.ExportedFunction("vael_alloc"),
		run:     module.ExportedFunction("vael_run"),
		memory:  module.ExportedMemory("memory"),
	}
	if plugin.alloc == nil || plugin.run == nil || plugin.memory == nil {
		_ = runtime.Close(ctx)
		return nil, fmt.Errorf("插件缺少 vael_alloc、vael_run 或 memory")
	}
	return plugin, nil
}

// Call runs a command of the plugin, and answers what it asks for.
func (p *WasmPlugin) Call(ctx context.Context, command, text string) (answer, error) {
	body, err := json.Marshal(request{Command: command, Text: text})
	if err != nil {
		return answer{}, err
	}
	size := uint64(len(body))
	at, err := p.alloc.Call(ctx, size)
	if err != nil {
		return answer{}, err
	}
	if len(at) == 0 || !p.memory.Write(uint32(at[0]), body) {
		return answer{}, fmt.Errorf("插件没有给出写入请求的内存")
	}
	packed, err := p.run.Call(ctx, at[0], size)
	if err != nil {
		return answer{}, err
	}
	if len(packed) == 0 || packed[0] == 0 {
		return answer{}, nil
	}
	where, length := uint32(packed[0]>>32), uint32(packed[0])
	out, ok := p.memory.Read(where, length)
	if !ok {
		return answer{}, fmt.Errorf("插件给出的答案是越界的")
	}
	var result answer
	if err := json.Unmarshal(out, &result); err != nil {
		return answer{}, fmt.Errorf("插件的答案不是 JSON：%w", err)
	}
	return result, nil
}

// Close ends the plugin.
func (p *WasmPlugin) Close(ctx context.Context) error {
	return p.runtime.Close(ctx)
}
