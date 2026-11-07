// Labels and icons for activity kinds, shared by the flyout and the Status
// window so both say the same thing (Deck #987: the Status tab showed raw kinds
// like "delete-local", and a rename showed as "move").

const icons: Record<string, string> = {
  download: "↓", upload: "↑", "delete-local": "🗑", "delete-remote": "🗑",
  "move-local": "↪", "move-remote": "↪", move: "↪", rename: "✎",
  "mkdir-local": "📁", "mkdir-remote": "📁",
  conflict: "⚠", unshared: "🔗", "delete-kept": "↺", waiting: "⏸",
};

const labels: Record<string, string> = {
  download: "Downloaded", upload: "Uploaded", "delete-local": "Deleted locally",
  "delete-remote": "Deleted on server", "move-local": "Moved", "move-remote": "Moved",
  move: "Moved", rename: "Renamed",
  "mkdir-local": "New folder", "mkdir-remote": "New folder", conflict: "Conflict",
  unshared: "No longer shared with you — copy kept",
  "delete-kept": "Kept on server (not deleted)",
  waiting: "Waiting — open in another program",
  "unshared-empty": "No longer shared with you — nothing was downloaded",
  corrupt: "Damaged cloud file",
};

// A row with an error is something that didn't happen: a failed upload read
// "Uploaded" followed by its error (Deck #1129).
const failedLabels: Record<string, string> = {
  download: "Download failed", upload: "Upload failed",
  "delete-local": "Couldn't delete locally", "delete-remote": "Couldn't delete on server",
  "move-local": "Move failed", "move-remote": "Move failed", move: "Move failed",
  rename: "Rename failed", "mkdir-local": "Couldn't create folder",
  "mkdir-remote": "Couldn't create folder",
};

export const kindIcon = (k: string) => icons[k] ?? "•";
export const kindLabel = (k: string) => labels[k] ?? k;
// activityLabel is an activity row's label: kindLabel, or what failed when the
// row carries an error.
export const activityLabel = (k: string, err?: string) =>
  (err && failedLabels[k]) || kindLabel(k);
