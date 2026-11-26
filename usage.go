package main

import (
	"fmt"
	"math"
	"strconv"
	"sync"

	"github.com/egoist/mygo/ui"
)

// The session's context window and cost, as ACP's usage_update says:
// a ring beside the composer shows how full the window is, and a warning
// says when it fills.
// https://agentclientprotocol.com/protocol/v1/prompt-turn#session-usage-updates

// agentUsagePercent is how much of the context window the session filled,
// as the ring shows it.
func (a *app) agentUsagePercent() int {
	if a.agentSize <= 0 {
		return 0
	}
	percent := int(math.Round(float64(a.agentUsed) / float64(a.agentSize) * 100))
	if percent < 0 {
		return 0
	}
	return percent
}

func (a *app) resetAgentUsage() {
	a.agentUsed, a.agentSize = 0, 0
	a.agentCost, a.agentCostCurrency = 0, ""
	a.agentUsageWarned = false
}

// usageColor is the color of the ring: quiet while the window has room,
// warm as it fills, and red when the next prompt may fail.
func usageColor(percent int, p palette) ui.Color {
	switch {
	case percent >= 90:
		return p.danger
	case percent >= 75:
		return p.warn
	}
	return p.accent
}

// tokenCount writes a number of tokens as short as it reads.
func tokenCount(tokens int64) string {
	switch {
	case tokens >= 1_000_000:
		return strconv.FormatFloat(float64(tokens)/1_000_000, 'f', 1, 64) + "M"
	case tokens >= 1_000:
		return strconv.FormatFloat(float64(tokens)/1_000, 'f', 1, 64) + "K"
	}
	return strconv.FormatInt(tokens, 10)
}

// usageCost writes what a session has spent.
func usageCost(amount float64, currency string) string {
	if currency == "" {
		currency = "USD"
	}
	return currency + " " + strconv.FormatFloat(amount, 'f', 3, 64)
}

// usageRings are the ring of each percentage, parsed once, as each frame
// draws the same few.
var usageRings sync.Map

// usageRing is the circle of the percentage: a track, and the part of it
// the session filled.
func usageRing(percent int) *ui.SVG {
	percent = min(max(percent, 0), 100)
	if cached, ok := usageRings.Load(percent); ok {
		return cached.(*ui.SVG)
	}
	const circumference = 2 * math.Pi * 9
	filled := circumference * float64(percent) / 100
	if filled < 0.01 {
		// A zero-length dash and its round cap still show a dot of the
		// ring; keep it under one.
		filled = 0.01
	}
	markup := fmt.Sprintf(
		`<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24" width="24" height="24">`+
			`<circle cx="12" cy="12" r="9" fill="none" stroke="currentColor" stroke-opacity="0.22" stroke-width="3"/>`+
			`<circle cx="12" cy="12" r="9" fill="none" stroke="currentColor" stroke-width="3" stroke-linecap="round" `+
			`stroke-dasharray="%.2f %.2f" transform="rotate(-90 12 12)"/>`+
			`</svg>`, filled, circumference-filled)
	svg, err := ui.ParseSVG([]byte(markup))
	if err != nil {
		return nil
	}
	usageRings.Store(percent, svg)
	return svg
}

// agentUsageView is the ring of the context window, at the right of a
// row above the composer: how full it is, and the tokens it holds.
func (a *app) agentUsageView(c *ui.Context, p palette) {
	if a.agentSize <= 0 {
		return
	}
	percent := a.agentUsagePercent()
	color := usageColor(percent, p)
	detail := fmt.Sprintf("上下文 %s / %s tokens（%d%%）", tokenCount(a.agentUsed), tokenCount(a.agentSize), percent)
	if a.agentCost > 0 {
		detail += "\n本次会话费用 " + usageCost(a.agentCost, a.agentCostCurrency)
	}
	ui.Row(c).FillWidth().AlignItems(ui.Center).Children(func() {
		ui.Box(c).Grow(1)
		b := ui.ButtonBase(c).Height(20).Padding(0, 6).Radius(10).Gap(4).
			AlignItems(ui.Center).Label(detail)
		if b.Hovered() {
			b.Background(p.edge)
		}
		b.Children(func() {
			if ring := usageRing(percent); ring != nil {
				ui.Icon(c, ring).Size(13, 13).TextColor(color)
			}
			ui.Textf(c, "%d%%", percent).FontSize(9.5).TextColor(color).FontFeatures("tnum")
			ui.Textf(c, "%s/%s", tokenCount(a.agentUsed), tokenCount(a.agentSize)).
				FontSize(9).TextColor(p.muted).FontFeatures("tnum")
		})
		b.Tooltip(detail)
	})
}
