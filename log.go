package main

import (
	"context"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
)

// logCommit is a commit of the log: what it says, and the lanes of the
// graph it lies on.
type logCommit struct {
	Hash, Subject, Author, Date, Refs string
	Parents                           []string
	Head                              bool
	Row                               int
	Lane, Lanes                       int
	In, Out                           []string
	Joins                             []logJoin
	DiagIn, DiagOut                   logDiagonal
}

// logDiagonal is the diagonal of a commit's row: the line that comes in
// from the row above, or leaves to the row below. A lane of -1 is no
// diagonal at all.
type logDiagonal struct {
	Lane, Color int
	Own         bool
}

// logJoin is a lane the graph joins to a commit's row: one that waited for
// the commit, coming from above, or the lane of another parent, going on
// below.
type logJoin struct {
	Lane int
	Up   bool
}

// logFile is a file a commit changed.
type logFile struct{ Code, Path string }

// logDetail is what the log shows of the commit that is chosen: the files
// it changed, the branches it is on, and its message.
type logDetail struct {
	Files    []logFile
	Branches []string
	Body     string
}

// loadLog reads the commits of the repository, filtered by the branch, the
// date and the path of the toolbar.
func (a *app) loadLog() {
	if a.repo == nil || a.logLoading {
		return
	}
	if a.logCancel != nil {
		a.logCancel()
	}
	a.logLoading = true
	ctx, cancel := context.WithCancel(a.projectCtx)
	a.logCancel = cancel
	generation, root := a.generation, a.repo.Root
	backend := a.backend
	branch, since, path := a.logBranch, a.logSince, strings.TrimSpace(a.logPath)
	go func() {
		// The commits come in the order of the graph, so that a parent
		// never comes before its children.
		args := []string{"log", "--topo-order", "--date=format:%Y/%m/%d %H:%M",
			"--pretty=format:%H%x1f%P%x1f%an%x1f%ad%x1f%s%x1f%D", "-n", "400"}
		switch branch {
		case "":
		case "all":
			args = append(args, "--all")
		default:
			args = append(args, branch)
		}
		if since != "" {
			args = append(args, "--since="+since)
		}
		if path != "" {
			args = append(args, "--", path)
		}
		output, err := backend.Git(ctx, root, args...)
		a.update(func() {
			if generation != a.generation {
				return
			}
			a.logLoading = false
			a.logCancel = nil
			if ctx.Err() != nil {
				return
			}
			a.report(err)
			if err != nil {
				a.logLoaded = true
				return
			}
			a.logCommits = parseLog(output)
			a.logShownKey = ""
			a.logLoaded = true
			if a.logSelected >= len(a.logCommits) {
				a.logSelected = -1
			}
			if a.logSelected < 0 && len(a.logCommits) > 0 {
				a.logSelected = 0
			}
			if a.logSelected >= 0 {
				a.loadLogDetail(a.logCommits[a.logSelected].Hash)
			} else {
				a.logDetail = nil
			}
		})
	}()
}

// parseLog reads the commits of git log, whose fields are separated by the
// unit separator.
func parseLog(output string) []logCommit {
	var commits []logCommit
	for _, line := range strings.Split(strings.TrimRight(output, "\n"), "\n") {
		if line == "" {
			continue
		}
		fields := strings.Split(line, "\x1f")
		if len(fields) < 6 {
			continue
		}
		commit := logCommit{
			Hash:    fields[0],
			Author:  fields[2],
			Date:    fields[3],
			Subject: fields[4],
			Refs:    fields[5],
			Head:    strings.Contains(fields[5], "HEAD ->"),
		}
		if fields[1] != "" {
			commit.Parents = strings.Fields(fields[1])
		}
		commits = append(commits, commit)
	}
	return commits
}

// assignLanes lays the commits out in lanes, as a graph does: a commit
// takes the lane that waits for it, or a free one, and its parents take
// the lanes that wait for them, or new ones. A lane that waits for a
// commit another lane already waits for joins it.
func assignLanes(commits []logCommit) {
	var lanes []string
	for i := range commits {
		commit := &commits[i]
		lane := -1
		var dups []int
		for j, hash := range lanes {
			if hash != commit.Hash {
				continue
			}
			if lane < 0 {
				lane = j
				continue
			}
			dups = append(dups, j)
		}
		if lane < 0 {
			if lane = freeLane(lanes); lane < 0 {
				lane = len(lanes)
				lanes = append(lanes, "")
			}
			lanes[lane] = commit.Hash
		}
		commit.Lane = lane
		commit.DiagIn = logDiagonal{Lane: -1, Color: -1}
		commit.DiagOut = logDiagonal{Lane: -1, Color: -1}
		commit.In = append([]string(nil), lanes...)
		if i == 0 {
			// Nothing comes into the first row of the graph.
			commit.In = nil
		}
		for _, j := range dups {
			lanes[j] = ""
			commit.Joins = append(commit.Joins, logJoin{Lane: j, Up: true})
		}
		lanes[lane] = ""
		if len(commit.Parents) > 0 {
			lanes[lane] = commit.Parents[0]
		}
		for _, parent := range commit.Parents[min(1, len(commit.Parents)):] {
			j := laneOf(lanes, parent)
			if j < 0 {
				if j = freeLane(lanes); j < 0 {
					j = len(lanes)
					lanes = append(lanes, "")
				}
				lanes[j] = parent
			}
			commit.Joins = append(commit.Joins, logJoin{Lane: j})
		}
		for len(lanes) > 0 && lanes[len(lanes)-1] == "" {
			lanes = lanes[:len(lanes)-1]
		}
		commit.Out = append([]string(nil), lanes...)
		commit.Lanes = max(len(commit.In), len(commit.Out))
		for _, join := range commit.Joins {
			commit.Lanes = max(commit.Lanes, join.Lane+1)
		}
	}
}

