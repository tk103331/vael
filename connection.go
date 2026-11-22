package main

import (
	"fmt"
	"path"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/egoist/mygo/ui"
	"vael/internal/core"
)

func (a *app) joinPath(root, name string) string {
	if a.connection != nil {
		return path.Join(root, name)
	}
	return filepath.Join(root, name)
}
func (a *app) relativePath(root, name string) (string, error) {
	if a.connection != nil {
		prefix := strings.TrimSuffix(root, "/") + "/"
		if strings.HasPrefix(name, prefix) {
			return strings.TrimPrefix(name, prefix), nil
		}
		return "", fmt.Errorf("outside project")
	}
	return filepath.Rel(root, name)
}
func (a *app) showConnectionPrompt() {
	c := core.Connection{Kind: core.KindSSH, Root: "/home", Binary: ""}
	if a.connection != nil {
		c = *a.connection
	}
	a.connectionPrompt = &c
	a.profileName = ""
	a.connectionPort = ""
	if c.Port > 0 {
		a.connectionPort = strconv.Itoa(c.Port)
	}
}
func (a *app) connectEnvironment(c core.Connection, discard ...bool) {
	if err := c.Validate(); err != nil {
		a.report(err)
		return
	}
	if a.dirty() && len(discard) == 0 {
		a.confirm = &confirmation{"切换工作环境？", "切换会关闭当前文件和终端，未保存的修改会丢失。", func() { a.connectEnvironment(c, true) }}
		return
	}
	version, ctx := a.beginConnection()
	edits := a.captureEdits()
	steps := []string{}
	go func() {
		var backend *core.Client
		var err error
		var info core.FileInfo
		// The host is asked what it is, and the core that sits beside the
		// editor is sent to it if there is not one, as Lapce sends its
		// proxy before it opens a remote project.
		if local := core.LocalCorePath(); local != "" {
			steps = append(steps, "安装核心")
			installed, installErr := c.EnsureCore(ctx, local)
			if installErr != nil && c.Kind == core.KindSSH {
				// Without a core on the host nothing works; the editor
				// still tries the command the user asked for, which may
				// point at a core of its own.
				err = installErr
			} else if installErr == nil && c.Binary == "" {
				c.Binary = installed
			}
		}
		if err == nil {
			steps = append(steps, "启动核心")
			backend, err = core.Connect(ctx, c)
		}
		if err == nil {
			steps = append(steps, "打开项目")
			info, err = backend.Stat(ctx, c.Root)
			if err == nil && !info.Directory {
				err = fmt.Errorf("目标项目不是文件夹")
			}
		}
		a.update(func() {
			if version != a.connectionVersion {
				if backend != nil {
					backend.Close()
				}
				return
			}
			a.finishConnection()
			if err != nil {
				if backend != nil {
					backend.Close()
				}
				a.report(err)
				return
			}
			c.Root = info.Path
			open := func() { a.connectionPrompt = nil; a.useProject(backend, &c, info.Path) }
			// A user may have continued typing while the connection was starting.
			if a.newEditsSince(edits) {
				backend.Close()
				a.confirm = &confirmation{"切换工作环境？", "连接过程中有文件被修改。切换会丢弃未保存的修改。", func() { a.connectEnvironment(c, true) }}
			} else {
				open()
			}
		}, func() {
			if backend != nil {
				backend.Close()
			}
		})
	}()
}
func (a *app) connectionDialog(c *ui.Context, p palette) {
	if a.connectionPrompt == nil {
		return
	}
	config := a.connectionPrompt
	open := true
	ui.Modal(c, &open, func() {
		ui.Column(c).Width(500).Padding(22).Gap(14).Children(func() {
			ui.Text(c, "连接工作环境").FontSize(18).FontWeight(650)
			if quiet(c, "已保存的环境与最近项目", false, p) {
				open = false
				a.connectionPrompt = nil
				a.settingsOpen = true
				a.settingsSection = settingsEnv
			}
			ui.Row(c).Gap(6).Children(func() {
				for _, kind := range []string{"ssh", "wsl", "docker"} {
					if quiet(c, strings.ToUpper(kind), config.Kind == kind, p) && !a.connecting {
						config.Kind = kind
						config.Target = ""
					}
				}
			})
			if config.Kind == core.KindSSH {
				if port, err := strconv.Atoi(strings.TrimSpace(a.connectionPort)); err == nil && port > 0 && port < 65536 {
					config.Port = port
				} else if strings.TrimSpace(a.connectionPort) != "" {
					config.Port = 0
				}
			} else {
				config.Port = 0
			}
			targetLabel := "SSH 别名或 user@host"
			hint := "使用本机 OpenSSH 配置及密钥代理；请先在终端完成首次连接。"
			if config.Kind == "wsl" {
				targetLabel = "WSL 发行版（留空使用默认）"
				hint = "UI 运行在 Windows；核心运行在对应 WSL 发行版中。"
			}
			if config.Kind == "docker" {
				targetLabel = "运行中的容器名称或 ID"
				hint = "使用本机 Docker CLI 当前上下文；目标容器需已运行。"
			}
			ui.Text(c, targetLabel).FontSize(12).TextColor(p.muted)
			ui.TextInput(c, &config.Target).FillWidth().ReadOnly(a.connecting).Label("环境目标")
			if config.Kind == core.KindSSH {
				ui.Row(c).FillWidth().Gap(8).Children(func() {
					ui.Text(c, "端口").FontSize(12).TextColor(p.muted)
					ui.TextInput(c, &a.connectionPort).Width(90).Placeholder("默认").Label("SSH 端口").ReadOnly(a.connecting)
					ui.Spacer(c)
				})
			}
			ui.Text(c, "目标环境中的项目绝对路径").FontSize(12).TextColor(p.muted)
			ui.TextInput(c, &config.Root).FillWidth().ReadOnly(a.connecting).Label("环境项目路径")
			ui.Text(c, "目标环境中的核心程序").FontSize(12).TextColor(p.muted)
			ui.TextInput(c, &config.Binary).FillWidth().Placeholder("留空则自动安装").Label("核心程序路径").ReadOnly(a.connecting)
			ui.TextInput(c, &a.profileName).FillWidth().Placeholder("配置名称（可选）").Label("连接配置名称").ReadOnly(a.connecting)
			if ui.Button(c, "保存连接配置").Disabled(a.connecting).Clicked() {
				a.saveConnectionProfile(*config)
			}
			ui.Text(c, hint).FontSize(11).TextColor(p.muted)
			ui.Text(c, "连接时会把编辑器旁的 vael-core 上传到目标环境并自动安装（SSH）；目标环境还需 Git。").FontSize(11).TextColor(p.muted)
			ui.Row(c).FillWidth().Gap(8).Children(func() {
				if ui.Button(c, "打开本地项目").Disabled(a.connecting).Clicked() {
					open = false
					a.chooseProject()
				}
				ui.Spacer(c)
				if ui.Button(c, "取消").Clicked() {
					open = false
				}
				label := "连接"
				if a.connecting {
					label = "正在连接…"
				}
				if ui.PrimaryButton(c, label).Disabled(a.connecting).Clicked() {
					a.connectEnvironment(*config)
				}
			})
		})
	})
	if !open {
		a.cancelConnection()
		a.connectionPrompt = nil
	}
}
