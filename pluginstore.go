package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/egoist/mygo/ui"
	"vael/internal/workspace"
)

// The store of the plugins: an index the editor reads, and installs from.
// The index is a JSON file with the plugins it offers; the editor reads
// it now and then, and keeps it for a while.

// loadPluginStore reads the index of the plugins, from the address the
// user gave, and keeps it for a while.
func (a *app) loadPluginStore(force bool) {
	url := strings.TrimSpace(a.pluginStore.URL)
	if url == "" {
		return
	}
	if !force && a.pluginStore.Fresh() {
		return
	}
	a.pluginStoreLoading = true
	a.pluginStoreErr = ""
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		entries, err := fetchPluginStore(ctx, url)
		a.update(func() {
			a.pluginStoreLoading = false
			if err != nil {
				a.pluginStoreErr = err.Error()
				return
			}
			a.pluginStore.Plugins = entries
			a.pluginStore.URL = url
			a.pluginStore.UpdatedAt = time.Now().Format(time.RFC3339)
			if a.pluginStorePath != "" {
				a.report(workspace.SavePluginStore(a.pluginStorePath, a.pluginStore))
			}
		})
	}()
}

// fetchPluginStore reads an index of plugins.
func fetchPluginStore(ctx context.Context, url string) ([]workspace.PluginStoreEntry, error) {
	if !strings.HasPrefix(url, "http://") && !strings.HasPrefix(url, "https://") {
		url = "https://" + url
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "Vael")
	resp, err := (&http.Client{Timeout: 30 * time.Second}).Do(req)
	if err != nil {
		return nil, fmt.Errorf("读取插件索引失败：%w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("读取插件索引失败：%s", resp.Status)
	}
	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("读取插件索引失败：%w", err)
	}
	var index struct {
		Plugins []workspace.PluginStoreEntry `json:"plugins"`
	}
	if err := json.Unmarshal(data, &index); err != nil {
		// An index that is a plain list of plugins is read as well.
		var list []workspace.PluginStoreEntry
		if err := json.Unmarshal(data, &list); err != nil {
			return nil, fmt.Errorf("插件索引不是合法的 JSON：%w", err)
		}
		index.Plugins = list
	}
	return index.Plugins, nil
}

// installPluginFromStore installs a plugin of the index, or updates the
// one that is there.
func (a *app) installPluginFromStore(entry workspace.PluginStoreEntry) error {
	if entry.URL == "" {
		return errors.New("索引里的插件没有下载地址")
	}
	url := entry.URL
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
	if entry.SHA256 != "" && !verifySHA(data, entry.SHA256) {
		return errors.New("下载的插件包校验失败")
	}
	if err := a.installPluginPackage(data, url); err != nil {
		return err
	}
	// The package must be what its entry in the index says it is.
	if entry.Version != "" && a.installedVersion(entry.ID) != entry.Version {
		return fmt.Errorf("包里的版本与索引里的 %s 不一致", entry.Version)
	}
	return nil
}

// installedVersion is the version of a plugin that is installed.
func (a *app) installedVersion(name string) string {
	for _, item := range a.plugins {
		if item != nil && item.manifest.Name == name {
			return item.manifest.Version
		}
	}
	return ""
}

// pluginStoreView is the store of the plugins: where its index is, and
// what it offers.
func (a *app) pluginStoreView(c *ui.Context, p palette) {
	ui.Column(c).FillWidth().Gap(6).Children(func() {
		ui.Text(c, "插件商店").FontSize(11.5).FontWeight(650).TextColor(p.text)
		ui.Text(c, "填一个索引地址（JSON，里面是插件的清单），编辑器会隔一段时间读一次，装插件时校验 SHA-256。").
			FontSize(10).TextColor(p.muted)
		ui.Row(c).FillWidth().Gap(6).Children(func() {
			input := ui.TextInput(c, &a.pluginStore.URL).Grow(1).MinWidth(0).
				Placeholder("https://…/plugins.json")
			if input.Submitted() {
				a.loadPluginStore(true)
			}
			if quiet(c, "刷新", false, p) {
				a.loadPluginStore(true)
			}
		})
		if a.pluginStoreLoading {
			ui.Text(c, "读取中…").FontSize(10).TextColor(p.muted)
		}
		if a.pluginStoreErr != "" {
			ui.Text(c, a.pluginStoreErr).FontSize(10).TextColor(p.danger)
		}
		for _, entry := range a.pluginStore.Plugins {
			entry := entry
			installed := a.installedVersion(entry.ID)
			ui.Row(c).FillWidth().Gap(8).AlignItems(ui.Center).Padding(4, 2).Children(func() {
				ui.Column(c).Grow(1).MinWidth(0).Gap(2).Children(func() {
					ui.Text(c, entry.Label()).FontSize(11.5).TextColor(p.text).SingleLine().Ellipsis("…")
					detail := entry.Description
					if entry.Version != "" {
						if detail != "" {
							detail += " · "
						}
						detail += entry.Version
					}
					if detail != "" {
						ui.Text(c, detail).FontSize(9.5).TextColor(p.muted).SingleLine().Ellipsis("…")
					}
				})
				label := "安装"
				if installed != "" {
					label = "重新安装"
					if entry.Newer(installed) {
						label = "更新到 " + entry.Version
					}
				}
				if quiet(c, label, false, p) {
					go func() {
						err := a.installPluginFromStore(entry)
						a.update(func() {
							if err != nil {
								a.message, a.messageKind = err.Error(), "error"
								return
							}
							a.message, a.messageKind = "已安装 "+entry.Label(), "success"
						})
					}()
				}
			})
		}
	})
}