// logShownCommits are the commits the filters keep, with the lanes of the
// graph they make.
func (a *app) logShownCommits() []logCommit {
	key := strings.Join([]string{a.logQuery, a.logAuthor, a.logBranch, a.logSince, a.logPath,
		strconv.FormatBool(a.logRegex), strconv.FormatBool(a.logCase)}, "\x00")
	if a.logShownKey == key && a.logShown != nil {
		return a.logShown
	}
	rows := make([]logCommit, 0, len(a.logCommits))
	for i, commit := range a.logCommits {
		if !a.logMatches(commit) {
			continue
		}
		commit.Row = i
		rows = append(rows, commit)
	}
	assignLanes(rows)
	logDiagonals(rows)
	a.logShown, a.logShownKey = rows, key
	a.logCols = logColumns(rows)
	return rows
}

// logDiagonals marks the rows whose line to the row above or below is a
// diagonal, as a graph draws a branch that leaves a commit, or joins it.
func logDiagonals(rows []logCommit) {
	index := map[string]int{}
	for i, commit := range rows {
		index[commit.Hash] = i
	}
	for i := range rows {
		commit := &rows[i]
		for _, join := range commit.Joins {
			if join.Up {
				// The lane came from above and joins the dot.
				if i > 0 && rows[i-1].Lane == join.Lane {
					rows[i-1].DiagOut = logDiagonal{Lane: commit.Lane, Color: join.Lane, Own: true}
					commit.DiagIn = logDiagonal{Lane: join.Lane, Color: join.Lane}
				}
				continue
			}
			// The dot goes into the lane, which the next row's commit
			// takes when it is there.
			if join.Lane >= len(commit.Out) {
				continue
			}
			if at, ok := index[commit.Out[join.Lane]]; ok && at == i+1 {
				rows[i+1].DiagIn = logDiagonal{Lane: commit.Lane, Color: join.Lane, Own: true}
				commit.DiagOut = logDiagonal{Lane: rows[i+1].Lane, Color: join.Lane}
			}
		}
	}
}

// logColumns is the widest row of the graph, so that every row leaves the
// same room for it.
func logColumns(commits []logCommit) int {
	cols := 1
	for _, commit := range commits {
		cols = max(cols, commit.Lanes)
	}
	return cols
}

// freeLane is the first lane that waits for nothing, or -1.
func freeLane(lanes []string) int {
	for i, hash := range lanes {
		if hash == "" {
			return i
		}
	}
	return -1
}

// laneOf is the lane that expects a commit, or -1.
func laneOf(lanes []string, hash string) int {
	for i, expected := range lanes {
		if expected == hash {
			return i
		}
	}
	return -1
}

// loadLogDetail reads the files, the message and the branches of a commit.
func (a *app) loadLogDetail(hash string) {
	if a.repo == nil || hash == "" {
		return
	}
	if a.logDetailCancel != nil {
		a.logDetailCancel()
	}
	ctx, cancel := context.WithCancel(a.projectCtx)
	a.logDetailCancel = cancel
	generation, root := a.generation, a.repo.Root
	backend := a.backend
	a.logDetail, a.logDetailLoading, a.logDetailHash = nil, true, hash
	go func() {
		files, filesErr := backend.Git(ctx, root, "diff-tree", "-r", "--no-commit-id", "--name-status", "--root", "-m", hash)
		body, _ := backend.Git(ctx, root, "show", "-s", "--pretty=format:%B", hash)
		branches, _ := backend.Git(ctx, root, "branch", "-a", "--contains", hash, "--format=%(refname:short)")
		a.update(func() {
			if generation != a.generation || a.logDetailHash != hash {
				return
			}
			a.logDetailLoading = false
			a.logDetailCancel = nil
			if ctx.Err() != nil {
				return
			}
			a.report(filesErr)
			detail := &logDetail{Body: strings.TrimSpace(body)}
			seen := map[string]bool{}
			for _, line := range strings.Split(files, "\n") {
				line = strings.TrimSpace(line)
				if line == "" {
					continue
				}
				parts := strings.SplitN(line, "\t", 2)
				if len(parts) != 2 || seen[parts[1]] {
					continue
				}
				seen[parts[1]] = true
				detail.Files = append(detail.Files, logFile{Code: parts[0], Path: parts[1]})
			}
			for _, line := range strings.Split(branches, "\n") {
				line = strings.TrimSpace(line)
				if line != "" {
					detail.Branches = append(detail.Branches, line)
				}
			}
			a.logDetail = detail
		})
	}()
}

// logMatches reports whether a commit passes the text and the author of the
// toolbar, which filter what is read without asking Git again.
func (a *app) logMatches(commit logCommit) bool {
	query := strings.TrimSpace(a.logQuery)
	if query != "" {
		match := func(text string) bool {
			if a.logRegex {
				if re := a.logPatternFor(query); re != nil {
					return re.MatchString(text)
				}
			}
			return logContains(text, query, a.logCase)
		}
		if !match(commit.Subject) && !match(commit.Hash) && !match(commit.Author) {
			return false
		}
	}
	author := strings.TrimSpace(a.logAuthor)
	if author != "" && !logContains(commit.Author, author, false) {
		return false
	}
	return true
}

