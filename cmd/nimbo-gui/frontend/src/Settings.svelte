<script lang="ts">
  import { App } from "../bindings/github.com/otherworld/nimbo/cmd/nimbo-gui";
  import { ask, tell } from "./dialogs.svelte";
  import { Events } from "@wailsio/runtime";
  import AccountPage from "./AccountPage.svelte";
  import Icon, { type IconName } from "./Icon.svelte";
  import { initialSelection, nextSelection, needsAttention, type AccountEntry, type AppPage, type Selection } from "./accounts";

  let brandName = $state("OtherSync");
  let brandCompany = $state("Otherworld Dev Ltd");
  let brandWebsite = $state("https://www.othersync.com");
  let brandSupport = $state("contact@otherworld.dev");
  let brandHelp = $state("");
  (async () => {
    const b = await App.Brand();
    brandName = b.name || brandName; brandCompany = b.company || brandCompany;
    brandWebsite = b.website || brandWebsite; brandSupport = b.support || brandSupport;
    brandHelp = b.help || "";
  })();
  const hostname = (u: string) => { try { return new URL(u).host.replace(/^www\./, ""); } catch { return u; } };
  // Help links hide themselves when the brand has no help site (white-label).
  const helpPage = (slug: string) => brandHelp + slug + ".html";

  // Admin policy (managed deployments): some settings may be locked by IT.
  let policy = $state<{ managed: boolean; lockServer: boolean; serverUrl: string; allowSignOut: boolean; lockBandwidth: boolean; lockSyncMode: boolean }>(
    { managed: false, lockServer: false, serverUrl: "", allowSignOut: true, lockBandwidth: false, lockSyncMode: false });
  (async () => { policy = await App.PolicyInfo(); })();

  // The left-hand list: one page per account, then the app-wide pages.
  const APP_PAGES: { page: AppPage; label: string; icon: IconName }[] = [
    { page: "sync", label: "Sync", icon: "sync" },
    { page: "exclusions", label: "Exclusions", icon: "exclude" },
    { page: "appearance", label: "Appearance", icon: "appearance" },
    { page: "general", label: "General", icon: "general" },
  ];

  // Deep-link support: a toast (e.g. "update available") opens this window on a
  // specific page. First open carries it in the URL hash (/#settings?tab=general);
  // an already-open window gets a "settings-tab" event instead. Either goes
  // through initialSelection, which falls back to the shown account for a tab
  // it doesn't know.
  function tabFromHash(): string {
    const h = window.location.hash;
    const q = h.indexOf("?");
    if (q < 0) return "";
    return new URLSearchParams(h.slice(q + 1)).get("tab") || "";
  }
  let accounts = $state<AccountEntry[]>([]);
  // null until the account list has loaded once: picking from the empty list
  // before then would land on the signed-out view.
  let sel = $state.raw<Selection | null>(null);
  let pendingTab = tabFromHash();
  Events.On("settings-tab", (e: any) => {
    const t = (e?.data as string) || "";
    if (sel === null) { pendingTab = t; return; }
    sel = initialSelection(accounts, t);
  });
  const page = $derived(sel?.kind === "app" ? sel.page : "");
  // The account page is keyed on this id string, never on the account entry:
  // the list is refetched every 3 s, and a remount mid-flow would drop an add
  // or Choose folders in progress.
  const selAcctId = $derived(sel?.kind === "account" ? sel.id : "");
  const selAcct = $derived(accounts.find(a => a.id === selAcctId));

  // Auto-save feedback: flash a small "Saved ✓" next to the control that just
  // persisted (keyed so several controls don't fight over one indicator).
  let savedFlash = $state("");
  let savedTimer: ReturnType<typeof setTimeout> | undefined;
  function flashSaved(key: string) {
    savedFlash = key;
    clearTimeout(savedTimer);
    savedTimer = setTimeout(() => (savedFlash = ""), 1500);
  }

  // Bandwidth
  let up_ = $state(0), down_ = $state(0);
  (async () => { const l = await App.GetLimits(); up_ = l.up; down_ = l.down; })();
  const saveLimits = () => { App.SetLimits(Number(up_) || 0, Number(down_) || 0); flashSaved("limits"); };

  // Disguised file types: server-forbidden names synced via escaped server-side
  // copies. Reuses the blocked-files bindings (no regen): add derives the ext
  // from a synthetic filename; remove runs the (now checkpoint-warm) sweep that
  // deletes the disguised server copies. List rides the diag poll.
  let newEscape = $state("");
  // The remove sweep runs in a Go goroutine (the bound call returns at once);
  // the row's ext vanishes from the diag-fed list when the sweep completes, so
  // busy-state is "this ext is still listed after I asked to remove it".
  let removingExt = $state("");
  // …which never happens if the sweep FAILS: the ext stays listed, so without a
  // deadline the row reads "Removing…" for good and every Remove button stays
  // disabled until the window is reopened. Time it out generously instead — long
  // enough not to interrupt a real sweep, and re-enabling a button is a far
  // cheaper failure than a permanently wedged panel.
  let removeTimer: ReturnType<typeof setTimeout> | undefined;
  $effect(() => {
    const list: string[] = (diag as any)?.escapeExtensions ?? [];
    if (removingExt && !list.includes(removingExt)) {
      clearTimeout(removeTimer);
      removingExt = "";
    }
  });
  async function addEscape() {
    let ext = newEscape.trim();
    if (!ext) return;
    if (!ext.startsWith(".")) ext = "." + ext;
    newEscape = "";
    await App.RenameBlocked("x" + ext, "//escape");
  }
  async function rmEscape(ext: string) {
    if (!await ask({
      title: `Stop syncing ${ext} files?`,
      message: "Their disguised copies are removed from the server and the files stay on this device only. Nothing is deleted locally.",
      ok: "Stop syncing",
    })) return;
    removingExt = ext;
    clearTimeout(removeTimer);
    removeTimer = setTimeout(() => { if (removingExt === ext) removingExt = ""; }, 300000);
    await App.RenameBlocked("", "//unescape:" + ext);
  }

  // Exclusions: global ignore patterns (globs excluded from every sync pair, on
  // top of built-in defaults) and allowed filenames (normally-blocked names like
  // .htaccess the user wants to sync anyway).
  let ignore = $state<string[]>([]);
  let newPat = $state("");
  (async () => { ignore = (await App.GetIgnorePatterns()) ?? []; })();
  // Built-in default excludes — kept in sync with internal/engine/ignore.go's
  // defaultIgnore; shown read-only so "sync everything" is honest about them.
  const builtinIgnores = ["*~","~$*",".~lock.*","*.tmp","*.part",".DS_Store","Thumbs.db","desktop.ini",".sync_*.db",".sync_*.db-shm",".sync_*.db-wal","._sync_*.db",".owncloudsync.log*",".nextcloudsync.log*","*.~syncpart"];
  function addIgnore(p: string) { p = p.trim(); if (!p || ignore.includes(p)) return; ignore = [...ignore, p]; App.SetIgnorePatterns(ignore); }
  function addPat() { addIgnore(newPat); newPat = ""; }
  function rmPat(p: string) { ignore = ignore.filter(x => x !== p); App.SetIgnorePatterns(ignore); }

  // Flyout appearance (icon size / panel width / density / visible sections).
  let appearance = $state<{ dockIconSize: string; panelWidth: string; density: string; sections: string[] }>(
    { dockIconSize: "medium", panelWidth: "standard", density: "comfortable", sections: ["search", "activity", "storage"] });
  // Copy into a plain object — the Wails binding returns a class instance, which
  // Svelte 5 won't deep-proxy, so in-place edits (setIconSize etc.) wouldn't react.
  (async () => {
    const a = await App.FlyoutAppearance();
    appearance = { dockIconSize: a.dockIconSize, panelWidth: a.panelWidth, density: a.density, sections: [...(a.sections ?? [])] };
  })();
  const allSections = ["search", "activity", "storage"];
  const sectionLabels: Record<string, string> = { search: "Search bar", activity: "Recent activity", storage: "Storage bar" };
  function saveAppearance() { App.SetFlyoutAppearance($state.snapshot(appearance)); }
  function setIconSize(v: string) { appearance.dockIconSize = v; saveAppearance(); }
  function setWidth(v: string) { appearance.panelWidth = v; saveAppearance(); }
  function setDensity(v: string) { appearance.density = v; saveAppearance(); }
  function toggleSection(k: string) {
    appearance.sections = appearance.sections.includes(k)
      ? appearance.sections.filter(s => s !== k)
      : [...appearance.sections, k];
    saveAppearance();
  }
  // Drag-to-reorder the visible sections. We reorder live as the pointer passes
  // over a sibling row, then persist once on drop/end.
  let dragKey = $state<string | null>(null);
  function onSecDragStart(k: string) { dragKey = k; }
  function onSecDragEnter(k: string) {
    if (dragKey === null || dragKey === k) return;
    const list = [...appearance.sections];
    const from = list.indexOf(dragKey), to = list.indexOf(k);
    if (from === -1 || to === -1) return;
    list.splice(to, 0, list.splice(from, 1)[0]);
    appearance.sections = list;
  }
  function onSecDragEnd() { if (dragKey !== null) { dragKey = null; saveAppearance(); } }

  let allowed = $state<string[]>([]);
  let newAllowed = $state("");
  (async () => { allowed = (await App.GetAllowedFilenames()) ?? []; })();
  function addAllowed() { const p = newAllowed.trim(); if (!p || allowed.includes(p)) return; allowed = [...allowed, p]; newAllowed = ""; App.SetAllowedFilenames(allowed); }
  function rmAllowed(p: string) { allowed = allowed.filter(x => x !== p); App.SetAllowedFilenames(allowed); }

  // Accounts. Every configured account syncs side by side and has its own page
  // (AccountPage), which acts on it by id. Background accounts emit "account"
  // on every status change; the list refreshes each time, and the 3 s poll
  // below keeps it current too. The selection follows the list: an account
  // that goes away (signed out) hands over to the shown one.
  async function loadAccounts() {
    accounts = (await App.ListAccounts()) ?? [];
    sel = sel === null ? initialSelection(accounts, pendingTab) : nextSelection(accounts, sel);
  }
  loadAccounts();
  Events.On("account", loadAccounts);

  let autoSupported = $state(false), auto = $state(false);
  (async () => { autoSupported = await App.AutostartSupported(); auto = await App.AutostartEnabled(); })();
  async function toggleAuto() { auto = !auto; const err = await App.SetAutostart(auto); if (err) { auto = !auto; tell({ title: "Couldn't change starting when you log in", message: err }); } }

  let shellSupported = $state(false), shellOn = $state(false);
  (async () => { shellSupported = await App.ShellMenuSupported(); shellOn = await App.ShellMenuEnabled(); })();
  async function toggleShell() { shellOn = !shellOn; const err = await App.SetShellMenu(shellOn); if (err) { shellOn = !shellOn; tell({ title: "Couldn't change the Explorer menu", message: err }); } }

  let navSupported = $state(false), navOn = $state(false), navBusy = $state(false);
  (async () => { navSupported = await App.SidebarSupported(); navOn = await App.SidebarEnabled(); })();
  // SetSidebar returns once Explorer has been updated (a second or two on a
  // packaged build), so the box is held until then: a second click mid-way
  // used to queue a second change on top of the first.
  async function toggleNav() { if (navBusy) return; navBusy = true; navOn = !navOn; const err = await App.SetSidebar(navOn); navBusy = false; if (err) { navOn = !navOn; tell({ title: "Couldn't change the Explorer sidebar", message: err }); } }

  let notifyOn = $state(true);
  (async () => { notifyOn = await App.NotificationsEnabled(); })();
  function toggleNotify() { notifyOn = !notifyOn; App.SetNotifications(notifyOn); }

  let dockOn = $state(true);
  (async () => { dockOn = await App.ShowAppDock(); })();
  function toggleDock() { dockOn = !dockOn; App.SetShowAppDock(dockOn); }

  let dockSide = $state("right");
  (async () => { dockSide = await App.AppDockSide(); })();
  function saveDockSide() { App.SetAppDockSide(dockSide); }

  let conflictPolicy = $state("ask");
  (async () => { conflictPolicy = await App.GetConflictPolicy(); })();
  function saveConflictPolicy() { App.SetConflictPolicy(conflictPolicy); }

  let theme = $state("system");
  (async () => { theme = await App.GetTheme(); })();
  function saveTheme() { App.SetTheme(theme); }

  let onDemandSupported = $state(false);
  let syncMode = $state("live");
  let syncModeBusy = $state(false);
  // Bumped when a mode switch finishes: the account page reloads its folders
  // on it as well as on syncMode, since the folder list can change at the end
  // of a switch without the mode changing again.
  let folderRefresh = $state(0);

  (async () => { onDemandSupported = await App.OnDemandSupported(); syncMode = await App.GetSyncMode(); })();
  // Switching to virtual files over a folder that already holds files needs the
  // adopt pass, so the files already on disk stay put instead of being left
  // outside the placeholder system. Scan first, show the cost, then confirm.
  //
  // The scan crawls the whole account and can run for MINUTES on a big one, so
  // it gets a blocking overlay the moment it starts — with a moving folder
  // count (via the diag poll) so a long crawl doesn't read as a hang — and a
  // Cancel that aborts the crawl server-side. Nothing changes until confirm.
  let adopt: any = $state(null);
  let scanning = $state(false);
  let scanCancelled = false;
  // Leaving VFS gets the mirror treatment: instant local scan -> forecast
  // dialog (files converted back in place, online-only re-download cost) ->
  // background revert with overlay. revert holds the forecast; null = closed.
  let revert: any = $state(null);
  async function saveSyncMode() {
    if (syncMode === "live") {
      // The overlay opens the INSTANT the mode is picked — even a fast scan
      // must not leave a dead gap between the click and the next UI.
      scanning = true; scanCancelled = false;
      const raw = await App.SetSyncMode("live-scan");
      scanning = false;
      let sum: any = null;
      try { sum = JSON.parse(raw); } catch { sum = null; }
      if (sum?.error) { tell({ title: "File availability", message: sum.error }); syncMode = await App.GetSyncMode(); return; }
      if (sum && (sum.hydrated || sum.dehydrated || sum.ignored || sum.unsynced)) { revert = sum; return; }
      // Nothing mounted/nothing to revert — plain switch below.
    }
    if (syncMode === "ondemand") {
      scanning = true; scanCancelled = false;
      const raw = await App.SetSyncMode("ondemand-scan");
      scanning = false;
      if (scanCancelled) { scanCancelled = false; return; } // user aborted; control already reset
      let sum: any = null;
      try { sum = JSON.parse(raw); } catch { sum = null; }
      if (sum?.error) {
        tell({ title: "File availability", message: sum.error });
        syncMode = await App.GetSyncMode(); // the mode didn't change; put the control back
        return;
      }
      // Nothing already on disk to reason about — switch straight over.
      if (sum && (sum.keep || sum.conflict || sum.upload || sum.replace)) { adopt = sum; return; }
    }
    syncModeBusy = true;
    const err = await App.SetSyncMode(syncMode);
    syncModeBusy = false;
    if (err) tell({ title: "File availability", message: err });
    await refreshModeAndFolders(); // the plain switch restores/clears pairs synchronously
  }
  async function cancelScan() {
    scanCancelled = true;
    scanning = false;
    await App.SetSyncMode("ondemand-cancel"); // aborts the crawl in Go
    syncMode = await App.GetSyncMode(); // nothing changed; put the control back
  }
  // Every completed mode switch changes BOTH the mode and the folder list
  // (leaving virtual files restores the remembered pairs at the very END of
  // the switch; entering it clears them). Refresh the two together, or the
  // Folders section shows the pre-switch list until something else reloads
  // it — "sync folders restored" with an empty list (bit Adam, 2026-08-20).
  // The account page reloads its folders when syncMode or folderRefresh moves.
  async function refreshModeAndFolders() {
    syncMode = await App.GetSyncMode();
    folderRefresh++;
  }
  // The revert finishes at an unknown later time — sub-second on a small
  // account (too fast for the 3s diag poll to ever observe, so a
  // counter-transition watcher can never fire) up to an hour on a big one.
  // Poll the REAL mode until it settles on live; that works at every speed.
  let revertPoll: ReturnType<typeof setInterval> | null = null;
  function stopRevertPoll() { if (revertPoll) { clearInterval(revertPoll); revertPoll = null; } }
  async function confirmRevert() {
    revert = null;
    syncModeBusy = true;
    const err = await App.SetSyncMode("live-revert"); // returns immediately; overlay takes over
    syncModeBusy = false;
    if (err) { tell({ title: "File availability", message: err }); }
    stopRevertPoll();
    // The mode is persisted at the START of the switch but the remembered
    // pairs are restored at its END (after the engine restart), so the first
    // "live" answer can predate the restored folder list. Refresh on three
    // consecutive live ticks rather than one, so the list settles too.
    let liveTicks = 0;
    revertPoll = setInterval(async () => {
      const m = await App.GetSyncMode();
      if (m !== "live") { liveTicks = 0; return; }
      syncMode = m;
      folderRefresh++;
      if (++liveTicks >= 3) stopRevertPoll();
    }, 1000);
  }
  async function cancelRevertDialog() {
    revert = null;
    syncMode = await App.GetSyncMode(); // nothing changed; put the control back
  }
  // The revert runs for many minutes after the "live-revert" call returns; the
  // mode only flips at the END. Refresh the selector when the overlay closes,
  // or it shows "Virtual" after a completed switch (bit Adam on the first run).
  let revertWasRunning = false;
  $effect(() => {
    const t = (diag as any)?.revertTotal || 0;
    if (t > 0) { revertWasRunning = true; }
    else if (revertWasRunning) {
      revertWasRunning = false;
      refreshModeAndFolders();
    }
  });
  async function cancelRevertRun() {
    stopRevertPoll();
    await App.SetSyncMode("revert-cancel"); // remounts VFS; already-reverted files stay plain
    await refreshModeAndFolders();
  }
  // Cancelling the background conversion = switching back to live: the unmount
  // cancels the conversion cleanly (converted files are correct placeholders,
  // the rest are untouched plain files).
  async function cancelConvert() {
    syncModeBusy = true;
    const err = await App.SetSyncMode("live");
    syncModeBusy = false;
    if (err) tell({ title: "File availability", message: err });
    await refreshModeAndFolders();
  }
  async function confirmAdopt() {
    adopt = null;
    syncModeBusy = true;
    const err = await App.SetSyncMode("ondemand-adopt");
    syncModeBusy = false;
    if (err) tell({ title: "File availability", message: err });
    await refreshModeAndFolders();
  }
  // "Start fresh": switch WITHOUT keeping the local files. A second, explicit
  // confirmation with exact counts — this deletes data, and in the on-demand
  // direction local-only files are gone for good.
  let fresh = $state<null | { target: "ondemand" | "live"; dir: string; total: number; localOnly: number; changed: number }>(null);
  let freshBusy = $state(false);
  function askFreshFromAdopt() {
    if (!adopt) return;
    fresh = {
      target: "ondemand", dir: adopt.dir,
      total: (adopt.keep || 0) + (adopt.replace || 0) + (adopt.upload || 0) + (adopt.conflict || 0),
      localOnly: adopt.upload || 0, changed: adopt.conflict || 0,
    };
    adopt = null;
  }
  function askFreshFromRevert() {
    if (!revert) return;
    fresh = {
      target: "live", dir: revert.dir,
      total: (revert.hydrated || 0) + (revert.dehydrated || 0) + (revert.ignored || 0) + (revert.unsynced || 0),
      localOnly: 0, changed: 0,
    };
    revert = null;
  }
  async function cancelFresh() {
    const t = fresh?.target;
    fresh = null;
    if (t === "ondemand") await App.SetSyncMode("ondemand-cancel"); // drop the held adopt plan
    syncMode = await App.GetSyncMode(); // nothing changed; put the control back
  }
  // One-UAC badge registration for installs that never ran the elevated
  // Setup step (Store, in-app updates). Shown in live mode while the current
  // badge generation is missing.
  let badgesBusy = $state(false);
  let badgesMsg = $state("");
  async function enableBadges() {
    badgesBusy = true; badgesMsg = "";
    const err = await App.SetSyncMode("badges-enable");
    badgesBusy = false;
    badgesMsg = err || "Badges enabled — restart Explorer if they don't show yet.";
    diag = await App.Diagnostics();
  }
  async function confirmFresh() {
    if (!fresh) return;
    const cmd = fresh.target === "ondemand" ? "ondemand-fresh" : "live-fresh";
    fresh = null;
    freshBusy = true;
    const err = await App.SetSyncMode(cmd);
    freshBusy = false;
    if (err) tell({ title: "File availability", message: err });
    await refreshModeAndFolders();
  }
  async function cancelAdopt() {
    adopt = null;
    await App.SetSyncMode("ondemand-cancel"); // drop the held plan; nothing was changed
    syncMode = await App.GetSyncMode(); // put the control back to the real mode
  }
  function mb(bytes: number): string {
    if (!bytes) return "0 MB";
    if (bytes < 1024 * 1024) return Math.max(1, Math.round(bytes / 1024)) + " KB";
    if (bytes < 1024 * 1024 * 1024) return Math.round(bytes / (1024 * 1024)) + " MB";
    return (bytes / (1024 * 1024 * 1024)).toFixed(1) + " GB";
  }

  let logVerbose = $state(false);
  (async () => { logVerbose = await App.Verbose(); })();
  function toggleVerbose() { logVerbose = !logVerbose; App.SetVerbose(logVerbose); }

  // Connection health, refreshed while the Settings panel is open.
  let diag = $state(null);
  $effect(() => {
    const load = async () => { diag = await App.Diagnostics(); await loadAccounts(); };
    load();
    const id = setInterval(load, 3000);
    return () => clearInterval(id);
  });

  // Releasing rides SetSyncMode's sentinel dispatch (see its comment in
  // service.go) so no new Wails binding is needed; it returns "" or a message.
  let releasingLocks = $state(false);
  async function toggleFileLockout(on: boolean) {
    const err = await App.SetSyncMode(on ? "lockout-enable" : "lockout-disable");
    if (err) tell({ title: "Couldn't change the file lockout", message: err });
    diag = await App.Diagnostics();
  }
  async function toggleFileLocking(on: boolean) {
    const err = await App.SetSyncMode(on ? "lock-enable" : "lock-disable");
    if (err) tell({ title: "Couldn't change file locking", message: err });
    diag = await App.Diagnostics();
  }
  async function releaseLocks() {
    releasingLocks = true;
    const err = await App.SetSyncMode("lock-release-all");
    releasingLocks = false;
    if (err) tell({ title: "Couldn't release the locks", message: err });
    diag = await App.Diagnostics();
  }

  let lowMem = $state(true);
  (async () => { lowMem = await App.LowMemoryMode(); })();
  function toggleLowMem() { lowMem = !lowMem; App.SetLowMemoryMode(lowMem); }

  function minToTime(m: number) { return `${String(Math.floor(m / 60) % 24).padStart(2, "0")}:${String(m % 60).padStart(2, "0")}`; }
  function timeToMin(t: string) { const [h, mm] = t.split(":").map(Number); return (h || 0) * 60 + (mm || 0); }
  let schedEnabled = $state(false);
  let schedFrom = $state("22:00");
  let schedTo = $state("07:00");
  (async () => {
    const s = await App.GetPauseSchedule();
    schedEnabled = s.enabled;
    if (s.fromMin || s.toMin) { schedFrom = minToTime(s.fromMin); schedTo = minToTime(s.toMin); }
  })();
  function saveSched() { App.SetPauseSchedule(schedEnabled, timeToMin(schedFrom), timeToMin(schedTo)); }

  let version = $state("");
  (async () => { version = await App.Version(); })();

  // Business licence: paste a key to unlock business-tier features (central
  // policy, white-label …). Everything free today stays free without one.
  let lic = $state<{ hasLicence: boolean; licensed: boolean; customer: string; tier: string; seats: number; expires: string; expired: boolean; err: string }>(
    { hasLicence: false, licensed: false, customer: "", tier: "", seats: 0, expires: "", expired: false, err: "" });
  let licInput = $state("");
  let licBusy = $state(false);
  let licMsg = $state("");
  async function loadLicence() { lic = await App.LicenceInfo(); }
  loadLicence();
  async function activateLicence() {
    licBusy = true; licMsg = "";
    try {
      const err = await App.ActivateLicence(licInput);
      if (err) { licMsg = "Couldn't activate: " + err; }
      else { licMsg = "Licence activated."; licInput = ""; await loadLicence(); }
    } catch (e) { licMsg = "Couldn't activate: " + e; }
    finally { licBusy = false; }
  }
  async function removeLicence() {
    licBusy = true; licMsg = "";
    try { await App.RemoveLicence(); await loadLicence(); }
    finally { licBusy = false; }
  }

  // Report a problem: bundles logs + diagnostics into a zip (revealed in
  // Explorer) and opens the GitHub issue page — nothing is sent automatically.
  let reportBusy = $state(false);
  let reportMsg = $state("");
  async function reportProblem() {
    reportBusy = true; reportMsg = "Gathering logs…";
    try {
      const err = await App.ReportProblem();
      reportMsg = err ? "Couldn't create the report: " + err
        : "Report zip saved to Downloads — attach it to the GitHub issue that just opened.";
    } catch (e) {
      reportMsg = "Couldn't create the report: " + e;
    } finally {
      reportBusy = false;
    }
  }

  // Updates (GitHub releases check + in-app App Installer apply).
  let updateMsg = $state("");
  let updateURL = $state("");        // release page (fallback for loose builds)
  let updateNotes = $state("");      // release notes ("what's in this update")
  let updateAvail = $state(false);
  let updateBusy = $state(false);
  let canApply = $state(false);      // true only in the packaged install
  (async () => { canApply = await App.CanApplyUpdate(); })();
  // Pre-release channel. Only meaningful where we can self-update, so the UI
  // below is gated on canApply — which is false on the Store build (the Store
  // updates the app itself) and on loose dev builds.
  let beta = $state(false);
  let betaConfirm = $state(false);
  (async () => { beta = await App.BetaUpdates(); })();
  async function toggleBeta(e: Event) {
    if (!beta) {
      // Turning betas ON needs eyes-open consent — put the checkbox back and
      // show the warning; nothing changes unless it's confirmed.
      (e.currentTarget as HTMLInputElement).checked = false;
      betaConfirm = true;
      return;
    }
    beta = false;
    await App.SetBetaUpdates(false);
    // Whatever the last check wrote described the channel we just left —
    // clear it rather than leave a stale verdict on screen.
    updateMsg = ""; updateURL = ""; updateNotes = ""; updateAvail = false;
  }
  async function confirmBeta() {
    betaConfirm = false;
    beta = true;
    await App.SetBetaUpdates(true);
    updateMsg = ""; updateURL = ""; updateNotes = ""; updateAvail = false;
  }
  async function checkUpdate() {
    updateBusy = true; updateMsg = "Checking…"; updateURL = ""; updateNotes = ""; updateAvail = false;
    const u = await App.CheckForUpdate();
    updateBusy = false;
    if (u.err) { updateMsg = "Couldn't check: " + u.err; return; }
    // Show the latest release's notes either way: as "what you'd get" when an
    // update is available, or "what's new in this version" when up to date.
    // Not when "ahead": u.notes there belongs to the older stable release,
    // and showing it under "you're on a newer build" reads as if that older
    // release is what the running build contains.
    updateNotes = u.ahead ? "" : u.notes;
    if (u.available) { updateMsg = "Update available: " + u.latest; updateURL = u.url; updateAvail = true; }
    else if (u.ahead) { updateMsg = "You're on a newer build than the current release" + (u.latest ? " (" + u.latest + ")" : ""); }
    else { updateMsg = "You're up to date" + (u.latest ? " (" + u.latest + ")" : ""); }
  }
  async function applyUpdate() {
    updateBusy = true; updateMsg = `Updating… ${brandName} will restart.`;
    const err = await App.ApplyUpdate();
    if (err) { updateBusy = false; updateMsg = "Update failed: " + err; }
    // On success the app is shut down by the installer and relaunched.
  }
