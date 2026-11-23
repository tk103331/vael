package main

import (
	"archive/tar"
	"archive/zip"
	"bytes"
	"compress/bzip2"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"time"

	"vael/internal/workspace"
)

// The ACP Registry: the indexed agents an editor can add, with how each
// one is distributed—an npx or uvx package, or a binary for the platform.
// https://agentclientprotocol.com/get-started/registry

// acpRegistryURL is the index of the registry, as its site names it.
const acpRegistryURL = "https://cdn.agentclientprotocol.com/registry/v1/latest/registry.json"

// registryMaxAge is how long a fetched index is kept without asking the
// network again.
const registryMaxAge = 6 * time.Hour

// acpRegistryIndex is what the registry's index holds.
type acpRegistryIndex struct {
	Version string             `json:"version"`
	Agents  []acpRegistryAgent `json:"agents"`
}

// acpRegistryAgent is an agent of the registry: where it comes from and
// how it is distributed.
type acpRegistryAgent struct {
	ID           string          `json:"id"`
	Name         string          `json:"name"`
	Version      string          `json:"version"`
	Description  string          `json:"description"`
	Repository   string          `json:"repository"`
	Website      string          `json:"website"`
	Authors      []string        `json:"authors"`
	License      string          `json:"license"`
	Distribution acpDistribution `json:"distribution"`
}

// acpDistribution is how an agent is shipped: a package that npx or uvx
// runs, or a binary for each platform.
type acpDistribution struct {
	Npx    *acpPackage                `json:"npx"`
	Uvx    *acpPackage                `json:"uvx"`
	Binary map[string]acpBinaryTarget `json:"binary"`
}

// acpPackage is a package an agent runs from.
type acpPackage struct {
	Package string            `json:"package"`
	Args    []string          `json:"args"`
	Env     map[string]string `json:"env"`
}

// acpBinaryTarget is an agent's binary for one platform, as an archive or
// a file of its own.
type acpBinaryTarget struct {
	Archive string            `json:"archive"`
	Cmd     string            `json:"cmd"`
	Args    []string          `json:"args"`
	Env     map[string]string `json:"env"`
	SHA256  string            `json:"sha256"`
}

// parseRegistry reads the registry's index.
func parseRegistry(data []byte) (acpRegistryIndex, error) {
	var index acpRegistryIndex
	if err := json.Unmarshal(data, &index); err != nil {
		return index, fmt.Errorf("读取 ACP Registry 失败：%w", err)
	}
	if len(index.Agents) == 0 {
		return index, errors.New("ACP Registry 里没有 Agent")
	}
	for i := range index.Agents {
		agent := &index.Agents[i]
		if agent.Name == "" {
			agent.Name = agent.ID
		}
	}
	sort.SliceStable(index.Agents, func(i, j int) bool {
		return strings.ToLower(index.Agents[i].Name) < strings.ToLower(index.Agents[j].Name)
	})
	return index, nil
}

// acpPlatformKey names a platform as the registry does.
func acpPlatformKey(goos, goarch string) string {
	switch goos + "/" + goarch {
	case "darwin/arm64":
		return "darwin-aarch64"
	case "darwin/amd64":
		return "darwin-x86_64"
	case "linux/arm64":
		return "linux-aarch64"
	case "linux/amd64":
		return "linux-x86_64"
	case "windows/arm64":
		return "windows-aarch64"
	case "windows/amd64":
		return "windows-x86_64"
	}
	return ""
}

// acpBinaryFor is the binary an agent has for a platform, and whether it
// is a musl build, as Alpine has: the glibc one is preferred.
func acpBinaryFor(agent acpRegistryAgent, goos, goarch string) (acpBinaryTarget, string, bool) {
	key := acpPlatformKey(goos, goarch)
	if key == "" {
		return acpBinaryTarget{}, "", false
	}
	if target, ok := agent.Distribution.Binary[key]; ok {
		return target, key, true
	}
	if goos == "linux" {
		if target, ok := agent.Distribution.Binary[key+"-musl"]; ok {
			return target, key + "-musl", true
		}
	}
	return acpBinaryTarget{}, "", false
}

// acpRegistryKind is how an agent would be added here, given what the
// machine has: an npx or uvx package when its tool is installed, else a
// binary for the platform, else a package whose tool is missing.
func acpRegistryKind(agent acpRegistryAgent, goos, goarch string, has func(string) bool) string {
	dist := agent.Distribution
	switch {
	case dist.Npx != nil && has("npx"):
		return "npx"
	case dist.Uvx != nil && has("uvx"):
		return "uvx"
	}
	if _, _, ok := acpBinaryFor(agent, goos, goarch); ok {
		return "binary"
	}
	switch {
	case dist.Npx != nil:
		return "npx"
	case dist.Uvx != nil:
		return "uvx"
	}
	return ""
}

