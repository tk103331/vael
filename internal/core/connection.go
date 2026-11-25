package core

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"time"
)

// Version is the version of the core and of the app that speaks with it.
// It changes with the core's behavior, so that a remote one is replaced
// when it is older.
const Version = "0.5.2"

// Connection describes how to reach the machine the core runs on. The
// secrets belong to SSH, WSL and Docker, not to the editor's settings.
type Connection struct {
	Kind, Target, Root, Binary string
	Port                       int
}

// Command is a program to run, with its arguments.
type Command struct {
	Program string
	Args    []string
}

// The kinds of connection the editor knows.
const (
	KindSSH    = "ssh"
	KindWSL    = "wsl"
	KindDocker = "docker"
)

// Validate says whether the connection can be made.
func (c Connection) Validate() error {
	switch c.Kind {
	case KindSSH, KindWSL, KindDocker:
	default:
		return fmt.Errorf("不支持的连接方式: %s", c.Kind)
	}
	if c.Kind != KindWSL && (c.Target == "" || strings.HasPrefix(c.Target, "-") || strings.ContainsAny(c.Target, "\r\n\x00")) {
		return fmt.Errorf("请输入有效的目标名称")
	}
	if c.Kind == KindSSH && strings.ContainsAny(c.Target, " \t") {
		return fmt.Errorf("SSH 目标应为配置别名或 user@host")
	}
	if !strings.HasPrefix(c.Root, "/") || strings.ContainsRune(c.Root, 0) {
		return fmt.Errorf("目标项目需要 Linux/Unix 绝对路径")
	}
	if c.Binary != "" && strings.ContainsAny(c.Binary, "\r\n\x00") {
		return fmt.Errorf("核心程序路径不应包含换行")
	}
	return nil
}

// DefaultCorePath is where the editor installs the core on a host, as Lapce
// installs its proxy at a path of its own. The home directory is written as
// $HOME, which the shell of the host expands.
const DefaultCorePath = "$HOME/.local/share/vael/core/vael-core"

// corePath is the path the core is run from: the one the connection names,
// or the one the editor installs it at.
func (c Connection) corePath() string {
	if c.Binary != "" {
		return expandTilde(c.Binary)
	}
	return DefaultCorePath
}

// expandTilde turns a leading ~ of a path into $HOME, which the shell of the
// host expands; a quoted ~ would name a directory of that literal name.
func expandTilde(path string) string {
	switch {
	case path == "~":
		return "$HOME"
	case strings.HasPrefix(path, "~/"):
		return "$HOME/" + path[2:]
	}
	return path
}

// shellPath quotes a path for the shell of the host. A path that holds $ is
// quoted with double quotes, so that $HOME expands.
func shellPath(path string) string {
	if strings.Contains(path, "$") {
		return `"` + strings.ReplaceAll(path, `"`, `\"`) + `"`
	}
	return QuoteShell(path)
}

// QuoteShell quotes a text for the shell of the host.
func QuoteShell(s string) string { return "'" + strings.ReplaceAll(s, "'", "'\"'\"'") + "'" }

// sshArgs are the options of every SSH connection the editor makes. The
// master connection is shared by the core's channel and the terminal's, as
// Lapce does, so that a remote project opens with one handshake.
func sshArgs() []string {
	return []string{
		"-o", "ControlMaster=auto",
		"-o", "ControlPath=~/.ssh/cm_%C",
		"-o", "ControlPersist=30m",
		"-o", "ConnectTimeout=15",
	}
}

// hostCommand is a command run on the host: a probe, an installation, the
// core itself.
func (c Connection) hostCommand(command string) (Command, error) {
	if err := c.Validate(); err != nil {
		return Command{}, err
	}
	switch c.Kind {
	case KindSSH:
		args := append([]string{"-T"}, sshArgs()...)
		if c.Port > 0 {
			args = append(args, "-p", fmt.Sprintf("%d", c.Port))
		}
		return Command{"ssh", append(args, c.Target, command)}, nil
	case KindDocker:
		return Command{"docker", []string{"exec", "--", c.Target, "/bin/sh", "-c", command}}, nil
	case KindWSL:
		args := []string{}
		if c.Target != "" {
			args = append(args, "--distribution", c.Target)
		}
		return Command{"wsl.exe", append(args, "--exec", "/bin/sh", "-c", command)}, nil
	}
	return Command{}, errors.New("未知的连接方式")
}

