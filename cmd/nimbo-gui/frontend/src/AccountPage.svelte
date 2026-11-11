<script lang="ts">
  // One account's page in the Settings window: its header (status, Sign in
  // again, Sign out), sync folder location, folders, local network address and
  // Explorer sidebar name. Every App call names this account (acct.id), so the
  // page never acts on whichever account the tray happens to show. It emits
  // nothing: the backend raises "account" after a change and the parent
  // reloads the account list.
  import { untrack } from "svelte";
  import { App } from "../bindings/github.com/otherworld/nimbo/cmd/nimbo-gui";
  import { ask, tell } from "./dialogs.svelte";
  import FolderPicker from "./FolderPicker.svelte";
  import { planChanges, planEmpty, asksKeepOrDelete, freedBytes, downloadBytes, fmtSize } from "./folderSelection";
  import { needsAttention, sidebarClashes, type AccountEntry } from "./accounts";

  let {
    acct,
    accounts,
    policy,
    brandName,
    syncMode,
    folderRefresh = 0,
  }: {
    acct: AccountEntry;
    accounts: AccountEntry[];
    policy: { allowSignOut: boolean };
    brandName: string;
    // File availability is app-wide and owned by the parent; the folder views
    // follow it. folderRefresh is bumped by the parent when a mode switch
    // finishes, which can change the folder list without changing the mode.
    syncMode: string;
    folderRefresh?: number;
  } = $props();

  // An account with no running engine (needs signing in again, or failed to
  // start) shows its folders from disk, read-only.
  const readOnly = $derived(!acct.running);

  type Entry = { name: string; path: string; isDir: boolean };
  // The guard fields are optional because they ride on PairDTO without a
  // bindings regen, so the generated model doesn't declare them (it copies them
  // through at runtime). See PairDTO in service.go.
  type Pair = {
    localDir: string; remoteRoot: string; excludes: string[];
    frozen?: boolean;
    freezeReason?: string; freezeSample?: string[];
    heldReason?: string; // not syncing: another account uses this folder (#11)
  };

  // Folders
  let cur = $state("");
  let entries = $state<Entry[]>([]);
  let pairs = $state<Pair[]>([]);
  let synced = $state<string[]>([]);
  let baseDir = $state("");

  // Add-a-sync flow: select one or more remote folders, then sync them all into
  // a single local parent folder (each as a subfolder).
  let adding = $state(false);                       // false = list view, true = add flow
  let step = $state<"browse" | "dest">("browse");
  let selected = $state<string[]>([]);              // remote paths chosen to sync
  let parent = $state("");                          // local parent folder
  let names = $state<Record<string, string>>({});   // remote path -> local subfolder name
  let addErr = $state("");
  let busy = $state(false);
  let everything = $state(false);                   // syncing the whole account into one folder

  // Manage flow: deselect (or re-select) subfolders of an existing connection.
  // Clicks in the tree are staged in mTarget (the excludes after the user's
  // changes) and nothing reaches the engine until Apply; see folderSelection.ts.
  let managing = $state(false);
  let managePair = $state<Pair | null>(null);
  let mTarget = $state<string[]>([]);
  let mSizes = $state<Record<string, number>>({});
  let mBusy = $state(false);
  let mErrors = $state<{ rel: string; msg: string }[]>([]);
  // The one "keep or delete the downloaded files?" question Apply asks when
  // folders stop syncing.
  let askKeep = $state(false);
  const mCommitted = $derived(managePair?.excludes ?? []);
  const mPlan = $derived(planChanges(mTarget, mCommitted));

  async function loadFolders() {
    pairs = (await App.GetPairs(acct.id)) ?? [];
    synced = (await App.SyncedRemotes(acct.id)) ?? [];
    baseDir = await App.GetBaseDir(acct.id);
    if (adding && step === "browse") entries = (await App.BrowseRemote(acct.id, cur)) ?? [];
  }

  async function startAdding() {
    adding = true; step = "browse"; selected = []; addErr = ""; cur = ""; everything = false;
    await loadFolders();
  }
  function exitAdding() { adding = false; step = "browse"; selected = []; addErr = ""; everything = false; }

  // Sync the whole account into one local folder (no per-folder picking).
  function chooseEverything() {
    everything = true;
    selected = [];
    parent = baseDir;
    addErr = "";
    step = "dest";
  }

  const trim = (s: string) => s.replace(/^\/+|\/+$/g, "");
  const base = (p: string) => trim(p).split("/").pop() || trim(p);
  const localName = (d: string) => d.replace(/[\\/]+$/, "").split(/[\\/]/).pop() || d;
  const coveringPair = (p: string): Pair | undefined =>
    pairs.find(pr => { const r = trim(pr.remoteRoot); return r !== p && (r === "" || p.startsWith(r + "/")); });
  const relWithin = (p: string, root: string) => (root === "" ? p : p.slice(trim(root).length + 1));

  function folderState(p: string): "synced" | "covered" | "none" {
    if (synced.includes(p)) return "synced";
    return coveringPair(p) ? "covered" : "none";
  }

  // --- Manage an existing connection: deselect/re-select its subfolders ---
  async function startManage(p: Pair) {
    await loadFolders(); // the latest excludes, not the ones the list was drawn with
    const fresh = pairs.find(pr => pr.localDir === p.localDir) ?? p;
    managing = true; managePair = fresh; mTarget = [...(fresh.excludes ?? [])];
    mSizes = {}; mErrors = []; askKeep = false;
  }
  function closeManage() { managing = false; managePair = null; askKeep = false; mErrors = []; }
  async function exitManage() {
    if (!planEmpty(mPlan) && !(await ask({
      title: "Discard your changes?",
      message: "You’ve changed which folders sync but haven’t applied it yet.",
      ok: "Discard", cancel: "Keep editing", danger: true,
    }))) return;
    closeManage();
  }
  function discardManage() { mTarget = [...mCommitted]; mErrors = []; }
  function applyManage() {
    if (planEmpty(mPlan) || mBusy) return;
    if (asksKeepOrDelete(mPlan, mCommitted)) askKeep = true;
    else runApply(false);
  }
  // Folders that stop syncing go first, then the ones that sync again: when a
  // folder is re-ticked with one inside it unticked, the inner exclude must be
  // saved before the outer one is lifted, or the next sync starts downloading it.
  // A failure is shown against its folder and that folder stays staged; the rest
  // still apply.
  async function runApply(deleteLocal: boolean) {
    if (!managePair) return;
    askKeep = false; mBusy = true; mErrors = [];
    const dir = managePair.localDir;
    const plan = mPlan;
    const errs: { rel: string; msg: string }[] = [];
    for (const rel of plan.exclude) {
      try {
        const err = await App.DeselectFolder(acct.id, dir, rel, deleteLocal);
        if (err) errs.push({ rel, msg: err });
      } catch (e) { errs.push({ rel, msg: String(e) }); }
    }
    for (const rel of plan.include) {
      try { await App.RemoveExclude(acct.id, dir, rel); }
      catch (e) { errs.push({ rel, msg: String(e) }); }
    }
    await loadFolders();
    const fresh = pairs.find(pr => pr.localDir === dir);
    if (!fresh) { mBusy = false; closeManage(); return; } // the connection went away meanwhile
    managePair = fresh;
    // Saved state, plus whatever failed to apply, so it shows as still to do.
    const failedEx = errs.filter(e => plan.exclude.includes(e.rel)).map(e => e.rel);
    const failedIn = new Set(errs.filter(e => plan.include.includes(e.rel)).map(e => e.rel));
    mTarget = [...new Set([...(fresh.excludes ?? []).filter(x => !failedIn.has(x)), ...failedEx])];
    mErrors = errs;
    mBusy = false;
  }
  const mStops = $derived(mPlan.exclude.length);
  const mStarts = $derived(mPlan.include.length);
  const mFreed = $derived(freedBytes(mPlan, mCommitted, mSizes));
  const mDownload = $derived(downloadBytes(mPlan, mTarget, mSizes));
  const folders = (n: number) => `${n} folder${n === 1 ? "" : "s"}`;
  const mSummary = $derived([
    mStops ? `${folders(mStops)} will stop syncing` : "",
    mStarts ? `${folders(mStarts)} will sync again` : "",
  ].filter(Boolean).join(", "));
  const nameOf = (rel: string) => rel.split("/").pop() || rel;

  function toggleSelect(path: string) {
    const t = trim(path);
    selected = selected.includes(t) ? selected.filter(x => x !== t) : [...selected, t];
  }
  // Move to the destination step, seeding the parent and a subfolder name per pick.
  function toDest() {
    if (!selected.length) return;
    parent = baseDir;
    const m: Record<string, string> = {};
    for (const r of selected) m[r] = base(r);
    names = m;
    addErr = "";
    step = "dest";
  }
  async function browseParent() {
    const p = await App.PickLocalFolder(parent || baseDir);
    if (p) parent = p;
  }
  function joinLocal(name: string): string {
    const b = parent.replace(/[\\/]+$/, "");
    const sep = b.includes("\\") ? "\\" : "/";
    return b ? b + sep + name : name;
  }
  async function confirmAdd() {
    busy = true; addErr = "";
    if (everything) {
      const dir = parent.trim();
      const err = !dir ? "Choose a local folder" : await App.AddSyncPair(acct.id, dir, "");
      busy = false;
      if (err) { addErr = err; return; }
      exitAdding();
      await loadFolders();
      return;
    }
    const errs: string[] = [];
    for (const r of selected) {
      // If this folder is already synced by a broader connection (e.g. the whole-
      // account one), pull it out of that connection first — removing its old local
      // copy so it isn't kept in two places — then add it to the new group. The
      // server copy is always kept; the folder simply re-downloads into its new home.
      const cov = coveringPair(r);
      if (cov) {
        const derr = await App.DeselectFolder(acct.id, cov.localDir, relWithin(r, trim(cov.remoteRoot)), true);
        if (derr) { errs.push(`/${r}: ${derr}`); continue; }
      }
      const err = await App.AddSyncPair(acct.id, joinLocal((names[r] || base(r)).trim()), r);
      if (err) errs.push(`/${r}: ${err}`);
    }
    busy = false;
    if (errs.length) { addErr = errs.join("\n"); await loadFolders(); return; }
    exitAdding();
    await loadFolders();
  }
  async function removePair(p: Pair) {
    await App.RemoveSyncFolder(acct.id, trim(p.remoteRoot), false); // keep local files
    await loadFolders();
  }

  // --- Damage-guard freeze: resume after review ---
  //
  // Rides SetSyncMode's sentinel dispatch (see its comment in service.go) so no
  // new Wails binding is needed; returns "" or a message. The account and the
  // folder's local directory follow the colon, separated by a newline.
  // resumeBusy is cleared only AFTER the row has been rebuilt from the engine,
  // so a stale banner's button cannot be clicked against a folder whose state
  // has already changed.
  let resumeBusy = $state("");
  async function resumeFrozen(p: Pair) {
    resumeBusy = p.localDir;
    const err = await App.SetSyncMode("guard-resume:" + acct.id + "\n" + p.localDir);
    if (err) tell({ title: "Couldn't resume syncing", message: err });
    await loadFolders();
    resumeBusy = "";
  }
  // "Move…" is disabled while this account is syncing. The engine also refuses
  // a mid-sync move, but disabling the button prevents the click and explains why.
  let moveBusy = $state(false);
  async function movePair(p: Pair) {
    if (moveBusy) return;
    // Ask for the PARENT folder to move into, then append this sync folder's own
    // name — so you pick WHERE and Nimbo makes the correctly-named subfolder. This
    // avoids accidentally landing on a populated folder (the "already has files"
    // refusal) and never merges into someone else's data.
    const curParent = p.localDir.replace(/[\\/][^\\/]*$/, "") || p.localDir;
    const newParent = await App.PickLocalFolder(curParent);
    if (!newParent) return;
    const sep = newParent.includes("\\") ? "\\" : "/";
    const dest = newParent.replace(/[\\/]+$/, "") + sep + localName(p.localDir);
    if (dest === p.localDir) return; // same place — nothing to do
    if (!await ask({
      title: "Move sync folder?",
      message: `It moves to:\n${dest}\n\n${brandName} moves your files there itself and keeps syncing, nothing is re-downloaded. Don't move the folder yourself in Explorer while ${brandName} is running.`,
      ok: "Move",
    })) return;
    moveBusy = true;
    const err = await App.MoveSyncFolder(acct.id, p.localDir, dest);
    moveBusy = false;
    if (err) { tell({ title: "Couldn't move the folder", message: err }); return; }
    await loadFolders();
  }

  const navigate = async (p: string) => { cur = trim(p); await loadFolders(); };
  const up = () => { if (cur) navigate(cur.split("/").slice(0, -1).join("/")); };
  // Auto-save feedback: flash a small "Saved ✓" next to the control that just
  // persisted (keyed so several controls don't fight over one indicator).
  let savedFlash = $state("");
  let savedTimer: ReturnType<typeof setTimeout> | undefined;
  function flashSaved(key: string) {
    savedFlash = key;
    clearTimeout(savedTimer);
    savedTimer = setTimeout(() => (savedFlash = ""), 1500);
  }
  // SetBaseDir refuses (with its own dialog) a folder another account uses, so
  // read the folder back rather than assuming it was saved.
  const saveBase = async () => {
    const want = baseDir.trim();
    await App.SetBaseDir(acct.id, want);
    baseDir = await App.GetBaseDir(acct.id);
    if (baseDir === want) flashSaved("base");
  };

  // "Keep on this PC (offline)" browser over the virtual root's local placeholder tree.
  type OffEntry = { name: string; rel: string; pinned: boolean };
  let offCur = $state("");
  let offEntries = $state<OffEntry[]>([]);
  let pinBusy = $state(false);
  async function loadOffline() { offEntries = (await App.BrowseOffline(acct.id, offCur)) ?? []; }
  const offNav = async (rel: string) => { offCur = rel; await loadOffline(); };
  const offUp = () => offNav(offCur.split("/").slice(0, -1).join("/"));
  async function togglePin(e: OffEntry) {
    pinBusy = true;
    const err = await App.SetOfflinePin(acct.id, e.rel, !e.pinned);
    pinBusy = false;
    if (err) { tell({ title: "Couldn't change Keep on this PC", message: err }); return; }
    await loadOffline();
  }

  // Every completed mode switch changes BOTH the mode and the folder list
  // (leaving virtual files restores the remembered pairs at the very END of
  // the switch; entering it clears them), so reload on either signal, and on
  // first show. Untracked, so the 3 s account-list refresh (a new acct object
  // each time) doesn't reload the page with it.
  $effect(() => {
    syncMode; folderRefresh;
    untrack(() => {
      loadFolders();
      if (syncMode === "ondemand") loadOffline();
    });
  });

  // Sign out: one button for every account. For the shown account the backend
  // hands the shown role to the next one; any other account goes on its own.
  let signOutOpen = $state(false);
  let clearOnSignOut = $state(false);
  async function signOut() {
    const err = await App.SignOut(acct.id, clearOnSignOut);
    if (err) { tell({ title: "Couldn't sign out", message: err }); return; }
    signOutOpen = false;
  }

  // Local network route (spec 2026-09-13). Test → (Trust) → Use this address.
  // The saved address and the live route ride the account list, which the
  // parent refreshes every 3 s. The box follows the SAVED address only when that
  // changes (a derived string), so the refresh doesn't overwrite typing.
  type LocalTest = { result: string; message: string; fingerprint: string; subject: string; issuer: string; expires: string; address: string };
  let localAddr = $state("");
  let localPin = $state("");
  let localBusy = $state(false);
  let localTest = $state<LocalTest | null>(null);
  const savedLocal = $derived(acct.localAddress ?? "");
  $effect(() => { localAddr = savedLocal; });
  async function testLocal(pin = "") {
    localBusy = true; localTest = null; localPin = pin;
    try { localTest = await App.TestLocalAddress(acct.id, localAddr, pin); }
    finally { localBusy = false; }
  }
  async function saveLocal() {
    localBusy = true;
    try {
      const err = await App.SaveLocalAddress(acct.id, localAddr, localPin);
      if (err) { tell({ title: "Couldn't save the local address", message: err }); return; }
      localTest = null; localPin = "";
      flashSaved("local");
    } finally { localBusy = false; }
  }
  async function clearLocal() {
    localBusy = true;
    try {
      const err = await App.SaveLocalAddress(acct.id, "", "");
      if (err) { tell({ title: "Couldn't clear the local address", message: err }); return; }
      localTest = null; localPin = ""; localAddr = "";
    } finally { localBusy = false; }
  }
  function routeText(): string {
    if (!acct.localAddress) return "";
    if (acct.route === "local") return "Connected directly over your local network.";
    if (acct.routeReason === "certificate changed") return `The certificate at the local address isn't the one ${brandName} trusts, so ${brandName} is using the internet. If you renewed your server's certificate, test the address again while you're on your own network. If you're away from home, another device may be answering at that address — leave it.`;
    if (acct.routeReason === "different server") return `The local address answers, but it's a different server or account, so ${brandName} is using the internet.`;
    return "Using the internet — the local address isn't reachable right now.";
  }

  // The account's Explorer sidebar entry is named automatically ("OtherSync - adam", or "Nimbo - adam" for an account from before the rename); the
  // user can give it a name of their own instead (GitHub #19). Saved on change
  // (Enter or leaving the box); an empty box goes back to the automatic name.
  let navSupported = $state(false);
  (async () => { navSupported = await App.SidebarSupported(); })();
  async function saveNavName(name: string) {
    const err = await App.SetAccountSidebarName(acct.id, name);
    if (err) tell({ title: "Couldn't rename the sidebar entry", message: err });
  }
  const navNameClash = $derived(sidebarClashes(accounts).has(acct.id));
