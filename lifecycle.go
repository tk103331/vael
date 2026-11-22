package main

import (
	"context"
	"time"

	"vael/internal/core"
)

func (a *app) update(fn func(), dropped ...func()) {
	drop := func() {
		for _, fn := range dropped {
			fn()
		}
	}
	if a.closed.Load() {
		drop()
		return
	}
	guarded := func() {
		if !a.closed.Load() {
			fn()
		} else {
			drop()
		}
	}
	if a.dispatch != nil {
		a.dispatch(guarded)
	} else {
		a.win.Update(guarded)
	}
}
func (a *app) beginConnection() (int, context.Context) {
	a.cancelConnection()
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	a.connectionCancel = cancel
	a.connecting = true
	return a.connectionVersion, ctx
}
func (a *app) cancelConnection() {
	a.connectionVersion++
	if a.connectionCancel != nil {
		a.connectionCancel()
		a.connectionCancel = nil
	}
	a.connecting = false
}
func (a *app) finishConnection() {
	if a.connectionCancel != nil {
		a.connectionCancel()
		a.connectionCancel = nil
	}
	a.connecting = false
}
func (a *app) resetProjectRequests() {
	if a.projectCancel != nil {
		a.projectCancel()
	}
	if a.indexCancel != nil {
		a.indexCancel()
	}
	if a.searchCancel != nil {
		a.searchCancel()
	}
	a.projectCtx, a.projectCancel = context.WithCancel(context.Background())
	a.loading = map[string]bool{}
	a.refreshing, a.gitBusy, a.indexing, a.searching = false, false, false, false
}
func (a *app) captureEdits() map[*fileTab]uint64 {
	edits := map[*fileTab]uint64{}
	for _, f := range a.tabs {
		if f.state != nil {
			edits[f] = f.state.Buffer.Version()
		}
	}
	return edits
}
func (a *app) newEditsSince(edits map[*fileTab]uint64) bool {
	for _, f := range a.tabs {
		if f.state != nil && f.state.Buffer.Modified() {
			version, ok := edits[f]
			if !ok || version != f.state.Buffer.Version() {
				return true
			}
		}
	}
	return false
}
func (a *app) watchBackend(backend *core.Client) {
	done := backend.Done()
	if done == nil {
		return
	}
	ctx := a.projectCtx
	go func() {
		select {
		case <-ctx.Done():
			return
		case <-done:
		}
		a.update(func() {
			if a.backend != backend || a.disconnected {
				return
			}
			a.disconnected = true
			a.connectionError = "核心连接已断开"
			if err := backend.Err(); err != nil {
				a.connectionError = err.Error()
			}
			a.generation++
			a.reconnectOff = false
			a.scheduleReconnect(time.Second, 1)
			if a.projectCancel != nil {
				a.projectCancel()
			}
			a.loading = map[string]bool{}
			a.refreshing, a.gitBusy, a.indexing, a.searching = false, false, false, false
			for _, f := range a.tabs {
				f.saving = false
			}
		})
	}()
}

// scheduleReconnect tries the connection again after a delay, and again
// after a longer one, until it works, the user stops it, or the project
// changes. Everything it decides is decided on the editor's thread.
func (a *app) scheduleReconnect(delay time.Duration, attempt int) {
	if a.connection == nil || a.reconnectOff || a.closed.Load() {
		return
	}
	generation, config := a.generation, *a.connection
	a.reconnectTimer = time.AfterFunc(delay, func() {
		a.update(func() {
			if a.closed.Load() || generation != a.generation {
				return
			}
			if !a.disconnected || a.connecting {
				// Someone else restored the connection, or is trying to.
				return
			}
			connect := a.connectForReconnect
			if connect == nil {
				connect = core.Connect
			}
			go func() {
				backend, err := connect(context.Background(), config)
				a.update(func() {
					if generation != a.generation {
						if backend != nil {
							backend.Close()
						}
						return
					}
					if err != nil {
						if backend != nil {
							backend.Close()
						}
						a.connectionError = err.Error()
						a.reconnectAttempt = attempt
						next := delay * 2
						if next > 30*time.Second {
							next = 30 * time.Second
						}
						a.scheduleReconnect(next, attempt+1)
						return
					}
					a.reconnectAttempt = 0
					a.reconnectTimer = nil
					a.installReconnected(backend)
				}, func() {
					if backend != nil {
						backend.Close()
					}
				})
			}()
		})
	})
}

// cancelReconnectTimer takes a pending attempt back.
func (a *app) cancelReconnectTimer() {
	if a.reconnectTimer != nil {
		a.reconnectTimer.Stop()
		a.reconnectTimer = nil
	}
	a.reconnectAttempt = 0
}

// stopReconnect stops the automatic attempts, until the connection breaks
// again.
func (a *app) stopReconnect() {
	a.cancelReconnectTimer()
	a.reconnectOff = true
}

// Reconnecting swaps only the transport. Editor buffers, their disk baselines,
// cursor positions and terminal sessions remain owned by the UI.
func (a *app) installReconnected(backend *core.Client) {
	previous := a.backend
	a.generation++
	a.resetProjectRequests()
	a.backend = backend
	a.disconnected, a.connectionError = false, ""
	a.cancelReconnectTimer()
	for _, f := range a.tabs {
		f.saving = false
	}
	if previous != nil {
		_ = previous.Close()
	}
	a.loadDir(a.root)
	a.refreshGit()
	a.refreshIndex()
	a.watchBackend(backend)
	a.updateWatchTargets()
	go a.watchFiles(a.projectCtx, backend)
}
func (a *app) reconnect() {
	if a.connection == nil || a.connecting {
		return
	}
	config := *a.connection
	version, ctx := a.beginConnection()
	go func() {
		backend, err := core.Connect(ctx, config)
		if err == nil {
			_, err = backend.Stat(ctx, config.Root)
		}
		if a.closed.Load() {
			if backend != nil {
				backend.Close()
			}
			return
		}
		a.update(func() {
			if version != a.connectionVersion {
				if backend != nil {
					backend.Close()
				}
				return
			}
			a.finishConnection()
			if err != nil {
				if backend != nil {
					backend.Close()
				}
				a.connectionError = err.Error()
				a.report(err)
				return
			}
			a.installReconnected(backend)
		}, func() {
			if backend != nil {
				backend.Close()
			}
		})
	}()
}
func (a *app) shutdown() {
	a.closed.Store(true)
	a.cancelConnection()
	a.cancelReconnectTimer()
	if a.projectCancel != nil {
		a.projectCancel()
	}
	if a.backend != nil {
		_ = a.backend.Close()
	}
	for _, t := range a.terms {
		t.term.Close()
	}
	a.stopAgent()
}
