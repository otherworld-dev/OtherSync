package shellns

// NavGUID identifies the app's original navigation-pane node. With several
// accounts it stays with the first account, so that account's entry keeps its
// place; every other account gets its own (EntryGUID).
const NavGUID = "{B7A4C3E0-9D2F-4E18-A6C1-2F5E8B0D7A10}"

// NavNode is an Explorer navigation-pane (sidebar) folder entry in HKCU.
type NavNode struct {
	CLSID  string
	Target string // TargetFolderPath
	Icon   string // DefaultIcon
}
