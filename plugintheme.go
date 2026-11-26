package main

import (
	"fmt"
	"regexp"
	"strings"

	"github.com/egoist/mygo/ui"
	"vael/internal/workspace"
)

// Themes and icon themes a plugin brings: they are data in the manifest,
// like the themes of an editor extension. No program has to run for them.

var pluginHexColor = regexp.MustCompile(`^#[0-9a-fA-F]{6}$`)

// paletteFromColors reads the colours a theme gives, in the keys of
// paletteOf: wash, card, text, muted, edge, accent, selected, warn,
// success and danger. What is left out keeps Vael's dark colours.
func paletteFromColors(colors map[string]string) (palette, error) {
	base := paletteOf("#141418", "#232328", "#ececf0", "#a9a9b4", "#34343d",
		"#6b93ff", "#30394f", "#f0a33a", "#4fd17a", "#ff6961")
	set := map[string]*ui.Color{
		"wash": &base.wash, "card": &base.card, "text": &base.text,
		"muted": &base.muted, "edge": &base.edge, "accent": &base.accent,
		"selected": &base.selected, "warn": &base.warn, "success": &base.success,
		"danger": &base.danger,
	}
	for key, value := range colors {
		target, ok := set[strings.ToLower(key)]
		if !ok {
			continue
		}
		if !pluginHexColor.MatchString(value) {
			return palette{}, fmt.Errorf("主题的颜色 %s 不是 #rrggbb", key)
		}
		*target = ui.Hex(value)
	}
	base.warnSoft = base.warn.Alpha(0.14)
	return base, nil
}

// registerPluginTheme collects the themes and icon themes a plugin gives.
func (a *app) registerPluginTheme(item *plugin) {
	for _, spec := range item.manifest.Contributes.Themes {
		colors, err := paletteFromColors(spec.Colors)
		if err != nil {
			item.err = err.Error()
			continue
		}
		family := spec.Family
		if family == "" {
			family = spec.ID
		}
		a.pluginThemes = append(a.pluginThemes, uiTheme{
			ID: spec.ID, Name: spec.Name, Family: family, Dark: spec.Dark,
			Palette: colors, Icons: "badges",
		})
	}
	a.pluginIconThemes = append(a.pluginIconThemes, item.manifest.Contributes.IconThemes...)
}

// pluginTheme is a theme of the editor that came from a plugin.
func (a *app) pluginTheme(id string) *uiTheme {
	for i := range a.pluginThemes {
		if a.pluginThemes[i].ID == id {
			return &a.pluginThemes[i]
		}
	}
	return nil
}

// pluginIconTheme is the icon theme the editor is showing, if it came
// from a plugin.
func (a *app) pluginIconTheme() *workspace.PluginIconTheme {
	for i := range a.pluginIconThemes {
		if a.pluginIconThemes[i].ID == a.iconID {
			return &a.pluginIconThemes[i]
		}
	}
	return nil
}

// pluginIconChoices are the icon themes the settings offers: the editor's
// own, and those the plugins brought.
func pluginIconChoices(a *app) []struct{ ID, Name string } {
	choices := append([]struct{ ID, Name string }(nil), iconThemes...)
	for _, theme := range a.pluginIconThemes {
		choices = append(choices, struct{ ID, Name string }{theme.ID, theme.Name})
	}
	return choices
}

// pluginMark is how a plugin's icon theme marks one kind of file.
func (a *app) pluginMark(name string) (mark, bool) {
	theme := a.pluginIconTheme()
	if theme == nil {
		return mark{}, false
	}
	ext := filepathExt(name)
	spec, ok := theme.Marks[ext]
	if !ok {
		spec, ok = theme.Marks[strings.TrimPrefix(ext, ".")]
	}
	if !ok {
		return mark{}, false
	}
	color := func(p palette) ui.Color { return pluginTone(spec.Tone, p) }
	if icon := pluginIcon(spec.Icon); icon != nil {
		return mark{icon: icon, color: color}, true
	}
	if spec.Text != "" {
		return mark{text: spec.Text, color: color}, true
	}
	return mark{}, false
}