// logContains reports whether text has needle, ignoring case unless the
// toolbar asks otherwise.
func logContains(text, needle string, sensitive bool) bool {
	if sensitive {
		return strings.Contains(text, needle)
	}
	return strings.Contains(strings.ToLower(text), strings.ToLower(needle))
}

// logPatternFor compiles the query as a regular expression, and keeps it
// while the query and the flags stay the same.
func (a *app) logPatternFor(query string) *regexp.Regexp {
	key := query
	if !a.logCase {
		key = "(?i)" + query
	}
	if a.logPattern != nil && a.logPatternKey == key {
		return a.logPattern
	}
	re, err := regexp.Compile(key)
	if err != nil {
		re = nil
	}
	a.logPattern, a.logPatternKey = re, key
	return re
}

// laneColor is the color of a lane of the graph.
func laneColor(lane int, p palette) ui.Color {
	colors := []ui.Color{ui.Hex("#d16bb8"), ui.Hex("#4ec9b0"), ui.Hex("#e0a458"), ui.Hex("#6a9fd8"), ui.Hex("#c678dd"), ui.Hex("#56b6c2")}
	return colors[lane%len(colors)]
}

// logGraph draws the lanes of a commit's row, as a graph does: the lanes
// that pass through it, the half lanes that start or end at it, the
// diagonals that leave a commit or join it, and the commit's dot.
func (a *app) logGraph(c *ui.Context, p palette, commit logCommit) {
	lanes := a.logCols
	if lanes < 1 {
		lanes = 1
	}
	const laneWidth = 13
	ui.Box(c).Width(float32(lanes)*laneWidth + 12).FillHeight().Shrink(0).
		Draw(func(painter *ui.Painter, r ui.Rect) {
			laneX := func(lane int) float32 { return r.X + float32(lane*laneWidth) + 6 }
			centerY := r.Y + r.H/2
			line := func(x, y0, y1 float32, color ui.Color) {
				var path ui.Path
				path.MoveTo(x, y0).LineTo(x, y1)
				painter.StrokePath(&path, 1.2, color)
			}
			// The lanes that join the commit's row: one that comes from
			// above and turns into the dot, one the dot turns into.
			joining := map[int]bool{}
			for _, join := range commit.Joins {
				joining[join.Lane] = true
			}
			for lane := 0; lane < lanes; lane++ {
				in := lane < len(commit.In) && commit.In[lane] != ""
				out := lane < len(commit.Out) && commit.Out[lane] != ""
				x := laneX(lane)
				switch {
				case lane == commit.Lane:
					// A diagonal that is the lane's own takes the place
					// of the half line it comes from.
					if in && !commit.DiagIn.Own {
						line(x, r.Y, centerY, laneColor(lane, p))
					}
					if out && !commit.DiagOut.Own {
						line(x, centerY, r.Y+r.H, laneColor(lane, p))
					}
				case joining[lane]:
					// The join draws the lane of the row.
				case in && out:
					line(x, r.Y, r.Y+r.H, laneColor(lane, p))
				case in:
					line(x, r.Y, centerY, laneColor(lane, p))
				case out:
					line(x, centerY, r.Y+r.H, laneColor(lane, p))
				}
			}
			dot := laneX(commit.Lane)
			for _, join := range commit.Joins {
				x := laneX(join.Lane)
				if join.Up {
					if commit.DiagIn.Lane < 0 {
						// The lane comes from above and slopes into the dot.
						slope := centerY - r.Y
						d := x - dot
						if d < 0 {
							d = -d
						}
						if d < slope {
							slope = d
						}
						var path ui.Path
						path.MoveTo(x, r.Y)
						path.LineTo(x, centerY-slope)
						path.LineTo(dot, centerY)
						painter.StrokePath(&path, 1.2, laneColor(join.Lane, p))
					}
					continue
				}
				if commit.DiagOut.Lane < 0 {
					// The dot slopes into the lane, which goes on below.
					var path ui.Path
					path.MoveTo(dot, centerY)
					path.LineTo(x, r.Y+r.H)
					painter.StrokePath(&path, 1.2, laneColor(join.Lane, p))
				}
			}
			if commit.DiagIn.Lane >= 0 {
				// The line of the row above comes in as a diagonal.
				middle := (laneX(commit.DiagIn.Lane) + dot) / 2
				var path ui.Path
				path.MoveTo(middle, r.Y).LineTo(dot, centerY)
				painter.StrokePath(&path, 1.2, laneColor(commit.DiagIn.Color, p))
			}
			if commit.DiagOut.Lane >= 0 {
				// The line of the row below leaves as a diagonal.
				middle := (dot + laneX(commit.DiagOut.Lane)) / 2
				var path ui.Path
				path.MoveTo(dot, centerY).LineTo(middle, r.Y+r.H)
				painter.StrokePath(&path, 1.2, laneColor(commit.DiagOut.Color, p))
			}
			x := dot
			if commit.Row == a.logSelected {
				// The commit that is chosen wears a ring.
				painter.Fill(ui.Rect{X: x - 5, Y: centerY - 5, W: 10, H: 10}, p.accent, 5)
			}
			painter.Fill(ui.Rect{X: x - 3, Y: centerY - 3, W: 6, H: 6}, laneColor(commit.Lane, p), 3)
		})
}

