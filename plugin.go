package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/MoeForge/moejs"
	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
	"vael/internal/workspace"
)

// A plugin is a small program the editor runs for the user: it reads
// what the editor has, gives back what it wants the editor to do, and
// never draws its own pixels. The manifest says what it may do, and the
// editor checks that before every call.
//
// The plugins live in one directory, each in a folder of its own with a
// plugin.json in it. The commands a plugin brings show in the command
// palette, like the editor's own.

const (
	// pluginTimeout is how long a call of a plugin may take. A plugin
	// that loops past it is stopped, as an editor must be able to.
	// pluginMemoryLimit is how much a plugin may hold, in bytes.
	pluginMemoryLimit = 64 << 20
)

// pluginTimeout is a variable so that the tests do not have to wait for
// it.
var pluginTimeout = 2 * time.Second

// plugin is one plugin the editor found in its directory.
type plugin struct {
	manifest workspace.PluginManifest
	dir      string
	app      *app
	runtime  *moejs.Runtime
	hook     moejs.Hook
	// wasm is the sandbox of a plugin built to wasm; the JavaScript
	// plugins have none.
	wasm *wasmPlugin
	// callTab is the file a command is working on while it runs.
	callTab *fileTab
	// loaded says the program of the plugin is ready to run.
	loaded bool
	// err says why the plugin did not load, if it did not.
	err string
	// disabled says the user turned it off.
	disabled bool
}

// pluginRequest is what the editor gives a plugin when it runs a command.
type pluginRequest struct {
	Command   string `json:"command"`
	Text      string `json:"text"`
	Content   string `json:"content,omitempty"`
	Path      string `json:"path,omitempty"`
	Selection []int  `json:"selection,omitempty"`
}

// pluginAnswer is what a plugin gives back.
type pluginAnswer struct {
	Notify  string `json:"notify,omitempty"`
	Replace string `json:"replace,omitempty"`
	Error   string `json:"error,omitempty"`
	// View is a panel the plugin asks for: a tree of the editor's own
	// parts, which the editor draws with its theme.
	View []pluginNode `json:"view,omitempty"`
}

// loadPlugins reads the plugins of the editor's directory, and opens the
// ones the user left on.
func (a *app) loadPlugins() {
	// The plugins that were open are closed first: their sandboxes hold
	// memory until they are.
	for _, item := range a.plugins {
		item.close()
	}
	// A theme of a plugin that goes away cannot stay chosen.
	themeFromPlugin := a.pluginTheme(a.themeID) != nil
	iconsFromPlugin := a.pluginIconTheme() != nil
	a.plugins = nil
	a.pluginThemes = nil
	a.pluginIconThemes = nil
	if a.pluginsDir == "" {
		return
	}
	entries, err := os.ReadDir(a.pluginsDir)
	if err != nil {
		if !os.IsNotExist(err) {
			a.pluginErr = err.Error()
		}
		return
	}
	a.pluginErr = ""
	for _, entry := range entries {
		if !entry.IsDir() || strings.HasPrefix(entry.Name(), ".") {
			continue
		}
		item := &plugin{dir: filepath.Join(a.pluginsDir, entry.Name()), app: a}
		data, err := os.ReadFile(filepath.Join(item.dir, "plugin.json"))
		if err != nil {
			item.err = "没有 plugin.json"
			a.plugins = append(a.plugins, item)
			continue
		}
		if err := json.Unmarshal(data, &item.manifest); err != nil {
			item.err = "plugin.json 不是合法的 JSON"
			a.plugins = append(a.plugins, item)
			continue
		}
		if err := item.manifest.Check(); err != nil {
			item.err = err.Error()
			a.plugins = append(a.plugins, item)
			continue
		}
		item.disabled = a.pluginStates[item.manifest.Name]
		if !item.disabled {
			a.registerPluginTheme(item)
		}
		if !item.disabled && item.manifest.Main != "" {
			if err := item.open(); err != nil {
				item.err = err.Error()
			}
		}
		a.plugins = append(a.plugins, item)
	}
	// If the chosen theme or icon theme came from a plugin that is now
	// gone, go back to one of the editor's own.
	if themeFromPlugin && a.pluginTheme(a.themeID) == nil {
		id := "vael-dark"
		if !a.dark {
			id = "vael-light"
		}
		a.setTheme(id)
	}
	if iconsFromPlugin && a.pluginIconTheme() == nil {
		a.iconID = "badges"
	}
}

