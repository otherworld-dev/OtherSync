package main

import (
	"errors"
	"os"

	"github.com/otherworld/nimbo/internal/account"
	"github.com/otherworld/nimbo/internal/brand"
	"github.com/otherworld/nimbo/internal/config"
	"github.com/otherworld/nimbo/internal/notify"
)

// The rename from Nimbo to OtherSync is display-only (Deck #996): what people
// read changes, what Windows and an existing install recognise the app by does
// not. migrateRename is the one thing an existing install has to do about it,
// once, on the first launch of a renamed build.

// migrateRename records that this install has met the new name and reports
// whether it should be told (renameNotice). An install that ran under the old
// name (it has settings or accounts) is told; a fresh one is not, now or later.
//
// Nothing else needs migrating: the names Explorer shows (the sidebar entries,
// the sync roots' provider name, the Start-menu folder's display name) are
// written under the new name by the code that writes them anyway, against the
// same ids, on the next mount or start.
//
// It must run before anything writes settings.json, which is half of how an
// existing install is told apart from a fresh one. If the settings or the
// accounts can't be read it records nothing, so no notice is lost, and returns
// the error; a later launch finishes the job.
func migrateRename(d config.Dirs, b brand.Brand) (bool, error) {
	if !b.Renamed() {
		return false, nil
	}
	s, err := d.LoadSettings()
	if err != nil {
		return false, err
	}
	if s.RenamedFrom == b.FormerName {
		return false, nil
	}
	st, err := account.LoadStore(d.AccountsFile())
	if err != nil {
		return false, err
	}
	existing := len(st.Accounts) > 0 || fileExists(d.SettingsFile())
	if err := d.UpdateSettings(func(s *config.Settings) { s.RenamedFrom = b.FormerName }); err != nil {
		return false, err
	}
	return existing, nil
}

// renameNoticeDue is set at launch when migrateRename says this install is to
// be told about the new name.
var renameNoticeDue bool

// showRenameNotice tells an existing install about the new name, once, as a
// toast (unless the user has turned toasts off).
func (a *App) showRenameNotice() {
	if a.NotificationsEnabled() {
		notify.Toast(brand.Current.Name, renameNotice(brand.Current), "")
	}
}

// renameNotice is the one-time message an existing install is shown.
func renameNotice(b brand.Brand) string {
	return b.FormerName + " is now " + b.Name + ". Same app, nothing else has changed."
}

func fileExists(path string) bool {
	_, err := os.Stat(path)
	return !errors.Is(err, os.ErrNotExist)
}
