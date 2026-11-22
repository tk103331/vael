package main

import (
	"os"
	"path"
	"path/filepath"
	"strings"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
	"vael/internal/workspace"
)

// projectPrompt is the dialog that creates a project: a folder of a name
// in a parent folder.
type projectPrompt struct {
	name, parent string
}

// clonePrompt is the dialog that clones a repository.
type clonePrompt struct {
	url, parent, name string
}

// projectBadge is the badge of a project, as an IDE's: the language it is
// written in, from the files of its root.
func (a *app) projectBadge(root string) (string, ui.Color, bool) {
	files := make(map[string]bool, len(a.index.Files))
	for _, f := range a.index.Files {
		files[f] = true
	}
	has := func(name string) bool {
		if files[name] {
			return true
		}
		if root == "" || a.connection != nil {
			return false
		}
		_, err := os.Stat(filepath.Join(root, name))
		return err == nil
	}
	switch {
	case has("go.mod"):
		return "GO", ui.Hex("#00a7d0"), true
	case has("package.json"):
		return "JS", ui.Hex("#b8a01a"), true
	case has("Cargo.toml"):
		return "RS", ui.Hex("#b7410e"), true
	case has("pyproject.toml") || has("requirements.txt"):
		return "PY", ui.Hex("#3572a5"), true
	case has("pom.xml") || has("build.gradle") || has("build.gradle.kts"):
		return "JV", ui.Hex("#b07219"), true
	case has("Gemfile"):
		return "RB", ui.Hex("#701516"), true
	case has("composer.json"):
		return "PHP", ui.Hex("#4f5d95"), true
	case has("CMakeLists.txt"):
		return "C", ui.Hex("#5c6bc0"), true
	}
	return "", ui.Color{}, false
}

// projectBadgeView draws the badge of a project, or a folder when its
// language is not known.
func (a *app) projectBadgeView(c *ui.Context, p palette, root string) {
	text, color, ok := a.projectBadge(root)
	if !ok {
		ui.Icon(c, folderIcon).Size(14, 14).TextColor(p.accent)
		return
	}
	ui.Box(c).Size(20, 20).Radius(4).Background(color).Center().Children(func() {
		ui.Text(c, text).FontSize(8.5).FontWeight(750).TextColor(ui.Hex("#ffffff"))
	})
}

// menuRow is a row of a popup menu: an icon, a label, and a hint at the
// end.
func menuRow(c *ui.Context, p palette, icon *ui.SVG, label, hint string, fn func()) {
	b := ui.ButtonBase(c).FillWidth().Height(26).Padding(0, 6).Radius(5).Gap(6).AlignItems(ui.Center).Label(label)
	if b.Hovered() {
		b.Background(p.edge)
	}
	b.Children(func() {
		ui.Icon(c, icon).Size(13, 13).TextColor(p.muted)
		ui.Text(c, label).FontSize(12).TextColor(p.text).Grow(1).SingleLine().Ellipsis("…")
		if hint != "" {
			ui.Text(c, hint).FontSize(10).TextColor(p.muted)
		}
	})
	if b.Clicked() {
		fn()
	}
}

// menuHeader is the title of a section of a popup menu.
func menuHeader(c *ui.Context, p palette, text string) {
	ui.Text(c, text).FontSize(10).FontWeight(650).TextColor(p.muted).Padding(4, 6, 1, 6)
}

// projectRow is a project of the switcher: its badge, name and path.
func (a *app) projectRow(c *ui.Context, p palette, e workspace.Environment, current bool) {
	name := filepath.Base(e.Root)
	if name == "." || name == string(filepath.Separator) || name == "" {
		name = e.Root
	}
	detail := e.Root
	if e.Kind != "" {
		detail = strings.ToUpper(e.Kind) + " · " + e.Target + " · " + e.Root
	}
	b := ui.ButtonBase(c).FillWidth().Height(36).Padding(0, 6).Radius(5).Gap(6).AlignItems(ui.Center).Label(name)
	if current {
		b.Background(p.selected)
	} else if b.Hovered() {
		b.Background(p.edge)
	}
	b.Children(func() {
		a.projectBadgeView(c, p, e.Root)
		ui.Column(c).Grow(1).Children(func() {
			ui.Text(c, name).FontSize(12).TextColor(p.text).SingleLine().Ellipsis("…")
			ui.Text(c, detail).FontSize(9.5).TextColor(p.muted).SingleLine().Ellipsis("…")
		})
		if current {
			ui.Icon(c, checkIcon).Size(12, 12).TextColor(p.accent)
		}
	})
	if b.Clicked() {
		if current {
			return
		}
		a.projectMenuOpen = false
		if e.Kind != "" {
			a.openEnvironment(e)
		} else {
			a.openProject(e.Root)
		}
	}
}

