import test from "node:test";
import assert from "node:assert/strict";
import { notifyUnauthorized, onUnauthorized, shouldRetryRequest } from "../src/api/requestPolicy.ts";
import { RECENTS_KEY, loadPaletteRecents, savePaletteRecent } from "../src/components/paletteRecents.ts";

class MemoryStorage {
  values = new Map();

  getItem(key) {
    return this.values.get(key) ?? null;
  }

  setItem(key, value) {
    this.values.set(key, value);
  }
}

test("retry policy rejects authentication, authorization, and throttle failures", () => {
  for (const status of [401, 403, 429]) {
    assert.equal(shouldRetryRequest(0, { status }), false, `${status} must not retry`);
  }
  assert.equal(shouldRetryRequest(0, { status: 503 }), true, "first transient failure retries");
  assert.equal(shouldRetryRequest(1, { status: 503 }), false, "transient failure retries only once");
  assert.equal(shouldRetryRequest(0, new Error("network")), true);
});

test("global unauthorized event fires only for 401 and unsubscribe removes state listener", () => {
  let transitions = 0;
  const unsubscribe = onUnauthorized(() => {
    transitions += 1;
  });
  notifyUnauthorized(403);
  notifyUnauthorized(429);
  assert.equal(transitions, 0);
  notifyUnauthorized(401);
  assert.equal(transitions, 1);
  unsubscribe();
  notifyUnauthorized(401);
  assert.equal(transitions, 1);
});

test("viewer load filters and permanently removes admin palette recents", () => {
  const storage = new MemoryStorage();
  const stored = [
    { kind: "kick", key: "one", label: "Kick One" },
    { kind: "nav", key: "/map", label: "Live map" },
    { kind: "broadcast", key: "", label: "Broadcast" },
    { kind: "player", key: "two", label: "Two" },
    { kind: "ban", key: "three", label: "Ban Three" },
    { kind: "saved", key: "info", label: "Info" },
    { kind: "unban", key: "four", label: "Unban Four" },
  ];
  storage.setItem(RECENTS_KEY, JSON.stringify(stored));

  const viewer = loadPaletteRecents(storage, false);
  assert.deepEqual(
    viewer.map((entry) => entry.kind),
    ["nav", "player", "saved"],
  );
  assert.deepEqual(JSON.parse(storage.getItem(RECENTS_KEY)), viewer, "filtered entries must replace persisted admin recents");

  const afterRejectedSave = savePaletteRecent(storage, { kind: "kick", key: "two", label: "Kick Two" }, false);
  assert.deepEqual(afterRejectedSave, viewer);
  assert.deepEqual(JSON.parse(storage.getItem(RECENTS_KEY)), viewer);
});
