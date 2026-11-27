<script lang="ts">
  import Flyout from "./Flyout.svelte";
  import Status from "./Status.svelte";
  import Settings from "./Settings.svelte";
  import Login from "./Login.svelte";
  import Share from "./Share.svelte";
  import Versions from "./Versions.svelte";
  import Logs from "./Logs.svelte";
  import AppSplash from "./AppSplash.svelte";
  import Dialog from "./Dialog.svelte";
  import { Events } from "@wailsio/runtime";
  import { App } from "../bindings/github.com/otherworld/nimbo/cmd/nimbo-gui";
  import { darken, fitAccent, isHex, onAccent } from "./accent";

  // Each Wails window loads a different hash route.
  let route = $state(window.location.hash.replace("#", ""));
  window.addEventListener("hashchange", () => { route = window.location.hash.replace("#", ""); });

  // Light/dark theme. Modes: "light", "dark", or "system" (labelled "Match
  // Nextcloud theme"): use the user's explicit Nextcloud appearance when set,
  // otherwise — if their Nextcloud follows the system — track the OS via
  // prefers-color-scheme. Re-applied live when changed in settings.
  const sysDark = window.matchMedia("(prefers-color-scheme: dark)");
  let themeMode = "system";
  let ncAppearance = ""; // "dark" | "light" | "default" | "" (not yet loaded)
  function applyTheme() {
    let dark: boolean;
    if (themeMode === "dark") dark = true;
    else if (themeMode === "light") dark = false;
    else if (ncAppearance === "dark") dark = true;       // Nextcloud: dark
    else if (ncAppearance === "light") dark = false;     // Nextcloud: light
    else dark = sysDark.matches;                         // default/unknown -> OS
    if (dark) document.documentElement.setAttribute("data-theme", "dark");
    else document.documentElement.removeAttribute("data-theme");
    applyAccent(); // a custom accent is fitted per theme
  }
  sysDark.addEventListener("change", applyTheme);
  Events.On("theme", (e: any) => { themeMode = e.data || "system"; applyTheme(); });
  (async () => { themeMode = await App.GetTheme(); applyTheme(); })();
  // Fetch the Nextcloud appearance once the engine is up (retry briefly), then
  // re-apply so "Match Nextcloud theme" reflects the server's light/dark choice.
  let appearanceGen = 0; // a newer fetch supersedes any loop still retrying
  async function fetchNcAppearance() {
    const gen = ++appearanceGen;
    for (let i = 0; i < 12 && gen === appearanceGen; i++) {
      const a = await App.NextcloudAppearance();
      if (a) { ncAppearance = a; applyTheme(); return; }
      await new Promise(r => setTimeout(r, 1000));
    }
  }
  fetchNcAppearance();

  // Accent colour. By default the windows wear the product's own accent: the
  // stylesheet's valley green pair for the stock brand, or a partner brand's
  // accentHex. Settings > Appearance can switch to the Nextcloud server's theme
  // colour instead ("server"), which is what every window did before the
  // valley-green rebrand.
  const STOCK_ACCENT = "#5fa05b";
  let accentSource = "brand";
  let brandAccent = "";  // the build's accentHex ("" until App.Brand answers)
  let serverAccent = ""; // the Nextcloud theme colour ("" until known)
  let customAccent = ""; // the user's own colour, "#RRGGBB" ("" = none)
  function applyAccent() {
    const root = document.documentElement;
    let hex = "";
    if (accentSource === "custom" && isHex(customAccent)) {
      hex = fitAccent(customAccent, root.getAttribute("data-theme") === "dark");
    } else if (accentSource === "server" && isHex(serverAccent)) hex = serverAccent;
    else if (isHex(brandAccent) && brandAccent.toLowerCase() !== STOCK_ACCENT) hex = brandAccent;
    if (hex) {
      root.style.setProperty("--accent", hex);
      root.style.setProperty("--accent-dark", darken(hex, 0.88));
      root.style.setProperty("--on-accent", onAccent(hex));
    } else {
      // The stylesheet's own light/dark pair.
      root.style.removeProperty("--accent");
      root.style.removeProperty("--accent-dark");
      root.style.removeProperty("--on-accent");
    }
  }
  App.Brand()
    .then((b: any) => { brandAccent = b?.accentHex || ""; applyAccent(); })
    .catch(() => { /* stock accent */ });
  App.GetAccentSource()
    .then((s: string) => { accentSource = s || "brand"; applyAccent(); })
    .catch(() => { /* brand */ });
  Events.On("accentSource", (e: any) => { accentSource = e.data || "brand"; applyAccent(); });
  App.GetCustomAccent()
    .then((h: string) => { customAccent = h || ""; applyAccent(); })
    .catch(() => { /* none */ });
  Events.On("customAccent", (e: any) => { customAccent = e.data || ""; applyAccent(); });
  // The engine pushes the server theme when it starts (this window may load
  // long before that: on a fresh install the flyout exists while the user is
  // still signing in, and on an account switch the colour changes). Re-read
  // the appearance too.
  Events.On("accent", (e: any) => { serverAccent = e.data || ""; applyAccent(); fetchNcAppearance(); });
  (async () => {
    // The engine (which fetches the theme) may not be ready at window load, so
    // retry briefly until a colour is available.
    for (let i = 0; i < 12; i++) {
      try {
        const hex = await App.ThemeColor();
        if (isHex(hex)) {
          serverAccent = hex;
          applyAccent();
          return;
        }
      } catch { /* default accent */ }
      await new Promise(r => setTimeout(r, 1000));
    }
  })();
</script>

{#if route.startsWith("appsplash")}
  <AppSplash />
{:else if route === "status"}
  <Status />
{:else if route.startsWith("settings")}
  <Settings />
{:else if route === "login"}
  <Login />
{:else if route === "share"}
  <Share />
{:else if route === "versions"}
  <Versions />
{:else if route === "logs"}
  <Logs />
{:else}
  <Flyout />
{/if}

<!-- Titled replacements for alert/confirm/prompt, shared by every window. -->
<Dialog />