// logRef is a branch or a tag on a commit.
type logRef struct {
	Name string
	Tag  bool
}

// logRefs reads the decorations of a commit as its branches and tags: the
// branch it is on first, then the other local ones, and the remote ones
// last.
func logRefs(decorations string) []logRef {
	var head, local, remote []logRef
	for _, ref := range strings.Split(decorations, ", ") {
		ref = strings.TrimSpace(ref)
		if ref == "" {
			continue
		}
		onHead := strings.HasPrefix(ref, "HEAD -> ")
		tag := strings.HasPrefix(ref, "tag: ")
		ref = strings.TrimPrefix(ref, "HEAD -> ")
		ref = strings.TrimPrefix(ref, "tag: ")
		item := logRef{Name: ref, Tag: tag}
		switch {
		case onHead:
			head = append(head, item)
		case strings.Contains(ref, "/"):
			remote = append(remote, item)
		default:
			local = append(local, item)
		}
	}
	refs := append(head, local...)
	return append(refs, remote...)
}

// logShortDate is a commit's date as a row shows it, without the year that
// the details give.
func logShortDate(date string) string {
	if len(date) >= 5 && date[4] == '/' {
		return date[5:]
	}
	return date
}

// logChip draws the name of a branch or a tag on a commit, with its icon.
func logChip(c *ui.Context, p palette, ref logRef) {
	icon, color := branchIcon, p.accent
	if ref.Tag {
		icon, color = tagIcon, p.warn
	}
	chip := ui.Row(c).Height(18).Shrink(1).MinWidth(0).Padding(0, 5).Gap(3).Radius(4).
		Background(p.edge).AlignItems(ui.Center)
	chip.Tooltip(ref.Name)
	chip.Children(func() {
		ui.Icon(c, icon).Size(10, 10).Shrink(0).TextColor(color)
		ui.Text(c, ref.Name).FontSize(10.5).TextColor(color).Grow(1).MinWidth(0).MaxWidth(56).SingleLine().Ellipsis("…")
	})
}

// logCodeColor is the color of the status of a changed file.
func logCodeColor(code string, p palette) ui.Color {
	switch {
	case strings.HasPrefix(code, "A"), code == "?":
		return p.success
	case strings.HasPrefix(code, "D"), code == "U":
		return p.danger
	case strings.HasPrefix(code, "M"), code == "T":
		return p.warn
	case strings.HasPrefix(code, "R"), strings.HasPrefix(code, "C"):
		return p.accent
	}
	return p.muted
}

// logRow is a commit of the log: its lanes, its branches, its subject, its
// author and its date.
func (a *app) logRow(c *ui.Context, p palette, commit logCommit) {
	row := ui.ButtonBase(c).FillWidth().Height(30).Padding(0, 6).Gap(6).AlignItems(ui.Center).Label(commit.Subject)
	switch {
	case commit.Row == a.logSelected:
		row.Background(p.edge)
	case commit.Head:
		row.Background(p.selected)
	case row.Hovered():
		row.Background(p.edge)
	}
	row.Children(func() {
		if a.logGraphOn {
			a.logGraph(c, p, commit)
		}
		color := p.text
		if len(commit.Parents) > 1 {
			color = p.muted
		}
		// The subject takes the room the other columns leave, and never
		// asks for more than the row can give.
		ui.Box(c).Grow(1).MinWidth(110).Children(func() {
			ui.Text(c, commit.Subject).FontSize(12).TextColor(color).FillWidth().SingleLine().Ellipsis("…")
		})
		if refs := logRefs(commit.Refs); len(refs) > 0 {
			ui.Row(c).Width(104).Shrink(1).MinWidth(40).Gap(4).AlignItems(ui.Center).Children(func() {
				logChip(c, p, refs[0])
				if len(refs) > 1 {
					ui.Textf(c, "+%d", len(refs)-1).FontSize(10).TextColor(p.muted).SingleLine()
				}
			})
		}
		ui.Text(c, commit.Author).FontSize(11.5).FontWeight(600).TextColor(p.muted).
			Width(64).MinWidth(0).Shrink(1).SingleLine().Ellipsis("…")
		ui.Text(c, logShortDate(commit.Date)).FontSize(10.5).TextColor(p.muted).Width(84).MinWidth(0).
			Shrink(1).SingleLine().Ellipsis("…").TextAlign(ui.End)
	})
	row.Cursor(ui.CursorPointer)
	row.ContextMenu(func(m *ui.Menu) {
		if m.Item("查看提交 Diff").Chosen() {
			a.openCommitDiff(commit.Hash, commit.Subject)
		}
		m.Separator()
		if m.Item("复制哈希").Chosen() {
			mygo.Clipboard.WriteText(commit.Hash)
		}
		if m.Item("复制提交信息").Chosen() {
			mygo.Clipboard.WriteText(commit.Subject)
		}
		m.Separator()
		if m.Item("检出该提交").Chosen() {
			a.checkoutRevision(commit.Hash)
		}
		if m.Item("在此创建分支…").Chosen() {
			a.branchFrom, a.branchName = commit.Hash, ""
		}
	})
	if row.DoubleClicked() {
		a.openCommitDiff(commit.Hash, commit.Subject)
	} else if row.Clicked() {
		a.logSelected = commit.Row
		a.loadLogDetail(commit.Hash)
	}
}

