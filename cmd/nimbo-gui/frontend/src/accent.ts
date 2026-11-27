// Accent colour maths shared by App.svelte (which applies the accent) and
// Settings (which previews a custom one). Pure, so `npm test` can check it.

/** The window backgrounds an accent has to read against (style.css --bg). */
export const LIGHT_BG = "#F4F0E8"; // chalk
export const DARK_BG = "#23262B"; // slate

/**
 * How far a custom accent must stand off the background. The windows use the
 * accent as link and label text as well as for buttons, so it is held to the
 * text bar (4.5:1), not just the 3:1 for shapes.
 */
export const ACCENT_MIN_CONTRAST = 4.5;

export const isHex = (h: string): boolean => /^#[0-9a-fA-F]{6}$/.test(h || "");

function rgb(hex: string): [number, number, number] {
  const m = hex.replace("#", "");
  return [0, 2, 4].map(i => parseInt(m.slice(i, i + 2), 16)) as [number, number, number];
}

function toHex(r: number, g: number, b: number): string {
  const c = (v: number) => Math.round(Math.min(255, Math.max(0, v))).toString(16).padStart(2, "0");
  return ("#" + c(r) + c(g) + c(b)).toUpperCase();
}

/** WCAG relative luminance of "#RRGGBB". */
export function luminance(hex: string): number {
  const lin = (v: number) => { v /= 255; return v <= 0.03928 ? v / 12.92 : Math.pow((v + 0.055) / 1.055, 2.4); };
  const [r, g, b] = rgb(hex).map(lin);
  return 0.2126 * r + 0.7152 * g + 0.0722 * b;
}

/** WCAG contrast ratio between two "#RRGGBB" colours (1 to 21). */
export function contrast(a: string, b: string): number {
  const la = luminance(a), lb = luminance(b);
  return (Math.max(la, lb) + 0.05) / (Math.min(la, lb) + 0.05);
}

/** Whichever of white and slate reads better on the accent. */
export function onAccent(hex: string): string {
  return contrast(hex, "#FFFFFF") >= contrast(hex, DARK_BG) ? "#FFFFFF" : DARK_BG;
}

/** The hover shade: a little darker. */
export function darken(hex: string, f: number): string {
  if (!isHex(hex)) return hex;
  const [r, g, b] = rgb(hex);
  return toHex(r * f, g * f, b * f);
}

function toHsl(hex: string): [number, number, number] {
  const [r, g, b] = rgb(hex).map(v => v / 255);
  const max = Math.max(r, g, b), min = Math.min(r, g, b);
  const l = (max + min) / 2;
  if (max === min) return [0, 0, l];
  const d = max - min;
  const s = l > 0.5 ? d / (2 - max - min) : d / (max + min);
  let h: number;
  if (max === r) h = ((g - b) / d + (g < b ? 6 : 0)) / 6;
  else if (max === g) h = ((b - r) / d + 2) / 6;
  else h = ((r - g) / d + 4) / 6;
  return [h, s, l];
}

function fromHsl(h: number, s: number, l: number): string {
  if (s === 0) return toHex(l * 255, l * 255, l * 255);
  const hue = (p: number, q: number, t: number) => {
    if (t < 0) t += 1;
    if (t > 1) t -= 1;
    if (t < 1 / 6) return p + (q - p) * 6 * t;
    if (t < 1 / 2) return q;
    if (t < 2 / 3) return p + (q - p) * (2 / 3 - t) * 6;
    return p;
  };
  const q = l < 0.5 ? l * (1 + s) : l + s - l * s;
  const p = 2 * l - q;
  return toHex(hue(p, q, h + 1 / 3) * 255, hue(p, q, h) * 255, hue(p, q, h - 1 / 3) * 255);
}

/**
 * Makes any colour usable as the accent on one theme: keeps its hue and
 * saturation and moves its lightness (darker on light, lighter on dark) just
 * far enough to reach ACCENT_MIN_CONTRAST against the window background. A
 * colour that already works comes back unchanged.
 */
export function fitAccent(hex: string, dark: boolean): string {
  const bg = dark ? DARK_BG : LIGHT_BG;
  const start = hex.toUpperCase();
  if (contrast(start, bg) >= ACCENT_MIN_CONTRAST) return start;
  const [h, s, l0] = toHsl(start);
  let l = l0;
  for (let i = 0; i < 100; i++) {
    l = dark ? Math.min(1, l + 0.01) : Math.max(0, l - 0.01);
    const c = fromHsl(h, s, l);
    if (contrast(c, bg) >= ACCENT_MIN_CONTRAST) return c;
  }
  return dark ? "#FFFFFF" : "#000000";
}
