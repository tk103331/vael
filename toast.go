package main

import "github.com/egoist/mygo/ui"

var toastInfoIcon = lineIcon(`<circle cx="12" cy="12" r="9"/><line x1="12" y1="11" x2="12" y2="17"/><line x1="12" y1="7" x2="12" y2="7.1"/>`)

// Keep the native toast lifecycle, but draw with the current app palette
// rather than the toolkit's inverse colors. Existing notices follow theme
// changes as well as newly added ones.
func (a *app) toastView(c *ui.Context, p palette) {
	ui.ToastViewportBase(c, func(viewport ui.Element, toasts []ui.Toast) {
		viewport.Padding(0, 16, 32, 16).AlignItems(ui.Center).Gap(8)
		for _, notice := range toasts {
			toast := ui.ToastBase(c, notice)
			color, icon := p.accent, toastInfoIcon
			switch notice.Type {
			case "success":
				color, icon = p.success, checkIcon
			case "error":
				color, icon = p.danger, xCircleIcon
			case "warning":
				color, icon = p.warn, alertIcon
			}
			toast.Root.Row().AlignItems(ui.Start).MaxWidth(560).Padding(10, 14).Gap(9).Radius(8).
				Background(p.card).Border(1, p.edge).TextColor(p.text).
				Shadow(0, 4, 16, 0, ui.RGBA(0, 0, 0, 0.2)).Children(func() {
				ui.Icon(c, icon).Size(14, 14).Shrink(0).TextColor(color)
				ui.Column(c).Shrink(1).MinWidth(0).Gap(3).Children(func() {
					ui.Text(c, notice.Title).FontSize(12).FontWeight(650).TextColor(p.text)
					if notice.Description != "" {
						ui.Text(c, notice.Description).FontSize(11.5).TextColor(p.muted)
					}
				})
				if notice.Action != "" {
					button := toast.ActionButton().Padding(2, 6).Radius(4).TextColor(p.accent)
					if button.Hovered() {
						button.Background(p.edge)
					}
					button.Children(func() { ui.Text(c, notice.Action).FontSize(11.5) })
				}
			})
		}
	})
}