// logSearchField is the text filter of the toolbar, with the flags that
// make it a regular expression or case-sensitive.
func (a *app) logSearchField(c *ui.Context, p palette) {
	ui.Row(c).Grow(1).MinWidth(140).MaxWidth(380).Height(28).Padding(0, 6).Gap(5).Radius(6).
		Background(p.wash).Border(1, p.edge).AlignItems(ui.Center).Children(func() {
		ui.Icon(c, searchIcon).Size(13, 13).TextColor(p.muted)
		ui.TextInputBase(c, &a.logQuery).Grow(1).MinWidth(60).Height(24).Placeholder("文本或哈希").Label("搜索提交")
		a.logFlag(c, p, ".*", "正则表达式", &a.logRegex)
		a.logFlag(c, p, "Cc", "区分大小写", &a.logCase)
	})
}

// logFlag is a small toggle of the search field.
func (a *app) logFlag(c *ui.Context, p palette, text, tip string, on *bool) {
	b := ui.ButtonBase(c).Height(18).Padding(0, 4).Radius(3).Center().Label(tip)
	color := p.muted
	if *on {
		b.Background(p.accent.Alpha(0.22))
		color = p.accent
	} else if b.Hovered() {
		b.Background(p.edge)
	}
	b.Children(func() {
		ui.Text(c, text).FontSize(10).FontWeight(600).TextColor(color)
	})
	b.Tooltip(tip)
	if b.Clicked() {
		*on = !*on
	}
}

// logFilterButton is a button of the log's toolbar that shows what a filter
// is set to, and opens its choices.
func (a *app) logFilterButton(c *ui.Context, p palette, label, value string, set bool, open *bool, fn func()) {
	text := label
	color := p.muted
	if set {
		text = label + "：" + value
		color = p.text
	}
	b := ui.ButtonBase(c).Height(26).Padding(0, 7).Radius(5).Gap(4).AlignItems(ui.Center).Label(text)
	if *open {
		b.Background(p.selected)
	} else if b.Hovered() {
		b.Background(p.edge)
	}
	b.Children(func() {
		ui.Text(c, text).FontSize(12).TextColor(color).SingleLine().Ellipsis("…")
		ui.Icon(c, chevronDownIcon).Size(9, 9).TextColor(p.muted)
	})
	if b.Clicked() {
		*open = !*open
	}
	ui.Popover(c, b, open, fn)
}

// logOption is a choice of a filter's popover.
func (a *app) logOption(c *ui.Context, p palette, text string, selected bool, fn func()) {
	b := ui.ButtonBase(c).FillWidth().Height(24).Padding(0, 6).Radius(4).AlignItems(ui.Center).Label(text)
	if selected {
		b.Background(p.selected)
	} else if b.Hovered() {
		b.Background(p.edge)
	}
	b.Children(func() {
		ui.Text(c, text).FontSize(11.5).TextColor(p.text).Grow(1).SingleLine().Ellipsis("…")
		if selected {
			ui.Icon(c, checkIcon).Size(11, 11).TextColor(p.accent)
		}
	})
	if b.Clicked() {
		fn()
	}
}

// logBranchFilter chooses the branch whose commits are read.
func (a *app) logBranchFilter(c *ui.Context, p palette) {
	label := "当前分支"
	switch a.logBranch {
	case "all":
		label = "所有分支"
	case "":
	default:
		label = a.logBranch
	}
	a.logFilterButton(c, p, "分支", label, a.logBranch != "", &a.logBranchOpen, func() {
		if len(a.branches) == 0 && !a.branchesLoading {
			a.loadBranches()
		}
		ui.Column(c).Width(240).Padding(4).Gap(1).Children(func() {
			choose := func(value string) {
				a.logBranch = value
				a.logBranchOpen = false
				a.logSelected = -1
				a.loadLog()
			}
			a.logOption(c, p, "当前分支", a.logBranch == "", func() { choose("") })
			a.logOption(c, p, "所有分支", a.logBranch == "all", func() { choose("all") })
			if len(a.branches) > 0 {
				ui.Divider(c)
				for _, name := range a.branches {
					name := name
					a.logOption(c, p, name, a.logBranch == name, func() { choose(name) })
				}
			}
		})
	})
}

// logAuthorFilter filters the commits by author, without asking Git again.
func (a *app) logAuthorFilter(c *ui.Context, p palette) {
	label := "任何人"
	if a.logAuthor != "" {
		label = a.logAuthor
	}
	a.logFilterButton(c, p, "作者", label, a.logAuthor != "", &a.logAuthorOpen, func() {
		ui.Column(c).Width(240).Padding(6).Gap(6).Children(func() {
			ui.TextInput(c, &a.logAuthor).FillWidth().Height(24).Padding(2, 6).Placeholder("作者名…").Label("作者").AutoFocus()
			ui.Row(c).FillWidth().Children(func() {
				ui.Spacer(c)
				if ui.Button(c, "清除").Clicked() {
					a.logAuthor = ""
					a.logAuthorOpen = false
				}
			})
		})
	})
}

