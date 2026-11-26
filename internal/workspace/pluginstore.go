package workspace

import (
	"encoding/json"
	"os"
	"time"
)

// PluginStore is the index of the plugins the editor offers: where it
// came from, when it was read, and what it holds.
type PluginStore struct {
	// URL is where the index was read from.
	URL string `json:"url,omitempty"`
	// UpdatedAt is when the index was read, so that it is not read
	// again for a while.
	UpdatedAt string `json:"updatedAt,omitempty"`
	// Plugins are the plugins the index offers.
	Plugins []PluginStoreEntry `json:"plugins,omitempty"`
}

// PluginStoreEntry is one plugin of an index: what it is, which version,
// and where its package is.
type PluginStoreEntry struct {
	ID          string `json:"id"`
	Name        string `json:"name,omitempty"`
	Version     string `json:"version,omitempty"`
	Description string `json:"description,omitempty"`
	// URL is where the package is; SHA256 is the sum it must have.
	URL    string `json:"url"`
	SHA256 string `json:"sha256,omitempty"`
	// Runtime is the runtime the plugin needs: js or wasm.
	Runtime string `json:"runtime,omitempty"`
}

// Label is the name the index shows a plugin by.
func (e PluginStoreEntry) Label() string {
	if e.Name != "" {
		return e.Name
	}
	return e.ID
}

// Fresh tells whether the index is old enough to be read again.
func (s PluginStore) Fresh() bool {
	if s.URL == "" || len(s.Plugins) == 0 {
		return false
	}
	at, err := time.Parse(time.RFC3339, s.UpdatedAt)
	if err != nil {
		return false
	}
	return time.Since(at) < 6*time.Hour
}

func LoadPluginStore(file string) (PluginStore, error) {
	var store PluginStore
	data, err := os.ReadFile(file)
	if err != nil {
		return store, err
	}
	err = json.Unmarshal(data, &store)
	return store, err
}

func SavePluginStore(file string, store PluginStore) error { return saveJSON(file, store) }

// Newer tells whether the version of the index is newer than the one that
// is installed.
func (e PluginStoreEntry) Newer(installed string) bool {
	if installed == "" {
		return true
	}
	return compareVersions(e.Version, installed) > 0
}

// compareVersions says how two versions stand to each other: -1, 0 or 1.
func compareVersions(a, b string) int {
	parts := func(v string) []int {
		var out []int
		for _, piece := range splitDots(v) {
			n := 0
			for _, r := range piece {
				if r < '0' || r > '9' {
					break
				}
				n = n*10 + int(r-'0')
			}
			out = append(out, n)
		}
		return out
	}
	left, right := parts(a), parts(b)
	for i := 0; i < len(left) || i < len(right); i++ {
		var x, y int
		if i < len(left) {
			x = left[i]
		}
		if i < len(right) {
			y = right[i]
		}
		if x != y {
			if x > y {
				return 1
			}
			return -1
		}
	}
	return 0
}

// splitDots cuts a version at its dots and its dashes.
func splitDots(v string) []string {
	var out []string
	piece := ""
	for _, r := range v {
		if r == '.' || r == '-' {
			out = append(out, piece)
			piece = ""
			continue
		}
		piece += string(r)
	}
	return append(out, piece)
}
