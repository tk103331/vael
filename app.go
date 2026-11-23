package main

import (
	"context"
	"fmt"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/plugins/terminal"
	"github.com/egoist/mygo/ui"
	"github.com/egoist/mygo/ui/editor"
	"vael/internal/core"
	"vael/internal/workspace"
)

const editorFind = editor.Find

type fileTab struct {
	path, original string
	state          *editor.State
	diff           *editor.DiffState
	saving         bool
	// The indentation of the text, cached for the buffer version it was
	// read from.
	indentTabs    bool
	indentWidth   int
	indentVersion uint64
	indentDone    bool
	// autoSaveAt is the version of the text the automatic save is waiting
	// on: a newer one waits for a save of its own.
	autoSaveAt uint64
}
type shellTab struct {
	term *terminal.Terminal
	name string
}
type namePrompt struct {
	name      string
	directory bool
}
type confirmation struct {
	title, message string
	action         func()
}
type app struct {
	closed                           atomic.Bool
	dispatch                         func(func())
	projectCtx                       context.Context
	projectCancel                    context.CancelFunc
	connectionCancel                 context.CancelFunc
	connectionVersion                int
	disconnected                     bool
	connectionError                  string
	environments                     workspace.Environments
	environmentsPath                 string
	profileName                      string
	backend                          *core.Client
	connection                       *core.Connection
	connectionPrompt                 *core.Connection
	connecting                       bool
	connectionPort                   string
	reconnectTimer                   *time.Timer
	reconnectAttempt                 int
	reconnectOff                     bool
	connectForReconnect              func(context.Context, core.Connection) (*core.Client, error)
	fontSize                         float32
	pendingSession                   *workspace.Session
	watchMu                          sync.Mutex
	watchPaths                       []string
	watchBefore                      map[string]workspace.Stamp
	stamps                           map[string]workspace.Stamp
	stale                            map[string]string
	watchVersion                     int
	settingsOpen                     bool
	projectMenuOpen                  bool
	projectPrompt                    *projectPrompt
	clonePrompt                      *clonePrompt
	branchMenuOpen                   bool
	branchLocalOpen                  bool
	branchRemoteOpen                 bool
	branchQuery                      string
	remoteBranches                   []string
	upstream                         string
	branchAhead, branchBehind        int
	recentBranches                   []string
	branchPrompt                     *branchPrompt
	revisionPrompt                   *revisionPrompt
	recentFiles                      []string
	recentOpen                       bool
	recentQuery                      string
	recentSelected                   int
	recentShown                      string
	zen                              bool
	paletteOpen                      bool
	paletteQuery                     string
	paletteIndex                     int
	renameTarget                     string
	renameValue                      string
	deleteTarget                     string
	task                             task
	taskCommand                      string
	taskHistory                      []string
	taskCancel                       context.CancelFunc
	symbols                          []workspace.Symbol
	symbolsLoading                   bool
	symbolCancel                     context.CancelFunc
	symbolQuery                      string
	index                            workspace.FileIndex
	indexing                         bool
	indexCancel                      context.CancelFunc
	indexVersion                     int
	quickOpen                        bool
	quickQuery                       string
	quickShown                       string
	quickSelected                    int
	paletteSelected                  int
	paletteShown                     string
	commandUses                      map[string]int
	searchQuery                      string
	searchCase, searching            bool
	replaceWith                      string
	searchResults                    workspace.SearchResults
	searchCancel                     context.CancelFunc
	searchVersion                    int
	sideWidth, editorShare           float32
	win                              *mygo.Window
	root                             string
	entries                          map[string][]workspace.Entry
	expanded                         map[string]bool
	loading                          map[string]bool
	tabs                             []*fileTab
	selected                         int
	terms                            []*shellTab
	termSelected                     int
	sidebar, showBottom, dark, wrap  bool
	tabSize                          int
	indent, whitespace               int
	lineHeight                       float32
	guides, autoClose, numbers       bool
	rightPanel                       bool
	rightShare, logShare             float32
	rightSection                     string
	bottomTab                        string
	section, filter, message, commit string
	repo                             *workspace.Repository
	gitError                         string
	branches                         []string
	branchesLoading                  bool
	branchCancel                     context.CancelFunc
	branchOpen                       bool
	commits                          []string
	commitsLoading                   bool
	commitCancel                     context.CancelFunc
	logCommits                       []logCommit
	logShown                         []logCommit
	logShownKey                      string
	logCols                          int
	logSelected                      int
	logLoading, logLoaded            bool
	logCancel                        context.CancelFunc
	logDetail                        *logDetail
	logDetailLoading                 bool
	logDetailCancel                  context.CancelFunc
	logDetailHash                    string
	logQuery, logAuthor, logBranch   string
	logSince, logPath, logPathInput  string
	logBranchOpen, logAuthorOpen     bool
	logSinceOpen, logPathOpen        bool
	logBranchQuery                   string
	logLocalOpen, logRemoteOpen      bool
	logRemoteClosed                  map[string]bool
	logRegex, logCase                bool
	logGraphOn, logDetailOn          bool
	logFileFolded                    map[string]bool
	logBranchHidden                  bool
	agents                           workspace.Agents
	agentsPath                       string
	agentChosen                      string
	agentMenuOpen                    bool
	agentConn                        *acpConn
	agentCancel                      context.CancelFunc
	agentSession                     string
	agentStatus                      string
	agentBusy                        bool
	agentPending                     []acpPending
	agentAllowed                     map[string]string
	agentPlan                        []acpPlanEntry
	agentPlanOpen                    bool
	agentRun                         int
	agentLines                       []agentLine
	agentInput                       string
	pluginDemoClick                  string
	themeID                          string
	iconID                           string
	fontFamily                       string
	fontMenuOpen                     bool
	fontQuery                        string
	fontFamilies                     []ui.FontFamily
	autoSave                         int
	trimOnSave, finalNewline         bool
	formatOnSave                     bool
	lastActive                       string
	lastActiveTab                    *fileTab
	symbolPrompt                     *symbolRename
	lspSet                           lspSettings
	lspPath                          string
	lsps                             map[string]*lspSession
	pendingCaret                     map[string]editor.Pos
	lspOn                            map[string]bool
	lspMenuOpen                      bool
	lspCmd                           map[string]*string
	settingsSection                  string
	agentAddOpen                     bool
	agentAddName, agentAddCommand    string
	agentRegistryOpen                bool
	branchFrom, branchName           string
	logPattern                       *regexp.Regexp
	logPatternKey                    string
	termMenuOpen                     bool
	envMenuOpen                      bool
	refreshing, gitBusy              bool
	generation                       int
	prompt                           *namePrompt
	confirm                          *confirmation
}