// logSinceFilter chooses how far back the commits are read.
func (a *app) logSinceFilter(c *ui.Context, p palette) {
	label := "全部时间"
	switch a.logSince {
	case "7.days":
		label = "最近 7 天"
	case "30.days":
		label = "最近 30 天"
	case "1.year":
		label = "最近一年"
	}
	a.logFilterButton(c, p, "日期", label, a.logSince != "", &a.logSinceOpen, func() {
		ui.Column(c).Width(200).Padding(4).Gap(1).Children(func() {
			choose := func(value string) {
				a.logSince = value
				a.logSinceOpen = false
				a.logSelected = -1
				a.loadLog()
			}
			for _, option := range []struct{ label, value string }{
				{"全部时间", ""},
				{"最近 7 天", "7.days"},
				{"最近 30 天", "30.days"},
				{"最近一年", "1.year"},
			} {
				option := option
				a.logOption(c, p, option.label, a.logSince == option.value, func() { choose(option.value) })
			}
		})
	})
}

// logPathFilter reads only the commits that touch a path.
func (a *app) logPathFilter(c *ui.Context, p palette) {
	label := "全部路径"
	if a.logPath != "" {
		label = a.logPath
	}
	a.logFilterButton(c, p, "路径", label, strings.TrimSpace(a.logPath) != "", &a.logPathOpen, func() {
		ui.Column(c).Width(260).Padding(6).Gap(6).Children(func() {
			input := ui.TextInput(c, &a.logPathInput).FillWidth().Height(24).Padding(2, 6).Placeholder("如 internal/core").Label("路径").AutoFocus()
			apply := func() {
				a.logPath = strings.TrimSpace(a.logPathInput)
				a.logPathOpen = false
				a.logSelected = -1
				a.loadLog()
			}
			ui.Row(c).FillWidth().Gap(6).Children(func() {
				ui.Spacer(c)
				if ui.Button(c, "清除").Clicked() {
					a.logPath, a.logPathInput = "", ""
					a.logPathOpen = false
					a.logSelected = -1
					a.loadLog()
				}
				if ui.PrimaryButton(c, "应用").Clicked() || input.Submitted() {
					apply()
				}
			})
		})
	})
}

// logDetailView is the side of the log: the files of the chosen commit,
// and what it says.
func (a *app) logDetailView(c *ui.Context, p palette) {
	ui.Column(c).FillWidth().FillHeight().Children(func() {
		ui.Row(c).FillWidth().Height(26).Padding(0, 8).AlignItems(ui.Center).Children(func() {
			ui.Text(c, "更改的文件").FontSize(10.5).FontWeight(650).TextColor(p.muted).Grow(1)
			if a.logDetail != nil {
				ui.Textf(c, "%d", len(a.logDetail.Files)).FontSize(10).TextColor(p.muted).FontFeatures("tnum")
			}
		})
		if a.logSelected < 0 || a.logSelected >= len(a.logCommits) {
			ui.Text(c, "选择一个提交查看详情").FontSize(11).TextColor(p.muted).Padding(8)
			return
		}
		commit := a.logCommits[a.logSelected]
		if a.logDetailLoading {
			ui.Text(c, "正在读取…").FontSize(11).TextColor(p.muted).Padding(8)
		}
		ui.Scroll(c).FillWidth().Grow(1).Children(func() {
			ui.Column(c).FillWidth().Children(func() {
				if a.logDetail != nil && len(a.logDetail.Files) == 0 && !a.logDetailLoading {
					ui.Text(c, "没有文件变更").FontSize(11).TextColor(p.muted).Padding(8)
				}
				if a.logDetail != nil {
					for _, group := range logFileGroups(a.logDetail.Files) {
						if group.Dir != "." {
							a.logFileGroupRow(c, p, group)
						}
						if a.logFileFolded[group.Dir] {
							continue
						}
						for _, file := range group.Files {
							a.logFileRow(c, p, group, file)
						}
					}
				}
			})
		})
		ui.Divider(c)
		ui.Scroll(c).FillWidth().MaxHeight(170).Children(func() {
			ui.Column(c).FillWidth().Padding(8).Gap(5).Children(func() {
				ui.Text(c, commit.Subject).FontSize(11.5).FontWeight(600).TextColor(p.text)
				ui.Row(c).FillWidth().Gap(4).AlignItems(ui.Center).Children(func() {
					hash := commit.Hash
					if len(hash) > 8 {
						hash = hash[:8]
					}
					ui.Text(c, hash).FontSize(10.5).FontFeatures("tnum").TextColor(p.muted)
					if smallIconButton(c, copyIcon, "复制哈希", p) {
						mygo.Clipboard.WriteText(commit.Hash)
					}
					ui.Box(c).Grow(1)
				})
				ui.Text(c, commit.Author+" · "+commit.Date).FontSize(10).TextColor(p.muted)
				if a.logDetail != nil && len(a.logDetail.Branches) > 0 {
					ui.Row(c).FillWidth().Gap(4).AlignItems(ui.Center).Children(func() {
						ui.Text(c, "包含于").FontSize(10).TextColor(p.muted).Shrink(0)
						for _, name := range a.logDetail.Branches {
							name := name
							ui.Box(c).Height(16).MaxWidth(110).Shrink(1).Padding(0, 5).Radius(3).
								Background(p.edge).Center().Children(func() {
								ui.Text(c, name).FontSize(9.5).TextColor(p.accent).SingleLine().Ellipsis("…")
							})
						}
					})
				}
				if a.logDetail != nil && a.logDetail.Body != "" {
					ui.Text(c, a.logDetail.Body).FontSize(10.5).TextColor(p.muted)
				}
			})
		})
	})
}

