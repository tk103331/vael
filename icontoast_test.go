package main

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/egoist/mygo/ui"
	"vael/internal/workspace"
)

const agentIconSVGData = `<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24"><circle cx="12" cy="12" r="8"/></svg>`

func TestTheRegistryIconIsKept(t *testing.T) {
	index, err := parseRegistry([]byte(`{"version":"1.0.0","agents":[
		{"id":"zeta","name":"Zeta","version":"1.0.0","icon":"https://example.com/zeta.svg",
		 "distribution":{"npx":{"package":"zeta@1"}}}
	]}`))
	if err != nil {
		t.Fatal(err)
	}
	if index.Agents[0].Icon != "https://example.com/zeta.svg" {
		t.Fatalf("the icon: %q", index.Agents[0].Icon)
	}
}

func TestAnAddedAgentKeepsItsIcon(t *testing.T) {
	a := newApp()
	defer a.shutdown()
	a.agentsPath = filepath.Join(t.TempDir(), "agents.json")
	a.addACPAgent(acpRegistryAgent{
		ID: "zeta", Name: "Zeta",
		Icon: "https://example.com/zeta.svg",
	}, "zeta --acp", nil)
	saved, err := workspace.LoadAgents(a.agentsPath)
	if err != nil || len(saved.Items) != 1 || saved.Items[0].Icon != "https://example.com/zeta.svg" {
		t.Fatalf("the saved icon: %+v %v", saved, err)
	}
	if a.agentSVG(workspace.Agent{}) != nil {
		t.Fatal("an agent with no icon still made one")
	}
}

func TestAnAgentIconIsReadFromItsCache(t *testing.T) {
	a := newApp()
	defer a.shutdown()
	a.agentsPath = filepath.Join(t.TempDir(), "agents.json")
	key := "https://example.com/zeta.svg"
	path := a.agentIconPath(key)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(agentIconSVGData), 0o644); err != nil {
		t.Fatal(err)
	}
	if svg := a.agentSVG(workspace.Agent{Icon: key}); svg == nil {
		t.Fatal("the cached icon was not read")
	}
	if _, ok := a.agentIcons[key]; !ok {
		t.Fatal("the icon was not kept in memory")
	}
}

func TestAnAgentIconIsFetchedAndCached(t *testing.T) {
	a, queue := testDispatchApp(t)
	a.agentsPath = filepath.Join(t.TempDir(), "agents.json")
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(agentIconSVGData))
	}))
	defer server.Close()
	key := server.URL + "/zeta.svg"
	if svg := a.agentSVG(workspace.Agent{Icon: key}); svg != nil {
		t.Fatal("the icon should wait for the network")
	}
	pumpUntil(t, queue, func() bool { return a.agentIcons[key] != nil })
	if a.agentSVG(workspace.Agent{Icon: key}) == nil {
		t.Fatal("the icon did not come")
	}
	if _, err := os.Stat(a.agentIconPath(key)); err != nil {
		t.Fatalf("the icon was not cached: %v", err)
	}
	if a.agentIconBusy[key] {
		t.Fatal("the icon is still being fetched")
	}
}

func TestNoticesBecomeToasts(t *testing.T) {
	for _, c := range []struct {
		text, kind, want string
	}{
		{"已添加 Zeta", "", "success"},
		{"替换了 3 处（2 个文件）", "", "success"},
		{"重命名 a → b：2 处", "", "success"},
		{"打开文件失败", "error", "error"},
		{"没有找到定义", "", "info"},
	} {
		a := newApp()
		a.message, a.messageKind = c.text, c.kind
		if got := a.toastKind(); got != c.want {
			t.Fatalf("%q became %q, not %q", c.text, got, c.want)
		}
		a.shutdown()
	}
	a := newApp()
	defer a.shutdown()
	a.root = t.TempDir()
	a.message, a.messageKind = "已添加 Zeta", "success"
	tt := ui.NewTester(a.view, 1280, 840)
	tt.Frame()
	if a.message != "" || a.messageKind != "" {
		t.Fatalf("the notice stayed after showing: %q %q", a.message, a.messageKind)
	}
	if _, ok := tt.Find("知道了"); ok {
		t.Fatal("the notice is still a modal")
	}
}
