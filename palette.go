package main

import (
	"sort"
	"strings"

	"github.com/egoist/mygo/ui"
)

// command is one thing the command palette can do.
type command struct {
	title string
	hint  string
	run   func()
}

// useCommand counts a command's uses, so that the palette offers what the
// user runs first.
func (a *app) useCommand(title string) {
	a.commandUses[title]++
}

// commands are what the palette offers, in the order it shows them.
func (a *app) commands() []command {
	list := []command{
		{title: "文件：快速打开…", hint: "按文件名或路径查找", run: func() { a.quickQuery = ""; a.quickOpen = true }},
		{title: "文件：最近的文件…", hint: "重新打开最近编辑的文件", run: func() { a.recentQuery = ""; a.recentOpen = true }},
		{title: "文件：打开文件夹…", hint: "选择本地项目", run: a.chooseProject},
		{title: "文件：新建项目…", hint: "创建一个文件夹并打开", run: a.openProjectPrompt},
		{title: "文件：克隆仓库…", hint: "从 Git 地址克隆并打开", run: a.openClonePrompt},
		{title: "文件：新建文件…", hint: "在项目根目录创建", run: func() { a.prompt = &namePrompt{} }},
		{title: "文件：保存", hint: "保存当前文件", run: a.save},
		{title: "文件：关闭标签页", hint: "关闭当前文件", run: func() { a.closeTab(a.selected) }},
		{title: "文件：重命名当前文件…", hint: "F2", run: a.renameActiveFile},
		{title: "搜索：全部替换…", hint: "把项目中的查询全部替换", run: func() { a.sidebar = true; a.section = "search" }},
		{title: "工作环境：连接…", hint: "SSH、WSL 或 Docker", run: a.showConnectionPrompt},
		{title: "工作环境：管理与最近项目", hint: "保存的连接配置", run: func() { a.settingsOpen = true; a.settingsSection = settingsEnv }},
		{title: "工作环境：重新连接当前环境", hint: "重建核心通道", run: a.reconnect},
		{title: "搜索：项目全文搜索", hint: "在项目文件中查找文本", run: func() { a.sidebar = true; a.section = "search" }},
		{title: "搜索：项目符号", hint: "查找函数、类型和类", run: a.symbolsCommand},
		{title: "视图：大纲", hint: "当前文件的结构", run: func() { a.rightSection = "outline"; a.rightPanel = true }},
		{title: "代码：格式化文件", hint: "用语言服务器格式化当前文件", run: a.lspFormat},
		{title: "视图：语言服务器设置", hint: "启用服务器、改启动命令", run: func() { a.settingsOpen = true; a.settingsSection = settingsLSP }},
		{title: "视图：Agent", hint: "与 Agent 对话", run: func() { a.rightSection = "agent"; a.rightPanel = true }},
		{title: "视图：插件面板（原型）", hint: "插件用声明式视图树画的界面", run: func() { a.rightSection = "plugin-demo"; a.rightPanel = true }},
		{title: "任务：运行命令…", hint: "在项目目录中运行命令", run: func() { a.sidebar = true; a.section = "tasks" }},
		{title: "视图：切换侧栏", hint: "显示或隐藏文件树", run: func() { a.sidebar = !a.sidebar }},
		{title: "视图：切换终端", hint: "显示或隐藏终端面板", run: func() {
			if a.showBottom && a.bottomTab == "terminal" {
				a.showBottom = false
			} else {
				a.showTerminal()
			}
		}},
		{title: "视图：切换右侧面板", hint: "显示或隐藏大纲与 Agent", run: func() { a.rightPanel = !a.rightPanel }},
		{title: "视图：专注模式", hint: "隐藏工具窗，只留编辑器", run: func() { a.zen = !a.zen }},
		{title: "视图：切换明暗主题", hint: "在浅色与深色之间切换", run: func() { a.dark = !a.dark }},
		{title: "视图：自动换行", hint: "长行是否折行显示", run: func() { a.wrap = !a.wrap }},
		{title: "视图：刷新文件树与仓库", hint: "重新读取目录和 Git 状态", run: func() { a.refreshIndex(); a.loadDir(a.root); a.refreshGit() }},
		{title: "终端：新建终端", hint: "在项目目录启动 shell", run: a.newTerminal},
		{title: "Git：提交已暂存的更改", hint: "使用暂存区内容创建提交", run: func() { a.sidebar = true; a.section = "commit" }},
		{title: "Git：查看日志", hint: "提交图、筛选与文件变更", run: a.showLog},
		{title: "Git：切换分支…", hint: "搜索分支与 Git 操作", run: func() { a.branchMenuOpen = true; a.loadBranches() }},
		{title: "Git：新建分支…", hint: "创建并切换到一个新分支", run: func() { a.branchPrompt = &branchPrompt{} }},
		{title: "Git：检出标签或修订…", hint: "检出标签、提交或远程分支", run: func() { a.revisionPrompt = &revisionPrompt{} }},
	}
	if a.active() != nil {
		list = append(list, command{title: "文件：重新加载当前文件", hint: "放弃窗口中的内容并读取磁盘版本", run: func() {
			if tab := a.active(); tab != nil {
				a.reloadFile(tab)
			}
		}})
	}
	return list
}