// projectMenu is the switcher of projects, as an IDE's: the ways to open
// one, the project that is open, and the ones opened lately.
func (a *app) projectMenu(c *ui.Context, p palette) {
	current := a.currentEnvironment()
	ui.Column(c).Width(340).Padding(4).Gap(1).Children(func() {
		menuRow(c, p, plusIcon, "新建项目…", "", func() {
			a.projectMenuOpen = false
			a.openProjectPrompt()
		})
		menuRow(c, p, folderIcon, "打开…", "", func() {
			a.projectMenuOpen = false
			a.chooseProject()
		})
		menuRow(c, p, gitIcon, "克隆仓库…", "", func() {
			a.projectMenuOpen = false
			a.openClonePrompt()
		})
		ui.Divider(c)
		menuHeader(c, p, "打开的项目")
		if a.root != "" {
			a.projectRow(c, p, current, true)
		} else {
			ui.Text(c, "还没有打开项目").FontSize(11).TextColor(p.muted).Padding(6, 6, 3, 6)
		}
		recent := make([]workspace.Environment, 0, len(a.environments.Recent))
		for _, e := range a.environments.Recent {
			if a.root != "" && e.Key() == current.Key() {
				continue
			}
			recent = append(recent, e)
			if len(recent) == 6 {
				break
			}
		}
		if len(recent) > 0 {
			ui.Divider(c)
			menuHeader(c, p, "最近的项目")
			for _, e := range recent {
				a.projectRow(c, p, e, false)
			}
		}
		ui.Divider(c)
		menuRow(c, p, globeIcon, "管理工作环境…", "", func() {
			a.projectMenuOpen = false
			a.environmentsOpen = true
		})
	})
}

// openProjectPrompt opens the dialog that creates a project, in the folder
// of the project that is open, or in the home folder.
func (a *app) openProjectPrompt() {
	a.projectPrompt = &projectPrompt{parent: a.parentFolder()}
}

// parentFolder is the folder a project is made or cloned in: next to the
// project that is open, or the home folder.
func (a *app) parentFolder() string {
	root := a.root
	if a.connection != nil {
		root = a.connection.Root
	}
	parent := ""
	if root != "" {
		parent = filepath.Dir(root)
	}
	if parent == "" || parent == "." {
		parent, _ = os.UserHomeDir()
	}
	return parent
}

