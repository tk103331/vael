package main

import (
	"strings"
	"time"

	"github.com/egoist/mygo/ui"
	"github.com/egoist/mygo/ui/editor"
)

// Saving: what the settings do to a file as it is saved, and the files
// saved on their own, as Zed and Sublime Text do.

// How a file is saved on its own, from the settings.
const (
	autoSaveOff = iota
	autoSaveOnLeave
	autoSaveAfterDelay
)

// autoSaveChoices are what the settings offer for the automatic save.
var autoSaveChoices = []settingsOption{
	{"关闭", autoSaveOff},
	{"切换文件时", autoSaveOnLeave},
	{"停顿一秒后", autoSaveAfterDelay},
}

// saveText is the text a file is saved with: what the settings take off its
// lines, and add to its end.
func saveText(text string, trim, finalNewline bool) string {
	if trim {
		lines := strings.Split(text, "\n")
		for i, line := range lines {
			lines[i] = strings.TrimRight(line, " \t")
		}
		text = strings.Join(lines, "\n")
	}
	if finalNewline && text != "" && !strings.HasSuffix(text, "\n") {
		text += "\n"
	}
	return text
}

// prepareSave puts in the buffer what the settings take off a file as it is
// saved, so that what is saved is what is seen, in one change the editor
// can undo.
func (a *app) prepareSave(tab *fileTab) {
	if tab == nil || tab.state == nil {
		return
	}
	buffer := tab.state.Buffer
	if text := saveText(buffer.String(), a.trimOnSave, a.finalNewline); text != buffer.String() {
		buffer.Edit(editor.Edit{Start: 0, End: buffer.Len(), Text: text})
	}
}

// saveTab saves a file, as the settings ask it to be saved: the text is
// tidied, the language server may format it, and then it is written.
func (a *app) saveTab(tab *fileTab) {
	if tab == nil || tab.state == nil || tab.saving {
		return
	}
	// Saving is working on the file for good, so the tab stops being a
	// preview.
	tab.preview = false
	a.prepareSave(tab)
	if a.formatOnSave && a.formatDocument(tab, func() { a.writeFile(tab) }) {
		return
	}
	a.writeFile(tab)
}

// writeFile writes what the buffer of a file holds.
func (a *app) writeFile(tab *fileTab) {
	if tab == nil || tab.state == nil || tab.saving {
		return
	}
	tab.saving = true
	text, original := tab.state.Buffer.String(), tab.original
	generation := a.generation
	backend := a.backend
	ctx := a.projectCtx
	go func() {
		stamp, err := backend.Save(ctx, tab.path, original, text)
		a.update(func() {
			if generation != a.generation {
				return
			}
			tab.saving = false
			a.report(err)
			if err == nil {
				tab.original = text
				a.stamps[tab.path] = stamp
				delete(a.stale, tab.path)
				a.lspDidSave(tab, text)
				if text == tab.state.Buffer.String() {
					tab.state.Buffer.MarkSaved()
				}
				a.refreshGit()
			}
		})
	}()
}

// autoSaveOnChange saves a file a moment after its text stopped changing,
// as the settings ask.
func (a *app) autoSaveOnChange(tab *fileTab) {
	if a.autoSave != autoSaveAfterDelay || tab == nil || tab.state == nil {
		return
	}
	if tab.saving || !tab.state.Buffer.Modified() {
		return
	}
	version := tab.state.Buffer.Version()
	if tab.autoSaveAt == version {
		// This version is already on its way to be saved.
		return
	}
	tab.autoSaveAt = version
	time.AfterFunc(time.Second, func() {
		a.update(func() {
			if tab.state == nil || tab.state.Buffer.Version() != version {
				return
			}
			a.saveTab(tab)
		})
	})
}

// autoSaveOnLeave saves the file that was open when another one takes its
// place, as the settings ask.
func (a *app) autoSaveOnLeave() {
	active := a.active()
	path := ""
	if active != nil {
		path = active.path
	}
	if a.lastActive == path {
		return
	}
	previous := a.lastActiveTab
	a.lastActiveTab, a.lastActive = active, path
	if a.autoSave != autoSaveOnLeave || previous == nil || previous == active {
		return
	}
	if previous.state != nil && previous.state.Buffer.Modified() && !previous.saving {
		a.saveTab(previous)
	}
}

// savingSettings is what the settings say of saving files.
func (a *app) savingSettings(c *ui.Context, p palette) {
	a.settingsChoice(c, p, "自动保存", "文件自己保存的时机", autoSaveChoices, &a.autoSave)
	a.settingsSwitch(c, p, "保存时去掉行尾空格", "去掉每行末尾的空格与制表符", &a.trimOnSave)
	a.settingsSwitch(c, p, "保存时补上末尾换行", "文件末尾没有换行时补上一个", &a.finalNewline)
	a.settingsSwitch(c, p, "保存时格式化", "先让语言服务器格式化，再写盘", &a.formatOnSave)
}
