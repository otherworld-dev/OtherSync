// Custom accent fitting. Runs on Node's own test runner: `npm test`.
import { test } from "node:test";
import assert from "node:assert/strict";
import { ACCENT_MIN_CONTRAST, DARK_BG, LIGHT_BG, contrast, fitAccent, onAccent } from "../src/accent.ts";

const awkward = ["#FFFFFF", "#000000", "#FFFF00", "#F4F0E8", "#23262B", "#7F7F7F", "#00FFFF", "#120A2A", "#5FA05B", "#FF0000"];

test("any custom colour is fitted to read against both window backgrounds", () => {
  for (const c of awkward) {
    for (const dark of [false, true]) {
      const fitted = fitAccent(c, dark);
      const bg = dark ? DARK_BG : LIGHT_BG;
      assert.ok(contrast(fitted, bg) >= ACCENT_MIN_CONTRAST, `${c} dark=${dark} -> ${fitted} vs bg`);
    }
  }
});

test("button text on a fitted accent stays readable", () => {
  for (const c of awkward) {
    for (const dark of [false, true]) {
      const fitted = fitAccent(c, dark);
      assert.ok(contrast(fitted, onAccent(fitted)) >= 4.5, `${c} dark=${dark} -> ${fitted} text`);
    }
  }
});

test("a colour that already works is left exactly as picked", () => {
  assert.equal(fitAccent("#2F6B3A", false), "#2F6B3A");
  assert.equal(fitAccent("#B3262D", false), "#B3262D");
  assert.equal(fitAccent("#c9b44a", true), "#C9B44A");
});

test("fitting keeps the hue: a pale blue darkens to a blue", () => {
  const fitted = fitAccent("#A0C8FF", false);
  const [r, g, b] = [1, 3, 5].map(i => parseInt(fitted.slice(i, i + 2), 16));
  assert.ok(b > r && b > g, fitted);
});