// open loads the program of the plugin and gives it the host API.
func (p *plugin) open() error {
	main, err := fileIn(p.dir, p.manifest.Main)
	if err != nil {
		return err
	}
	if p.manifest.Runtime == "wasm" {
		ctx, cancel := context.WithTimeout(context.Background(), pluginTimeout)
		defer cancel()
		sandbox, err := openWasmPlugin(ctx, main)
		if err != nil {
			return err
		}
		p.wasm = sandbox
		p.loaded = true
		return nil
	}
	source, err := os.ReadFile(main)
	if err != nil {
		return err
	}
	module, err := moejs.Compile(main, string(source))
	if err != nil {
		return err
	}
	hook, err := module.Hook("run")
	if err != nil {
		return err
	}
	p.hook = hook
	p.runtime = moejs.NewRuntime(moejs.Options{MemoryLimit: pluginMemoryLimit})
	if err := p.runtime.Load(module); err != nil {
		return err
	}
	p.loaded = true
	return p.installAPI()
}

// close ends the plugin and gives its memory back.
func (p *plugin) close() {
	if p == nil {
		return
	}
	if p.wasm != nil {
		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		p.wasm.close(ctx)
		cancel()
		p.wasm = nil
	}
	p.runtime = nil
	p.loaded = false
}

// installAPI gives the plugin the functions it may use: what the editor
// shows it, and what it may keep of its own.
func (p *plugin) installAPI() error {
	a := p.app
	return p.runtime.SetGlobal("vael", map[string]any{
		"notify": moejs.NativeFunc(func(realm *moejs.Realm, this moejs.Value, args []moejs.Value) (moejs.Value, error) {
			if !p.manifest.Has("notify") {
				return moejs.Undefined(), fmt.Errorf("插件没有 notify 能力")
			}
			text := moejs.Arg(args, 0).String()
			a.update(func() {
				a.message, a.messageKind = text, ""
			})
			return moejs.Undefined(), nil
		}),
		"replace": moejs.NativeFunc(func(realm *moejs.Realm, this moejs.Value, args []moejs.Value) (moejs.Value, error) {
			if !p.manifest.Has("editor") {
				return moejs.Undefined(), fmt.Errorf("插件没有 editor 能力")
			}
			text := moejs.Arg(args, 0).String()
			tab := p.callTab
			if tab == nil {
				tab = a.active()
			}
			a.update(func() { a.replacePluginText(tab, text) })
			return moejs.Undefined(), nil
		}),
		"storage": map[string]any{
			"get": moejs.NativeFunc(func(realm *moejs.Realm, this moejs.Value, args []moejs.Value) (moejs.Value, error) {
				if !p.manifest.Has("storage") {
					return moejs.Undefined(), fmt.Errorf("插件没有 storage 能力")
				}
				return moejs.String(a.pluginStorageGet(p.manifest.Name, moejs.Arg(args, 0).String())), nil
			}),
			"set": moejs.NativeFunc(func(realm *moejs.Realm, this moejs.Value, args []moejs.Value) (moejs.Value, error) {
				if !p.manifest.Has("storage") {
					return moejs.Undefined(), fmt.Errorf("插件没有 storage 能力")
				}
				a.pluginStorageSet(p.manifest.Name, moejs.Arg(args, 0).String(), moejs.Arg(args, 1).String())
				return moejs.Undefined(), nil
			}),
		},
	})
}

// call runs the plugin's own function with a request, and waits for its
// answer.
func (p *plugin) call(ctx context.Context, body string) (pluginAnswer, error) {
	if !p.loaded {
		return pluginAnswer{}, fmt.Errorf("插件没有载入")
	}
	if p.wasm != nil {
		out, err := p.wasm.call(ctx, body)
		if err != nil {
			return pluginAnswer{}, err
		}
		return parsePluginAnswer(out)
	}
	if p.runtime == nil {
		return pluginAnswer{}, fmt.Errorf("插件没有载入")
	}
	stop := context.AfterFunc(ctx, func() { p.runtime.Interrupt(context.Cause(ctx)) })
	defer stop()
	res, err := p.runtime.Call(p.hook, moejs.String(body))
	p.runtime.ClearInterrupt()
	p.runtime.ReleaseCallData()
	if err != nil {
		return pluginAnswer{}, err
	}
	return parsePluginAnswer([]byte(res.String()))
}

// parsePluginAnswer reads what a plugin answered.
func parsePluginAnswer(out []byte) (pluginAnswer, error) {
	words := strings.TrimSpace(string(out))
	if words == "" {
		return pluginAnswer{}, nil
	}
	var answer pluginAnswer
	if err := json.Unmarshal([]byte(words), &answer); err != nil {
		return pluginAnswer{}, fmt.Errorf("插件的答案不是 JSON：%w", err)
	}
	return answer, nil
}

// pluginCommands are the commands of the plugins that are on, as the
// command palette shows them.
func (a *app) pluginCommands() []command {
	var list []command
	for _, item := range a.plugins {
		if item == nil || item.disabled || item.err != "" || !item.loaded {
			continue
		}
		for _, spec := range item.manifest.Contributes.Commands {
			item, spec := item, spec
			list = append(list, command{
				title: "插件：" + spec.Title,
				hint:  spec.Hint,
				run:   func() { a.runPluginCommand(item, spec.ID) },
			})
		}
	}
	return list
}