// CoreCommand is the command that starts the core on the host.
func (c Connection) CoreCommand() (Command, error) {
	if err := c.Validate(); err != nil {
		return Command{}, err
	}
	switch c.Kind {
	case KindSSH:
		args := append([]string{"-T"}, sshArgs()...)
		if c.Port > 0 {
			args = append(args, "-p", fmt.Sprintf("%d", c.Port))
		}
		return Command{"ssh", append(args, c.Target, shellPath(c.corePath())+" --stdio")}, nil
	case KindDocker:
		// exec runs no shell: the core is started by the shell of the
		// container, so that ~ and $HOME in its path expand there.
		return Command{"docker", []string{"exec", "-i", "--", c.Target, "/bin/sh", "-c", shellPath(c.corePath()) + " --stdio"}}, nil
	case KindWSL:
		args := []string{}
		if c.Target != "" {
			args = append(args, "--distribution", c.Target)
		}
		return Command{"wsl.exe", append(args, "--exec", "/bin/sh", "-c", shellPath(c.corePath())+" --stdio")}, nil
	}
	return Command{}, errors.New("未知的连接方式")
}

// TerminalCommand is the command that runs a shell on the host, in the
// project.
func (c Connection) TerminalCommand() (Command, error) {
	if err := c.Validate(); err != nil {
		return Command{}, err
	}
	switch c.Kind {
	case KindSSH:
		args := append([]string{"-tt"}, sshArgs()...)
		if c.Port > 0 {
			args = append(args, "-p", fmt.Sprintf("%d", c.Port))
		}
		shell := `cd ` + QuoteShell(c.Root) + ` && exec "${SHELL:-/bin/sh}" -l`
		return Command{"ssh", append(args, c.Target, shell)}, nil
	case KindDocker:
		return Command{"docker", []string{"exec", "-it", "--workdir", c.Root, "--", c.Target, "/bin/sh", "-l"}}, nil
	case KindWSL:
		args := []string{}
		if c.Target != "" {
			args = append(args, "--distribution", c.Target)
		}
		return Command{"wsl.exe", append(args, "--cd", c.Root, "--exec", "/bin/sh", "-l")}, nil
	}
	return Command{}, errors.New("未知的连接方式")
}

// HostInfo is what the host is: its system and its processor.
type HostInfo struct {
	Platform string
	Arch     string
}

// Probe asks the host what it is, as Lapce does before it chooses the
// program to send it.
func (c Connection) Probe(ctx context.Context) (HostInfo, error) {
	command, err := c.hostCommand("uname -s; uname -m")
	if err != nil {
		return HostInfo{}, err
	}
	out, err := runCommand(ctx, command)
	if err != nil {
		return HostInfo{}, err
	}
	lines := strings.Split(strings.TrimSpace(out), "\n")
	info := HostInfo{}
	if len(lines) > 0 {
		info.Platform = strings.ToLower(strings.TrimSpace(lines[0]))
	}
	if len(lines) > 1 {
		info.Arch = strings.ToLower(strings.TrimSpace(lines[1]))
	}
	return info, nil
}

// CorePath is where the core lives on the host.
func (c Connection) CorePath() string { return c.corePath() }

// EnsureCore makes sure the host has a core of this version, and says where
// it is. The binary that sits beside the editor is sent to the host, as
// Lapce sends its proxy.
func (c Connection) EnsureCore(ctx context.Context, local string) (string, error) {
	path := c.CorePath()
	if c.Kind != KindSSH {
		// WSL and Docker have a core of their own: the editor only asks
		// whether it is there.
		command, err := c.hostCommand(shellPath(path) + " --version")
		if err != nil {
			return path, err
		}
		if _, err = runCommand(ctx, command); err != nil {
			return path, fmt.Errorf("目标环境中没有可用的核心（%s）：%w", path, err)
		}
		return path, nil
	}
	if local == "" {
		return path, nil
	}
	command, err := c.hostCommand(shellPath(path) + " --version")
	if err != nil {
		return path, err
	}
	if out, err := runCommand(ctx, command); err == nil && strings.TrimSpace(out) == "vael-core "+Version {
		return path, nil
	}
	if err := c.upload(ctx, local, path); err != nil {
		return path, err
	}
	if out, err := runCommand(ctx, command); err != nil || strings.TrimSpace(out) != "vael-core "+Version {
		return path, fmt.Errorf("核心已上传，但无法在目标环境运行：%v %s", err, out)
	}
	return path, nil
}