// matchCommands keeps the commands whose title or hint holds the query, the
// ones whose words hold it in order first, the ones used often ahead.
func matchCommands(commands []command, query string, uses map[string]int) []command {
	ordered := append([]command(nil), commands...)
	sort.SliceStable(ordered, func(i, j int) bool {
		return uses[ordered[i].title] > uses[ordered[j].title]
	})
	commands = ordered
	query = strings.ToLower(strings.TrimSpace(query))
	if query == "" {
		return commands
	}
	exact := []command{}
	fuzzy := []command{}
	for _, c := range commands {
		title := strings.ToLower(c.title)
		if strings.Contains(title, query) {
			exact = append(exact, c)
			continue
		}
		if strings.Contains(strings.ToLower(c.hint), query) {
			exact = append(exact, c)
			continue
		}
		needle := []rune(query)
		next := 0
		for _, r := range title {
			if next < len(needle) && r == needle[next] {
				next++
			}
		}
		if next == len(needle) {
			fuzzy = append(fuzzy, c)
		}
	}
	return append(exact, fuzzy...)
}

// runPaletteCommand runs the chosen command and closes the palette.
func (a *app) runPaletteCommand(matches []command, index int) {
	if index < 0 || index >= len(matches) {
		return
	}
	a.paletteOpen = false
	a.paletteQuery = ""
	matches[index].run()
}

func (a *app) paletteView(c *ui.Context, p palette) {
	if !a.paletteOpen {
		return
	}
	matches := matchCommands(a.commands(), a.paletteQuery, a.commandUses)
	if a.paletteQuery != a.paletteShown {
		a.paletteShown = a.paletteQuery
		a.paletteSelected = 0
	}
	if a.paletteSelected >= len(matches) {
		a.paletteSelected = max(0, len(matches)-1)
	}
	run := func(index int) {
		if index >= 0 && index < len(matches) {
			a.useCommand(matches[index].title)
		}
		a.runPaletteCommand(matches, index)
	}
	ui.Modal(c, &a.paletteOpen, func() {
		ui.Column(c).Width(560).Padding(12).Gap(8).Children(func() {
			ui.TextInput(c, &a.paletteQuery).FillWidth().Placeholder("输入命令名称…").Label("命令面板").AutoFocus()
			if len(matches) == 0 {
				ui.Text(c, "没有匹配的命令").FontSize(12).TextColor(p.muted)
				ui.Text(c, "↑↓ 选择 · Enter 执行 · Esc 关闭").FontSize(10).TextColor(p.muted)
				return
			}
			state := ui.ListState{
				Selected: &a.paletteSelected,
				Key:      func(i int) any { return matches[i].title },
				Label:    func(i int) string { return matches[i].title },
			}
			list := ui.List(c, &state, len(matches), func(i int) {
				item := matches[i]
				b := ui.ButtonBase(c).FillWidth().Height(38).Padding(4, 8).Radius(5).Label(item.title)
				b.Children(func() {
					ui.Column(c).FillWidth().Gap(2).Children(func() {
						ui.Text(c, item.title).FontSize(12.5).TextColor(p.text).SingleLine().Ellipsis("…")
						ui.Text(c, item.hint).FontSize(10).TextColor(p.muted).SingleLine().Ellipsis("…")
					})
				})
			})
			if list.Submitted() {
				run(a.paletteSelected)
			}
			ui.Text(c, "↑↓ 选择 · Enter 执行 · Esc 关闭").FontSize(10).TextColor(p.muted)
		})
	})
}
