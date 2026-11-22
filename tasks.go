package main

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/egoist/mygo/ui"
)

// task is a command that was run in the project, and what it left behind.
type task struct {
	command  string
	output   string
	code     int
	duration int64
	timedOut bool
	running  bool
}

// runTask runs a command in the project directory, through the core, so that
// it runs where the project is: on this machine, or in the environment the
// editor is connected to.
func (a *app) runTask(command string) {
	if !a.requireProject() {
		return
	}
	command = strings.TrimSpace(command)
	if command == "" {
		return
	}
	if a.task.running {
		return
	}
	a.task = task{command: command, running: true}
	ctx, cancel := context.WithCancel(a.projectCtx)
	a.taskCancel = cancel
	generation, root := a.generation, a.root
	backend := a.backend
	go func() {
		result, err := backend.RunStream(ctx, root, command, 5*time.Minute, func(chunk string) {
			// What the command prints shows as it prints it, where the
			// project is: on this machine or where the editor is connected.
			a.update(func() {
				if generation != a.generation {
					return
				}
				a.task.output = tailOf(a.task.output + chunk)
			})
		})
		a.update(func() {
			if generation != a.generation {
				return
			}
			a.task.running = false
			a.taskCancel = nil
			if ctx.Err() != nil {
				return
			}
			a.report(err)
			if err != nil {
				a.task.output = tailOf(err.Error())
				a.task.code = -1
				return
			}
			a.task.output = tailOf(result.Output)
			a.task.code = result.Code
			a.task.duration = result.Duration
			a.task.timedOut = result.TimedOut
			a.rememberTask(command)
		})
	}()
}

// tailOf keeps the last lines of a task's output, so that a noisy command
// cannot grow the window's memory without end.
func tailOf(output string) string {
	const limit = 128 << 10
	if len(output) <= limit {
		return output
	}
	output = output[len(output)-limit:]
	if i := strings.IndexByte(output, '\n'); i >= 0 {
		output = output[i+1:]
	}
	return output
}

// sendToTerminal types a command into the terminal that is open, as if the
// user had, so that an interactive program can take it from there.
func (a *app) sendToTerminal(command string) {
	command = strings.TrimSpace(command)
	if command == "" {
		return
	}
	if a.termSelected < 0 || a.termSelected >= len(a.terms) {
		a.message = "请先在下方打开一个终端，再发送命令。"
		return
	}
	a.terms[a.termSelected].term.Paste(command + "\n")
}

// rememberTask keeps the commands that were run, so that they can be run
// again with one keystroke.
func (a *app) rememberTask(command string) {
	for _, old := range a.taskHistory {
		if old == command {
			return
		}
	}
	a.taskHistory = append([]string{command}, a.taskHistory...)
	if len(a.taskHistory) > 20 {
		a.taskHistory = a.taskHistory[:20]
	}
}

// taskView shows the command that was run, and what it printed.
func (a *app) taskView(c *ui.Context, p palette) {
	ui.Text(c, "任务").FontSize(13).FontWeight(650).TextColor(p.text)
	ui.Text(c, "在项目目录中运行命令").FontSize(11).TextColor(p.muted)
	input := ui.TextInput(c, &a.taskCommand).FillWidth().Placeholder("例如 go build ./...").Label("任务命令")
	submitted := input.Submitted()
	ui.Row(c).FillWidth().Gap(4).Children(func() {
		label := "运行"
		if a.task.running {
			label = "运行中…"
		}
		if ui.PrimaryButton(c, label).Disabled(a.task.running).Clicked() || submitted {
			a.runTask(a.taskCommand)
		}
		if a.task.running && iconButton(c, stopIcon, "停止", false, p) {
			if a.taskCancel != nil {
				a.taskCancel()
			}
			a.task.running = false
			a.taskCancel = nil
		}
		if !a.task.running && strings.TrimSpace(a.taskCommand) != "" && iconButton(c, terminalIcon, "发送到终端", false, p) {
			a.sendToTerminal(a.taskCommand)
		}
		if !a.task.running && a.task.command != "" && iconButton(c, xCircleIcon, "清空输出", false, p) {
			a.task = task{}
		}
	})
	if len(a.taskHistory) > 0 {
		ui.Text(c, "最近运行").FontSize(11).FontWeight(650).TextColor(p.muted).Padding(6, 0, 0, 0)
		ui.Scroll(c).FillWidth().MaxHeight(120).Children(func() {
			ui.Column(c).FillWidth().Gap(1).Children(func() {
				for _, command := range a.taskHistory {
					command := command
					row := ui.ButtonBase(c).FillWidth().Height(24).Padding(0, 6).Radius(4).Label(command)
					if row.Hovered() {
						row.Background(p.edge)
					}
					row.Children(func() {
						ui.Text(c, command).Font("monospace").FontSize(11).TextColor(p.muted).SingleLine().Ellipsis("…")
					})
					if row.Clicked() {
						a.taskCommand = command
						a.runTask(command)
					}
				}
			})
		})
	}
	if a.task.command != "" {
		ui.Text(c, "$ "+a.task.command).Font("monospace").FontSize(11.5).TextColor(p.text).Padding(8, 0, 0, 0)
	}
	status := ""
	if a.task.running {
		status = "运行中…"
	} else if a.task.command != "" {
		status = fmt.Sprintf("退出码 %d · %d ms", a.task.code, a.task.duration)
		if a.task.timedOut {
			status = "超时停止 · " + status
		}
	}
	if status != "" {
		color := p.muted
		if a.task.code != 0 && !a.task.running {
			color = p.warn
		}
		ui.Text(c, status).FontSize(11).TextColor(color)
	}
	if a.task.output != "" {
		ui.Scroll(c).FillWidth().Grow(1).Children(func() {
			ui.Text(c, a.task.output).Font("monospace").FontSize(11.5).TextColor(p.text).LineHeight(1.4)
		})
	} else if !a.task.running {
		ui.Text(c, "还没有运行过任务。").FontSize(12).TextColor(p.muted)
	}
}