func newApp() *app {
	ctx, cancel := context.WithCancel(context.Background())
	return &app{projectCtx: ctx, projectCancel: cancel, backend: core.NewLocal(), lsps: map[string]*lspSession{}, pendingCaret: map[string]editor.Pos{}, sideWidth: 248, rightShare: 72, logShare: 74, rightSection: "outline", bottomTab: "terminal", editorShare: 62, stamps: map[string]workspace.Stamp{}, stale: map[string]string{}, commandUses: map[string]int{}, entries: map[string][]workspace.Entry{}, expanded: map[string]bool{}, loading: map[string]bool{}, themeID: "vael-dark", iconID: "badges", fontFamily: "monospace", sidebar: true, section: "files", guides: true, autoClose: true, numbers: true, selected: -1, termSelected: -1, logLocalOpen: true, logRemoteOpen: true, logRemoteClosed: map[string]bool{}, logGraphOn: true, logDetailOn: true}
}
func (a *app) active() *fileTab {
	if a.selected < 0 || a.selected >= len(a.tabs) {
		return nil
	}
	return a.tabs[a.selected]
}
func (a *app) dirty() bool {
	for _, f := range a.tabs {
		if f.state != nil && f.state.Buffer.Modified() {
			return true
		}
	}
	return false
}
func (a *app) report(err error) {
	if err != nil {
		a.message = err.Error()
	}
}
func (a *app) requireProject() bool {
	if a.root == "" {
		a.message = "请先打开项目或连接工作环境。"
		return false
	}
	if a.disconnected {
		a.message = "核心连接已断开，请重新连接后重试。当前编辑内容仍保留在窗口中。"
		return false
	}
	return true
}
func (a *app) chooseProject() {
	go func() {
		paths, err := mygo.Dialog.Open(mygo.OpenDialogOptions{Parent: a.win, Title: "打开项目", Directory: true})
		a.update(func() {
			a.report(err)
			if len(paths) == 0 {
				return
			}
			a.openProject(paths[0])
		})
	}()
}
func (a *app) openProject(root string, discard ...bool) {
	if a.dirty() && len(discard) == 0 {
		a.confirm = &confirmation{"切换项目？", "切换项目将关闭所有文件并丢弃未保存的修改。", func() { a.openProject(root, true) }}
		return
	}
	version, ctx := a.beginConnection()
	edits := a.captureEdits()
	backend := core.NewLocal()
	go func() {
		info, err := backend.Stat(ctx, root)
		a.update(func() {
			if version != a.connectionVersion {
				_ = backend.Close()
				return
			}
			a.finishConnection()
			a.report(err)
			if err != nil {
				return
			}
			if !info.Directory {
				a.report(fmt.Errorf("请选择文件夹"))
				return
			}
			if a.newEditsSince(edits) {
				_ = backend.Close()
				a.confirm = &confirmation{"切换项目？", "打开过程中有新的编辑，继续切换将丢弃这些修改。", func() { a.openProject(root, true) }}
				return
			}
			a.useProject(backend, nil, info.Path)
		})
	}()
}
func (a *app) useProject(backend *core.Client, connection *core.Connection, root string) {
	a.resetProjectRequests()
	if a.backend != nil {
		_ = a.backend.Close()
	}
	a.backend, a.connection = backend, connection
	a.disconnected, a.connectionError = false, ""
	if a.indexCancel != nil {
		a.indexCancel()
	}
	if a.searchCancel != nil {
		a.searchCancel()
	}
	a.index = workspace.FileIndex{}
	a.quickOpen, a.indexing, a.searching = false, false, false
	a.quickQuery, a.searchQuery = "", ""
	a.searchResults = workspace.SearchResults{}
	for _, t := range a.terms {
		t.term.Close()
	}
	a.generation++
	a.root, a.tabs, a.terms = root, nil, nil
	a.selected, a.termSelected = -1, -1
	a.entries, a.expanded, a.loading = map[string][]workspace.Entry{}, map[string]bool{root: true}, map[string]bool{}
	a.forgetFileStamps()
	a.repo, a.refreshing, a.gitBusy = nil, false, false
	a.cancelReconnectTimer()
	a.branches, a.commits, a.branchOpen = nil, nil, false
	a.branchesLoading, a.commitsLoading = false, false
	if a.logCancel != nil {
		a.logCancel()
	}
	if a.logDetailCancel != nil {
		a.logDetailCancel()
	}
	a.logCommits, a.logSelected, a.logDetail = nil, -1, nil
	a.logLoading, a.logLoaded, a.logDetailLoading = false, false, false
	a.logQuery, a.logAuthor, a.logBranch = "", "", ""
	a.logSince, a.logPath, a.logPathInput = "", "", ""
	a.logBranchOpen, a.logAuthorOpen, a.logSinceOpen, a.logPathOpen = false, false, false, false
	a.logBranchQuery, a.logLocalOpen, a.logRemoteOpen = "", true, true
	a.logRemoteClosed = map[string]bool{}
	a.filter, a.message, a.commit = "", "", ""
	if a.win != nil {
		a.win.SetTitle(filepath.Base(root) + " — Vael")
	}
	a.loadDir(root)
	a.refreshGit()
	a.refreshIndex()
	a.rememberEnvironment()
	a.watchBackend(backend)
	go a.watchFiles(a.projectCtx, backend)
	if a.pendingSession != nil {
		session := *a.pendingSession
		a.pendingSession = nil
		a.restoreFiles(session)
	}
}
func (a *app) loadDir(path string) {
	if a.root == "" || a.disconnected {
		return
	}
	if a.loading[path] {
		return
	}
	a.loading[path] = true
	generation := a.generation
	backend := a.backend
	ctx := a.projectCtx
	go func() {
		entries, err := backend.List(ctx, path)
		a.update(func() {
			if generation != a.generation {
				return
			}
			delete(a.loading, path)
			a.report(err)
			if err == nil {
				a.entries[path] = entries
			}
		})
	}()
}

