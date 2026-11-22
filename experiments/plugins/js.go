// Package plugins is the exploration of how Vael could run plugins. This
// file is the JavaScript half: moejs runs the plugin, and the editor's own
// functions are installed as globals the script calls.
package plugins

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"sync"

	"github.com/MoeForge/moejs"
)

// JSPlugin is a plugin written in JavaScript, run by moejs.
type JSPlugin struct {
	runtime *moejs.Runtime
	module  *moejs.Module
	run     moejs.Hook

	mu   sync.Mutex
	said []string
}

// LoadJS opens a JavaScript plugin: the module exports `run`, which takes
// the request and answers it. The script may also call `vael.notify`,
// which says something at once, as a plugin that reports while it works.
func LoadJS(ctx context.Context, path string) (*JSPlugin, error) {
	source, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	module, err := moejs.Compile(path, string(source))
	if err != nil {
		return nil, err
	}
	hook, err := module.Hook("run")
	if err != nil {
		return nil, err
	}
	plugin := &JSPlugin{runtime: moejs.NewRuntime(moejs.Options{}), module: module, run: hook}
	if err := plugin.runtime.Load(module); err != nil {
		return nil, err
	}
	err = plugin.runtime.SetGlobal("vael", map[string]any{
		"notify": moejs.NativeFunc(func(realm *moejs.Realm, this moejs.Value, args []moejs.Value) (moejs.Value, error) {
			plugin.mu.Lock()
			defer plugin.mu.Unlock()
			plugin.said = append(plugin.said, moejs.Arg(args, 0).String())
			return moejs.Undefined(), nil
		}),
	})
	if err != nil {
		return nil, err
	}
	return plugin, nil
}

// Call runs a command of the plugin. A plugin that loops past the
// context's deadline is stopped, as an editor must be able to do.
func (p *JSPlugin) Call(ctx context.Context, command, text string) (answer, error) {
	body, err := json.Marshal(request{Command: command, Text: text})
	if err != nil {
		return answer{}, err
	}
	stop := context.AfterFunc(ctx, func() {
		p.runtime.Interrupt(context.Cause(ctx))
	})
	defer stop()
	res, err := p.runtime.Call(p.run, moejs.String(string(body)))
	p.runtime.ClearInterrupt()
	p.runtime.ReleaseCallData()
	if err != nil {
		return answer{}, err
	}
	defer p.runtime.ReleaseCallData()
	var result answer
	words := res.String()
	if words == "" {
		return answer{}, nil
	}
	if err := json.Unmarshal([]byte(words), &result); err != nil {
		return answer{}, fmt.Errorf("插件的答案不是 JSON：%w", err)
	}
	return result, nil
}

// Said is what the plugin said of its own accord during the calls.
func (p *JSPlugin) Said() []string {
	p.mu.Lock()
	defer p.mu.Unlock()
	return append([]string(nil), p.said...)
}
