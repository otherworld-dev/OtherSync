package main

import (
	"errors"

	"github.com/otherworld/nimbo/internal/agent"
	"github.com/otherworld/nimbo/internal/config"
)

// errAccountNotRunning is returned when a settings action targets an account
// that has no running engine.
var errAccountNotRunning = errors.New("this account isn't syncing right now — sign in again to change it")

// acctEngine resolves the engine for an account id. "" means the shown
// account. Unlike engineFor it never falls back to the shown engine for an
// unknown id, so a settings change can't land on the wrong account.
func (a *App) acctEngine(id string) (*agent.Engine, error) {
	if id == "" || (a.eng != nil && id == a.eng.Account.ID) {
		if a.eng == nil {
			return nil, errors.New("not signed in")
		}
		return a.eng, nil
	}
	if se := a.secondary(id); se != nil && se.eng != nil {
		return se.eng, nil
	}
	return nil, errAccountNotRunning
}

// isShown reports whether eng is the shown (primary) engine.
func (a *App) isShown(eng *agent.Engine) bool {
	return eng != nil && eng == a.eng
}

// pairsFromDisk reads an account's saved sync pairs without needing a running
// engine. It returns nil on error.
func pairsFromDisk(d config.Dirs, id string) []PairDTO {
	pairs, err := d.WithAccount(id).LoadPairs()
	if err != nil {
		return nil
	}
	out := make([]PairDTO, 0, len(pairs))
	for _, p := range pairs {
		out = append(out, PairDTO{LocalDir: p.LocalDir, RemoteRoot: p.RemoteRoot, Excludes: p.Excludes})
	}
	return out
}

// baseDirFromDisk returns the account's folder: the whole-account pair's local
// dir if there is one, else the remembered base dir.
func baseDirFromDisk(d config.Dirs, id string) string {
	ad := d.WithAccount(id)
	if pairs, err := ad.LoadPairs(); err == nil {
		for _, p := range pairs {
			if p.RemoteRoot == "" {
				return p.LocalDir
			}
		}
	}
	if st, err := ad.LoadAccountState(); err == nil {
		return st.BaseDir
	}
	return ""
}
