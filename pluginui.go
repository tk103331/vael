package main

import (
	"encoding/json"
	"fmt"

	"github.com/egoist/mygo/ui"
)

// A plugin may draw its own interface, but not in pixels: it describes
// what it shows as a tree of the editor's own parts, and the editor draws
// it with its theme, its type scale and its spacing. A plugin chooses
// structure and meaning, never size, colour or radius, so a panel of a
// plugin looks like a panel of Vael by construction.
//
// The tree crosses the plugin ABI as JSON, once per view:
//
//	{"type":"column","gap":8,"children":[
//	  {"type":"text","text":"更改的文件","role":"title"},
//	  {"type":"list","items":[{"title":"main.go","detail":"M","icon":"file"}]},
//	  {"type":"buttons","children":[{"text":"提交","command":"commit","primary":true}]}
//	]}

// pluginNode is one part of a view a plugin asks for.
type pluginNode struct {
	Type     string          `json:"type"`
	Text     string          `json:"text,omitempty"`
	Role     string          `json:"role,omitempty"`     // title, body, meta, mono
	Tone     string          `json:"tone,omitempty"`     // default, muted, accent, success, warn, danger
	Icon     string          `json:"icon,omitempty"`     // a name of the editor's icons
	Wrap     bool            `json:"wrap,omitempty"`     // text that wraps instead of one line
	Gap      int             `json:"gap,omitempty"`      // a step of the spacing scale
	Align    string          `json:"align,omitempty"`    // start, center, end
	Primary  bool            `json:"primary,omitempty"`  // a button of the accent color
	Command  string          `json:"command,omitempty"`  // what a button runs
	Value    string          `json:"value,omitempty"`    // what an input holds
	Selected bool            `json:"selected,omitempty"` // a list row that is chosen
	Items    []pluginItem    `json:"items,omitempty"`
	Children []pluginNode    `json:"children,omitempty"`
	Meta     json.RawMessage `json:"meta,omitempty"`
}

// pluginItem is a row of a list of a plugin.
type pluginItem struct {
	Title    string `json:"title"`
	Detail   string `json:"detail,omitempty"`
	Icon     string `json:"icon,omitempty"`
	Code     string `json:"code,omitempty"`
	Selected bool   `json:"selected,omitempty"`
}

// Plugin views are bounded: a plugin cannot make the editor draw a tree
// deeper than this, or more parts than that.
const (
	pluginNodeDepth = 12
	pluginNodeCount = 400
)

// parsePluginView reads the view a plugin asked for.
func parsePluginView(data []byte) ([]pluginNode, error) {
	var nodes []pluginNode
	if err := json.Unmarshal(data, &nodes); err != nil {
		return nil, fmt.Errorf("插件的界面不是合法的视图树：%w", err)
	}
	count := 0
	var walk func(nodes []pluginNode, depth int) error
	walk = func(nodes []pluginNode, depth int) error {
		if depth > pluginNodeDepth {
			return fmt.Errorf("插件的界面超过 %d 层", pluginNodeDepth)
		}
		for _, node := range nodes {
			count++
			if count > pluginNodeCount {
				return fmt.Errorf("插件的界面超过 %d 个元素", pluginNodeCount)
			}
			if err := walk(node.Children, depth+1); err != nil {
				return err
			}
		}
		return nil
	}
	if err := walk(nodes, 0); err != nil {
		return nil, err
	}
	return nodes, nil
}

// pluginGap is the room a plugin asks for between what it shows, from the
// editor's spacing scale: a plugin picks a step, not a number of pixels.
func pluginGap(step int) float32 {
	scale := []float32{0, 2, 4, 6, 8, 10, 12}
	if step < 0 || step >= len(scale) {
		return scale[0]
	}
	return scale[step]
}

// pluginTone is the colour a plugin asks for, by what it means rather
// than by what it looks like.
func pluginTone(tone string, p palette) ui.Color {
	switch tone {
	case "muted":
		return p.muted
	case "accent":
		return p.accent
	case "success":
		return p.success
	case "warn":
		return p.warn
	case "danger":
		return p.danger
	}
	return p.text
}