// logFileGroupRow is the folder of a group of files, which folds.
func (a *app) logFileGroupRow(c *ui.Context, p palette, group logFileGroup) {
	folded := a.logFileFolded[group.Dir]
	b := ui.ButtonBase(c).FillWidth().Height(22).Padding(0, 6).Gap(4).Radius(4).AlignItems(ui.Center).Label(group.Dir)
	if b.Hovered() {
		b.Background(p.edge)
	}
	b.Children(func() {
		icon := chevronDownIcon
		if folded {
			icon = chevronRightIcon
		}
		ui.Icon(c, icon).Size(9, 9).TextColor(p.muted)
		ui.Icon(c, folderIcon).Size(11, 11).TextColor(p.muted)
		ui.Text(c, group.Dir).FontSize(10.5).TextColor(p.muted).Grow(1).SingleLine().Ellipsis("…")
		ui.Textf(c, "%d", len(group.Files)).FontSize(9.5).TextColor(p.muted).FontFeatures("tnum")
	})
	if b.Clicked() {
		if a.logFileFolded == nil {
			a.logFileFolded = map[string]bool{}
		}
		a.logFileFolded[group.Dir] = !folded
	}
}

// logFileRow is a file a commit changed, which opens when it is chosen.
func (a *app) logFileRow(c *ui.Context, p palette, group logFileGroup, file logFile) {
	name := file.Path
	left := float32(8)
	if group.Dir != "." {
		name, left = filepath.Base(file.Path), 20
	}
	b := ui.ButtonBase(c).FillWidth().Height(24).Padding(0, 8, 0, left).Gap(6).Radius(4).AlignItems(ui.Center).Label(file.Path)
	if b.Hovered() {
		b.Background(p.edge)
	}
	b.Children(func() {
		code := file.Code
		if code == "" {
			code = "?"
		}
		color := logCodeColor(code, p)
		ui.Box(c).Width(14).Height(14).Radius(3).Background(color.Alpha(0.18)).Center().Children(func() {
			ui.Text(c, code[:1]).FontSize(9).FontWeight(700).TextColor(color)
		})
		ui.Text(c, name).FontSize(11).TextColor(p.text).Grow(1).SingleLine().Ellipsis("…")
	})
	b.Cursor(ui.CursorPointer)
	if b.Clicked() {
		a.openFile(filepath.Join(a.root, file.Path))
	}
}

// showLog opens the Git log in the tool window at the bottom, with room
// for its graph.
func (a *app) showLog() {
	a.showBottom = true
	a.bottomTab = "git"
	if a.editorShare > 55 {
		a.editorShare = 52
	}
	if !a.logLoaded {
		a.loadLog()
	}
}

// logView is the Git log, as an IDE's: the branches on the left, the
// filters of the toolbar, the commits with their graph, and the files of
// the chosen commit.
func (a *app) logView(c *ui.Context, p palette) {
	if a.repo == nil {
		ui.Text(c, "当前文件夹没有可用的 Git 仓库。").FontSize(12).TextColor(p.muted).Padding(8)
		return
	}
	if !a.logLoaded && !a.logLoading {
		a.loadLog()
	}
	ui.Row(c).Fill().Children(func() {
		if !a.logBranchHidden {
			a.logBranchPanel(c, p)
			ui.Divider(c)
		}
		ui.Column(c).Grow(1).MinWidth(0).FillHeight().Children(func() {
			ui.Row(c).FillWidth().Height(40).Padding(0, 8).Gap(4).AlignItems(ui.Center).Children(func() {
				if iconButton(c, chevronLeftIcon, "隐藏分支面板", a.logBranchHidden, p) {
					a.logBranchHidden = !a.logBranchHidden
				}
				a.logSearchField(c, p)
				a.logBranchFilter(c, p)
				a.logAuthorFilter(c, p)
				a.logSinceFilter(c, p)
				a.logPathFilter(c, p)
				ui.Box(c).Grow(1)
				if iconButton(c, refreshIcon, "刷新日志", false, p) {
					a.loadLog()
				}
				if iconButton(c, graphIcon, "显示提交图", a.logGraphOn, p) {
					a.logGraphOn = !a.logGraphOn
				}
				if iconButton(c, eyeIcon, "显示提交详情", a.logDetailOn, p) {
					a.logDetailOn = !a.logDetailOn
				}
			})
			ui.Divider(c)
			list := func() {
				ui.Scroll(c).FillHeight().Grow(1).MinWidth(0).Children(func() {
					ui.Column(c).FillWidth().Children(func() {
						shown := 0
						for _, commit := range a.logShownCommits() {
							shown++
							a.logRow(c, p, commit)
						}
						if a.logLoading && shown == 0 {
							ui.Text(c, "正在读取提交…").FontSize(11).TextColor(p.muted).Padding(10)
						} else if shown == 0 {
							ui.Text(c, "没有匹配的提交").FontSize(11).TextColor(p.muted).Padding(10)
						}
					})
				})
			}
			if a.logDetailOn {
				// The detail panel takes its share of the room, on the
				// right, and the divider between them can be dragged.
				ui.SplitPercent(c, &a.logShare, list, func() { a.logDetailView(c, p) }).
					FillWidth().FillHeight().MinWidth(0)
			} else {
				list()
			}
		})
	})
}

