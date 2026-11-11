import { test } from "node:test";
import assert from "node:assert/strict";
import {
  needsAttention,
  initialSelection,
  nextSelection,
  sidebarClashes,
  type AccountEntry,
} from "../src/accounts.ts";

function acct(id: string, over: Partial<AccountEntry> = {}): AccountEntry {
  return {
    id, user: id, server: "https://example.com", active: false, status: "Up to date",
    route: "", routeReason: "", running: true, syncing: false, localAddress: "",
    localPinned: false, sidebarName: "", sidebarAuto: "", sidebarLabel: id, ...over,
  };
}

const a = acct("a", { active: true });
const b = acct("b");

test("initialSelection with no accounts is signedout", () => {
  assert.deepEqual(initialSelection([], "folders"), { kind: "signedout" });
});

test("initialSelection picks the active account or an app page", () => {
  assert.deepEqual(initialSelection([b, a], "folders"), { kind: "account", id: "a" });
  assert.deepEqual(initialSelection([b, a], ""), { kind: "account", id: "a" });
  assert.deepEqual(initialSelection([a, b], "general"), { kind: "app", page: "general" });
});

test("initialSelection falls back to the first account when none is active", () => {
  assert.deepEqual(initialSelection([b, acct("c")], ""), { kind: "account", id: "b" });
});

test("nextSelection falls back when the selected account vanishes", () => {
  assert.deepEqual(nextSelection([a], { kind: "account", id: "b" }), { kind: "account", id: "a" });
  assert.deepEqual(nextSelection([], { kind: "account", id: "b" }), { kind: "signedout" });
});

test("nextSelection keeps a present account and app pages", () => {
  assert.deepEqual(nextSelection([a, b], { kind: "account", id: "a" }), { kind: "account", id: "a" });
  assert.deepEqual(nextSelection([], { kind: "app", page: "sync" }), { kind: "app", page: "sync" });
});

test("needsAttention", () => {
  assert.equal(needsAttention(acct("x", { running: false })), true);
  assert.equal(needsAttention(acct("x", { status: "Sign in again" })), true);
  assert.equal(needsAttention(acct("x")), false);
});

test("sidebarClashes lists accounts sharing a label", () => {
  const set = sidebarClashes([
    acct("a", { sidebarLabel: "OtherSync - adam" }),
    acct("b", { sidebarLabel: "OtherSync - adam" }),
    acct("c", { sidebarLabel: "OtherSync - bob" }),
  ]);
  assert.deepEqual([...set].sort(), ["a", "b"]);
});