// pluginIcon is an icon of the editor, which a plugin may name.
func pluginIcon(name string) *ui.SVG {
	switch name {
	case "file":
		return fileIcon
	case "folder":
		return folderIcon
	case "git":
		return gitIcon
	case "commit":
		return commitIcon
	case "agent":
		return agentIcon
	case "terminal":
		return terminalIcon
	case "check":
		return checkIcon
	case "close":
		return xIcon
	case "plus":
		return plusIcon
	case "log":
		return logIcon
	case "outline":
		return outlineIcon
	}
	return nil
}

// pluginView draws the view a plugin asked for with the editor's own
// parts: the same type scale, the same spacing, the same radii, and the
// same colours of the theme that is showing.
func (a *app) pluginView(c *ui.Context, p palette, nodes []pluginNode) {
	for _, node := range nodes {
		node := node
		a.pluginNode(c, p, node)
	}
}

func (a *app) pluginNode(c *ui.Context, p palette, node pluginNode) {
	switch node.Type {
	case "column":
		ui.Column(c).FillWidth().Gap(pluginGap(node.Gap)).Children(func() {
			a.pluginView(c, p, node.Children)
		})
	case "row":
		row := ui.Row(c).FillWidth().Gap(pluginGap(node.Gap))
		switch node.Align {
		case "center":
			row.AlignItems(ui.Center)
		case "end":
			row.Justify(ui.End)
		}
		row.Children(func() { a.pluginView(c, p, node.Children) })
	case "spacer":
		ui.Spacer(c)
	case "divider":
		ui.Divider(c).FillWidth()
	case "text":
		a.pluginText(c, p, node)
	case "code":
		ui.Box(c).FillWidth().Padding(6, 8).Radius(4).Background(p.wash).Children(func() {
			ui.Text(c, node.Text).Font("monospace").FontSize(10.5).TextColor(p.text)
		})
	case "badge":
		a.pluginBadge(c, p, node.Text, node.Tone)
	case "notice":
		ui.Row(c).FillWidth().Padding(6, 8).Radius(4).Background(p.wash).Gap(6).
			AlignItems(ui.Center).Children(func() {
			ui.Icon(c, alertIcon).Size(12, 12).TextColor(pluginTone(node.Tone, p))
			ui.Text(c, node.Text).FontSize(11).TextColor(pluginTone(node.Tone, p)).Grow(1).MinWidth(0)
		})
	case "button":
		label := node.Text
		if node.Primary {
			if ui.PrimaryButton(c, label).Clicked() {
				a.runPluginCommand(node.Command)
			}
			return
		}
		if quiet(c, label, false, p) {
			a.runPluginCommand(node.Command)
		}
	case "buttons":
		ui.Row(c).FillWidth().Wrap().Gap(6).Children(func() {
			a.pluginView(c, p, node.Children)
		})
	case "input":
		ui.TextInput(c, &node.Value).FillWidth().Height(28).Padding(2, 6).
			Placeholder(node.Text).Label(node.Text)
	case "list":
		ui.Column(c).FillWidth().Gap(1).Children(func() {
			for _, item := range node.Items {
				a.pluginListItem(c, p, item)
			}
		})
	case "markdown":
		// Words a plugin wrote for people, drawn as the editor draws
		// text: the host decides the shapes, not a renderer of the
		// plugin's.
		ui.Text(c, node.Text).FontSize(11.5).TextColor(p.text)
	default:
		// A part this editor does not know: say so where it would be,
		// rather than draw nothing.
		ui.Text(c, "插件用了不支持的界面元素："+node.Type).FontSize(10.5).TextColor(p.danger)
	}
}