</script>

<div class="win">
  <nav class="side" aria-label="Settings">
    <div class="glabel">Accounts</div>
    {#each accounts as ac (ac.id)}
      {@const warn = needsAttention(ac)}
      <button class="srow acctrow" class:sel={selAcctId === ac.id} aria-current={selAcctId === ac.id ? "page" : undefined}
              title={`${ac.user} on ${ac.server}${ac.status ? " — " + ac.status : ""}`}
              onclick={() => (sel = { kind: "account", id: ac.id })}>
        <span class="av" aria-hidden="true">{(ac.user || "?").slice(0, 1).toUpperCase()}<span class="dot" class:warn></span></span>
        <span class="who">
          <span class="user">{ac.user}</span>
          <span class="host">{hostname(ac.server)}</span>
        </span>
        {#if warn}<span class="sr">(needs attention)</span>{/if}
      </button>
    {/each}
    <button class="srow addacct" onclick={() => App.AddAccount()}>＋ Add account</button>

    <div class="glabel">App</div>
    {#each APP_PAGES as ap (ap.page)}
      <button class="srow" class:sel={page === ap.page} aria-current={page === ap.page ? "page" : undefined}
              onclick={() => (sel = { kind: "app", page: ap.page })}>
        <span class="ico"><Icon name={ap.icon} size={15} /></span>{ap.label}
      </button>
    {/each}
  </nav>

  <div class="body">
    {#if policy.managed}
      <div class="mgbanner">🔒 Some settings are managed by your organisation.</div>
    {/if}
    {#if selAcct}
      {#key selAcctId}
        <AccountPage acct={selAcct} {accounts} {policy} {brandName} {syncMode} {folderRefresh} />
      {/key}
    {:else if sel?.kind === "signedout"}
      <!-- A button rather than pointing at "the sign-in window": once that
           window was closed, nothing here led back to it (GitHub #20). -->
      <p class="empty">Signed out. <button class="link" onclick={() => App.AddAccount()}>Sign in</button></p>

    {:else if page === "sync"}
      {#if onDemandSupported}
        <h3>File availability</h3>
        <div class="field">
          <p class="fhint">Live keeps every file on your disk. Virtual file system shows your whole account as online-only placeholders in its sync folder and downloads each file when you open it. This applies to every account and persists across restarts.{#if brandHelp} <button class="link" onclick={() => App.OpenURL(helpPage("file-modes"))}>Which should I pick?</button>{/if}</p>
          <select aria-label="File availability" bind:value={syncMode} onchange={saveSyncMode} disabled={syncModeBusy || policy.lockSyncMode}>
            <option value="live">Live file system</option>
            <option value="ondemand">Virtual file system</option>
          </select>
          {#if policy.lockSyncMode}<p class="managed">🔒 Set by your organisation.</p>{/if}
        </div>
      {/if}
      {#if revert}
        <div class="modalback">
          <div class="addpanel modalbox">
            <div class="addhead">Switch back to normal files?</div>
            <p class="hint">Your files in <b>{revert.dir}</b> go back to being ordinary files, in place.</p>
            <ul class="hint">
              {#if revert.hydrated}<li><b>{revert.hydrated.toLocaleString()}</b> {revert.hydrated === 1 ? "is" : "are"} on this PC — converted back instantly, nothing transferred.</li>{/if}
              {#if revert.dehydrated}<li><b>{revert.dehydrated.toLocaleString()}</b> {revert.dehydrated === 1 ? "is" : "are"} online-only. {revert.dehydrated === 1 ? "It'll" : "They'll"} download through normal syncing after the switch ({mb(revert.downloadBytes)}).</li>{/if}
              {#if revert.ignored}<li><b>{revert.ignored.toLocaleString()}</b> {revert.ignored === 1 ? "is" : "are"} online-only in folders that normal syncing skips (your exclusions, such as node_modules). {revert.ignored === 1 ? "It stays" : "They stay"} on the server and won't download.</li>{/if}
              {#if revert.unsynced}<li><b>{revert.unsynced.toLocaleString()}</b> {revert.unsynced === 1 ? "is" : "are"} online-only and no sync folder covers {revert.unsynced === 1 ? "it, so it" : "them, so they"} won't download. {revert.unsynced === 1 ? "It stays" : "They stay"} on the server.</li>{/if}
            </ul>
            {#if revert.folders}
              <p class="hint">Your sync folders are restored automatically.</p>
            {:else}
              <p class="hint">No sync folder is set up, so nothing will sync after the switch until you add one (pick an account on the left, then Add folder).</p>
            {/if}
            <div class="addbtns">
              <button class="primary" onclick={confirmRevert}>Switch to normal files</button>
              <button onclick={askFreshFromRevert}>Start fresh…</button>
              <button onclick={cancelRevertDialog}>Cancel</button>
            </div>
          </div>
        </div>
      {/if}
      {#if (diag as any)?.revertTotal > 0}
        <div class="modalback">
          <div class="addpanel modalbox">
            <div class="addhead">Converting your files back…</div>
            <p class="hint">Leave {brandName} running until this finishes. Your files stay usable throughout.</p>
            <p class="hint"><b>{(diag as any).revertDone.toLocaleString()}</b> of <b>{(diag as any).revertTotal.toLocaleString()}</b> files</p>
            <div class="addbtns">
              <button onclick={cancelRevertRun}>Cancel and stay on virtual files</button>
            </div>
          </div>
        </div>
      {/if}
      {#if (diag as any)?.adoptConvertTotal > 0}
        <!-- Background conversion in progress (driven purely by the diag poll,
             so it shows regardless of which call started the switch). -->
        <div class="modalback">
          <div class="addpanel modalbox">
            <div class="addhead">Converting your files…</div>
            <p class="hint">{brandName} is adopting the files already in your folder as virtual files. You can keep using your computer — just leave {brandName} running until this finishes.</p>
            <p class="hint"><b>{(diag as any).adoptConvertDone.toLocaleString()}</b> of <b>{(diag as any).adoptConvertTotal.toLocaleString()}</b> files</p>
            <div class="addbtns">
              <button onclick={cancelConvert} disabled={syncModeBusy}>Cancel and switch back</button>
            </div>
          </div>
        </div>
      {/if}
      {#if scanning}
        <!-- Blocks the window for the whole crawl: without this, a multi-minute
             scan behind a small hint line reads as "the switch did nothing". -->
        <div class="modalback">
          <div class="addpanel modalbox">
            <div class="addhead">Checking your files…</div>
            <p class="hint">{brandName} is comparing everything in your folder with your server before switching. On a large account this can take several minutes. Nothing is changed until you confirm.</p>
            <p class="hint"><b>{(diag as any)?.adoptScanDirs || 0}</b> folders checked so far</p>
            <div class="addbtns">
              <button onclick={cancelScan}>Cancel</button>
            </div>
          </div>
        </div>
      {/if}
      {#if adopt}
        <!-- Confirm before touching a folder that already holds files: the scan
             changed nothing, so cancelling here is free. -->
        <div class="modalback">
          <div class="addpanel modalbox">
            <div class="addhead">Keep the files already in this folder?</div>
            <p class="hint">There are already files in <b>{adopt.dir}</b>. {brandName} can keep them where they are instead of downloading everything again.</p>
            <ul class="hint">
              {#if adopt.keep}<li><b>{adopt.keep}</b> already {adopt.keep === 1 ? "matches" : "match"} your server — kept on this PC, nothing re-downloaded.</li>{/if}
              {#if adopt.replace}<li><b>{adopt.replace}</b> {adopt.replace === 1 ? "is an online-only leftover" : "are online-only leftovers"} from another client — replaced with {adopt.replace === 1 ? `a ${brandName} placeholder` : `${brandName} placeholders`}.</li>{/if}
              {#if adopt.upload}<li><b>{adopt.upload}</b> {adopt.upload === 1 ? "isn’t" : "aren’t"} on your server yet — uploaded.</li>{/if}
              {#if adopt.conflict}<li><b>{adopt.conflict}</b> {adopt.conflict === 1 ? "differs" : "differ"} from your server — both versions kept (your copy is uploaded as a “conflicted copy”).</li>{/if}
            </ul>
            {#if adopt.uploadBytes}<p class="hint">Upload size: {mb(adopt.uploadBytes)}</p>{/if}
            <div class="addbtns">
              <button class="primary" onclick={confirmAdopt}>Keep my files</button>
              <button onclick={askFreshFromAdopt}>Start fresh…</button>
              <button onclick={cancelAdopt}>Cancel</button>
            </div>
          </div>
        </div>
      {/if}
      {#if fresh}
        <div class="modalback">
          <div class="addpanel modalbox">
            <div class="addhead">{fresh.target === "ondemand" ? "Delete the local copies?" : "Clear the folder and re-download?"}</div>
            {#if fresh.target === "ondemand"}
              <p class="hint">Everything in <b>{fresh.dir}</b> ({fresh.total.toLocaleString()} files) is removed from this PC and becomes online-only. Your synced-folder setup is cleared too. Your files on the server are untouched and download when you open them.</p>
              {#if fresh.localOnly}<p class="hint danger"><b>{fresh.localOnly.toLocaleString()}</b> files aren’t on your server yet — deleting them here loses them <b>forever</b>.</p>{/if}
              {#if fresh.changed}<p class="hint danger"><b>{fresh.changed.toLocaleString()}</b> files have local changes that differ from the server — those changes are discarded.</p>{/if}
            {:else}
              <p class="hint">Everything in <b>{fresh.dir}</b> is removed from this PC and your synced-folder setup is cleared — you pick what to sync again afterwards. Your files on the server are untouched.</p>
              <p class="hint danger">Anything in this folder that never reached the server is lost <b>forever</b>.</p>
            {/if}
            <div class="addbtns">
              <button class="danger" onclick={confirmFresh}>Delete and start fresh</button>
              <button class="primary" onclick={cancelFresh}>Go back</button>
            </div>
          </div>
        </div>
      {/if}
      {#if freshBusy}
        <div class="modalback">
          <div class="addpanel modalbox">
            <div class="addhead">Starting fresh…</div>
            <p class="hint">Clearing the folder and switching over. This can take a while for large folders — leave {brandName} running.</p>
          </div>
        </div>
      {/if}
      {#if syncMode === "live" && diag && !(diag as any).badgesRegistered}
        <p class="fhint">
          Folder badges (the little status icons on your files) aren’t set up on this install.
          <button class="link" onclick={enableBadges} disabled={badgesBusy}>{badgesBusy ? "Waiting for Windows…" : "Enable badges…"}</button>
          Windows asks for permission once.
          {#if badgesMsg}<br />{badgesMsg}{/if}
        </p>
      {/if}

      <h3>Bandwidth</h3>
      <div class="field"><label>Upload limit (KiB/s, 0 = unlimited)</label><input type="number" bind:value={up_} onchange={saveLimits} disabled={policy.lockBandwidth} /></div>
      <div class="field"><label>Download limit (KiB/s, 0 = unlimited)</label><input type="number" bind:value={down_} onchange={saveLimits} disabled={policy.lockBandwidth} /></div>
      {#if policy.lockBandwidth}<p class="managed">🔒 Set by your organisation.</p>{:else if savedFlash === "limits"}<span class="saved">Saved ✓</span>{/if}

      <h3>Quiet hours</h3>
      <label class="check"><input type="checkbox" bind:checked={schedEnabled} onchange={saveSched} /> Auto-pause syncing during quiet hours</label>
      {#if schedEnabled}
        <div class="schedrow">From <input type="time" bind:value={schedFrom} onchange={saveSched} /> to <input type="time" bind:value={schedTo} onchange={saveSched} /></div>
      {/if}

      <h3>Conflicts</h3>
      <div class="field">
        <label>When a file changes in both places</label>
        <p class="fhint">How {brandName} resolves sync conflicts by default.</p>
        <select bind:value={conflictPolicy} onchange={saveConflictPolicy}>
          <option value="ask">Ask me each time</option>
          <option value="keepboth">Keep both versions</option>
          <option value="newest">Keep the newest</option>
        </select>
      </div>

      <h3>Files in use</h3>
      {#if !(diag as any)?.lockingAvailable}
        <p class="fhint">Your server doesn't have the <b>Files Lock</b> app, so {brandName} can't tell when
          someone else has a file open. Ask your administrator to enable it.</p>
      {:else}
        <p class="fhint">{brandName} warns you when someone else has a file open, so you don't both
          edit it at once. The list lives in the status window, under <b>In use</b>.</p>
        <label class="check"><input type="checkbox" checked={(diag as any)?.fileLocking ?? false}
          onchange={(ev) => toggleFileLocking((ev.currentTarget as HTMLInputElement).checked)} />
          Lock files I have open, so other users are told</label>
        <p class="fhint">When you open a document, {brandName} marks it in use on the server so other
          people see it before they start editing. Off by default. Your server does not expire locks
          on its own, so {brandName} releases them itself — on close, on quit, and at the next start
          if something crashed.</p>
        <label class="check"><input type="checkbox" checked={(diag as any)?.fileLockout ?? false}
          onchange={(ev) => toggleFileLockout((ev.currentTarget as HTMLInputElement).checked)} />
          Stop me editing files someone else has open</label>
        <p class="fhint">Holds the file open so Word, Excel and the rest refuse to edit it and show
          their own “locked by …” message, instead of letting you make a second copy by accident.
          Off by default — it is the stricter option, and it applies to every program, not just Office.</p>
        {#if ((diag as any)?.heldLocks ?? []).length}
          <p class="fhint"><b>{brandName} is holding {((diag as any).heldLocks).length}
            lock{((diag as any).heldLocks).length === 1 ? "" : "s"}</b>, so other people are being told
            these files are in use. They're released automatically when you close the file and when
            {brandName} quits — this button is here for when something goes wrong, because your server
            never expires a lock on its own.</p>
          {#each ((diag as any)?.heldLocks ?? []) as l}
            <div class="irow"><span>{l.path}</span></div>
          {/each}
          <div class="addrow"><button onclick={releaseLocks} disabled={releasingLocks}>
            {releasingLocks ? "Releasing…" : "Release my locks"}</button></div>
        {/if}
      {/if}

      <h3>Performance</h3>
      <label class="check"><input type="checkbox" checked={lowMem} onchange={toggleLowMem} /> Low memory mode</label>
      <p class="fhint">Keeps {brandName}'s footprint small by reading sync state from disk instead of holding it all in memory. Recommended. Turn off only if you want the fastest possible background re-syncs on a very large account (uses noticeably more RAM); local edits are unaffected either way.</p>


    {:else if page === "exclusions"}
      <h3>Ignore patterns</h3>
      <p class="fhint">Glob patterns excluded from every sync folder (left untouched on both sides — not synced, not deleted). A name with no slash matches at any depth (<code>node_modules</code>, <code>*.log</code>); a name with a slash matches a full path (<code>build/out</code>). Applies on the next sync.</p>
      <div class="addrow"><input placeholder="*.log or node_modules/" bind:value={newPat} onkeydown={(e) => e.key === "Enter" && addPat()} /><button onclick={addPat}>Add</button></div>
      {#each ignore as p}<div class="irow"><span>{p}</span><button class="link" onclick={() => rmPat(p)}>Remove</button></div>{/each}
      {#if ignore.length === 0}<p class="empty">No custom patterns yet.</p>{/if}
      <p class="fhint sugg">💡 Big dev folders sync by default. Exclude them for faster syncing — and <b>.git</b> is best excluded if you work on a repo across machines (file-syncing a live <code>.git</code> can corrupt it):
        {#each ["node_modules", ".git", ".svn", ".hg"] as q}{#if !ignore.includes(q)}<button class="chipadd" onclick={() => addIgnore(q)}>+ {q}</button>{/if}{/each}
      </p>
      <details class="builtins">
        <summary>Always skipped (built-in)</summary>
        <p class="fhint">Temp/lock files, OS cruft, and other sync clients' journal files — never user data:</p>
        <div class="binfo">{#each builtinIgnores as b}<code>{b}</code>{/each}</div>
      </details>

      <h3>Allowed filenames</h3>
      <p class="fhint">{brandName} normally blocks web-server config files — <b>.htaccess</b>, <b>.htpasswd</b>, <b>.user.ini</b> — because most Nextcloud servers reject them. Add any you want to sync anyway; this only works if your server actually accepts them. Takes effect after restarting {brandName}.</p>
      <div class="addrow"><input placeholder=".htaccess" bind:value={newAllowed} onkeydown={(e) => e.key === "Enter" && addAllowed()} /><button onclick={addAllowed}>Add</button></div>
      {#each allowed as p}<div class="irow"><span>{p}</span><button class="link" onclick={() => rmAllowed(p)}>Remove</button></div>{/each}
      {#if allowed.length === 0}<p class="empty">None — the standard blocks apply.</p>{/if}

      <h3>Disguised file types</h3>
      <p class="fhint">File types your server refuses to store under their real names (like <b>.htaccess</b>) can sync anyway: {brandName} stores them on the server under a disguised name and shows them here with their real one. Only kicks in for names the server actually rejects — listing an ordinary type does nothing.</p>
      <div class="addrow"><input placeholder=".htaccess" bind:value={newEscape} onkeydown={(e) => e.key === "Enter" && addEscape()} /><button onclick={addEscape}>Add</button></div>
      {#each ((diag as any)?.escapeExtensions ?? []) as ext}
        <div class="irow"><span>{ext}</span><button class="link" disabled={removingExt !== ""} onclick={() => rmEscape(ext)}>{removingExt === ext ? "Removing…" : "Remove"}</button></div>
      {/each}
      {#if !((diag as any)?.escapeExtensions?.length)}<p class="empty">None — server-rejected names stay on this device only.</p>{/if}

    {:else if page === "appearance"}
      <h3>Panel</h3>
      <p class="fhint">Customise the tray flyout panel — changes apply live.</p>
      <div class="approw"><label>Width</label>
        <div class="seg">
          {#each [["compact","Compact"],["standard","Standard"],["wide","Wide"]] as opt}
            <button class:sel={appearance.panelWidth===opt[0]} onclick={() => setWidth(opt[0])}>{opt[1]}</button>
          {/each}
        </div>
      </div>
      <div class="approw"><label>Spacing</label>
        <div class="seg">
          {#each [["comfortable","Comfortable"],["compact","Compact"]] as opt}
            <button class:sel={appearance.density===opt[0]} onclick={() => setDensity(opt[0])}>{opt[1]}</button>
          {/each}
        </div>
      </div>
      <p class="fhint">Width is how wide the panel opens. Compact spacing makes the header and rows smaller and fits 8 files into Recent activity instead of 6.</p>

      <h3>App dock</h3>
      <label class="check"><input type="checkbox" checked={dockOn} onchange={toggleDock} /> Show the app dock (a strip of your pinned apps along an edge of the menu)</label>
      {#if dockOn}
        <div class="approw"><label>Icon size</label>
          <div class="seg">
            {#each [["small","Small"],["medium","Medium"],["large","Large"]] as opt}
              <button class:sel={appearance.dockIconSize===opt[0]} onclick={() => setIconSize(opt[0])}>{opt[1]}</button>
            {/each}
          </div>
        </div>
        <div class="approw"><label>Position</label>
          <select class="possel" bind:value={dockSide} onchange={saveDockSide}>
            <option value="right">Right edge</option>
            <option value="left">Left edge</option>
            <option value="bottom">Bottom strip</option>
          </select>
        </div>
      {/if}

      <h3>Sections</h3>
      <p class="fhint">Drag to reorder; hide the ones you don't need. Applies live.</p>
      <div class="seclist" role="list">
        {#each appearance.sections as k (k)}
          <div class="secrow" class:drag={dragKey === k} role="listitem" draggable="true"
               ondragstart={() => onSecDragStart(k)}
               ondragenter={() => onSecDragEnter(k)}
               ondragover={(e) => e.preventDefault()}
               ondragend={onSecDragEnd}>
            <span class="sechandle" title="Drag to reorder">⠿</span>
            <span class="secname">{sectionLabels[k]}</span>
            <button class="link" onclick={() => toggleSection(k)}>Hide</button>
          </div>
        {/each}
        {#if appearance.sections.length === 0}
          <p class="empty">All sections hidden — the flyout shows just the header and dock.</p>
        {/if}
      </div>
      {#each allSections.filter(k => !appearance.sections.includes(k)) as k}
        <button class="link secadd" onclick={() => toggleSection(k)}>+ Show {sectionLabels[k]}</button>
      {/each}

    {:else if page === "general"}
      <h3>Preferences</h3>
      <div class="field">
        <label>Appearance</label>
        <select bind:value={theme} onchange={saveTheme}>
          <option value="system">Match Nextcloud theme</option>
          <option value="light">Light</option>
          <option value="dark">Dark</option>
        </select>
      </div>
      <label class="check"><input type="checkbox" checked={notifyOn} onchange={toggleNotify} /> Show desktop notifications</label>

      <h3>System integration</h3>
      {#if autoSupported}
        <label class="check"><input type="checkbox" checked={auto} onchange={toggleAuto} /> Start {brandName} when I log in</label>
      {:else}
        <p class="empty">Startup options aren't available on this platform.</p>
      {/if}
      {#if shellSupported}
        <label class="check"><input type="checkbox" checked={shellOn} onchange={toggleShell} /> Add the {brandName} entries to the classic Explorer right-click menu</label>
        <p class="fhint">Windows 11's new right-click menu shows {brandName} either way; this covers the classic one (Show more options).</p>
      {/if}
      {#if navSupported}
        <label class="check"><input type="checkbox" checked={navOn} disabled={navBusy} onchange={toggleNav} /> Show {brandName} in the Explorer sidebar (points at your default sync location)</label>
      {/if}

      <div class="row"><h3>Troubleshooting</h3>{#if brandHelp}<button class="link" onclick={() => App.OpenURL(helpPage("troubleshooting"))}>Troubleshooting guide</button>{/if}</div>
      <label class="check"><input type="checkbox" checked={logVerbose} onchange={toggleVerbose} /> Verbose (debug) logging</label>
      <h4 class="dhealth">Connection health</h4>
      {#if diag}
        <div class="diaggrid">
          <span class="dk">Server</span>
          <span class="dv">{diag.serverURL || "—"}{#if diag.serverVersion} · Nextcloud {diag.serverVersion}{/if}</span>
          <span class="dk">Account</span>
          <span class="dv">{diag.account || "—"}</span>
          <span class="dk">Real-time push</span>
          <span class="dv">
            {#if !diag.pushAvailable}<span class="dmuted">not set up on this server, so {brandName} checks for changes every 30 seconds instead.
              It needs the Client Push app on the server (restart {brandName} once it is)</span> · <button class="link" onclick={() => App.OpenURL("https://github.com/nextcloud/notify_push#readme")}>Client Push setup</button>
            {:else if diag.pushConnected}<span class="dok">● connected</span>{#if diag.pushUptime} · up {diag.pushUptime}{/if}
            {:else}<span class="dbad">● reconnecting…</span>{/if}
          </span>
          <span class="dk">Last sync</span>
          <span class="dv">{diag.lastSync}{#if diag.lastStatus} · {diag.lastStatus}{/if}</span>
        </div>
      {/if}

      <div class="logrow">
        <button class="primary small" onclick={() => App.OpenLogs()}>View logs</button>
        <button class="small reportbtn" disabled={reportBusy} onclick={reportProblem}>Report a problem…</button>
        {#if reportMsg}<span class="fhint">{reportMsg}</span>{/if}
      </div>

      <h3>About</h3>
      <div class="updaterow">
        <span class="about">{brandName} {version}</span>
        <!-- The whole updater is self-update machinery, so it only exists where
             self-update can actually happen: gated on canApply, which is false
             on the Store build (the Store delivers updates itself and must not
             be second-guessed by a check it can't fulfil) and on loose dev runs. -->
        {#if canApply}
          <button class="link" onclick={checkUpdate} disabled={updateBusy}>Check for updates</button>
          {#if updateMsg}<span class="upmsg">{updateMsg}</span>{/if}
          {#if updateAvail}
            <button class="link" onclick={applyUpdate} disabled={updateBusy}>Update now</button>
          {/if}
        {/if}
      </div>
      {#if canApply}
        <label class="check"><input type="checkbox" checked={beta} onchange={toggleBeta} /> Get beta releases early</label>
        <p class="fhint">Beta builds reach you before everyone else and are less tested. Turning this off stops future betas — it won't move you back, so you'll stay on your current build until a normal release overtakes it.</p>
      {/if}
      {#if betaConfirm}
        <div class="modalback">
          <div class="addpanel modalbox">
            <div class="addhead">Get beta releases?</div>
            <p class="hint">Beta builds come straight from active development: they change often, are less tested, and may contain bugs — including ones that could affect the files being synced.</p>
            <p class="hint">They are provided as-is, without warranty of any kind, and the developer accepts no liability for any loss or damage arising from their use. Keep a backup of anything you can't afford to lose.</p>
            <div class="addbtns">
              <button class="primary" onclick={confirmBeta}>I understand — enable betas</button>
              <button onclick={() => (betaConfirm = false)}>Cancel</button>
            </div>
          </div>
        </div>
      {/if}
      {#if updateNotes}
        <div class="relnotes">{updateNotes}</div>
      {/if}
      <h3>Licence</h3>
      {#if lic.licensed}
        <p class="fhint">
          <b>Business licence</b> — {lic.customer}{lic.seats ? ` · ${lic.seats} seats` : ""}{lic.expires ? ` · expires ${lic.expires}` : " · perpetual"}.
        </p>
        <div class="logrow"><button class="small reportbtn" disabled={licBusy} onclick={removeLicence}>Remove licence</button></div>
      {:else}
        <p class="fhint">
          {brandName} is <b>free for personal use</b>. A business licence unlocks central deployment policy and white-label branding for organisations — <button class="link" onclick={() => App.OpenURL(brandWebsite + "/business.html")}>about business licensing</button>.
          {#if lic.hasLicence && lic.err}<br /><span style="color:#c0392b">Installed licence: {lic.err}.</span>{/if}
        </p>
        <div class="addrow">
          <input placeholder="NIMBO-LIC-1.…" bind:value={licInput} onkeydown={(e) => e.key === "Enter" && activateLicence()} />
          <button disabled={licBusy} onclick={activateLicence}>Activate</button>
        </div>
      {/if}
      {#if licMsg}<p class="fhint">{licMsg}</p>{/if}

      <p class="aboutco">
        © {new Date().getFullYear()} {brandCompany}
        · <button class="link" onclick={() => App.OpenURL(brandWebsite)}>{hostname(brandWebsite)}</button>
        {#if brandHelp}· <button class="link" onclick={() => App.OpenURL(brandHelp)}>Help</button>{/if}
        · <button class="link" onclick={() => App.OpenURL("mailto:" + brandSupport)}>{brandSupport}</button>
      </p>
    {/if}
  </div>
</div>

<style>
  .win { height: 100%; display: flex; background: var(--bg); color: var(--fg); }
  /* Left-hand list: accounts, then the app-wide pages. */
  .side { flex: 0 0 170px; width: 170px; overflow-y: auto; display: flex; flex-direction: column; gap: 2px;
          padding: 12px 8px; background: var(--panel); border-right: 1px solid var(--border); }
  .glabel { font-size: 10.5px; font-weight: 600; text-transform: uppercase; letter-spacing: .05em; color: var(--muted);
            padding: 0 8px; margin: 4px 0 4px; }
  .glabel ~ .glabel { margin-top: 14px; }
  .srow { display: flex; align-items: center; gap: 8px; width: 100%; padding: 6px 8px; border: none; border-radius: 6px;
          background: none; color: var(--fg); cursor: pointer; text-align: left; font-size: 13px; }
  .srow:hover { background: var(--hover); }
  .srow.sel { background: var(--tint); color: var(--accent); font-weight: 600; }
  .srow .ico { flex: 0 0 auto; width: 26px; display: flex; justify-content: center; color: var(--fg2); }
  .srow.sel .ico { color: var(--accent); }
  .av { position: relative; flex: 0 0 auto; width: 26px; height: 26px; border-radius: 50%; display: flex;
        align-items: center; justify-content: center; background: var(--accent); color: #fff; font-size: 12px; font-weight: 700; }
  .dot { position: absolute; right: -2px; bottom: -2px; width: 9px; height: 9px; border-radius: 50%;
         background: #2e9e44; border: 2px solid var(--panel); }
  .dot.warn { background: #d08b00; }
  .who { display: flex; flex-direction: column; min-width: 0; line-height: 1.25; }
  .who .user, .who .host { overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
  .who .host { font-size: 11px; font-weight: 400; color: var(--muted); }
  .addacct { color: var(--accent); font-size: 12.5px; }
  .sr { position: absolute; width: 1px; height: 1px; overflow: hidden; clip: rect(0 0 0 0); white-space: nowrap; }
  .body { flex: 1; overflow-y: auto; padding: 14px 16px; }
  .hint { color: var(--fg2); font-size: 12px; margin: 0 0 12px; }
  .approw { display: flex; align-items: center; justify-content: space-between; gap: 12px; margin: 12px 0; }
  .approw label { font-size: 13px; color: var(--fg); }
  .seg { display: inline-flex; border: 1px solid var(--border-2); border-radius: 8px; overflow: hidden; }
  .seg button { padding: 6px 12px; border: none; border-left: 1px solid var(--border-2); background: var(--bg); color: var(--fg); cursor: pointer; font-size: 12px; }
  .seg button:first-child { border-left: none; }
  .seg button.sel { background: var(--accent); color: #fff; }
  .seg button:not(.sel):hover { background: var(--hover); }
  .fhint { color: var(--muted); font-size: 12px; margin: 0 0 6px; }
  .fhint code { font-size: 11px; background: var(--bg2, rgba(127,127,127,.12)); padding: 0 3px; border-radius: 3px; }
  .flabel { display: block; font-size: 13px; font-weight: 600; margin: 12px 0 4px; }
  .dhealth { font-size: 13px; font-weight: 600; margin: 16px 0 6px; }
  .diaggrid { display: grid; grid-template-columns: auto 1fr; gap: 4px 12px; font-size: 12px; margin-bottom: 6px; }
  .dk { color: var(--muted); white-space: nowrap; }
  .dv { color: var(--fg); word-break: break-word; }
  .dok { color: #2e9e44; }
  .dbad { color: #d08b00; }
  /* Flyout section reorder list */
  .seclist { display: flex; flex-direction: column; gap: 6px; margin-bottom: 8px; }
  .secrow { display: flex; align-items: center; gap: 10px; padding: 8px 10px; border: 1px solid var(--border-2);
            border-radius: 8px; background: var(--panel); cursor: grab; }
  .secrow.drag { opacity: .5; border-color: var(--accent); background: var(--tint, var(--hover)); }
  .sechandle { flex: 0 0 auto; color: var(--muted); font-size: 14px; line-height: 1; cursor: grab; user-select: none; }
  .secname { flex: 1; min-width: 0; font-size: 13px; color: var(--fg); }
  .secadd { display: inline-block; margin: 0 10px 6px 0; }
  .dmuted { color: var(--muted); }
  .sugg .chipadd { margin-left: 6px; padding: 2px 8px; border: 1px solid var(--border-2); border-radius: 999px;
                   background: var(--bg); color: var(--accent); cursor: pointer; font-size: 12px; }
  .sugg .chipadd:hover { background: var(--tint); border-color: var(--accent); }
  .builtins { margin: 8px 0 4px; }
  .builtins summary { cursor: pointer; color: var(--fg2); font-size: 12.5px; }
  .builtins .binfo { display: flex; flex-wrap: wrap; gap: 5px; margin-top: 6px; }
  .builtins .binfo code { font-size: 11px; background: var(--panel-2); border: 1px solid var(--border-2); border-radius: 4px; padding: 1px 5px; }
  .addbtns button.danger { color: #c0392b; }
  h3 { font-size: 12px; text-transform: uppercase; color: var(--muted); margin: 18px 0 8px; }
  h3:first-of-type { margin-top: 0; }
  /* Section header: heading left, action (Add folder / Back / Done) right, with
     clear air before the content below it. */
  .row { display: flex; align-items: center; justify-content: space-between; gap: 10px; margin: 18px 0 12px; }
  .row:first-child { margin-top: 0; }
  .row h3 { margin: 0; }
  .saved { color: #2e9e5b; font-size: 12px; font-weight: 600; align-self: center; animation: savedfade 1.5s forwards; }
  .managed { color: var(--muted); font-size: 12px; margin: 4px 0 0; }
  .mgbanner { background: color-mix(in srgb, var(--accent) 12%, var(--panel)); border: 1px solid var(--accent);
              border-radius: 8px; padding: 8px 12px; font-size: 13px; color: var(--fg); margin-bottom: 14px; }
  @keyframes savedfade { 0%, 70% { opacity: 1; } 100% { opacity: 0; } }
  .relnotes { white-space: pre-line; font-size: 12px; color: var(--fg2); background: var(--panel);
              border: 1px solid var(--border); border-radius: 6px; padding: 8px 10px; margin-top: 6px;
              max-height: 140px; overflow-y: auto; }
  .reportbtn { padding: 6px 12px; border: 1px solid var(--border-2); border-radius: 5px;
               background: var(--bg); color: var(--fg); cursor: pointer; font-size: 12px; }
  .aboutco { color: var(--muted); font-size: 12px; margin: 10px 0 0; }
  .aboutco .link { font-size: 12px; }
  .danger { color: #c0392b; }
  .hint code { font-size: 11px; background: var(--bg2, rgba(127,127,127,.12)); padding: 0 3px; border-radius: 3px; }
  .modalback { position: fixed; inset: 0; background: rgba(0,0,0,.45); display: flex;
               align-items: center; justify-content: center; z-index: 50; }
  .modalbox { max-width: 360px; width: 90%; margin: 0; box-shadow: 0 10px 40px rgba(0,0,0,.4); }
  .addpanel { border: 1px solid var(--border); background: var(--panel); border-radius: 8px; padding: 12px; margin-bottom: 12px; }
  .addhead { font-size: 13px; margin-bottom: 8px; }
  .addbtns { display: flex; gap: 8px; margin-top: 10px; }
  .addbtns .primary { padding: 7px 14px; }
  .addbtns button:not(.primary) { padding: 7px 12px; border: 1px solid var(--border-2); border-radius: 6px; background: var(--bg); color: var(--fg); cursor: pointer; }
  .acts { display: flex; align-items: center; gap: 10px; flex: 0 0 auto; }
  .field { display: flex; flex-direction: column; gap: 4px; margin-bottom: 12px; max-width: 340px; }
  .field label { font-size: 12px; color: var(--fg2); }
  .field input { padding: 7px 8px; border: 1px solid var(--border-2); border-radius: 5px; background: var(--bg); color: var(--fg); }
  .field select { padding: 7px 8px; border: 1px solid var(--border-2); border-radius: 5px; font-size: 13px; max-width: 240px; background: var(--bg); color: var(--fg); }
  .addrow { display: flex; gap: 8px; margin-bottom: 12px; }
  .addrow input { flex: 1; padding: 7px 8px; border: 1px solid var(--border-2); border-radius: 5px; background: var(--bg); color: var(--fg); }
  .irow { display: flex; justify-content: space-between; padding: 6px 0; border-bottom: 1px solid var(--border-soft); font-size: 13px; }
  .check { display: flex; gap: 8px; align-items: center; font-size: 13px; margin-bottom: 12px; }
  .possel { padding: 7px 8px; border: 1px solid var(--border-2); border-radius: 5px; font-size: 13px; max-width: 200px; background: var(--bg); color: var(--fg); }
  button { font-size: 13px; }
  .primary { padding: 8px 14px; border: none; border-radius: 6px; background: var(--accent); color: #fff; cursor: pointer; }
  .primary:hover { background: var(--accent-dark); }
  .primary.small { padding: 6px 12px; font-size: 12.5px; }
  .link { background: none; border: none; color: var(--accent); cursor: pointer; }
  .link:disabled { opacity: .45; cursor: not-allowed; }
  .empty { color: var(--muted); font-size: 13px; }
  .logrow { margin: 4px 0 4px; }
  .schedrow { display: flex; align-items: center; gap: 8px; font-size: 13px; color: var(--fg2); margin: 0 0 12px 24px; }
  .schedrow input { padding: 5px 7px; border: 1px solid var(--border-2); border-radius: 5px; background: var(--bg); color: var(--fg); }
  .about { color: var(--muted); font-size: 12px; }
  .updaterow { margin-top: 20px; display: flex; align-items: center; gap: 10px; flex-wrap: wrap; }
  .upmsg { color: var(--fg2); font-size: 12px; }
</style>
