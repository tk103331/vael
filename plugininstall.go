package main

import (
	"archive/tar"
	"archive/zip"
	"bytes"
	"compress/gzip"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
	"vael/internal/workspace"
)

// Installing a plugin: a package is a zip or a tar.gz with a plugin.json
// in it and the plugin's own files beside it. The editor unpacks it into
// a temporary folder, reads the manifest, checks it, and puts the plugin
// in its place — the way an editor installs an extension.

// pluginPackageKind reads the kind of a package from what it starts
// with, so that a download without an extension is still read.
func pluginPackageKind(data []byte) string {
	switch {
	case len(data) > 4 && bytes.HasPrefix(data, []byte("PK\x03\x04")):
		return "zip"
	case len(data) > 2 && data[0] == 0x1f && data[1] == 0x8b:
		return "tar.gz"
	case len(data) > 262 && string(data[257:262]) == "ustar":
		return "tar"
	}
	return ""
}

// extractPluginPackage unpacks a package into dir, keeping every file
// inside it.
func extractPluginPackage(data []byte, kind, dir string) error {
	switch kind {
	case "zip":
		reader, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
		if err != nil {
			return err
		}
		for _, file := range reader.File {
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
	case "tar":
		return extractTar(tar.NewReader(bytes.NewReader(data)), dir)
	}
	return fmt.Errorf("不认识的插件包格式")
}

// findPluginManifest finds the manifest in what was unpacked: at the top
// of the package, or in the one folder it holds.
func findPluginManifest(dir string) (string, workspace.PluginManifest, error) {
	var manifest workspace.PluginManifest
	read := func(path string) error {
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		return json.Unmarshal(data, &manifest)
	}
	if err := read(filepath.Join(dir, "plugin.json")); err == nil {
		return dir, manifest, nil
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		return "", manifest, err
	}
	for _, entry := range entries {
		if !entry.IsDir() || strings.HasPrefix(entry.Name(), ".") {
			continue
		}
		inner := filepath.Join(dir, entry.Name())
		if err := read(filepath.Join(inner, "plugin.json")); err == nil {
			return inner, manifest, nil
		}
	}
	return "", manifest, errors.New("包里没有 plugin.json")
}

// installPluginPackage puts a plugin in the editor's plugin directory,
// replacing the one of the same name if it is there.
func (a *app) installPluginPackage(data []byte, source string) error {
	if a.pluginsDir == "" {
		return errors.New("没有插件目录")
	}
	kind := pluginPackageKind(data)
	if kind == "" {
		return fmt.Errorf("这不像一个插件包：%s", source)
	}
	if err := os.MkdirAll(a.pluginsDir, 0o755); err != nil {
		return err
	}
	temp, err := os.MkdirTemp(a.pluginsDir, ".install-*")
	if err != nil {
		return err
	}
	defer os.RemoveAll(temp)
	if err := extractPluginPackage(data, kind, temp); err != nil {
		return fmt.Errorf("解压失败：%w", err)
	}
	dir, manifest, err := findPluginManifest(temp)
	if err != nil {
		return err
	}
	if err := manifest.Check(); err != nil {
		return err
	}
	target := filepath.Join(a.pluginsDir, manifest.Name)
	if err := os.RemoveAll(target); err != nil {
		return err
	}
	if err := os.Rename(dir, target); err != nil {
		return fmt.Errorf("安装失败：%w", err)
	}
	a.loadPlugins()
	return nil
}

// installPluginFromURL downloads a package and installs it. The sum, when
// it is given, must be the one the file has.
func (a *app) installPluginFromURL(url, sum string) error {
	url = strings.TrimSpace(url)
	if url == "" {
		return errors.New("没有地址")
	}
	if !strings.HasPrefix(url, "http://") && !strings.HasPrefix(url, "https://") {
		url = "https://" + url
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return err
	}
	req.Header.Set("User-Agent", "Vael")
	resp, err := (&http.Client{Timeout: 5 * time.Minute}).Do(req)
	if err != nil {
		return fmt.Errorf("下载失败：%w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("下载失败：%s", resp.Status)
	}
	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("下载失败：%w", err)
	}
	if sum != "" && !verifySHA(data, sum) {
		return errors.New("下载的插件包校验失败")
	}
	return a.installPluginPackage(data, url)
}

// uninstallPlugin takes a plugin away, with what it kept of its own.
func (a *app) uninstallPlugin(name string) error {
	if a.pluginsDir == "" || name == "" {
		return errors.New("没有插件目录")
	}
	target := filepath.Join(a.pluginsDir, name)
	if _, err := os.Stat(target); err != nil {
		return err
	}
	if err := os.RemoveAll(target); err != nil {
		return err
	}
	delete(a.pluginStates, name)
	if a.pluginStorage != nil {
		delete(a.pluginStorage, name)
	}
	if a.pluginStatesPath != "" {
		a.report(workspace.SavePluginStates(a.pluginStatesPath, a.pluginStates))
	}
	if a.pluginStoragePath != "" {
		a.report(workspace.SavePluginStorage(a.pluginStoragePath, a.pluginStorage))
	}
	delete(a.pluginViews, name)
	if a.pluginPanelPlugin == name {
		a.pluginPanelPlugin = ""
	}
	a.loadPlugins()
	return nil
}

// choosePluginPackage asks for a package of a plugin and installs it.
func (a *app) choosePluginPackage() {
	go func() {
		paths, err := mygo.Dialog.Open(mygo.OpenDialogOptions{
			Parent: a.win, Title: "安装插件包", Directory: false,
		})
		a.update(func() {
			a.report(err)
			if err != nil || len(paths) == 0 {
				return
			}
			data, err := os.ReadFile(paths[0])
			if err != nil {
				a.report(err)
				return
			}
			if err := a.installPluginPackage(data, paths[0]); err != nil {
				a.message, a.messageKind = err.Error(), "error"
				return
			}
			a.message, a.messageKind = "插件已安装", "success"
		})
	}()
}

// pluginInstallDialog asks for the address of a package, and the sum it
// should have.
func (a *app) pluginInstallDialog(c *ui.Context, p palette) {
	if a.pluginURL == nil {
		return
	}
	open := true
	prompt := a.pluginURL
	ui.Modal(c, &open, func() {
		ui.Column(c).Width(460).Padding(22).Gap(14).Children(func() {
			ui.Text(c, "从网址安装插件").FontSize(17).FontWeight(650)
			ui.Text(c, "插件包是 zip 或 tar.gz，里面有 plugin.json。").FontSize(12).TextColor(p.muted)
			url := ui.TextInput(c, &prompt.url).FillWidth().Placeholder("https://…/plugin.zip").AutoFocus()
			sum := ui.TextInput(c, &prompt.sum).FillWidth().Placeholder("SHA-256（可不填）")
			submitted := url.Submitted() || sum.Submitted()
			ui.Row(c).FillWidth().Gap(8).Children(func() {
				ui.Spacer(c)
				if ui.Button(c, "取消").Clicked() {
					open = false
				}
				if ui.PrimaryButton(c, "安装").Clicked() || submitted {
					address, check := prompt.url, prompt.sum
					a.pluginURL = nil
					open = false
					go a.installPluginFromURLChecked(address, check)
				}
			})
		})
	})
	if !open {
		a.pluginURL = nil
	}
}

// installPluginFromURLChecked installs a plugin from the web, and says
// how it went.
func (a *app) installPluginFromURLChecked(url, sum string) {
	err := a.installPluginFromURL(url, sum)
	a.update(func() {
		if err != nil {
			a.message, a.messageKind = err.Error(), "error"
			return
		}
		a.message, a.messageKind = "插件已安装", "success"
	})
}
