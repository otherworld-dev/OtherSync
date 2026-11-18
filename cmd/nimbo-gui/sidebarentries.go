package main

import (
	"errors"
	"log/slog"
	"sync"

	"github.com/otherworld/nimbo/internal/account"
	"github.com/otherworld/nimbo/internal/config"
	"github.com/otherworld/nimbo/internal/shellns"
)

// Live mode gives every account its own Explorer sidebar entry. There used to
// be one, pointed at the shown account's folder, so adding an account moved it
// to the new account (before its folder was even chosen) and the first
// account vanished from the sidebar. The first account keeps the original
// node (shellns.NavGUID), so its entry stays where it was; the others get one
// derived from their ID (shellns.EntryGUID). An account whose setup is still
// open gets none until setup has chosen its folder.
//
// On-demand mode is different: each mounted root gets its node from Windows,
// and these entries are all removed (see syncSidebar).

// sidebarMu serialises working out and applying the entries, which start-up,
// setup and Settings can all ask for at once.
var sidebarMu sync.Mutex

// sidebarWant is one entry that should exist.
type sidebarWant struct {
	ID, GUID, Target, Name string
}

// sidebarGUIDFor is the node an account's entry lives under.
func sidebarGUIDFor(id string, st account.Store) string {
	if len(st.Accounts) > 0 && st.Accounts[0].ID == id {
		return shellns.NavGUID
	}
	return shellns.EntryGUID(id)
}

// planSidebarEntries compares the entries wanted with the ones recorded. It
// returns the entries to (re)write, the nodes to remove and the accounts whose
// record goes. A node that is about to be written is never removed, so a
// record that changes hands (the first account signed out, the next one takes
// the original node) is rewritten rather than deleted under it.
func planSidebarEntries(want []sidebarWant, have map[string]config.SidebarEntry) (write []sidebarWant, remove, drop []string) {
	writing := map[string]bool{}
	wanted := map[string]bool{}
	for _, w := range want {
		wanted[w.ID] = true
		writing[w.GUID] = true
		if rec, ok := have[w.ID]; ok && rec == (config.SidebarEntry{GUID: w.GUID, Target: w.Target, Name: w.Name}) {
			continue
		}
		write = append(write, w)
	}
	for id, rec := range have {
		gone := !wanted[id]
		if gone {
			drop = append(drop, id)
		}
		var now string
		for _, w := range want {
			if w.ID == id {
				now = w.GUID
			}
		}
		if rec.GUID != "" && rec.GUID != now && !writing[rec.GUID] {
			remove = append(remove, rec.GUID)
		}
	}
	return write, remove, drop
}

// wantedSidebarEntries lists an entry for every account with a folder of its
// own, other than one still in setup.
func (a *App) wantedSidebarEntries() []sidebarWant {
	d, st, ok := accountsAndDirs()
	if !ok {
		return nil
	}
	var out []sidebarWant
	for _, acc := range st.Accounts {
		if a.setupPending(acc.ID) {
			continue // its folder isn't chosen yet
		}
		target := baseDirFromDisk(d, acc.ID)
		if target == "" {
			continue
		}
		out = append(out, sidebarWant{ID: acc.ID, GUID: sidebarGUIDFor(acc.ID, st), Target: target, Name: sidebarLabel(acc, st.Accounts)})
	}
	return out
}

func loadSidebarEntries() map[string]config.SidebarEntry {
	if d, err := config.Resolve(); err == nil {
		if s, e := d.LoadSettings(); e == nil && s.SidebarEntries != nil {
			return s.SidebarEntries
		}
	}
	return map[string]config.SidebarEntry{}
}

func updateSidebarEntries(fn func(map[string]config.SidebarEntry)) {
	d, err := config.Resolve()
	if err != nil {
		return
	}
	_ = d.UpdateSettings(func(s *config.Settings) {
		if s.SidebarEntries == nil {
			s.SidebarEntries = map[string]config.SidebarEntry{}
		}
		fn(s.SidebarEntries)
		if len(s.SidebarEntries) == 0 {
			s.SidebarEntries = nil
		}
	})
}

// applySidebarEntries makes the live-mode entries match the accounts: each
// account with a folder has its own, and an account that has gone loses its.
// Only what differs from the record is written (each write is a scheduled-task
// run on a packaged build). Returns the first failure.
func (a *App) applySidebarEntries() error {
	sidebarMu.Lock()
	defer sidebarMu.Unlock()
	write, remove, drop := planSidebarEntries(a.wantedSidebarEntries(), loadSidebarEntries())
	var first error
	for _, g := range remove {
		if err := shellns.UnregisterEntry(g); err != nil {
			slog.Warn("could not remove a sidebar entry", "clsid", g, "err", err)
			first = errors.Join(first, err)
		}
	}
	if len(drop) > 0 {
		updateSidebarEntries(func(m map[string]config.SidebarEntry) {
			for _, id := range drop {
				delete(m, id)
			}
		})
	}
	if len(write) == 0 {
		return first
	}
	icon, err := navIconPath()
	if err != nil {
		slog.Warn("sidebar icon unavailable", "err", err)
		return err
	}
	for _, w := range write {
		if err := shellns.RegisterEntry(w.GUID, sidebarIconFolder(), w.Name, w.Target, icon); err != nil {
			slog.Warn("sidebar registration failed", "account", w.ID, "err", err)
			if first == nil {
				first = err
			}
			continue
		}
		slog.Info("sidebar entry written", "account", w.ID, "dir", w.Target, "name", w.Name)
		updateSidebarEntries(func(m map[string]config.SidebarEntry) {
			m[w.ID] = config.SidebarEntry{GUID: w.GUID, Target: w.Target, Name: w.Name}
		})
	}
	return first
}

// removeSidebarEntries removes every live-mode entry: the original node (also
// whatever an older build left there) and each account's recorded one.
func (a *App) removeSidebarEntries() error {
	sidebarMu.Lock()
	defer sidebarMu.Unlock()
	if err := shellns.Unregister(); err != nil {
		return err
	}
	var failed error
	for id, rec := range loadSidebarEntries() {
		if rec.GUID != "" && rec.GUID != shellns.NavGUID {
			if err := shellns.UnregisterEntry(rec.GUID); err != nil {
				slog.Warn("could not remove a sidebar entry", "account", id, "err", err)
				failed = err
				continue
			}
		}
		updateSidebarEntries(func(m map[string]config.SidebarEntry) { delete(m, id) })
	}
	return failed
}