// runPluginCommand gives the plugin the file that is open, and does what
// it answers.
func (a *app) runPluginCommand(item *plugin, id string) {
	if item == nil || !item.loaded {
		return
	}
	tab := a.active()
	request := pluginRequest{Command: id}
	if tab != nil && tab.state != nil {
		request.Path = tab.path
		request.Content = tab.state.Buffer.String()
		request.Text = tab.state.SelectedText()
		if request.Text == "" {
			// Nothing is selected: the plugin works on the whole file.
			request.Text = request.Content
		}
		start, end := tab.state.Selection().Start(), tab.state.Selection().End()
		request.Selection = []int{start, end}
	}
	body, err := json.Marshal(request)
	if err != nil {
		a.report(err)
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), pluginTimeout)
	item.callTab = tab
	go func() {
		defer cancel()
		defer func() { item.callTab = nil }()
		answer, err := item.call(ctx, string(body))
		a.update(func() {
			name := item.manifest.Name
			switch {
			case err != nil:
				a.message, a.messageKind = "插件 "+name+"："+err.Error(), "error"
			case answer.Error != "":
				a.message, a.messageKind = "插件 "+name+"："+answer.Error, "error"
			case answer.Replace != "" && !item.manifest.Has("editor"):
				a.message, a.messageKind = "插件 "+name+" 没有 editor 能力，不能改文件", "error"
			case answer.Notify != "" && !item.manifest.Has("notify"):
				a.message, a.messageKind = "插件 "+name+" 没有 notify 能力，不能说话", "error"
			default:
				if answer.Replace != "" && tab != nil && tab.state != nil {
					tab.state.Insert(answer.Replace)
				}
				if answer.Notify != "" {
					a.message, a.messageKind = answer.Notify, ""
				}
				if len(answer.View) > 0 {
					if a.pluginViews == nil {
						a.pluginViews = map[string][]pluginNode{}
					}
					a.pluginViews[name] = answer.View
					a.pluginPanelPlugin = name
					a.rightSection, a.rightPanel = "plugin", true
				}
			}
		})
	}()
}

// replacePluginText puts the text a plugin gave into the editor, in the
// place of what was selected.
func (a *app) replacePluginText(tab *fileTab, text string) {
	if tab == nil || tab.state == nil {
		return
	}
	tab.state.Insert(text)
}

// mygoShellOpenPath shows a folder in the file manager of the system.
func mygoShellOpenPath(path string) {
	_ = mygo.Shell.OpenPath(path)
}

// pluginStorageGet is what a plugin kept of its own.
func (a *app) pluginStorageGet(name, key string) string {
	if a.pluginStorage == nil {
		return ""
	}
	return a.pluginStorage[name][key]
}

// pluginStorageSet keeps what a plugin wants to remember, and writes it
// down.
func (a *app) pluginStorageSet(name, key, value string) {
	if a.pluginStorage == nil {
		a.pluginStorage = map[string]map[string]string{}
	}
	if a.pluginStorage[name] == nil {
		a.pluginStorage[name] = map[string]string{}
	}
	a.pluginStorage[name][key] = value
	if a.pluginStoragePath != "" {
		a.report(workspace.SavePluginStorage(a.pluginStoragePath, a.pluginStorage))
	}
}

// pluginPanel is the panel a plugin asked for: the tree it gave in its
// last answer, drawn by the editor with its own parts.
func (a *app) pluginPanel(c *ui.Context, p palette) {
	name := a.pluginPanelPlugin
	if name == "" {
		for _, item := range a.plugins {
			if item != nil && len(a.pluginViews[item.manifest.Name]) > 0 {
				name = item.manifest.Name
				break
			}
		}
	}
	if name == "" {
		ui.Text(c, "还没有插件给出面板。").FontSize(11).TextColor(p.muted).Padding(10, 10)
		return
	}
	a.pluginPanelPlugin = name
	ui.Column(c).FillWidth().Gap(8).Padding(10).Children(func() {
		var names []string
		for _, item := range a.plugins {
			if item != nil && len(a.pluginViews[item.manifest.Name]) > 0 {
				names = append(names, item.manifest.Name)
			}
		}
		if len(names) > 1 {
			ui.Row(c).FillWidth().Gap(4).Children(func() {
				for _, item := range names {
					item := item
					b := ui.ButtonBase(c).Height(22).Padding(0, 8).Radius(9).Label(item)
					if item == name {
						b.Background(p.selected)
					} else if b.Hovered() {
						b.Background(p.edge)
					}
					b.Children(func() {
						ui.Text(c, item).FontSize(10).TextColor(p.text).SingleLine().Ellipsis("…")
					})
					if b.Clicked() {
						a.pluginPanelPlugin = item
					}
				}
			})
		}
		a.pluginView(c, p, a.pluginViews[name])
	})
}

