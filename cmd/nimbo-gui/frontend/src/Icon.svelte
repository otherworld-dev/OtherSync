<script lang="ts" module>
  // Small line icons drawn in the text colour (currentColor), so they follow
  // light/dark mode and the accent wherever they sit. Add a shape to PATHS to
  // grow the set; emoji render differently on every Windows build and can't be
  // themed, which is why the folder picker uses these instead.
  const PATHS = {
    folder: "M2.5 4.5a1 1 0 0 1 1-1h3l1.5 1.5h4.5a1 1 0 0 1 1 1v6a1 1 0 0 1-1 1h-9a1 1 0 0 1-1-1z",
    chevron: "M6 3.5 10.5 8 6 12.5",
    // Settings window, App pages.
    sync: "M12.5 6A4.8 4.8 0 0 0 3.6 6.2M3.5 10a4.8 4.8 0 0 0 8.9-.2M12.7 2.8V6H9.5M3.3 13.2V10h3.2",
    exclude: "M8 2.5a5.5 5.5 0 1 0 0 11a5.5 5.5 0 1 0 0-11M4.1 11.9l7.8-7.8",
    appearance: "M8 2.5a5.5 5.5 0 1 0 0 11a5.5 5.5 0 1 0 0-11M8 2.5v11M8 5h3.8M8 8h5.5M8 11h3.8",
    general: "M2.5 5h11M2.5 11h11M6 3.2v3.6M10 9.2v3.6",
  } as const;
  export type IconName = keyof typeof PATHS;
</script>

<script lang="ts">
  let { name, size = 16 }: { name: IconName; size?: number } = $props();
</script>

<svg class="icon" width={size} height={size} viewBox="0 0 16 16" aria-hidden="true"
     fill="none" stroke="currentColor" stroke-width="1.3" stroke-linecap="round" stroke-linejoin="round">
  <path d={PATHS[name]} />
</svg>

<style>
  .icon { flex: 0 0 auto; display: block; }
</style>