// pluginText draws words of a plugin in one of the editor's roles.
func (a *app) pluginText(c *ui.Context, p palette, node pluginNode) {
	font := 11.5
	weight := 0
	switch node.Role {
	case "title":
		font, weight = 12.5, 650
	case "meta":
		font = 10
	case "mono":
		font = 10.5
	}
	text := ui.Text(c, node.Text).FontSize(float32(font)).TextColor(pluginTone(node.Tone, p))
	if node.Role == "mono" {
		text = text.Font("monospace")
	}
	if weight != 0 {
		text = text.FontWeight(weight)
	}
	if node.Wrap {
		text.FillWidth()
	} else {
		text.SingleLine().Ellipsis("…")
	}
}

// pluginBadge draws what a plugin counts, as the editor draws counts.
func (a *app) pluginBadge(c *ui.Context, p palette, text, tone string) {
	ui.Box(c).Height(17).Padding(1, 6).Radius(4).Background(p.wash).Center().Children(func() {
		ui.Text(c, text).FontSize(9.5).FontWeight(600).
			TextColor(pluginTone(tone, p)).SingleLine()
	})
}

// pluginListItem is a row of a list of a plugin: an icon, a title, what
// the editor says about it, and the way it looks when chosen.
func (a *app) pluginListItem(c *ui.Context, p palette, item pluginItem) {
	row := ui.ButtonBase(c).FillWidth().Height(26).Padding(0, 6).Gap(6).
		Radius(4).AlignItems(ui.Center).Label(item.Title)
	if item.Selected {
		row.Background(p.selected)
	} else if row.Hovered() {
		row.Background(p.edge)
	}
	row.Children(func() {
		if icon := pluginIcon(item.Icon); icon != nil {
			ui.Icon(c, icon).Size(12, 12).TextColor(p.muted)
		}
		ui.Text(c, item.Title).FontSize(11.5).TextColor(p.text).Grow(1).
			MinWidth(0).SingleLine().Ellipsis("…")
		if item.Code != "" {
			ui.Text(c, item.Code).Font("monospace").FontSize(9.5).TextColor(p.accent)
		}
		if item.Detail != "" {
			ui.Text(c, item.Detail).FontSize(10).TextColor(p.muted).SingleLine()
		}
	})
}

// runPluginCommand runs what a button of a plugin asks for. In a plugin
// that really runs, this calls the plugin back through the ABI; here it
// shows that the editor took it.
func (a *app) runPluginCommand(command string) {
	if command == "" {
		return
	}
	a.pluginDemoClick = command
	a.message = "插件命令：" + command
}

// pluginDemoView is the view the example plugin asks for, as JSON, the
// way it would come over the ABI.
const pluginDemoViewJSON = `[
  {"type":"column","gap":5,"children":[
    {"type":"row","gap":3,"align":"center","children":[
      {"type":"text","text":"提交摘要","role":"title"},
      {"type":"spacer"},
      {"type":"badge","text":"3 项","tone":"accent"}
    ]},
    {"type":"text","text":"工作区有 1 个文件改动，提交前可以看一遍。","role":"meta","wrap":true},
    {"type":"divider"},
    {"type":"list","items":[
      {"title":"main.go","detail":"+12 −3","code":"M","icon":"file","selected":true},
      {"title":"store.go","detail":"+40","code":"A","icon":"file"},
      {"title":"README.md","detail":"+2 −1","code":"M","icon":"file"}
    ]},
    {"type":"text","text":"提交信息","role":"meta"},
    {"type":"code","text":"feat(store): 补上计数器"},
    {"type":"notice","text":"提交前会先跑 go test ./...","tone":"warn"},
    {"type":"buttons","children":[
      {"type":"button","text":"提交","command":"commit","primary":true},
      {"type":"button","text":"查看差异","command":"diff"}
    ]}
  ]}
]`

// pluginDemoPanel is a hidden panel that shows the example plugin's view,
// so that a plugin's panel can be seen in the editor's own chrome.
func (a *app) pluginDemoPanel(c *ui.Context, p palette) {
	nodes, err := parsePluginView([]byte(pluginDemoViewJSON))
	if err != nil {
		ui.Text(c, err.Error()).FontSize(11).TextColor(p.danger)
		return
	}
	ui.Column(c).FillWidth().Padding(10).Children(func() {
		a.pluginView(c, p, nodes)
	})
}
