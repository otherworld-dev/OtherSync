// Activity row labels (Deck #1129). Runs on Node's own test runner: `npm test`.
import { test } from "node:test";
import assert from "node:assert/strict";
import { activityLabel, kindLabel } from "../src/kinds.ts";

test("a failed row says what failed", () => {
  assert.equal(activityLabel("upload", "Server error 507"), "Upload failed");
  assert.equal(activityLabel("download", "timed out"), "Download failed");
  assert.equal(activityLabel("delete-remote", "423 Locked"), "Couldn't delete on server");
});

test("a row without an error keeps its usual label", () => {
  assert.equal(activityLabel("upload"), "Uploaded");
  assert.equal(activityLabel("upload", ""), "Uploaded");
  assert.equal(activityLabel("conflict", "x"), kindLabel("conflict"));
  assert.equal(activityLabel("something-new", "x"), "something-new");
});