</script>

<div class="acct">
  <span class="avatar" aria-hidden="true">{(acct.user || "?").slice(0, 1).toUpperCase()}</span>
  <div class="acctinfo">
    <b>{acct.user}</b>
    <span>{acct.server}</span>
    {#if acct.status || !acct.running}
      <span class="acctline" class:warn={needsAttention(acct)} title={acct.status}>{acct.status || "Not syncing"}</span>
    {/if}
  </div>
  <div class="acctacts">
    {#if needsAttention(acct)}<button class="link" onclick={() => App.SignInAgain(acct.id)}>Sign in again</button>{/if}
    {#if policy.allowSignOut}<button class="signout" onclick={() => (signOutOpen = !signOutOpen)}>Sign out</button>{:else}<span class="managed">🔒 Managed by your organisation</span>{/if}
  </div>
</div>
{#if signOutOpen && policy.allowSignOut}
  <div class="signoutbox">
    <label class="check"><input type="checkbox" bind:checked={clearOnSignOut} /> Also remove this device's sync setup &amp; database</label>
    <p class="fhint">Leave unticked for a temporary sign-out — your synced folders are remembered for a quick re-login. Tick it for a clean reset: recommended if you'll delete the local sync folder, since it stops a fresh login from mistaking the missing files for deletions and removing them on the server. Your preferences are kept either way.</p>
    <div class="addbtns">
      <button class="signout" onclick={signOut}>{clearOnSignOut ? "Sign out & clear data" : "Sign out"}</button>
      <button onclick={() => (signOutOpen = false)}>Cancel</button>
    </div>
  </div>
{/if}

{#if readOnly}
  <p class="fhint ronote">This account isn't syncing right now. Sign in again to change its folders.</p>
{:else}
  <div class="field">
    <label for={"basedir-" + acct.id}>Sync folder location</label>
    <p class="fhint">Where newly-added folders are placed by default.</p>
    <div class="baserow"><input id={"basedir-" + acct.id} bind:value={baseDir} onchange={saveBase} />{#if savedFlash === "base"}<span class="saved">Saved ✓</span>{/if}</div>
  </div>
{/if}

{#if syncMode === "ondemand"}
  <!-- On-demand mode: the whole account is virtual; the list below pins folders, it doesn't pick what syncs. -->
  <div class="row"><h3>Virtual file system</h3></div>
  <p class="fhint">Your whole Nextcloud account is in your sync folder. Files stay online-only and download when you open them. To copy chosen folders to this PC as ordinary files instead, switch <b>File availability</b> to <b>Live file system</b> on the <b>Sync</b> page.</p>
  <div class="logrow"><button class="primary small" onclick={() => App.OpenFolder(baseDir)} disabled={!baseDir}>Open sync folder</button></div>

  {#if !readOnly}
    <h3>Keep on this PC (offline)</h3>
    <p class="fhint">Tick a folder to keep everything in it on this PC for offline use. It downloads now and stays up to date. This is the same as right-clicking the folder in Explorer and choosing <b>Always keep on this device</b>. Unticked folders stay online-only. Unticking doesn't remove files that are already downloaded; to do that, right-click → <b>Free up space</b> in Explorer. Folders show up here once you've opened them in Explorer.</p>
    <div class="crumb"><button onclick={offUp} disabled={!offCur}>⬆ Up</button><span>/{offCur}</span></div>
    {#if offEntries.length === 0}<p class="empty">(no folders here yet — open the sync folder and browse to populate it)</p>{/if}
    {#each offEntries as e}
      <div class="frow">
        <label class="cov"><input type="checkbox" checked={e.pinned} disabled={pinBusy} onchange={() => togglePin(e)} /> Keep on this PC</label>
        <button class="name" onclick={() => offNav(e.rel)}>📁 {e.name}</button>
      </div>
    {/each}
  {/if}
{:else if readOnly || (!adding && !managing)}
  <!-- List view: existing synced folders -->
  <div class="row"><h3>Synced folders</h3>{#if !readOnly}<button class="primary small" onclick={startAdding}>＋ Add folder</button>{/if}</div>
  {#if pairs.length === 0}
    <p class="empty">{readOnly ? "No folders synced yet." : "No folders synced yet. Click “Add folder” to choose one."}</p>
  {:else}
    {#each pairs as p}
      {#if p.heldReason}
        <!-- Another account syncs this folder too (GitHub #11). It is
             held back so neither account uploads the other's files, and
             resumes once one of them uses a different folder. -->
        <div class="freeze">
          <div class="freezehead">⚠ Not syncing — another account uses this folder</div>
          <p class="freezewhy">{p.heldReason}</p>
          <p class="freezewhat">
            Nothing in <b>{localName(p.localDir)}</b> was changed. Give one of the accounts a
            different folder (remove this one and add it again elsewhere) and syncing resumes.
          </p>
        </div>
      {/if}
      {#if p.frozen}
        <!-- The damage guard stopped this folder: a pass looked like the
             server losing files rather than the user changing them.
             Nothing was downloaded or removed — it waits for a human. -->
        <div class="freeze">
          <div class="freezehead">⚠ Sync paused — needs review</div>
          <p class="freezewhy">{p.freezeReason}</p>
          <p class="freezewhat">
            <b>{localName(p.localDir)}</b> was not changed: nothing was downloaded or removed.
            This usually means the folder on the server was moved, emptied, or restored from
            an older backup. If that was expected, resume; if not, check the server first.
          </p>
          {#if p.freezeSample?.length}
            <ul class="freezelist">
              {#each p.freezeSample as s}<li>{s}</li>{/each}
            </ul>
          {/if}
          {#if !readOnly}
            <button class="primary small" disabled={resumeBusy === p.localDir}
                    onclick={() => resumeFrozen(p)}>Resume syncing</button>
          {/if}
        </div>
      {/if}
      <div class="conn">
        <div class="connmain">
          <span class="remote">/{trim(p.remoteRoot)}</span>
          <span class="arrow">→</span>
          <span class="local" title={p.localDir}>{p.localDir}</span>
        </div>
        {#if !readOnly}
          <div class="connacts">
            <button class="link" title="Pick which subfolders sync; deselect to free space" onclick={() => startManage(p)}>Choose folders…</button>
            <button class="link" disabled={acct.syncing || moveBusy}
                    title={acct.syncing ? "Can’t move while a sync is running — wait for it to finish, then try again" : "Move this sync folder to a new location without re-downloading"}
                    onclick={() => movePair(p)}>Move…</button>
            <button class="link danger" title="Stop syncing this connection (local files are kept)" onclick={() => removePair(p)}>Remove</button>
          </div>
        {/if}
      </div>
    {/each}
  {/if}

{:else if managing && managePair}
  <!-- Manage view: pick which subfolders of one connection sync. A tree
       rather than one level at a time, so there's no breadcrumb: where a
       folder sits is shown by its indent, and the connection's own
       server and local folders are named once, below the heading.
       Clicks are staged; nothing changes until Apply. -->
  <div class="row"><h3>Choose folders</h3><button class="link" onclick={exitManage} disabled={mBusy}>← Back</button></div>
  <p class="mconn">
    <span class="remote">/{trim(managePair.remoteRoot)}</span>
    <span class="arrow">→</span>
    <span class="local">{managePair.localDir}</span>
  </p>
  <p class="hint mhint">Untick a folder to stop syncing it, tick it to sync it again. A half-ticked folder has some folders inside it that don’t sync; tick it to sync all of it. Your files on the server are always kept, and nothing changes until you press <b>Apply</b>.</p>
  {#key managePair.localDir}
    <FolderPicker root={managePair.remoteRoot} committed={mCommitted} bind:target={mTarget} bind:sizes={mSizes} disabled={mBusy}
                  browse={(p) => App.BrowseRemote(acct.id, p)} />
  {/key}
  {#if mErrors.length}
    <div class="merrs">
      <div class="merrhead">{mErrors.length === 1 ? "One change didn’t apply" : `${mErrors.length} changes didn’t apply`} (left as you set them, so you can try again):</div>
      <ul>{#each mErrors as e}<li><b>{nameOf(e.rel)}</b>: {e.msg}</li>{/each}</ul>
    </div>
  {/if}
  <div class="applybar">
    <div class="applysum">
      {#if planEmpty(mPlan)}
        <span class="dmuted">No changes</span>
      {:else}
        <span>{mSummary}</span>
        {#if mStops && mFreed}<span class="dmuted">Frees about {fmtSize(mFreed)} if you delete the downloaded copies</span>{/if}
        {#if mStarts && mDownload}<span class="dmuted">About {fmtSize(mDownload)} to download</span>{/if}
      {/if}
    </div>
    <div class="applybtns">
      {#if !planEmpty(mPlan)}<button class="link" onclick={discardManage} disabled={mBusy}>Undo changes</button>{/if}
      <button class="primary small" onclick={applyManage} disabled={mBusy || planEmpty(mPlan)}>{mBusy ? "Applying…" : "Apply"}</button>
    </div>
  </div>

  {#if askKeep}
    <div class="modalback">
      <div class="addpanel modalbox">
        <div class="addhead">Stop syncing {mStops === 1 ? `“${nameOf(mPlan.exclude[0])}”` : folders(mStops)}?</div>
        <p class="hint">Your files on the server are kept. What about the copies already downloaded to this PC?{mFreed ? ` Deleting them frees about ${fmtSize(mFreed)}.` : ""}</p>
        {#if mStops > 1}
          <ul class="hint mlist">{#each mPlan.exclude as rel}<li>{nameOf(rel)}{#if mSizes[rel]}{" "}<span class="dmuted">({fmtSize(mSizes[rel])})</span>{/if}</li>{/each}</ul>
        {/if}
        <div class="addbtns">
          <button class="primary" onclick={() => runApply(false)}>Keep them here</button>
          <button class="danger" onclick={() => runApply(true)}>Delete them</button>
        </div>
        <button class="link" onclick={() => (askKeep = false)}>Cancel</button>
      </div>
    </div>
  {/if}

{:else if step === "dest" && everything}
  <!-- Whole-account: one local folder mirrors the entire Nextcloud. -->
  <div class="row"><h3>Sync your entire Nextcloud</h3><button class="link" onclick={() => (step = "browse")}>← Back</button></div>
  <div class="addpanel">
    <div class="addhead">Local folder to mirror your whole account into:</div>
    <div class="addrow2">
      <input bind:value={parent} placeholder="e.g. C:\Users\You\Nextcloud" />
      <button onclick={browseParent}>Browse…</button>
    </div>
    <p class="hint">Everything in your Nextcloud syncs here — you can exclude individual subfolders afterwards from the folder list. Big dev folders like <b>node_modules</b> and <b>.git</b> sync too; exclude them under Exclusions for faster syncing (<b>.git</b> is best excluded if you work on a repo across machines).</p>
    {#if addErr}<pre class="err">{addErr}</pre>{/if}
    <div class="addbtns">
      <button class="primary" onclick={confirmAdd} disabled={busy || !parent.trim()}>{busy ? "Setting up…" : "Sync everything here"}</button>
      <button onclick={exitAdding}>Cancel</button>
    </div>
  </div>

{:else if step === "dest"}
  <!-- Step 2: one local parent folder; each picked folder becomes a subfolder -->
  <div class="row"><h3>Choose where to sync</h3><button class="link" onclick={() => (step = "browse")}>← Back</button></div>
  <div class="addpanel">
    <div class="addhead">Local parent folder (on this computer):</div>
    <div class="addrow2">
      <input bind:value={parent} placeholder="e.g. C:\Users\You\Nextcloud" />
      <button onclick={browseParent}>Browse…</button>
    </div>
    <p class="hint">Each Nextcloud folder below syncs into a subfolder of that parent. Final path = parent \ subfolder name.</p>
    {#if selected.some(r => coveringPair(r))}
      <p class="hint warnhint">Some of these are already synced elsewhere — they'll <b>move</b> to this location: the old local copy is removed and re-downloaded here. Your server copy is untouched.</p>
    {/if}
    <div class="destlist">
      <div class="destrow desthead">
        <span class="remote">Nextcloud folder</span>
        <span class="arrow"></span>
        <span class="namehdr">Local subfolder name</span>
      </div>
      {#each selected as r}
        <div class="destrow">
          <span class="remote" title={"/" + r}>/{r}</span>
          <span class="arrow">→</span>
          <input class="nameinput" bind:value={names[r]} />
        </div>
      {/each}
    </div>
    {#if addErr}<pre class="err">{addErr}</pre>{/if}
    <div class="addbtns">
      <button class="primary" onclick={confirmAdd} disabled={busy}>{busy ? "Adding…" : `Add ${selected.length} folder${selected.length > 1 ? "s" : ""}`}</button>
      <button onclick={exitAdding}>Cancel</button>
    </div>
  </div>

{:else}
  <!-- Step 1: browse Nextcloud and tick folders to sync -->
  <div class="row"><h3>{cur === "" && pairs.length === 0 ? "How do you want to sync?" : "Pick folders to sync"}</h3><button class="link" onclick={exitAdding}>← Back</button></div>
  {#if cur === "" && pairs.length === 0}
    <button class="everything" onclick={chooseEverything}>
      <span class="etop"><span class="etitle">☁ Sync my entire Nextcloud into one folder</span><span class="ebadge">Recommended</span></span>
      <span class="ehint">Mirror the whole account into one folder — the simplest setup. You can exclude individual folders later.</span>
    </button>
    <div class="orsep">— or tick individual folders below to sync only those —</div>
  {/if}
  <div class="crumb"><button onclick={up} disabled={!cur}>⬆ Up</button><span>/{cur}</span></div>
  {#if entries.length === 0}<p class="empty">(empty)</p>{/if}
  {#each entries as e}
    <div class="frow">
      {#if e.isDir}
        {@const st = folderState(e.path)}
        {#if st === "synced"}
          <!-- Already its own sync connection — manage or move it from the list. -->
          <span class="selspace"></span>
          <button class="name" onclick={() => navigate(e.path)}>📁 {e.name}</button>
          <span class="tag">✓ its own sync</span>
        {:else}
          <!-- Selectable for a NEW group. "covered" folders (already in a broader
               sync, e.g. the whole-account one) can be ticked to move here. -->
          <input class="selbox" type="checkbox" checked={selected.includes(trim(e.path))} onchange={() => toggleSelect(e.path)} />
          <button class="name" onclick={() => navigate(e.path)}>📁 {e.name}</button>
          {#if st === "covered"}
            {@const cov = coveringPair(e.path)}
            <span class="tag moving" title={cov ? "Currently synced in " + cov.localDir : ""}>in {cov ? localName(cov.localDir) : "another sync"}{selected.includes(trim(e.path)) ? " → moves here" : ""}</span>
          {/if}
        {/if}
      {:else}
        <span class="selspace"></span>
        <span class="name file">📄 {e.name}</span>
      {/if}
    </div>
  {/each}
  <div class="addbar">
    <span>{selected.length} selected</span>
    <button class="primary small" disabled={!selected.length} onclick={toDest}>Next →</button>
  </div>
{/if}

{#if !readOnly && acct.server.toLowerCase().startsWith("https://")}
  <div class="field localroute">
    <label for={"localaddr-" + acct.id}>Local network address</label>
    <p class="fhint">Sync directly with your server when you're on the same network; {brandName} uses the internet address whenever the local one isn't reachable. Needs HTTPS on that address — a self-signed certificate is fine. Leave empty if your server isn't on your network. Your local address must serve the same site, including the /push/ws notification socket.</p>
    <div class="baserow">
      <input id={"localaddr-" + acct.id} placeholder="192.168.1.100" bind:value={localAddr} disabled={localBusy} oninput={() => { localTest = null; localPin = ""; }} onkeydown={(e) => { if (e.key === "Enter" && localAddr.trim()) testLocal(); }} />
      <button class="small" disabled={localBusy || !localAddr.trim()} onclick={() => testLocal()}>{localBusy ? "Testing…" : "Test"}</button>
      {#if acct.localAddress}<button class="link danger" disabled={localBusy} onclick={clearLocal}>Remove</button>{/if}
      {#if savedFlash === "local"}<span class="saved">Saved ✓</span>{/if}
    </div>
    {#if localTest}
      {#if localTest.result === "ok"}
        <p class="fhint">✓ {localTest.message}</p>
        <div class="addbtns"><button class="primary small" disabled={localBusy} onclick={saveLocal}>Use this address</button></div>
      {:else if localTest.result === "untrusted"}
        <p class="fhint">{localTest.message}</p>
        <pre class="cert">{localTest.subject}
Issued by: {localTest.issuer}
Expires: {localTest.expires}
SHA-256: {localTest.fingerprint}</pre>
        <p class="fhint">Compare the SHA-256 fingerprint with the one shown on your server before trusting it. If you use your own certificate authority, you can instead import it into Windows and no pin is needed. Trusting pins exactly this certificate; if it is later replaced, you'll need to trust the new one.</p>
        <div class="addbtns"><button class="primary small" disabled={localBusy} onclick={() => testLocal(localTest?.fingerprint ?? "")}>Trust this certificate</button></div>
      {:else}
        <pre class="err">{localTest.message}</pre>
      {/if}
    {:else if acct.localAddress}
      <p class="fhint">{routeText()}{#if acct.localPinned}{" (certificate pinned)"}{/if}</p>
    {/if}
  </div>
{/if}

{#if navSupported}
  <div class="field navname">
    <label for={"navname-" + acct.id}>Explorer sidebar name</label>
    <input id={"navname-" + acct.id} type="text" maxlength="80" placeholder={acct.sidebarAuto} value={acct.sidebarName} onchange={(e) => saveNavName(e.currentTarget.value)} />
    <p class="fhint">Leave it empty to use the name shown in grey.</p>
    {#if navNameClash}<p class="fhint danger">Two accounts have the same sidebar name, so you won't be able to tell their entries apart.</p>{/if}
  </div>
{/if}

<style>
  /* Header: avatar initial, who and where, status line; Sign in again and
     Sign out on the right. */
  .acct { display: flex; align-items: center; justify-content: space-between; gap: 10px; padding: 10px 12px;
          border: 1px solid var(--border); border-radius: 8px; background: var(--panel); margin-bottom: 16px; }
  .avatar { flex: 0 0 auto; width: 34px; height: 34px; border-radius: 50%; display: flex; align-items: center; justify-content: center;
            background: var(--accent); color: #fff; font-size: 15px; font-weight: 700; }
  .acctinfo { display: flex; flex-direction: column; min-width: 0; flex: 1; }
  .acctinfo b { font-size: 14px; }
  .acctinfo span { font-size: 12px; color: var(--muted); overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
  .acctinfo .acctline { color: var(--fg2); }
  .acctinfo .acctline.warn { color: #b26a00; }
  .acctacts { display: flex; align-items: center; gap: 10px; flex: 0 0 auto; }
  .ronote { margin: 0 0 12px; }
  .hint { color: var(--fg2); font-size: 12px; margin: 0 0 12px; }
  .warnhint { color: #8a6d1a; background: #fff8e6; border: 1px solid #f5e3b0; border-radius: 6px; padding: 7px 9px; }
  .fhint { color: var(--muted); font-size: 12px; margin: 0 0 6px; }
  .navname { margin-top: 18px; }
  .dmuted { color: var(--muted); }
  .baserow { display: flex; gap: 8px; align-items: center; margin-bottom: 12px; }
  .baserow input { flex: 1; padding: 6px 8px; border: 1px solid var(--border-2); border-radius: 5px; background: var(--bg); color: var(--fg); }
  .everything { display: flex; flex-direction: column; gap: 3px; width: 100%; text-align: left; cursor: pointer;
                padding: 14px 15px; border: 2px solid var(--accent); border-radius: 10px;
                background: color-mix(in srgb, var(--accent) 12%, var(--bg)); color: var(--fg); }
  .everything:hover { background: color-mix(in srgb, var(--accent) 20%, var(--bg)); }
  .everything .etop { display: flex; align-items: center; justify-content: space-between; gap: 8px; }
  .everything .etitle { font-size: 14.5px; font-weight: 700; }
  .everything .ebadge { flex: 0 0 auto; font-size: 10px; font-weight: 700; text-transform: uppercase; letter-spacing: .04em;
                        background: var(--accent); color: #fff; border-radius: 999px; padding: 2px 8px; }
  .everything .ehint { font-size: 12px; color: var(--fg2); }
  .orsep { text-align: center; color: var(--muted); font-size: 12px; margin: 12px 0; }
  .crumb { display: flex; gap: 10px; align-items: center; margin-bottom: 8px; color: var(--fg2); font-size: 13px; }
  /* Choose folders (the manage view). Long paths and the hint wrap anywhere
     rather than pushing the window wider than it is. */
  .mconn { margin: -4px 0 8px; font-size: 13px; overflow-wrap: anywhere; }
  .mconn .remote { font-weight: 600; color: var(--accent); }
  .mconn .arrow { color: var(--muted); margin: 0 4px; }
  .mconn .local { color: var(--fg2); }
  .mhint { overflow-wrap: anywhere; }
  .applybar { position: sticky; bottom: -14px; z-index: 1; display: flex; align-items: center; justify-content: space-between;
              flex-wrap: wrap; gap: 8px 12px; margin: 10px 0 -14px; padding: 10px 0 14px;
              border-top: 1px solid var(--border); background: var(--bg); font-size: 13px; color: var(--fg); }
  .applysum { display: flex; flex-direction: column; gap: 2px; min-width: 0; flex: 1 1 200px; }
  .applysum .dmuted { font-size: 12px; }
  .applybtns { display: flex; align-items: center; gap: 12px; flex: 0 0 auto; margin-left: auto; }
  .applybtns .primary:disabled { opacity: .45; cursor: not-allowed; }
  .addbtns button.danger { color: #c0392b; }
  .merrs { margin-top: 10px; padding: 8px 10px; border: 1px solid #e6b3ad; background: #fdf3f2; color: #7a2f26;
           border-radius: 8px; font-size: 12.5px; overflow-wrap: anywhere; }
  .merrs ul { margin: 4px 0 0; padding-left: 18px; }
  .merrhead { font-weight: 600; }
  .mlist { margin: -4px 0 8px; padding-left: 18px; max-height: 120px; overflow-y: auto; overflow-wrap: anywhere; }
  h3 { font-size: 12px; text-transform: uppercase; color: var(--muted); margin: 18px 0 8px; }
  /* Section header: heading left, action (Add folder / Back) right, with
     clear air before the content below it. */
  .row { display: flex; align-items: center; justify-content: space-between; gap: 10px; margin: 18px 0 12px; }
  .row h3 { margin: 0; }
  .conn { display: flex; align-items: center; justify-content: space-between; gap: 10px; padding: 8px 10px;
          border: 1px solid var(--border); border-radius: 8px; margin-bottom: 6px; background: var(--panel); }
  .connmain { display: flex; align-items: center; gap: 8px; min-width: 0; font-size: 13px; }
  .connmain .remote { font-weight: 600; color: var(--accent); white-space: nowrap; }
  .connmain .arrow { color: var(--muted); }
  .connmain .local { color: var(--fg2); overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
  .connacts { display: flex; align-items: center; gap: 10px; flex: 0 0 auto; }
  .saved { color: #2e9e5b; font-size: 12px; font-weight: 600; align-self: center; animation: savedfade 1.5s forwards; }
  .managed { color: var(--muted); font-size: 12px; margin: 4px 0 0; }
  @keyframes savedfade { 0%, 70% { opacity: 1; } 100% { opacity: 0; } }
  .danger { color: #c0392b; }
  .freeze { border: 1px solid #e6b3ad; background: #fdf3f2; color: #7a2f26; border-radius: 8px;
            padding: 10px 12px; margin-bottom: 6px; font-size: 12.5px; }
  .freezehead { font-weight: 600; margin-bottom: 4px; }
  .freeze p { margin: 0 0 6px; }
  .freezewhy { font-weight: 600; }
  .freezelist { margin: 0 0 8px; padding-left: 18px; max-height: 96px; overflow-y: auto; }
  .freezelist li { font-family: ui-monospace, Consolas, monospace; font-size: 11px; }
  .modalback { position: fixed; inset: 0; background: rgba(0,0,0,.45); display: flex;
               align-items: center; justify-content: center; z-index: 50; }
  .modalbox { max-width: 360px; width: 90%; margin: 0; box-shadow: 0 10px 40px rgba(0,0,0,.4); }
  .addpanel { border: 1px solid var(--border); background: var(--panel); border-radius: 8px; padding: 12px; margin-bottom: 12px; }
  .addhead { font-size: 13px; margin-bottom: 8px; }
  .addrow2 { display: flex; gap: 8px; }
  .addrow2 input { flex: 1; padding: 7px 8px; border: 1px solid var(--border-2); border-radius: 5px; background: var(--bg); color: var(--fg); }
  .addrow2 button { padding: 7px 12px; border: 1px solid var(--border-2); border-radius: 5px; background: var(--bg); color: var(--fg); cursor: pointer; }
  .addbtns { display: flex; gap: 8px; margin-top: 10px; }
  .addbtns .primary { padding: 7px 14px; }
  .addbtns button:not(.primary) { padding: 7px 12px; border: 1px solid var(--border-2); border-radius: 6px; background: var(--bg); color: var(--fg); cursor: pointer; }
  .err { color: #e06b6b; font-size: 12px; margin: 8px 0 0; white-space: pre-wrap; font-family: inherit; }
  .frow { display: flex; align-items: center; justify-content: space-between; gap: 8px; padding: 4px 0; }
  .name { background: none; border: none; cursor: pointer; font-size: 13px; color: var(--fg); text-align: left; flex: 1; min-width: 0;
          overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
  .name:hover { color: var(--accent); }
  .name.file { cursor: default; color: var(--fg2); }
  .tag { font-size: 12px; color: #2e7d32; white-space: nowrap; flex: 0 0 auto; }
  .tag.moving { color: #b26a00; }
  .selbox { flex: 0 0 auto; margin: 0; }
  .selspace { flex: 0 0 auto; width: 13px; }
  .addbar { display: flex; align-items: center; justify-content: space-between; gap: 10px;
            margin-top: 14px; padding-top: 10px; border-top: 1px solid var(--border); font-size: 13px; color: var(--fg2); }
  .destlist { margin-top: 8px; display: flex; flex-direction: column; gap: 6px; }
  .destrow { display: flex; align-items: center; gap: 8px; }
  .destrow .remote { font-weight: 600; color: var(--accent); max-width: 45%; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
  .destrow .arrow { color: var(--muted); }
  .nameinput { flex: 1; padding: 5px 7px; border: 1px solid var(--border-2); border-radius: 5px; font-size: 12.5px; background: var(--bg); color: var(--fg); }
  .namehdr { flex: 1; }
  .desthead { margin-bottom: 2px; }
  .desthead .remote, .namehdr { font-size: 11px; text-transform: uppercase; letter-spacing: 0.3px; color: var(--muted); font-weight: 600; }
  .cov { display: flex; align-items: center; gap: 5px; font-size: 12px; color: var(--fg2); white-space: nowrap; }
  .field { display: flex; flex-direction: column; gap: 4px; margin-bottom: 12px; max-width: 340px; }
  .field label { font-size: 12px; color: var(--fg2); }
  .field input { padding: 7px 8px; border: 1px solid var(--border-2); border-radius: 5px; background: var(--bg); color: var(--fg); }
  .check { display: flex; gap: 8px; align-items: center; font-size: 13px; margin-bottom: 12px; }
  button { font-size: 13px; }
  .primary { padding: 8px 14px; border: none; border-radius: 6px; background: var(--accent); color: #fff; cursor: pointer; }
  .primary:hover { background: var(--accent-dark); }
  .primary.small { padding: 6px 12px; font-size: 12.5px; }
  .link { background: none; border: none; color: var(--accent); cursor: pointer; }
  .link:disabled { opacity: .45; cursor: not-allowed; }
  .empty { color: var(--muted); font-size: 13px; }
  .logrow { margin: 4px 0 4px; }
  .localroute { margin-top: 18px; max-width: 480px; }
  .localroute .baserow { margin-bottom: 6px; }
  .localroute .baserow button.small { padding: 6px 12px; font-size: 12.5px; border: 1px solid var(--border-2); border-radius: 6px; background: var(--bg); color: var(--fg); cursor: pointer; }
  .localroute .baserow button.small:disabled { opacity: .45; cursor: not-allowed; }
  .cert { font-size: 11px; white-space: pre-wrap; word-break: break-all; border: 1px solid var(--border-2); border-radius: 5px; background: var(--bg); color: var(--fg); padding: 6px 8px; margin: 4px 0; font-family: ui-monospace, Consolas, monospace; }
  .signout { flex: 0 0 auto; padding: 7px 14px; border: 1px solid var(--border-2); border-radius: 6px;
             background: var(--bg); color: #c0392b; cursor: pointer; font-size: 13px; }
  .signout:hover { background: #fdeceb; }
  .signoutbox { margin: 8px 0 12px; padding: 12px; border: 1px solid var(--border); border-radius: 8px; background: var(--panel); }
  .signoutbox .addbtns { margin-top: 8px; }
</style>
