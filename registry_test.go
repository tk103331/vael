package main

import (
	"archive/tar"
	"archive/zip"
	"bytes"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"
)

const testRegistry = `{
  "version": "1.0.0",
  "agents": [
    {"id": "zeta", "name": "Zeta", "version": "2.0.0", "description": "A package agent",
     "distribution": {"npx": {"package": "@zed/zeta@2.0.0", "args": ["--acp"], "env": {"Z": "1"}}}},
    {"id": "alpha", "name": "Alpha", "version": "1.2.3", "description": "A binary agent",
     "distribution": {"binary": {
       "darwin-aarch64": {"archive": "https://example.com/a.zip", "cmd": "./alpha", "args": ["serve"], "sha256": "00"},
       "linux-x86_64": {"archive": "https://example.com/a.tar.gz", "cmd": "./alpha", "sha256": "00"}}}},
    {"id": "only-zip", "name": "Only Zip", "version": "0.1.0",
     "distribution": {"binary": {"windows-x86_64": {"archive": "https://example.com/w.zip", "cmd": "./w.exe"}}}}
  ]
}`

func TestTheRegistryIsRead(t *testing.T) {
	index, err := parseRegistry([]byte(testRegistry))
	if err != nil {
		t.Fatal(err)
	}
	if len(index.Agents) != 3 || index.Agents[0].Name != "Alpha" || index.Agents[2].Name != "Zeta" {
		t.Fatalf("the agents: %+v", index.Agents)
	}
	alpha := index.Agents[0]
	target, platform, ok := acpBinaryFor(alpha, "darwin", "arm64")
	if !ok || platform != "darwin-aarch64" || target.Cmd != "./alpha" {
		t.Fatalf("the binary: %+v %q %v", target, platform, ok)
	}
	if _, _, ok := acpBinaryFor(alpha, "windows", "amd64"); ok {
		t.Fatal("Alpha has a binary for a platform it does not")
	}
	zeta := index.Agents[2]
	if zeta.Distribution.Npx == nil || zeta.Distribution.Npx.Package != "@zed/zeta@2.0.0" ||
		len(zeta.Distribution.Npx.Args) != 1 || zeta.Distribution.Npx.Env["Z"] != "1" {
		t.Fatalf("the package: %+v", zeta.Distribution.Npx)
	}
}

func TestThePlatformKeyIsTheRegistrys(t *testing.T) {
	for _, c := range []struct{ goos, goarch, want string }{
		{"darwin", "arm64", "darwin-aarch64"},
		{"darwin", "amd64", "darwin-x86_64"},
		{"linux", "arm64", "linux-aarch64"},
		{"linux", "amd64", "linux-x86_64"},
		{"windows", "arm64", "windows-aarch64"},
		{"windows", "amd64", "windows-x86_64"},
		{"freebsd", "amd64", ""},
	} {
		if got := acpPlatformKey(c.goos, c.goarch); got != c.want {
			t.Fatalf("%s/%s is %q, not %q", c.goos, c.goarch, got, c.want)
		}
	}
}

func TestTheRegistryUsesWhatTheMachineHas(t *testing.T) {
	index, _ := parseRegistry([]byte(testRegistry))
	alpha, zeta, onlyZip := index.Agents[0], index.Agents[2], index.Agents[1]
	has := func(tools ...string) func(string) bool {
		return func(tool string) bool {
			for _, has := range tools {
				if has == tool {
					return true
				}
			}
			return false
		}
	}
	// npx when it is there.
	if kind := acpRegistryKind(zeta, "darwin", "arm64", has("npx")); kind != "npx" {
		t.Fatalf("zeta with npx: %q", kind)
	}
	// The binary when npx is missing and the platform has one.
	if kind := acpRegistryKind(alpha, "darwin", "arm64", has()); kind != "binary" {
		t.Fatalf("alpha without npx: %q", kind)
	}
	// Neither tool nor binary for this platform: nothing.
	if kind := acpRegistryKind(onlyZip, "darwin", "arm64", has()); kind != "" {
		t.Fatalf("only-zip here: %q", kind)
	}
	// A package whose tool is missing is still offered, as installing the
	// tool is the user's to do.
	if kind := acpRegistryKind(zeta, "freebsd", "amd64", has()); kind != "npx" {
		t.Fatalf("zeta without npx: %q", kind)
	}
	if got := acpPackageCommand("npx", zeta.Distribution.Npx); got != "npx -y @zed/zeta@2.0.0 --acp" {
		t.Fatalf("the npx command: %q", got)
	}
	if got := acpPackageCommand("uvx", &acpPackage{Package: "fast-agent-acp==0.10.65", Args: []string{"-x"}}); got != "uvx fast-agent-acp==0.10.65 -x" {
		t.Fatalf("the uvx command: %q", got)
	}
}

