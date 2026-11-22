package main

import (
	"log"
	"os"
	"path/filepath"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
	"github.com/egoist/mygo/ui/editor"
	"vael/internal/core"
	"vael/internal/workspace"
)

func main() {
	sessionPath, pathErr := workspace.SessionPath()
	session, sessionErr := workspace.LoadSession(sessionPath)
	root, _ := os.Getwd()
	if sessionErr == nil && session.ConnectionKind == "" {
		if info, err := os.Stat(session.Root); err == nil && info.IsDir() {
			root = session.Root
		}
	}
	if len(os.Args) > 1 {
		root, _ = filepath.Abs(os.Args[1])
	}
	mygo.App.SetName("Vael")
	mygo.App.WhenReady(func() {
		a := newApp()
		if pathErr == nil {
			a.agentsPath = filepath.Join(filepath.Dir(sessionPath), "agents.json")
			a.loadAgents()
			a.environmentsPath = filepath.Join(filepath.Dir(sessionPath), "environments.json")
			if saved, err := workspace.LoadEnvironments(a.environmentsPath); err == nil {
				a.environments = saved
			} else if !os.IsNotExist(err) {
				log.Printf("读取连接配置失败: %v", err)
			}
		}
		if sessionErr == nil {
			a.applyLayout(session)
			a.pendingSession = &session
		}
		a.win = mygo.NewWindow(mygo.WindowOptions{Title: "Vael", Width: 1280, Height: 840, StateKey: "main", Frameless: true, CornerRadius: 10, Content: ui.View(a.view)})
		a.win.OnClose(func(e *mygo.CloseEvent) {
			if a.dirty() {
				e.PreventDefault()
				a.update(func() {
					a.confirm = &confirmation{"退出 Vael？", "仍有未保存的文件。退出将丢弃这些修改。", func() { a.win.Destroy() }}
				})
			}
		})
		a.win.OnClosed(func() {
			if pathErr == nil && a.root != "" {
				if err := workspace.SaveSession(sessionPath, a.snapshot()); err != nil {
					log.Printf("保存会话失败: %v", err)
				}
			}
			a.shutdown()
		})
		a.win.OnFileDrop(func(e *mygo.FileDropEvent) {
			a.update(func() { a.handleDrop(e.Paths) })
		})
		a.menus()
		if len(os.Args) == 1 && sessionErr == nil && session.ConnectionKind != "" {
			a.connectEnvironment(core.Connection{Kind: session.ConnectionKind, Target: session.Target, Root: session.Root, Binary: session.Binary})
		} else {
			a.openProject(root)
		}
	})
	if err := mygo.App.Run(); err != nil {
		log.Fatal(err)
	}
}

func (a *app) menus() {
	item := func(label, key string, fn func()) *mygo.MenuItem {
		return &mygo.MenuItem{Label: label, Accelerator: key, Click: func(*mygo.MenuItem, *mygo.Window) { a.update(fn) }}
	}
	menu := mygo.NewMenu([]*mygo.MenuItem{
		{Role: mygo.RoleAppMenu},
		{Label: "文件", Submenu: []*mygo.MenuItem{
			item("工作环境与最近项目…", "", func() { a.settingsOpen = true; a.settingsSection = settingsEnv }),
			item("重新连接当前环境", "", a.reconnect),
			item("连接工作环境…", "CmdOrCtrl+Shift+O", a.showConnectionPrompt),
			item("快速打开文件…", "CmdOrCtrl+P", func() { a.quickQuery = ""; a.quickOpen = true }),
			item("最近的文件…", "CmdOrCtrl+E", func() { a.recentQuery = ""; a.recentOpen = true }),
			item("打开文件夹…", "CmdOrCtrl+O", a.chooseProject),
			item("新建项目…", "", a.openProjectPrompt),
			item("克隆仓库…", "", a.openClonePrompt),
			item("新建文件…", "CmdOrCtrl+N", func() { a.prompt = &namePrompt{} }),
			item("重命名当前文件…", "F2", a.renameActiveFile),
			item("保存", "CmdOrCtrl+S", a.save),
			item("下一个标签页", "CmdOrCtrl+Shift+]", a.nextTab),
			item("上一个标签页", "CmdOrCtrl+Shift+[", a.prevTab),
			item("关闭标签页", "CmdOrCtrl+W", func() { a.closeTab(a.selected) }),
			item("关闭其他标签页", "", func() { a.closeOtherTabs(a.selected) }),
			item("关闭全部标签页", "", a.closeAllTabs),
		}},
		{Role: mygo.RoleEditMenu},
		{Label: "命令", Submenu: []*mygo.MenuItem{
			item("命令面板…", "CmdOrCtrl+Shift+P", func() { a.paletteOpen = true }),
			item("重新加载当前文件", "", func() {
				if f := a.active(); f != nil {
					a.reloadFile(f)
				}
			}),
		}},
		{Label: "视图", Submenu: []*mygo.MenuItem{
			item("项目全文搜索", "CmdOrCtrl+Shift+F", func() { a.sidebar = true; a.section = "search" }),
			item("显示/隐藏侧栏", "CmdOrCtrl+B", func() { a.sidebar = !a.sidebar }),
			item("显示/隐藏终端", "Ctrl+`", func() {
				if a.showBottom && a.bottomTab == "terminal" {
					a.showBottom = false
				} else {
					a.showTerminal()
				}
			}),
			item("转到行…", "CmdOrCtrl+G", func() {
				if f := a.active(); f != nil && f.state != nil {
					f.state.Run(editor.GoToLine)
				}
			}),
			item("查找与替换", "CmdOrCtrl+F", func() {
				if f := a.active(); f != nil && f.state != nil {
					f.state.Run(editorFind)
				}
			}),
			item("切换明暗主题", "CmdOrCtrl+Shift+L", func() { a.dark = !a.dark }),
			item("专注模式", "CmdOrCtrl+Shift+F12", func() { a.zen = !a.zen }),
			item("自动换行", "Alt+Z", func() { a.wrap = !a.wrap }),
			mygo.Separator(),
			item("放大", "CmdOrCtrl+=", func() { a.zoom(1) }),
			item("缩小", "CmdOrCtrl+-", func() { a.zoom(-1) }),
			item("重置缩放", "CmdOrCtrl+0", a.zoomReset),
		}},
		{Label: "Git", Submenu: []*mygo.MenuItem{
			item("切换分支…", "CmdOrCtrl+Shift+B", func() {
				a.branchMenuOpen = true
				a.loadBranches()
			}),
			item("新建分支…", "", func() { a.branchPrompt = &branchPrompt{} }),
			item("检出标签或修订…", "", func() { a.revisionPrompt = &revisionPrompt{} }),
			item("查看日志", "CmdOrCtrl+Shift+G", a.showLog),
			item("更新项目（拉取）", "", func() { a.gitRemote("pull", "--ff-only") }),
			item("推送", "", func() { a.gitRemote("push") }),
		}},
		{Label: "终端", Submenu: []*mygo.MenuItem{item("新建终端", "Ctrl+Shift+`", a.newTerminal)}},
	})
	mygo.App.SetMenu(menu)
	a.win.SetMenu(menu)
}
