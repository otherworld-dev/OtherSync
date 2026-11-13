// Account list logic for the Settings window: which entry is selected and
// which entries need the user's attention. Pure, so it is tested in node.

export type AccountEntry = {
  id: string;
  user: string;
  server: string;
  active: boolean;
  status: string;
  route: string;
  routeReason: string;
  running: boolean;
  syncing: boolean;
  localAddress: string;
  localPinned: boolean;
  sidebarName: string;
  sidebarAuto: string;
  sidebarLabel: string;
};

export type AppPage = "sync" | "exclusions" | "appearance" | "general";

export type Selection =
  | { kind: "account"; id: string }
  | { kind: "app"; page: AppPage }
  | { kind: "signedout" };

const appPages: readonly string[] = ["sync", "exclusions", "appearance", "general"];

export function needsAttention(a: AccountEntry): boolean {
  return !a.running || a.status === "Sign in again";
}

// The active account, else the first, else the signed-out view.
function fallback(accounts: AccountEntry[]): Selection {
  const pick = accounts.find((a) => a.active) ?? accounts[0];
  return pick ? { kind: "account", id: pick.id } : { kind: "signedout" };
}

export function initialSelection(accounts: AccountEntry[], tab: string): Selection {
  if (appPages.includes(tab)) return { kind: "app", page: tab as AppPage };
  return fallback(accounts);
}

export function nextSelection(accounts: AccountEntry[], cur: Selection): Selection {
  if (cur.kind === "app") return cur;
  if (cur.kind === "account" && accounts.some((a) => a.id === cur.id)) return cur;
  return fallback(accounts);
}

export function sidebarClashes(accounts: AccountEntry[]): Set<string> {
  const byLabel = new Map<string, string[]>();
  for (const a of accounts) {
    byLabel.set(a.sidebarLabel, [...(byLabel.get(a.sidebarLabel) ?? []), a.id]);
  }
  const out = new Set<string>();
  for (const ids of byLabel.values()) if (ids.length > 1) ids.forEach((i) => out.add(i));
  return out;
}
