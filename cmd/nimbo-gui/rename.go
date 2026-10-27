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
// Accounts that exist at that point keep the old product name in their
// automatic Explorer sidebar label (account.LabelBrand), because the label of
// an on-demand root is re-registered on every mount and changing it renames
// the root; updating those labels safely is its own, VM-tested change.
//
// It must run before any account mounts or the sidebar entry is written. The
// account marking never depends on reading the settings: if settings.json
// can't be read, accounts that are there are still marked (they mount next,
// and an unmarked one would be re-registered under the new name), nothing is
// recorded as done and no notice is due, and the error is returned so a later
// launch finishes the job. If the accounts can't be read it records nothing.
func migrateRename(d config.Dirs, b brand.Brand) (bool, error) {
	if !b.Renamed() {
		return false, nil
	}
	s, serr := d.LoadSettings()
	if serr == nil && s.RenamedFrom == b.FormerName {
		return false, nil
	}
	hadAccounts, err := markExistingAccounts(d, b.FormerName)
	if err != nil {
		return false, err
	}
	if serr != nil {
		return false, serr
	}
	existing := hadAccounts || fileExists(d.SettingsFile())
	if err := d.UpdateSettings(func(s *config.Settings) { s.RenamedFrom = b.FormerName }); err != nil {
		return false, err
	}
	return existing, nil
}

// markExistingAccounts gives every account on file that has none the former
// product name for its automatic sidebar label, and reports whether there
// were any accounts. A missing accounts file is no accounts.
func markExistingAccounts(d config.Dirs, former string) (bool, error) {
	if !fileExists(d.AccountsFile()) {
		return false, nil
	}
	has := false
	err := account.Update(d.AccountsFile(), func(st *account.Store) error {
		for i := range st.Accounts {
			if st.Accounts[i].LabelBrand == "" {
				st.Accounts[i].LabelBrand = former
			}
		}
		has = len(st.Accounts) > 0
		return nil
	})
	return has, err
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