// pluginViewCommand runs what a button of a plugin's panel asks for.
func (a *app) pluginViewCommand(command string) {
	if command == "" {
		return
	}
	if a.pluginPanelDemo {
		a.runPluginDemoCommand(command)
		return
	}
	if name := a.pluginPanelPlugin; name != "" {
		for _, item := range a.plugins {
			if item != nil && item.manifest.Name == name && item.loaded {
				a.runPluginCommand(item, command)
				return
			}
		}
	}
	a.runPluginDemoCommand(command)
}

// pluginSettings is the list of the plugins the editor found: what they
// are, what went wrong, and whether they are on.
func (a *app) pluginSettings(c *ui.Context, p palette) {
	if !a.settingsCustomAllowed(settingsPlugin) {
		return
	}
	ui.Column(c).FillWidth().Gap(10).Children(func() {
		a.pluginStoreView(c, p)
		ui.Divider(c)
		ui.Text(c, "插件放在 "+a.pluginsDir+" 里，每个插件一个目录，里面是 plugin.json 与它的代码；"+
			"插件带来的命令会出现在命令面板里。").FontSize(10.5).TextColor(p.muted)
		ui.Row(c).FillWidth().Gap(6).Children(func() {
			if quiet(c, "安装插件包…", false, p) {
				a.choosePluginPackage()
			}
			if quiet(c, "从网址安装…", false, p) {
				a.pluginURL = &pluginURLPrompt{}
			}
			if quiet(c, "刷新", false, p) {
				a.loadPlugins()
			}
			if quiet(c, "打开插件目录", false, p) && a.pluginsDir != "" {
				mygoShellOpenPath(a.pluginsDir)
			}
		})
		if a.pluginErr != "" {
			ui.Text(c, a.pluginErr).FontSize(10.5).TextColor(p.danger)
		}
		if len(a.plugins) == 0 {
			ui.Text(c, "还没有插件。").FontSize(11.5).TextColor(p.muted)
			return
		}
		for _, item := range a.plugins {
			item := item
			ui.Column(c).FillWidth().Gap(3).Padding(8, 6).Radius(6).Background(p.wash).Children(func() {
				ui.Row(c).FillWidth().Gap(8).AlignItems(ui.Center).Children(func() {
					ui.Column(c).Grow(1).MinWidth(0).Gap(2).Children(func() {
						ui.Text(c, item.manifest.Name).FontSize(12).TextColor(p.text).SingleLine().Ellipsis("…")
						detail := strings.TrimSpace(item.manifest.Description)
						if item.manifest.Version != "" {
							if detail != "" {
								detail += " · "
							}
							detail += item.manifest.Version
						}
						if detail != "" {
							ui.Text(c, detail).FontSize(10).TextColor(p.muted).SingleLine().Ellipsis("…")
						}
					})
					enabled := !item.disabled
					if ui.Switch(c, &enabled).Label("启用插件 " + item.manifest.Name).Changed() {
						a.setPluginDisabled(item.manifest.Name, !enabled)
						a.loadPlugins()
					}
					name := item.manifest.Name
					if name == "" {
						name = filepath.Base(item.dir)
					}
					if smallIconButton(c, trashIcon, "卸载插件 "+name, p) {
						if err := a.uninstallPlugin(name); err != nil {
							a.message, a.messageKind = err.Error(), "error"
						} else {
							a.message, a.messageKind = "已卸载 "+name, "success"
						}
					}
				})
				if len(item.manifest.Capabilities) > 0 {
					ui.Text(c, "能力："+strings.Join(item.manifest.Capabilities, "、")).
						FontSize(9.5).TextColor(p.muted)
				}
				if item.err != "" {
					ui.Text(c, item.err).FontSize(10).TextColor(p.danger)
				}
				if len(item.manifest.Contributes.Commands) > 0 {
					var names []string
					for _, spec := range item.manifest.Contributes.Commands {
						names = append(names, spec.Title)
					}
					ui.Text(c, "命令："+strings.Join(names, "、")).FontSize(9.5).TextColor(p.muted)
				}
			})
		}
	})
}

// setPluginDisabled turns a plugin off or on, and remembers it.
func (a *app) setPluginDisabled(name string, disabled bool) {
	if a.pluginStates == nil {
		a.pluginStates = workspace.PluginStates{}
	}
	a.pluginStates[name] = disabled
	if a.pluginStatesPath != "" {
		a.report(workspace.SavePluginStates(a.pluginStatesPath, a.pluginStates))
	}
}