// absent reports whether a tool is not on the path: a nil has asks the
// machine.
func absent(has func(string) bool, tool string) bool {
	if has == nil {
		_, err := exec.LookPath(tool)
		return err != nil
	}
	return !has(tool)
}

// acpPackageCommand is how a package distribution starts.
func acpPackageCommand(kind string, pkg *acpPackage) string {
	parts := []string{kind}
	if kind == "npx" {
		parts = append(parts, "-y")
	}
	parts = append(parts, pkg.Package)
	parts = append(parts, pkg.Args...)
	return strings.Join(parts, " ")
}

// agentCommand is a command that runs a file with arguments, quoted for
// the shell the editor starts agents with.
func agentCommand(path string, args []string, goos string) string {
	quote := shellQuote
	if goos == "windows" {
		quote = windowsQuote
	}
	parts := []string{quote(path)}
	for _, arg := range args {
		parts = append(parts, quote(arg))
	}
	return strings.Join(parts, " ")
}

// shellQuote quotes a word for sh.
func shellQuote(word string) string {
	if word == "" {
		return "''"
	}
	return "'" + strings.ReplaceAll(word, "'", `'\''`) + "'"
}

// windowsQuote quotes a word for cmd.
func windowsQuote(word string) string {
	if word == "" {
		return `""`
	}
	return `"` + strings.ReplaceAll(word, `"`, `\"`) + `"`
}

// registryFetch reads the index from the network. It is a variable so that
// a test can answer for the registry.
var registryFetch = func(ctx context.Context) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, acpRegistryURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "Vael")
	client := &http.Client{Timeout: 25 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("连接 ACP Registry 失败：%w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("ACP Registry 返回 %s", resp.Status)
	}
	// The index is tens of kilobytes; a limit keeps a wrong address from
	// filling memory.
	data, err := io.ReadAll(io.LimitReader(resp.Body, 8<<20))
	if err != nil {
		return nil, err
	}
	return data, nil
}

// loadRegistry reads the registry's index, from the cache when it is fresh
// unless asked to fetch, and keeps what the network gave.
func loadRegistryIndex(ctx context.Context, cache string, force bool) (acpRegistryIndex, error) {
	if !force {
		if index, err := readRegistryCache(cache); err == nil {
			return index, nil
		}
	}
	data, err := registryFetch(ctx)
	if err != nil {
		if index, cacheErr := readRegistryCache(cache); cacheErr == nil {
			return index, nil
		}
		return acpRegistryIndex{}, err
	}
	index, err := parseRegistry(data)
	if err != nil {
		return index, err
	}
	if cache != "" {
		if dir := filepath.Dir(cache); dir != "" {
			_ = os.MkdirAll(dir, 0o755)
		}
		_ = os.WriteFile(cache, data, 0o644)
	}
	return index, nil
}

// readRegistryCache reads a cached index, when it is young enough.
func readRegistryCache(path string) (acpRegistryIndex, error) {
	var index acpRegistryIndex
	if path == "" {
		return index, errors.New("没有缓存的 ACP Registry")
	}
	info, err := os.Stat(path)
	if err != nil {
		return index, err
	}
	if time.Since(info.ModTime()) > registryMaxAge {
		return index, errors.New("缓存的 ACP Registry 已过期")
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return index, err
	}
	return parseRegistry(data)
}

