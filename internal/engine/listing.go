package engine

import "strings"

// Presence is what a (possibly partial) remote listing knows about one path.
type Presence int

const (
	// Unknown: the listing never enumerated the folder the path would be in.
	// It must never be read as Absent — that reading is how an outage once
	// deleted 219 GB locally (the To Sort incident).
	Unknown Presence = iota
	// Present: the path is in the listing.
	Present
	// Absent: a folder on the path was enumerated and does not hold the next
	// component — positive knowledge from a successful listing.
	Absent
)

func (p Presence) String() string {
	switch p {
	case Present:
		return "present"
	case Absent:
		return "absent"
	}
	return "unknown"
}

// RemoteListing is a remote scan that may have enumerated only part of the
// tree (ScanOpts.Descend). Entries is keyed like RemoteScan's result.
type RemoteListing struct {
	Entries map[string]RemoteState
	// Listed holds every directory (root-relative, "" = the root) whose
	// children are all in Entries. Nil means the whole tree was crawled, so
	// every directory counts as listed.
	Listed map[string]bool
	// Opaque holds directories that exist on the server but whose contents
	// can never be listed (end-to-end encrypted): beneath them is Unknown.
	Opaque map[string]bool
}

// Lookup reports what the listing knows about rel (root-relative, "/"-separated).
//
// Absent is only ever concluded from a successful listing: walking down from
// the root, the first path component missing from a LISTED parent proves it,
// and everything beneath it, absent. A parent that was never listed, or an
// opaque one, makes the answer Unknown.
func (l RemoteListing) Lookup(rel string) Presence {
	rel = strings.Trim(rel, "/")
	if _, ok := l.Entries[rel]; ok && rel != "" {
		if l.Opaque[rel] {
			return Unknown
		}
		return Present
	}
	parent := ""
	for _, part := range strings.Split(rel, "/") {
		if l.Opaque[parent] {
			return Unknown
		}
		if l.Listed != nil && !l.Listed[parent] {
			return Unknown
		}
		cur := part
		if parent != "" {
			cur = parent + "/" + part
		}
		if _, ok := l.Entries[cur]; !ok {
			if l.Opaque[cur] {
				return Unknown
			}
			return Absent
		}
		parent = cur
	}
	// Every component is present, so rel itself would have matched above.
	return Present
}
