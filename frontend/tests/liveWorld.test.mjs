import assert from "node:assert/strict";
import test from "node:test";
import { selectPlayerMarkers } from "../src/app/liveWorld.ts";

function player(uid, name, x, y, online = true) {
  return { uid, name, online, location: { x, y } };
}

function snapshot(state = "ready", actors = [], truncated = false) {
  return { state, actors, truncated };
}

function actor(name, x, y, active = true) {
  return { kind: "Player", name, active, location: { x, y, z: 0 } };
}

test("ready live positions reconcile onto, but never replace, the REST roster", () => {
  const got = selectPlayerMarkers(
    [player("one", "Hunter", 1, 2), player("two", "Ryfyshy", 3, 4)],
    snapshot("ready", [actor("Hunter", 100, 200), actor("not-on-roster", 999, 999)]),
  );
  assert.equal(got.usedLive, true);
  assert.deepEqual(got.markers, [
    { key: "one", name: "Hunter", location: { x: 100, y: 200 } },
    { key: "two", name: "Ryfyshy", location: { x: 3, y: 4 } },
  ]);
});

test("stale, unavailable, and truncated snapshots fall back entirely to REST", () => {
  const players = [player("one", "Hunter", 1, 2)];
  for (const live of [snapshot("stale", [actor("Hunter", 9, 9)]), snapshot("unavailable", [actor("Hunter", 9, 9)]), snapshot("ready", [actor("Hunter", 9, 9)], true)]) {
    assert.deepEqual(selectPlayerMarkers(players, live), {
      markers: [{ key: "one", name: "Hunter", location: { x: 1, y: 2 } }],
      usedLive: false,
    });
  }
});

test("inactive and ambiguous actors are ignored", () => {
  const players = [player("one", "Hunter", 1, 2)];
  for (const actors of [[actor("Hunter", 9, 9, false)], [actor("Hunter", 9, 9), actor("Hunter", 10, 10)]]) {
    assert.deepEqual(selectPlayerMarkers(players, snapshot("ready", actors)), {
      markers: [{ key: "one", name: "Hunter", location: { x: 1, y: 2 } }],
      usedLive: false,
    });
  }
});

test("duplicate REST display names are ambiguous even with one matching actor", () => {
  const players = [player("one", "Hunter", 1, 2), player("two", "Hunter", 3, 4)];
  assert.deepEqual(selectPlayerMarkers(players, snapshot("ready", [actor("Hunter", 9, 9)])), {
    markers: [
      { key: "one", name: "Hunter", location: { x: 1, y: 2 } },
      { key: "two", name: "Hunter", location: { x: 3, y: 4 } },
    ],
    usedLive: false,
  });
});

test("every non-ready state, missing active evidence, and invalid live coordinates fall back", () => {
  const players = [player("one", "Hunter", 1, 2)];
  for (const state of ["disabled", "pending", "stale", "unsupported", "unauthorized", "unavailable"]) {
    assert.equal(selectPlayerMarkers(players, snapshot(state, [actor("Hunter", 9, 9)])).usedLive, false);
  }
  assert.equal(selectPlayerMarkers(players, snapshot("ready", [{ ...actor("Hunter", 9, 9), active: undefined }])).usedLive, false);
  assert.equal(selectPlayerMarkers(players, snapshot("ready", [actor("Hunter", Number.NaN, 9)])).usedLive, false);
});

test("offline players and players without coordinates remain absent", () => {
  const missing = { ...player("missing", "Missing", 0, 0), location: null };
  const got = selectPlayerMarkers([player("off", "Offline", 1, 2, false), missing], snapshot("ready", [actor("Offline", 9, 9)]));
  assert.deepEqual(got, { markers: [], usedLive: false });
});
