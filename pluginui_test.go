package main

import (
	"strings"
	"testing"

	"github.com/egoist/mygo/ui"
)

func TestAPluginViewKeepsItsBounds(t *testing.T) {
	deep := strings.Repeat(`{"type":"column","children":[`, 20) + `{"type":"text","text":"x"}`
	deep += strings.Repeat("]}", 20)
	if _, err := parsePluginView([]byte(deep)); err == nil {
		t.Fatal("a view too deep was accepted")
	}
	wide := `[{"type":"column","children":[`
	for range 500 {
		wide += `{"type":"text","text":"x"},`
	}
	wide += `{"type":"text","text":"x"}]}]`
	if _, err := parsePluginView([]byte(wide)); err == nil {
		t.Fatal("a view with too many parts was accepted")
	}
	if _, err := parsePluginView([]byte(`{"type":"text"}`)); err == nil {
		t.Fatal("a view that is not a list was accepted")
	}
}

func TestAPluginViewIsDrawnByTheEditor(t *testing.T) {
	a := newApp()
	defer a.shutdown()
	a.root = t.TempDir()
	nodes, err := parsePluginView([]byte(pluginDemoViewJSON))
	if err != nil {
		t.Fatal(err)
	}
	view := func(c *ui.Context) {
		ui.Column(c).Size(460, 620).Padding(10).Children(func() {
			a.pluginView(c, a.colors(), nodes)
		})
	}
	tt := ui.NewTester(view, 460, 620)
	tt.Frame()
	for _, text := range []string{"提交摘要", "提交信息", "feat(store): 补上计数器", "提交前会先跑 go test ./..."} {
		if _, ok := tt.Find(text); !ok {
			t.Fatalf("%q is not drawn: %v", text, tt.Texts())
		}
	}
	if err := tt.Click("提交"); err != nil {
		t.Fatal(err)
	}
	if a.pluginDemoClick != "commit" || a.message != "插件命令：commit" {
		t.Fatalf("the command did not run: %q %q", a.pluginDemoClick, a.message)
	}
	// A part the editor does not know says so, and does not break the
	// rest of the view.
	unknown, err := parsePluginView([]byte(`[{"type":"chart","text":"x"}]`))
	if err != nil {
		t.Fatal(err)
	}
	view = func(c *ui.Context) {
		ui.Column(c).Size(300, 120).Children(func() {
			a.pluginView(c, a.colors(), unknown)
		})
	}
	tt = ui.NewTester(view, 300, 120)
	tt.Frame()
	if _, ok := tt.Find("插件用了不支持的界面元素：chart"); !ok {
		t.Fatalf("an unknown part passed silently: %v", tt.Texts())
	}
}

func TestAPluginCannotChooseItsOwnColours(t *testing.T) {
	// The tree has no colour, size or radius of its own: what a plugin
	// asks for by name is resolved from the theme, so the same view looks
	// right in both themes.
	light := newApp()
	defer light.shutdown()
	light.setTheme("vael-light")
	dark := newApp()
	defer dark.shutdown()
	dark.setTheme("vael-dark")
	for _, app := range []*app{light, dark} {
		nodes, err := parsePluginView([]byte(`[
			{"type":"text","text":"a","tone":"danger"},
			{"type":"text","text":"b","tone":"accent"}
		]`))
		if err != nil {
			t.Fatal(err)
		}
		p := app.colors()
		if pluginTone("danger", p) != p.danger || pluginTone("accent", p) != p.accent {
			t.Fatalf("a tone did not come from the theme: %+v", p)
		}
		if len(nodes) != 2 {
			t.Fatalf("%+v", nodes)
		}
	}
	if light.colors().text == dark.colors().text {
		t.Fatal("the two themes share a colour")
	}
}
