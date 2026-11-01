package agent

import (
	"io/fs"
	"path"
	"path/filepath"
	"strings"
)

// localFolderNeeds walks localDir (local disk only) and returns, lower-cased,
// every root-relative folder holding at least one non-ignored FILE somewhere
// beneath it: exactly the server folders the adopt scan must list to classify
// the local files (Deck #1003). Lower-cased because Windows names are
// case-insensitive; matching that way can only list more than needed, never
// less. Folders are never name-escaped, so local folder names are server names.
//
// It must cover at least every file vfs.Scan classifies, so it applies only the
// sync ignore rules (Scan also skips a few never-synced names, which costs at
// most a listing too many here). An unreadable subtree is skipped, as Scan
// skips it; an unreadable root fails, as Scan's does. A file that appears
// between this walk and Scan's lands under an unlisted folder and is left out
// of the plan as unknown, never classified as absent.
func localFolderNeeds(localDir string, skip func(string) bool) (map[string]bool, error) {
	need := map[string]bool{}
	err := filepath.WalkDir(localDir, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			if p == localDir {
				return err
			}
			if d != nil && d.IsDir() {
				return fs.SkipDir
			}
			return nil
		}
		if p == localDir {
			return nil
		}
		rel, rerr := filepath.Rel(localDir, p)
		if rerr != nil {
			return nil
		}
		rel = filepath.ToSlash(rel)
		if skip != nil && skip(rel) {
			if d.IsDir() {
				return fs.SkipDir
			}
			return nil
		}
		if d.IsDir() {
			return nil
		}
		for dir := path.Dir(rel); dir != "." && dir != ""; dir = path.Dir(dir) {
			key := strings.ToLower(dir)
			if need[key] {
				break // and so are all its ancestors
			}
			need[key] = true
		}
		return nil
	})
	return need, err
}