// installAgentBinary downloads the binary of an agent for this platform,
// unpacks it under root, and answers with how to start it.
func installAgentBinary(ctx context.Context, agent acpRegistryAgent, target acpBinaryTarget, root string) (string, error) {
	dir := filepath.Join(root, agent.ID, agent.Version)
	if binary := findAgentBinary(dir, target.Cmd); binary != "" {
		return agentCommand(binary, target.Args, runtime.GOOS), nil
	}
	url := target.Archive
	if !strings.Contains(url, "://") {
		url = "https://" + url
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("User-Agent", "Vael")
	client := &http.Client{Timeout: 10 * time.Minute}
	resp, err := client.Do(req)
	if err != nil {
		return "", fmt.Errorf("下载失败：%w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("下载失败：%s", resp.Status)
	}
	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("下载失败：%w", err)
	}
	if target.SHA256 != "" && !verifySHA(data, target.SHA256) {
		return "", errors.New("下载的文件校验失败")
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", err
	}
	if err := extractAgentArchive(data, archiveKind(url), dir, target.Cmd); err != nil {
		return "", fmt.Errorf("解压失败：%w", err)
	}
	binary := findAgentBinary(dir, target.Cmd)
	if binary == "" {
		return "", fmt.Errorf("下载的包里没有 %s", target.Cmd)
	}
	_ = os.Chmod(binary, 0o755)
	return agentCommand(binary, target.Args, runtime.GOOS), nil
}

// verifySHA tells whether data is what its SHA-256 sum says.
func verifySHA(data []byte, want string) bool {
	sum := sha256.Sum256(data)
	return strings.EqualFold(hex.EncodeToString(sum[:]), strings.TrimSpace(want))
}

// archiveKind reads the kind of an archive from its URL: gz, bz2, zip, or
// none for a file that is not an archive.
func archiveKind(url string) string {
	lower := strings.ToLower(url)
	switch {
	case strings.HasSuffix(lower, ".zip"):
		return "zip"
	case strings.HasSuffix(lower, ".tar.gz"), strings.HasSuffix(lower, ".tgz"):
		return "tar.gz"
	case strings.HasSuffix(lower, ".tar.bz2"):
		return "tar.bz2"
	case strings.HasSuffix(lower, ".tar"):
		return "tar"
	}
	// A download without an extension is a file of its own, as an
	// executable from a lookaside service.
	return ""
}

// extractAgentArchive unpacks an archive into dir, keeping it inside dir.
// A file that is not an archive is written as the file its command names,
// so that a bare executable works as a downloaded archive does.
func extractAgentArchive(data []byte, kind, dir, name string) error {
	switch kind {
	case "zip":
		r, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
		if err != nil {
			return err
		}
		for _, file := range r.File {
			if err := extractZipFile(file, dir); err != nil {
				return err
			}
		}
		return nil
	case "tar.gz":
		gz, err := gzip.NewReader(bytes.NewReader(data))
		if err != nil {
			return err
		}
		defer gz.Close()
		return extractTar(tar.NewReader(gz), dir)
	case "tar.bz2":
		return extractTar(tar.NewReader(bzip2.NewReader(bytes.NewReader(data))), dir)
	case "tar":
		return extractTar(tar.NewReader(bytes.NewReader(data)), dir)
	default:
		// The file is the agent's command, named as it is.
		file := filepath.Base(strings.ReplaceAll(name, "\\", "/"))
		if file == "" || file == "." {
			file = "agent"
		}
		return writeFileIn(filepath.Join(dir, file), data, 0o755)
	}
}

// extractZipFile writes one file of a zip into dir.
func extractZipFile(file *zip.File, dir string) error {
	path, err := fileIn(dir, file.Name)
	if err != nil {
		return err
	}
	if file.FileInfo().IsDir() {
		return os.MkdirAll(path, 0o755)
	}
	reader, err := file.Open()
	if err != nil {
		return err
	}
	defer reader.Close()
	data, err := io.ReadAll(reader)
	if err != nil {
		return err
	}
	// A zip keeps the executable bit in its mode; a file without one may
	// still be the agent's command, which installAgentBinary makes
	// executable once it finds it.
	mode := file.Mode().Perm()
	if mode == 0 {
		mode = 0o644
	}
	return writeFileIn(path, data, mode)
}

// extractTar writes the files of a tar stream into dir.
func extractTar(reader *tar.Reader, dir string) error {
	for {
		header, err := reader.Next()
		if errors.Is(err, io.EOF) {
			return nil
		}
		if err != nil {
			return err
		}
		path, err := fileIn(dir, header.Name)
		if err != nil {
			return err
		}
		switch header.Typeflag {
		case tar.TypeDir:
			if err := os.MkdirAll(path, 0o755); err != nil {
				return err
			}
		case tar.TypeSymlink:
			// The link must stay inside dir as well.
			target, err := fileIn(dir, filepath.Join(filepath.Dir(header.Name), header.Linkname))
			if err != nil {
				return err
			}
			if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
				return err
			}
			_ = os.Remove(path)
			if err := os.Symlink(target, path); err != nil {
				return err
			}
		case tar.TypeReg:
			data, err := io.ReadAll(reader)
			if err != nil {
				return err
			}
			if err := writeFileIn(path, data, os.FileMode(header.Mode).Perm()); err != nil {
				return err
			}
		}
	}
}

// fileIn is a path inside dir, from an archive's name, which may not leave
// dir.
func fileIn(dir, name string) (string, error) {
	clean := filepath.Clean(filepath.FromSlash(strings.TrimPrefix(name, "/")))
	if clean == "." || clean == "" {
		return "", errors.New("压缩包里有空的文件名")
	}
	if clean == ".." || strings.HasPrefix(clean, ".."+string(filepath.Separator)) || filepath.IsAbs(clean) {
		return "", fmt.Errorf("压缩包里的路径不安全：%s", name)
	}
	return filepath.Join(dir, clean), nil
}

