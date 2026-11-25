package main

import (
	"context"
	"time"

	"github.com/egoist/mygo"
)

// updater is what the update dialog shows: the check for a newer version,
// its download, and the version installed that waits for a restart. The
// updates are mygo's: signed archives and a manifest per platform, which
// the release workflow publishes with the app's installers.
type updater struct {
	open       bool // the dialog shows
	notice     bool // a version found at launch waits to be told of
	checking   bool
	installing bool
	installed  bool
	latest     bool
	disabled   bool
	version    string
	notes      string
	err        string
	progress   float64
	update     *mygo.Update
}

// showUpdates opens the update dialog, checking first unless a version is
// already installed and waits for a restart.
func (a *app) showUpdates() {
	if a.updater.installed {
		a.update(func() { a.updater.open = true })
		return
	}
	a.checkUpdates(true)
}

// checkUpdates asks the feed of the build for a newer version. A manual
// check opens the dialog; the one at launch only tells of a version found.
func (a *app) checkUpdates(manual bool) {
	u := &a.updater
	if u.checking || u.installing {
		return
	}
	if !mygo.Updater.Enabled() {
		if manual {
			a.update(func() { u.open, u.disabled, u.err = true, true, "" })
		}
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	a.update(func() {
		u.checking, u.err, u.latest, u.disabled = true, "", false, false
		if manual {
			u.open = true
		}
	})
	go func() {
		defer cancel()
		found, err := mygo.Updater.Check(ctx)
		a.update(func() {
			u.checking = false
			switch {
			case err != nil:
				if manual {
					u.err = err.Error()
				}
			case found == nil:
				u.update, u.version, u.notes = nil, "", ""
				if manual {
					u.latest = true
				}
			default:
				u.update, u.version, u.notes = found, found.Version, found.Notes
				if manual {
					u.open = true
				} else {
					u.notice = true
				}
			}
		})
	}()
}

// installUpdate downloads the update the dialog found and installs it,
// telling the progress from time to time.
func (a *app) installUpdate() {
	u := &a.updater
	up := u.update
	if up == nil || u.installing {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Minute)
	a.update(func() { u.installing, u.err, u.progress = true, "", 0 })
	go func() {
		defer cancel()
		last := -1
		err := up.Install(ctx, func(downloaded, total int64) {
			if total <= 0 {
				return
			}
			percent := int(float64(downloaded) * 100 / float64(total))
			if percent == last {
				return
			}
			last = percent
			a.update(func() { u.progress = float64(percent) / 100 })
		})
		a.update(func() {
			u.installing = false
			if err != nil {
				u.err, u.open = err.Error(), true
				return
			}
			u.installed, u.update, u.notice = true, nil, false
			u.open = true
		})
	}()
}
