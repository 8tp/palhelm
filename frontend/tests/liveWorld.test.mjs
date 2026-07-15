import assert from "node:assert/strict";
import test from "node:test";
import { selectLiveMapActors, selectPlayerMarkers } from "../src/app/liveWorld.ts";

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
    [player("one", "Player One", 1, 2), player("two", "Player Two", 3, 4)],
    snapshot("ready", [actor("Player One", 100, 200), actor("not-on-roster", 999, 999)]),
  );
  assert.equal(got.usedLive, true);
  assert.deepEqual(got.markers, [
    { key: "one", name: "Player One", location: { x: 100, y: 200 } },
    { key: "two", name: "Player Two", location: { x: 3, y: 4 } },
  ]);
});

test("stale, unavailable, and truncated snapshots fall back entirely to REST", () => {
  const players = [player("one", "Player One", 1, 2)];
  for (const live of [snapshot("stale", [actor("Player One", 9, 9)]), snapshot("unavailable", [actor("Player One", 9, 9)]), snapshot("ready", [actor("Player One", 9, 9)], true)]) {
    assert.deepEqual(selectPlayerMarkers(players, live), {
      markers: [{ key: "one", name: "Player One", location: { x: 1, y: 2 } }],
      usedLive: false,
    });
  }
});

test("inactive and ambiguous actors are ignored", () => {
  const players = [player("one", "Player One", 1, 2)];
  for (const actors of [[actor("Player One", 9, 9, false)], [actor("Player One", 9, 9), actor("Player One", 10, 10)]]) {
    assert.deepEqual(selectPlayerMarkers(players, snapshot("ready", actors)), {
      markers: [{ key: "one", name: "Player One", location: { x: 1, y: 2 } }],
      usedLive: false,
    });
  }
});

test("duplicate REST display names are ambiguous even with one matching actor", () => {
  const players = [player("one", "Player One", 1, 2), player("two", "Player One", 3, 4)];
  assert.deepEqual(selectPlayerMarkers(players, snapshot("ready", [actor("Player One", 9, 9)])), {
    markers: [
      { key: "one", name: "Player One", location: { x: 1, y: 2 } },
      { key: "two", name: "Player One", location: { x: 3, y: 4 } },
    ],
    usedLive: false,
  });
});

test("every non-ready state, missing active evidence, and invalid live coordinates fall back", () => {
  const players = [player("one", "Player One", 1, 2)];
  for (const state of ["disabled", "pending", "stale", "unsupported", "unauthorized", "unavailable"]) {
    assert.equal(selectPlayerMarkers(players, snapshot(state, [actor("Player One", 9, 9)])).usedLive, false);
  }
  assert.equal(selectPlayerMarkers(players, snapshot("ready", [{ ...actor("Player One", 9, 9), active: undefined }])).usedLive, false);
  assert.equal(selectPlayerMarkers(players, snapshot("ready", [actor("Player One", Number.NaN, 9)])).usedLive, false);
});

test("offline players and players without coordinates remain absent", () => {
  const missing = { ...player("missing", "Missing", 0, 0), location: null };
  const got = selectPlayerMarkers([player("off", "Offline", 1, 2, false), missing], snapshot("ready", [actor("Offline", 9, 9)]));
  assert.deepEqual(got, { markers: [], usedLive: false });
});

test("live-only map layers require a ready, non-truncated snapshot", () => {
  const actors = [
    { kind: "BaseCampPal", linked: true, instanceId: "worker-one", baseId: "base-one", location: { x: 10, y: 20, z: 0 } },
    { kind: "PalBox", location: { x: 30, y: 40, z: 0 } },
  ];
  for (const live of [snapshot("stale", actors), snapshot("unavailable", actors), snapshot("ready", actors, true), undefined]) {
    assert.deepEqual(selectLiveMapActors(live), { available: false, workers: [], palBoxes: [] });
  }
});

test("live-only map layers accept exact-linked workers and finite PalBoxes only", () => {
  const linkedWorker = { kind: "BaseCampPal", linked: true, instanceId: "worker-one", baseId: "base-one", location: { x: 10, y: 20, z: 0 } };
  const palBox = { kind: "PalBox", location: { x: 30, y: 40, z: 0 } };
  const got = selectLiveMapActors(snapshot("ready", [
    linkedWorker,
    { ...linkedWorker, instanceId: "", baseId: "base-two" },
    { ...linkedWorker, instanceId: "worker-two", baseId: undefined },
    { ...linkedWorker, instanceId: "worker-three", linked: false },
    { ...linkedWorker, instanceId: "worker-four", location: { x: Number.NaN, y: 20, z: 0 } },
    palBox,
    { ...palBox, location: { x: 30, y: Number.POSITIVE_INFINITY, z: 0 } },
    actor("Player One", 50, 60),
  ]));
  assert.equal(got.available, true);
  assert.deepEqual(got.workers, [linkedWorker]);
  assert.deepEqual(got.palBoxes, [palBox]);
});