func TestTheAgentCommandQuotes(t *testing.T) {
	if got := agentCommand("/Users/me/Agents/a b/tool", []string{"serve", "x' y"}, "darwin"); got != `'/Users/me/Agents/a b/tool' 'serve' 'x'\'' y'` {
		t.Fatalf("the command: %q", got)
	}
	if got := agentCommand(`C:\Agents\tool.exe`, []string{"acp"}, "windows"); got != `"C:\Agents\tool.exe" "acp"` {
		t.Fatalf("the windows command: %q", got)
	}
}

// tarGzOf makes an archive holding the files given.
func tarGzOf(t *testing.T, files map[string]string) []byte {
	t.Helper()
	var buf bytes.Buffer
	gz := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gz)
	for name, text := range files {
		if err := tw.WriteHeader(&tar.Header{Name: name, Mode: 0o755, Size: int64(len(text))}); err != nil {
			t.Fatal(err)
		}
		if _, err := tw.Write([]byte(text)); err != nil {
			t.Fatal(err)
		}
	}
	if err := tw.Close(); err != nil {
		t.Fatal(err)
	}
	if err := gz.Close(); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

// zipOf makes a zip holding the files given.
func zipOf(t *testing.T, files map[string]string) []byte {
	t.Helper()
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	for name, text := range files {
		w, err := zw.Create(name)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := w.Write([]byte(text)); err != nil {
			t.Fatal(err)
		}
	}
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

func TestTheRegistryUnpacksAnAgent(t *testing.T) {
	dir := t.TempDir()
	data := tarGzOf(t, map[string]string{
		"alpha-1.0/alpha":     "#!/bin/sh\necho hi\n",
		"alpha-1.0/README.md": "read me",
	})
	if err := extractAgentArchive(data, "tar.gz", dir, "./alpha"); err != nil {
		t.Fatal(err)
	}
	binary := findAgentBinary(dir, "./alpha")
	if binary == "" {
		t.Fatal("the binary was not found")
	}
	if info, err := os.Stat(binary); err != nil || info.Mode().Perm()&0o100 == 0 {
		t.Fatalf("the binary is not executable: %+v %v", info, err)
	}
	if !strings.HasSuffix(binary, filepath.Join("alpha-1.0", "alpha")) {
		t.Fatalf("the binary is at %q", binary)
	}

	zipDir := t.TempDir()
	zipData := zipOf(t, map[string]string{"bin/tool": "x"})
	if err := extractAgentArchive(zipData, "zip", zipDir, "./tool"); err != nil {
		t.Fatal(err)
	}
	if got := findAgentBinary(zipDir, "./tool"); got == "" || !strings.HasSuffix(got, filepath.Join("bin", "tool")) {
		t.Fatalf("the unpacked binary is at %q", got)
	}
	// A file that is not an archive is the agent itself.
	rawDir := t.TempDir()
	if err := extractAgentArchive([]byte("#!/bin/sh\n"), "", rawDir, "./muse"); err != nil {
		t.Fatal(err)
	}
	if got := findAgentBinary(rawDir, "./muse"); got == "" {
		t.Fatal("the bare executable was not written")
	}
}

func TestTheRegistryKeepsUnpackingInsideItsDirectory(t *testing.T) {
	dir := t.TempDir()
	data := tarGzOf(t, map[string]string{"../../escape": "no"})
	if err := extractAgentArchive(data, "tar.gz", dir, "./x"); err == nil {
		t.Fatal("an archive escaping its directory was unpacked")
	}
	if _, err := os.Stat(filepath.Join(filepath.Dir(dir), "escape")); err == nil {
		t.Fatal("a file escaped")
	}
}

func TestTheDownloadIsChecked(t *testing.T) {
	data := []byte("the agent")
	sum := sha256.Sum256(data)
	if !verifySHA(data, hex.EncodeToString(sum[:])) {
		t.Fatal("a right sum failed")
	}
	if verifySHA(data, strings.Repeat("0", 64)) {
		t.Fatal("a wrong sum passed")
	}
}

func TestTheRegistryFallsBackToItsCache(t *testing.T) {
	cache := filepath.Join(t.TempDir(), "registry.json")
	fetches := 0
	old := registryFetch
	defer func() { registryFetch = old }()
	registryFetch = func(ctx context.Context) ([]byte, error) {
		fetches++
		return []byte(testRegistry), nil
	}
	index, err := loadRegistryIndex(context.Background(), cache, false)
	if err != nil || len(index.Agents) != 3 {
		t.Fatalf("the fetch: %+v %v", index.Agents, err)
	}
	if _, err := os.Stat(cache); err != nil {
		t.Fatal("the index was not cached")
	}
	// A fresh cache is used without the network.
	registryFetch = func(ctx context.Context) ([]byte, error) {
		t.Fatal("the network was asked although the cache is fresh")
		return nil, nil
	}
	if _, err := loadRegistryIndex(context.Background(), cache, false); err != nil {
		t.Fatal(err)
	}
	// A failed fetch keeps the cached one.
	registryFetch = func(ctx context.Context) ([]byte, error) {
		return nil, errors.New("offline")
	}
	if _, err := loadRegistryIndex(context.Background(), cache, true); err != nil {
		t.Fatalf("the cache was not used: %v", err)
	}
	// Without a cache, the failure is the answer.
	if _, err := loadRegistryIndex(context.Background(), filepath.Join(t.TempDir(), "none.json"), true); err == nil {
		t.Fatal("a failure without a cache was swallowed")
	}
	if fetches != 1 {
		t.Fatalf("the fetches: %d", fetches)
	}
	// An old cache is not used as it stands; it is only kept if the
	// network fails.
	old2 := time.Now().Add(-2 * registryMaxAge)
	_ = os.Chtimes(cache, old2, old2)
	registryFetch = func(ctx context.Context) ([]byte, error) { return []byte(testRegistry), nil }
	if _, err := loadRegistryIndex(context.Background(), cache, false); err != nil {
		t.Fatalf("an old cache and a fetch: %v", err)
	}
}

func TestTheSettingsAddAnAgentOfTheRegistry(t *testing.T) {
	a := newApp()
	defer a.shutdown()
	a.root = t.TempDir()
	a.agentsPath = filepath.Join(t.TempDir(), "agents.json")
	index, _ := parseRegistry([]byte(testRegistry))
	a.registry = index.Agents
	a.registryHas["npx"] = true
	a.agentRegistryOpen = true
	a.settingsOpen = true
	a.settingsSection = settingsAgent
	tt := ui.NewTester(a.view, 1280, 840)
	tt.Frame()
	tt.Frame()
	if _, ok := tt.Find("Zeta"); !ok {
		t.Fatalf("the registry is not drawn: %v", tt.Texts())
	}
	if _, ok := tt.Find("二进制"); !ok {
		t.Fatalf("the binary agent is not shown as one: %v", tt.Texts())
	}
	if err := tt.Click("Zeta"); err != nil {
		t.Fatal(err)
	}
	if !a.agents.Has("Zeta") {
		t.Fatalf("Zeta was not added: %+v", a.agents.Items)
	}
	chosen := a.agents.Chosen("Zeta")
	if chosen.Command != "npx -y @zed/zeta@2.0.0 --acp" || chosen.Env["Z"] != "1" {
		t.Fatalf("the agent: %+v", chosen)
	}
	if a.agents.Default != "Zeta" {
		t.Fatalf("the default: %q", a.agents.Default)
	}
}

func TestTheRegistryIsSearched(t *testing.T) {
	a := newApp()
	defer a.shutdown()
	a.agentsPath = filepath.Join(t.TempDir(), "agents.json")
	index, _ := parseRegistry([]byte(testRegistry))
	a.registry = index.Agents
	a.agentRegistryOpen = true
	a.settingsOpen = true
	a.settingsSection = settingsAgent
	a.registryQuery = "binary"
	tt := ui.NewTester(a.view, 1280, 840)
	tt.Frame()
	if _, ok := tt.Find("Alpha"); !ok {
		t.Fatalf("the match is not drawn: %v", tt.Texts())
	}
	if _, ok := tt.Find("Zeta"); ok {
		t.Fatalf("an agent out of the query is drawn: %v", tt.Texts())
	}
	if len(a.registryMatches()) != 1 {
		t.Fatalf("the matches: %d", len(a.registryMatches()))
	}
}

// TestTheAgentStartsWithItsEnvironment checks that an agent's environment
// reaches the command that runs it.
func TestTheAgentStartsWithItsEnvironment(t *testing.T) {
	got := agentEnv([]string{"A=1"}, map[string]string{"B": "2", "C": "3"})
	if len(got) != 3 || got[0] != "A=1" || got[1] != "B=2" || got[2] != "C=3" {
		t.Fatalf("the environment: %v", got)
	}
}

// TestTheRegistryInstallsAnAgent checks the whole download: the archive,
// its sum, the unpacking, and the command that runs what came out.
func TestTheRegistryInstallsAnAgent(t *testing.T) {
	archive := tarGzOf(t, map[string]string{"fake-1.0/fake": "#!/bin/sh\necho acp\n"})
	sum := sha256.Sum256(archive)
	sumHex := hex.EncodeToString(sum[:])
	adds := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		adds++
		_, _ = w.Write(archive)
	}))
	defer server.Close()
	agent := acpRegistryAgent{ID: "fake", Name: "Fake", Version: "1.0.0"}
	target := acpBinaryTarget{Archive: server.URL + "/fake.tar.gz", Cmd: "./fake", Args: []string{"acp"}, SHA256: sumHex}
	root := t.TempDir()
	command, err := installAgentBinary(context.Background(), agent, target, root)
	if err != nil {
		t.Fatal(err)
	}
	binary := findAgentBinary(filepath.Join(root, "fake", "1.0.0"), "./fake")
	if binary == "" {
		t.Fatal("the unpacked binary was not found")
	}
	if got := agentCommand(binary, []string{"acp"}, runtime.GOOS); command != got {
		t.Fatalf("the command %q, not %q", command, got)
	}
	if info, err := os.Stat(binary); err != nil || info.Mode().Perm()&0o100 == 0 {
		t.Fatalf("the binary: %+v %v", info, err)
	}
	// What is installed is not downloaded again.
	server.Close()
	if _, err := installAgentBinary(context.Background(), agent, target, root); err != nil {
		t.Fatalf("an installed agent was downloaded again: %v", err)
	}
	if adds != 1 {
		t.Fatalf("the downloads: %d", adds)
	}
	// A wrong sum is not unpacked.
	bad := target
	bad.SHA256 = strings.Repeat("0", 64)
	if _, err := installAgentBinary(context.Background(), acpRegistryAgent{ID: "bad", Version: "1.0.0"}, bad, t.TempDir()); err == nil {
		t.Fatal("a wrong sum was accepted")
	}
}