// logFileGroup is the files a commit changed in one folder.
type logFileGroup struct {
	Dir   string
	Files []logFile
}

// logFileGroups groups the files of a commit by folder, the root first.
func logFileGroups(files []logFile) []logFileGroup {
	index := map[string]int{}
	var groups []logFileGroup
	for _, file := range files {
		dir := filepath.Dir(file.Path)
		at, ok := index[dir]
		if !ok {
			at = len(groups)
			index[dir] = at
			groups = append(groups, logFileGroup{Dir: dir})
		}
		groups[at].Files = append(groups[at].Files, file)
	}
	sort.Slice(groups, func(i, j int) bool { return groups[i].Dir < groups[j].Dir })
	return groups
}

// logBranchPanel is the left side of the log: the branches whose commits
// can be read, as an IDE's.
func (a *app) logBranchPanel(c *ui.Context, p palette) {
	if a.repo == nil {
		return
	}
	if len(a.branches) == 0 && len(a.remoteBranches) == 0 && !a.branchesLoading {
		a.loadBranches()
	}
	query := strings.ToLower(strings.TrimSpace(a.logBranchQuery))
	match := func(name string) bool {
		return query == "" || strings.Contains(strings.ToLower(name), query)
	}
	ui.Column(c).Width(120).Shrink(0).FillHeight().Children(func() {
		ui.Row(c).FillWidth().Height(32).Padding(0, 6).AlignItems(ui.Center).Children(func() {
			ui.TextInput(c, &a.logBranchQuery).FillWidth().Height(24).Padding(2, 6).Placeholder("搜索分支…").Label("搜索分支")
		})
		ui.Scroll(c).FillWidth().Grow(1).Children(func() {
			ui.Column(c).FillWidth().Padding(4, 4).Gap(1).Children(func() {
				ui.Text(c, "HEAD（当前分支）").FontSize(9.5).FontWeight(650).TextColor(p.muted).Padding(4, 4, 2, 6)
				if match(a.repo.Branch) {
					a.logBranchRow(c, p, a.repo.Branch, "", true)
				}
				local := 0
				for _, name := range a.branches {
					if match(name) {
						local++
					}
				}
				a.logBranchGroup(c, p, "本地", local, &a.logLocalOpen)
				if a.logLocalOpen {
					for _, name := range a.branches {
						if match(name) {
							a.logBranchRow(c, p, name, name, name == a.repo.Branch)
						}
					}
				}
				remotes := map[string][]string{}
				var names []string
				total := 0
				for _, name := range a.remoteBranches {
					if !match(name) {
						continue
					}
					remote := name
					if i := strings.Index(name, "/"); i > 0 {
						remote = name[:i]
					}
					if _, ok := remotes[remote]; !ok {
						names = append(names, remote)
					}
					remotes[remote] = append(remotes[remote], name)
					total++
				}
				a.logBranchGroup(c, p, "远程", total, &a.logRemoteOpen)
				if a.logRemoteOpen {
					for _, remote := range names {
						list := remotes[remote]
						open := !a.logRemoteClosed[remote]
						a.logBranchGroup(c, p, remote, len(list), &open)
						if open {
							delete(a.logRemoteClosed, remote)
							for _, name := range list {
								a.logBranchRow(c, p, strings.TrimPrefix(name, remote+"/"), name, false)
							}
						} else {
							a.logRemoteClosed[remote] = true
						}
					}
				}
			})
		})
	})
}

// logBranchGroup is a group of branches of the panel, which folds.
func (a *app) logBranchGroup(c *ui.Context, p palette, title string, count int, open *bool) {
	b := ui.ButtonBase(c).FillWidth().Height(22).Padding(0, 4).Gap(4).Radius(4).AlignItems(ui.Center).Label(title)
	if b.Hovered() {
		b.Background(p.edge)
	}
	b.Children(func() {
		icon := chevronDownIcon
		if !*open {
			icon = chevronRightIcon
		}
		ui.Icon(c, icon).Size(9, 9).TextColor(p.muted)
		ui.Text(c, title).FontSize(10.5).FontWeight(650).TextColor(p.muted).Grow(1)
		ui.Textf(c, "%d", count).FontSize(9.5).TextColor(p.muted).FontFeatures("tnum")
	})
	if b.Clicked() {
		*open = !*open
	}
}

// logBranchRow is a branch of the panel: choosing it reads its commits.
func (a *app) logBranchRow(c *ui.Context, p palette, label, value string, current bool) {
	selected := a.logBranch == value
	name := "日志分支：" + label
	if value == "" {
		name = "日志 HEAD：" + label
	}
	b := ui.ButtonBase(c).FillWidth().Height(24).Padding(0, 6).Gap(5).Radius(4).AlignItems(ui.Center).Label(name)
	if selected {
		b.Background(p.selected)
	} else if b.Hovered() {
		b.Background(p.edge)
	}
	b.Children(func() {
		if current {
			ui.Icon(c, starIcon).Size(11, 11).TextColor(p.warn)
		} else {
			ui.Box(c).Width(11)
		}
		ui.Text(c, label).FontSize(11.5).TextColor(p.text).Grow(1).SingleLine().Ellipsis("…")
	})
	if b.Clicked() {
		a.logBranch = value
		a.logSelected = -1
		a.loadLog()
	}
}
