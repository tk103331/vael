package plugins

import (
	"context"
	"errors"
	"fmt"
	"runtime"
	"testing"
	"time"
)

const text = "你好 世界\n第二个 段落\n"

func loadBoth(t *testing.T) (*WasmPlugin, *JSPlugin) {
	t.Helper()
	ctx := context.Background()
	wasm, err := LoadWasm(ctx, "plugin/wordcount.wasm")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = wasm.Close(ctx) })
	js, err := LoadJS(ctx, "js/wordcount.js")
	if err != nil {
		t.Fatal(err)
	}
	return wasm, js
}

func TestBothPluginsCountTheSame(t *testing.T) {
	wasm, js := loadBoth(t)
	ctx := context.Background()
	fromWasm, err := wasm.Call(ctx, "wordcount", text)
	if err != nil {
		t.Fatal(err)
	}
	fromJS, err := js.Call(ctx, "wordcount", text)
	if err != nil {
		t.Fatal(err)
	}
	if fromWasm.Notify == "" {
		t.Fatalf("the wasm plugin said nothing: %+v", fromWasm)
	}
	if fromWasm.Notify != fromJS.Notify {
		t.Fatalf("wasm %q, js %q", fromWasm.Notify, fromJS.Notify)
	}
	if want := "4 词 · 2 行 · 13 字符"; fromWasm.Notify != want {
		t.Fatalf("%q, not %q", fromWasm.Notify, want)
	}
}

func TestBothPluginsCanBeStopped(t *testing.T) {
	wasm, js := loadBoth(t)
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()
	start := time.Now()
	if _, err := wasm.Call(ctx, "loop", ""); err == nil || !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("the wasm plugin was not stopped: %v", err)
	}
	if time.Since(start) > 2*time.Second {
		t.Fatal("the wasm plugin ran past its time")
	}
	start = time.Now()
	if _, err := js.Call(ctx, "loop", ""); err == nil || !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("the js plugin was not stopped: %v", err)
	}
	if time.Since(start) > 2*time.Second {
		t.Fatal("the js plugin ran past its time")
	}
}

// TestLoadCostAndMemory measures what the two hosts cost, for the design
// document.
func TestLoadCostAndMemory(t *testing.T) {
	ctx := context.Background()
	runtime.GC()
	before := heapInUse()
	start := time.Now()
	wasm, err := LoadWasm(ctx, "plugin/wordcount.wasm")
	if err != nil {
		t.Fatal(err)
	}
	wasmLoad := time.Since(start)
	runtime.GC()
	wasmHeap := heapInUse() - before

	start = time.Now()
	js, err := LoadJS(ctx, "js/wordcount.js")
	if err != nil {
		t.Fatal(err)
	}
	jsLoad := time.Since(start)
	runtime.GC()
	both := heapInUse() - before

	const rounds = 200
	start = time.Now()
	for range rounds {
		if _, err := wasm.Call(ctx, "wordcount", text); err != nil {
			t.Fatal(err)
		}
	}
	wasmCall := time.Since(start) / rounds
	start = time.Now()
	for range rounds {
		if _, err := js.Call(ctx, "wordcount", text); err != nil {
			t.Fatal(err)
		}
	}
	jsCall := time.Since(start) / rounds
	t.Logf("wasm: load %v, one call %v, heap %s", wasmLoad, wasmCall, kib(wasmHeap))
	t.Logf("js:   load %v, one call %v, heap %s", jsLoad, jsCall, kib(both-wasmHeap))
	_ = js
	_ = wasm.Close(ctx)
}

func heapInUse() uint64 {
	var stats runtime.MemStats
	runtime.ReadMemStats(&stats)
	return stats.HeapInuse
}

func kib(n uint64) string {
	if n > 1<<20 {
		return fmt.Sprintf("%d MiB", n>>20)
	}
	return fmt.Sprintf("%d KiB", n>>10)
}

func BenchmarkWasmCall(b *testing.B) {
	ctx := context.Background()
	plugin, err := LoadWasm(ctx, "plugin/wordcount.wasm")
	if err != nil {
		b.Fatal(err)
	}
	defer plugin.Close(ctx)
	b.ResetTimer()
	for range b.N {
		if _, err := plugin.Call(ctx, "wordcount", text); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkJSCall(b *testing.B) {
	ctx := context.Background()
	plugin, err := LoadJS(ctx, "js/wordcount.js")
	if err != nil {
		b.Fatal(err)
	}
	b.ResetTimer()
	for range b.N {
		if _, err := plugin.Call(ctx, "wordcount", text); err != nil {
			b.Fatal(err)
		}
	}
}