// upload sends the core to the host. The bytes go through the shell, which
// the editor already trusts, so that no other tool is needed.
func (c Connection) upload(ctx context.Context, local, remote string) error {
	file, err := os.Open(local)
	if err != nil {
		return err
	}
	defer file.Close()
	command, err := c.hostCommand(fmt.Sprintf("mkdir -p %s && cat > %s && chmod +x %s",
		shellPath(filepath.Dir(remote)), shellPath(remote), shellPath(remote)))
	if err != nil {
		return err
	}
	cmd := exec.CommandContext(ctx, command.Program, command.Args...)
	cmd.Stdin = file
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("上传核心失败: %w\n%s", err, strings.TrimSpace(string(output)))
	}
	return nil
}

// Connect starts the core on the host and talks with it.
func Connect(ctx context.Context, c Connection) (*Client, error) {
	command, err := c.CoreCommand()
	if err != nil {
		return nil, err
	}
	cmd := exec.Command(command.Program, command.Args...)
	stderr := &tailBuffer{}
	cmd.Stderr = stderr
	input, err := cmd.StdinPipe()
	if err != nil {
		return nil, err
	}
	output, err := cmd.StdoutPipe()
	if err != nil {
		input.Close()
		return nil, err
	}
	if err = cmd.Start(); err != nil {
		input.Close()
		output.Close()
		return nil, err
	}
	stream := NewStream(input, output, func() error {
		_ = cmd.Process.Kill()
		_ = cmd.Wait()
		return nil
	})
	client := NewClient(stream)
	handshake, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	info, err := client.Info(handshake)
	if err != nil {
		client.Close()
		return nil, fmt.Errorf("%s 核心连接失败: %w\n%s", c.Kind, err, stderr.String())
	}
	if info.Protocol != ProtocolVersion {
		client.Close()
		return nil, fmt.Errorf("核心协议不兼容: UI=%d, core=%d", ProtocolVersion, info.Protocol)
	}
	// A core without a feature of ours is older than this app: it is said
	// plainly, for the next connect to upload the new core.
	missing := []string{}
	have := map[string]bool{}
	for _, f := range info.Features {
		have[f] = true
	}
	for _, f := range RequiredFeatures {
		if !have[f] {
			missing = append(missing, f)
		}
	}
	if len(missing) > 0 {
		client.Close()
		return nil, fmt.Errorf("远端核心过旧（缺少 %s）：重新连接会自动上传新核心", strings.Join(missing, ", "))
	}
	return client, nil
}

// LocalCorePath is the core that sits beside the editor, if there is one.
func LocalCorePath() string {
	executable, err := os.Executable()
	if err != nil {
		return ""
	}
	name := "vael-core"
	if runtime.GOOS == "windows" {
		name = "vael-core.exe"
	}
	dir := filepath.Dir(executable)
	candidate := filepath.Join(dir, name)
	if _, err := os.Stat(candidate); err == nil {
		return candidate
	}
	// A macOS bundle keeps the resources mygo build ships, such as the
	// core, in Contents/Resources, one directory over from Contents/MacOS.
	if runtime.GOOS == "darwin" {
		candidate := filepath.Join(dir, "..", "Resources", name)
		if _, err := os.Stat(candidate); err == nil {
			return candidate
		}
	}
	return ""
}

// runCommand runs a command and returns what it printed.
func runCommand(ctx context.Context, command Command) (string, error) {
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, command.Program, command.Args...)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return string(output), fmt.Errorf("%w: %s", err, strings.TrimSpace(string(output)))
	}
	return string(output), nil
}

type tailBuffer struct {
	mu   sync.Mutex
	data []byte
}

func (b *tailBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.data = append(b.data, p...)
	if len(b.data) > 8192 {
		b.data = b.data[len(b.data)-8192:]
	}
	return len(p), nil
}
func (b *tailBuffer) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return strings.TrimSpace(string(b.data))
}