// noteRecent puts path first in the files opened lately, keeping the list
// short, as an IDE's recent files.
func (a *app) noteRecent(path string) {
	kept := make([]string, 0, len(a.recentFiles)+1)
	for _, p := range a.recentFiles {
		if p != path {
			kept = append(kept, p)
		}
	}
	a.recentFiles = append([]string{path}, kept...)
	if len(a.recentFiles) > 20 {
		a.recentFiles = a.recentFiles[:20]
	}
}

func (a *app) openFile(path string) {
	a.openFileAt(path, -1, -1)
}
func (a *app) openFileAt(path string, start, end int) {
	for i, f := range a.tabs {
		if f.path == path && f.diff == nil {
			a.selected = i
			a.noteRecent(path)
			if start >= 0 {
				f.state.Select(start, end)
			}
			a.takeCaret(path, f)
			return
		}
	}
	if !a.requireProject() {
		return
	}
	generation := a.generation
	backend := a.backend
	ctx := a.projectCtx
	go func() {
		text, stamp, err := backend.ReadFile(ctx, path)
		a.update(func() {
			if generation != a.generation {
				return
			}
			a.report(err)
			if err != nil {
				return
			}
			for i, f := range a.tabs {
				if f.path == path && f.diff == nil {
					a.selected = i
					a.noteRecent(path)
					a.stamps[path] = stamp
					a.updateWatchTargets()
					if start >= 0 {
						f.state.Select(start, end)
					}
					return
				}
			}
			b := editor.NewBuffer(text)
			b.SetLanguage(editor.LanguageFor(path))
			font := a.editorFont()
			tab := &fileTab{path: path, original: text, state: &editor.State{Buffer: b, KeepScrollbars: true, Font: font}}
			tab.state.OnDefinition = func(offset int) { a.defineAt(tab, offset) }
			tab.state.ContextMenu = func(m *ui.Menu) {
				if m.Item("复制路径").Chosen() {
					mygo.Clipboard.WriteText(tab.path)
				}
				if m.Item("在文件树中显示").Chosen() {
					a.revealInTree(tab.path)
					a.sidebar = true
					a.section = "files"
				}
				m.Separator()
				if m.Item("重命名…").Chosen() {
					a.renameTarget, a.renameValue = tab.path, filepath.Base(tab.path)
				}
			}
			a.tabs = append(a.tabs, tab)
			a.selected = len(a.tabs) - 1
			a.noteRecent(path)
			a.stamps[path] = stamp
			a.watchVersion++
			a.updateWatchTargets()
			a.refreshDiffBase(a.tabs[a.selected])
			a.lspDidOpen(a.tabs[a.selected])
			if start >= 0 {
				a.tabs[a.selected].state.Select(start, end)
			}
			a.takeCaret(path, a.tabs[a.selected])
		})
	}()
}
func (a *app) save() {
	if !a.requireProject() {
		return
	}
	a.saveTab(a.active())
}
func (a *app) closeTab(index int) {
	if index < 0 || index >= len(a.tabs) {
		return
	}
	f := a.tabs[index]
	close := func() {
		for i, tab := range a.tabs {
			if tab == f {
				a.tabs = append(a.tabs[:i], a.tabs[i+1:]...)
				if i < a.selected {
					a.selected--
				}
				if a.selected >= len(a.tabs) {
					a.selected = len(a.tabs) - 1
				}
				delete(a.stamps, f.path)
				a.lspDidClose(f)
				a.updateWatchTargets()
				return
			}
		}
	}
	if f.state != nil && f.state.Buffer.Modified() {
		a.confirm = &confirmation{"关闭文件？", "“" + filepath.Base(f.path) + "”的修改尚未保存。", close}
	} else {
		close()
	}
}
func (a *app) refreshGit() {
	if a.refreshing || a.root == "" || a.disconnected {
		return
	}
	a.refreshing = true
	root, generation := a.root, a.generation
	backend := a.backend
	ctx := a.projectCtx
	go func() {
		repo, err := backend.Status(ctx, root)
		a.update(func() {
			if generation != a.generation {
				return
			}
			a.refreshing = false
			a.repo = repo
			a.gitError = ""
			if err != nil {
				a.gitError = err.Error()
			}
			// A file that was opened before the repository was known has
			// no base yet: it is given one.
			if a.repo != nil {
				for _, tab := range a.tabs {
					if tab.state != nil && tab.state.DiffBase == nil {
						a.refreshDiffBase(tab)
					}
				}
			}
		})
	}()
}
func (a *app) gitAction(fn func() error) {
	if !a.requireProject() {
		return
	}
	if a.gitBusy {
		return
	}
	a.gitBusy = true
	generation := a.generation
	go func() {
		err := fn()
		a.update(func() {
			if generation != a.generation {
				return
			}
			a.gitBusy = false
			a.report(err)
			a.refreshGit()
			if a.bottomTab == "git" {
				a.logLoaded = false
				a.loadLog()
			}
		})
	}()
}
func (a *app) openDiff(change workspace.Change, staged bool) {
	if !a.requireProject() {
		return
	}
	if a.repo == nil {
		return
	}
	root, generation := a.repo.Root, a.generation
	backend := a.backend
	workingPath := a.joinPath(root, change.Path)
	ctx := a.projectCtx
	go func() {
		oldPath := change.Path
		if change.Original != "" {
			oldPath = change.Original
		}
		var old, next string
		var err error
		if staged {
			if change.Code[0] != 'A' {
				old, err = backend.Git(ctx, root, "show", "HEAD:"+oldPath)
			}
			if err == nil && change.Code[0] != 'D' {
				next, err = backend.Git(ctx, root, "show", ":"+change.Path)
			}
		} else {
			if change.Code != "??" {
				old, err = backend.Git(ctx, root, "show", ":"+change.Path)
			}
			if err == nil && change.Code[1] != 'D' {
				next, err = backend.Read(ctx, workingPath)
			}
		}
		a.update(func() {
			if generation != a.generation {
				return
			}
			a.report(err)
			if err != nil {
				return
			}
			before, after := editor.NewBuffer(old), editor.NewBuffer(next)
			before.SetLanguage(editor.LanguageFor(change.Path))
			after.SetLanguage(editor.LanguageFor(change.Path))
			label := "工作区"
			if staged {
				label = "已暂存"
			}
			a.tabs = append(a.tabs, &fileTab{path: change.Path + " · " + label, diff: &editor.DiffState{Old: before, New: after, Layout: editor.DiffSplit, KeepScrollbars: true}})
			a.selected = len(a.tabs) - 1
		})
	}()
}
func (a *app) newTerminal() {
	root, generation := a.root, a.generation
	if root == "" {
		return
	}
	a.showBottom = true
	opts := terminal.Options{Dir: root, ShellIntegration: true, Padding: terminal.Padding{X: 12, Y: 8}, Font: terminal.Font{Size: a.fontSize}, CopyOnSelect: terminal.CopyOnSelectClipboard}
	if a.connection != nil {
		command, err := a.connection.TerminalCommand()
		if err != nil {
			a.report(err)
			return
		}
		opts.Dir = ""
		opts.ShellIntegration = false
		opts.Command = append([]string{command.Program}, command.Args...)
	}
	go func() {
		t, err := terminal.New(opts)
		a.update(func() {
			if generation != a.generation {
				if t != nil {
					t.Close()
				}
				return
			}
			a.report(err)
			if err != nil {
				return
			}
			a.terms = append(a.terms, &shellTab{term: t, name: fmt.Sprintf("shell %d", len(a.terms)+1)})
			a.termSelected = len(a.terms) - 1
		}, func() {
			if t != nil {
				t.Close()
			}
		})
	}()
}
func (a *app) create() {
	if !a.requireProject() {
		return
	}
	p := a.prompt
	if p == nil {
		return
	}
	root, generation := a.root, a.generation
	name, directory := strings.TrimSpace(p.name), p.directory
	backend := a.backend
	ctx := a.projectCtx
	go func() {
		err := backend.Create(ctx, root, name, directory)
		a.update(func() {
			if generation != a.generation {
				return
			}
			a.report(err)
			if err == nil {
				a.prompt = nil
				a.loadDir(root)
				a.refreshIndex()
				if !directory {
					a.openFile(a.joinPath(root, name))
				}
			}
		})
	}()
}
