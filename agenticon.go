package main

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/egoist/mygo/ui"
	"vael/internal/workspace"
)

// The icons the ACP registry gives an agent are SVG files: they are
// fetched once, cached beside the agents, and shown wherever the agent
// is named—its conversation and the settings.

// agentSVG is an agent's icon, or nil before it has been fetched. The
// fetch happens in the background, and the next frame draws it.
func (a *app) agentSVG(entry workspace.Agent) *ui.SVG {
	if entry.Icon == "" {
		return nil
	}
	key := entry.Icon
	if svg, ok := a.agentIcons[key]; ok {
		return svg
	}
	path := a.agentIconPath(key)
	if path != "" {
		if data, err := os.ReadFile(path); err == nil {
			if svg, err := ui.ParseSVG(data); err == nil {
				a.agentIcons[key] = svg
				return svg
			}
		}
	}
	if a.agentIconBusy[key] {
		return nil
	}
	a.agentIconBusy[key] = true
	go a.fetchAgentIcon(key, entry.Icon)
	return nil
}

// currentAgentSVG is the icon of the agent the panel uses.
func (a *app) currentAgentSVG() *ui.SVG {
	chosen := a.agents.Chosen(a.agentChosen)
	if chosen == nil {
		return nil
	}
	return a.agentSVG(*chosen)
}

// agentLogo is the icon of the current agent, or the built-in mark while
// its registry icon is missing or still being fetched.
func (a *app) agentLogo() *ui.SVG {
	if svg := a.currentAgentSVG(); svg != nil {
		return svg
	}
	return agentIcon
}

// agentIconPath is where an icon is kept, by the hash of its address.
func (a *app) agentIconPath(key string) string {
	if a.agentsPath == "" {
		return ""
	}
	sum := sha256.Sum256([]byte(key))
	return filepath.Join(filepath.Dir(a.agentsPath), "agent-icons", hex.EncodeToString(sum[:])[:16]+".svg")
}

// fetchAgentIcon downloads an icon of the registry and caches it.
func (a *app) fetchAgentIcon(key, icon string) {
	url := icon
	if !strings.Contains(url, "://") {
		url = "https://" + url
	}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err == nil {
		req.Header.Set("User-Agent", "Vael")
	}
	var data []byte
	if err == nil {
		var resp *http.Response
		resp, err = (&http.Client{}).Do(req)
		if err == nil {
			defer resp.Body.Close()
			if resp.StatusCode == http.StatusOK {
				data, err = io.ReadAll(io.LimitReader(resp.Body, 1<<20))
			} else {
				err = &os.PathError{Op: "下载图标", Path: url, Err: os.ErrInvalid}
			}
		}
	}
	if err == nil && !strings.Contains(string(data), "<svg") {
		err = &os.PathError{Op: "不是 SVG", Path: url, Err: os.ErrInvalid}
	}
	if err == nil {
		if path := a.agentIconPath(key); path != "" {
			if dir := filepath.Dir(path); dir != "" {
				_ = os.MkdirAll(dir, 0o755)
			}
			_ = os.WriteFile(path, data, 0o644)
		}
	}
	a.update(func() {
		delete(a.agentIconBusy, key)
		if err != nil {
			return
		}
		if svg, parseErr := ui.ParseSVG(data); parseErr == nil {
			a.agentIcons[key] = svg
		}
	})
}