// writeFileIn writes a file of an archive, making what it needs.
func writeFileIn(path string, data []byte, mode os.FileMode) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	if mode == 0 {
		mode = 0o644
	}
	if err := os.WriteFile(path, data, mode); err != nil {
		return err
	}
	return os.Chmod(path, mode)
}

// findAgentBinary finds the file an agent's command names in what was
// unpacked, wherever the archive put it.
func findAgentBinary(dir, cmd string) string {
	name := strings.TrimPrefix(strings.ReplaceAll(cmd, "\\", "/"), "./")
	name = filepath.Base(name)
	if name == "" || name == "." {
		return ""
	}
	var found string
	_ = filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() {
			return nil
		}
		if !strings.EqualFold(info.Name(), name) {
			return nil
		}
		if found == "" || len(path) < len(found) {
			found = path
		}
		return nil
	})
	return found
}

// registryCachePath is where the fetched index is kept, beside the agents
// the editor starts.
func (a *app) registryCachePath() string {
	if a.agentsPath == "" {
		return ""
	}
	return filepath.Join(filepath.Dir(a.agentsPath), "registry.json")
}

// agentsDir is where downloaded agents are kept.
func (a *app) agentsDir() string {
	if a.agentsPath == "" {
		return ""
	}
	return filepath.Join(filepath.Dir(a.agentsPath), "agents")
}

// loadRegistry reads the registry's index for the settings to offer, from
// the cache when it is fresh, and writes what the network gave.
func (a *app) loadRegistry(force bool) {
	if a.registryLoading {
		return
	}
	if !force && len(a.registry) > 0 {
		return
	}
	a.registryLoading = true
	a.registryErr = ""
	a.registryRun++
	run := a.registryRun
	cache := a.registryCachePath()
	go func() {
		index, err := loadRegistryIndex(context.Background(), cache, force)
		a.update(func() {
			if run != a.registryRun {
				return
			}
			a.registryLoading = false
			if err != nil {
				a.registryErr = err.Error()
				return
			}
			a.registry = index.Agents
		})
	}()
}

// hasRegistryTool tells whether the machine has a tool the registry runs
// packages with, asking once and keeping the answer.
func (a *app) hasRegistryTool(tool string) bool {
	if found, ok := a.registryHas[tool]; ok {
		return found
	}
	_, err := exec.LookPath(tool)
	a.registryHas[tool] = err == nil
	return err == nil
}

// addRegistryAgent adds an agent of the registry as it is distributed
// here: a package command, or the binary downloaded for the platform.
func (a *app) addRegistryAgent(agent acpRegistryAgent, kind string) {
	switch kind {
	case "npx":
		pkg := agent.Distribution.Npx
		a.addACPAgent(agent, acpPackageCommand("npx", pkg), pkg.Env)
	case "uvx":
		pkg := agent.Distribution.Uvx
		a.addACPAgent(agent, acpPackageCommand("uvx", pkg), pkg.Env)
	case "binary":
		target, _, ok := acpBinaryFor(agent, runtime.GOOS, runtime.GOARCH)
		if !ok {
			a.registryStatus[agent.ID] = "此平台没有二进制"
			return
		}
		a.installRegistryAgent(agent, target)
	default:
		a.registryStatus[agent.ID] = "此平台没有可用的分发"
	}
}

// installRegistryAgent downloads an agent's binary, then adds it, telling
// in its row how it goes.
func (a *app) installRegistryAgent(agent acpRegistryAgent, target acpBinaryTarget) {
	if a.registryStatus[agent.ID] == "下载中…" {
		return
	}
	root := a.agentsDir()
	if root == "" {
		a.registryStatus[agent.ID] = "没有配置目录"
		return
	}
	a.registryStatus[agent.ID] = "下载中…"
	go func() {
		command, err := installAgentBinary(context.Background(), agent, target, root)
		a.update(func() {
			if err != nil {
				a.registryStatus[agent.ID] = "失败：" + err.Error()
				return
			}
			delete(a.registryStatus, agent.ID)
			a.addACPAgent(agent, command, target.Env)
		})
	}()
}

// addACPAgent puts an agent among the ones the editor starts, and makes it
// the default when there is none.
func (a *app) addACPAgent(agent acpRegistryAgent, command string, env map[string]string) {
	entry := workspace.Agent{Name: agent.Name, Command: command, Env: env}
	if a.agents.Has(agent.Name) {
		return
	}
	if err := a.agents.Add(entry); err != nil {
		a.message = err.Error()
		return
	}
	if a.agents.Default == "" {
		a.agents.Default = agent.Name
	}
	a.saveAgents()
	a.message = "已添加 " + agent.Name
}
