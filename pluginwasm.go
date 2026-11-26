package main

import (
	"context"
	"fmt"
	"os"

	"github.com/tetratelabs/wazero"
	"github.com/tetratelabs/wazero/api"
)

// A plugin built to wasm: the same ABI as the JavaScript one — a JSON
// request in, a JSON answer out — but in a sandbox of its own, so that a
// plugin written in Rust, Zig or anything else can talk to the editor the
// same way.
//
// The module exports three things:
//
//	vael_alloc(size) -> pointer   room for the editor to write a request into
//	vael_run(ptr, size) -> u64     runs the command; the top half of the
//	                               answer is where it is, the bottom half
//	                               how long it is
//	memory                          the module's memory
//
// The answer is the same JSON a JavaScript plugin gives: what the editor
// should do — say something, replace the text, draw a panel.

const (
	// wasmMemoryPages is how much a wasm plugin may hold: 64 MiB, in
	// pages of 64 KiB.
	wasmMemoryPages = 1024
)

// wasmPlugin is a plugin built to wasm, run by wazero.
type wasmPlugin struct {
	runtime wazero.Runtime
	module  api.Module
	alloc   api.Function
	run     api.Function
	memory  api.Memory
}

// openWasmPlugin opens a wasm plugin: the module is compiled and
// instantiated, and the three things it must export are taken.
func openWasmPlugin(ctx context.Context, path string) (*wasmPlugin, error) {
	code, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	// CloseOnContextDone stops a plugin whose call is cancelled, even one
	// in an endless loop: what an editor needs of a plugin.
	config := wazero.NewRuntimeConfig().
		WithCloseOnContextDone(true).
		WithMemoryLimitPages(wasmMemoryPages)
	runtime := wazero.NewRuntimeWithConfig(ctx, config)
	module, err := runtime.InstantiateWithConfig(ctx, code, wazero.NewModuleConfig().
		WithStartFunctions())
	if err != nil {
		_ = runtime.Close(ctx)
		return nil, err
	}
	plugin := &wasmPlugin{
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

// call gives the plugin a request, and reads its answer.
func (p *wasmPlugin) call(ctx context.Context, body string) ([]byte, error) {
	size := uint64(len(body))
	at, err := p.alloc.Call(ctx, size)
	if err != nil {
		return nil, err
	}
	if len(at) == 0 || !p.memory.Write(uint32(at[0]), []byte(body)) {
		return nil, fmt.Errorf("插件没有给出写入请求的内存")
	}
	packed, err := p.run.Call(ctx, at[0], size)
	if err != nil {
		return nil, err
	}
	if len(packed) == 0 || packed[0] == 0 {
		return nil, nil
	}
	where, length := uint32(packed[0]>>32), uint32(packed[0])
	out, ok := p.memory.Read(where, length)
	if !ok {
		return nil, fmt.Errorf("插件给出的答案是越界的")
	}
	return out, nil
}

// close ends the plugin and its sandbox.
func (p *wasmPlugin) close(ctx context.Context) {
	if p == nil || p.runtime == nil {
		return
	}
	_ = p.runtime.Close(ctx)
}