// createProject makes the folder of the new project and opens it.
func (a *app) createProject(prompt *projectPrompt) {
	name := strings.TrimSpace(prompt.name)
	if name == "" {
		a.message = "请输入项目名称。"
		return
	}
	if strings.ContainsAny(name, `/\`) || name == "." || name == ".." {
		a.message = "项目名称不能包含路径分隔符。"
		return
	}
	parent := strings.TrimSpace(prompt.parent)
	if parent == "" {
		a.message = "请选择项目位置。"
		return
	}
	if a.connection != nil {
		// The folder is made where the core runs, and the editor connects
		// to it there.
		conn := *a.connection
		generation := a.generation
		backend := a.backend
		ctx := a.projectCtx
		go func() {
			err := backend.Create(ctx, parent, name, true)
			a.update(func() {
				if generation != a.generation {
					return
				}
				a.report(err)
				if err != nil {
					return
				}
				conn.Root = path.Join(parent, name)
				a.connectEnvironment(conn)
			})
		}()
		return
	}
	dir := filepath.Join(parent, name)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		a.report(err)
		return
	}
	a.openProject(dir)
}

// parentField is the folder a project is made or cloned in: a text field
// in a remote environment, where the native picker cannot reach, and the
// path with a picker on this machine.
func (a *app) parentField(c *ui.Context, p palette, parent *string, title string) {
	if a.connection != nil {
		ui.TextInput(c, parent).FillWidth().Placeholder("/home/user/projects").Label(title)
		return
	}
	ui.Row(c).FillWidth().Gap(8).AlignItems(ui.Center).Children(func() {
		ui.Text(c, *parent).FontSize(12).TextColor(p.text).Grow(1).SingleLine().Ellipsis("…")
		if ui.Button(c, "选择…").Clicked() {
			go func() {
				paths, err := mygo.Dialog.Open(mygo.OpenDialogOptions{Parent: a.win, Title: title, Directory: true})
				a.update(func() {
					a.report(err)
					if len(paths) > 0 {
						*parent = paths[0]
					}
				})
			}()
		}
	})
}

// projectPromptView is the dialog that creates a project.
func (a *app) projectPromptView(c *ui.Context, p palette) {
	if a.projectPrompt == nil {
		return
	}
	prompt := a.projectPrompt
	open := true
	ui.Modal(c, &open, func() {
		ui.Column(c).Width(460).Padding(22).Gap(12).Children(func() {
			ui.Text(c, "新建项目").FontSize(17).FontWeight(650)
			ui.Text(c, "项目名称").FontSize(11).TextColor(p.muted)
			input := ui.TextInput(c, &prompt.name).FillWidth().Placeholder("my-project").Label("项目名称").AutoFocus()
			submitted := input.Submitted()
			ui.Text(c, "位置").FontSize(11).TextColor(p.muted)
			a.parentField(c, p, &prompt.parent, "选择项目位置")
			ui.Row(c).FillWidth().Gap(8).Children(func() {
				ui.Spacer(c)
				if ui.Button(c, "取消").Clicked() {
					open = false
				}
				if ui.PrimaryButton(c, "创建").Clicked() || submitted {
					open = false
					a.createProject(prompt)
				}
			})
		})
	})
	if !open {
		a.projectPrompt = nil
	}
}

// openClonePrompt opens the dialog that clones a repository, into the
// folder of the project that is open, or the home folder.
func (a *app) openClonePrompt() {
	a.clonePrompt = &clonePrompt{parent: a.parentFolder()}
}

// cloneName is the folder a repository is cloned into: its name without
// the .git suffix.
func cloneName(url string) string {
	url = strings.TrimRight(strings.TrimSpace(url), "/")
	url = strings.TrimSuffix(url, ".git")
	name := filepath.Base(url)
	if name == "." || name == "/" || name == "" {
		return "repository"
	}
	return name
}

// cloneRepository clones a repository into the folder of the dialog, and
// opens it.
func (a *app) cloneRepository(prompt *clonePrompt) {
	url := strings.TrimSpace(prompt.url)
	if url == "" {
		a.message = "请输入仓库地址。"
		return
	}
	name := strings.TrimSpace(prompt.name)
	if name == "" {
		name = cloneName(url)
	}
	if strings.ContainsAny(name, `/\`) || name == "." || name == ".." {
		a.message = "目标文件夹名称不能包含路径分隔符。"
		return
	}
	parent := strings.TrimSpace(prompt.parent)
	if parent == "" {
		a.message = "请选择克隆位置。"
		return
	}
	conn := a.connection
	generation := a.generation
	backend := a.backend
	ctx := a.projectCtx
	a.message = "正在克隆 " + url + "…"
	go func() {
		dir, err := backend.Clone(ctx, parent, url, name)
		a.update(func() {
			if generation != a.generation {
				return
			}
			a.report(err)
			if err != nil {
				return
			}
			a.message = "已克隆到 " + dir
			if conn != nil {
				next := *conn
				next.Root = dir
				a.connectEnvironment(next)
				return
			}
			a.openProject(dir)
		})
	}()
}

// clonePromptView is the dialog that clones a repository.
func (a *app) clonePromptView(c *ui.Context, p palette) {
	if a.clonePrompt == nil {
		return
	}
	prompt := a.clonePrompt
	open := true
	ui.Modal(c, &open, func() {
		ui.Column(c).Width(480).Padding(22).Gap(12).Children(func() {
			ui.Text(c, "克隆仓库").FontSize(17).FontWeight(650)
			ui.Text(c, "仓库地址").FontSize(11).TextColor(p.muted)
			url := ui.TextInput(c, &prompt.url).FillWidth().Placeholder("https://github.com/user/repo.git").Label("仓库地址").AutoFocus()
			submitted := url.Submitted()
			ui.Text(c, "克隆到").FontSize(11).TextColor(p.muted)
			a.parentField(c, p, &prompt.parent, "选择克隆位置")
			ui.Text(c, "文件夹名称").FontSize(11).TextColor(p.muted)
			ui.TextInput(c, &prompt.name).FillWidth().Placeholder("留空则使用仓库名").Label("文件夹名称")
			ui.Row(c).FillWidth().Gap(8).Children(func() {
				ui.Spacer(c)
				if ui.Button(c, "取消").Clicked() {
					open = false
				}
				if ui.PrimaryButton(c, "克隆").Clicked() || submitted {
					open = false
					a.cloneRepository(prompt)
				}
			})
		})
	})
	if !open {
		a.clonePrompt = nil
	}
}
