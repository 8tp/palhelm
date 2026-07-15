import assert from "node:assert/strict";
import { readFile } from "node:fs/promises";
import test from "node:test";
import { clusterMapMarkers } from "../src/app/mapClustering.ts";
import { isWorkerInDanger, summarizeWorkerCluster } from "../src/app/liveWorld.ts";

function point(key, kind, layerId, x, y, worldX = x, worldY = y) {
  return { key, kind, layerId, x, y, value: { location: { x: worldX, y: worldY } } };
}

test("dense markers form deterministic bounded-diameter screen-space groups", () => {
  const points = [
    point("player:c", "player", "default", 80, 0),
    point("player:a", "player", "default", 0, 0),
    point("player:b", "player", "default", 40, 0),
  ];
  const groups = clusterMapMarkers(points, 48);
  const [group] = groups;
  assert.equal(group.type, "cluster");
  assert.deepEqual(group.members.map((member) => member.key), ["player:a", "player:b"]);
  assert.deepEqual({ x: group.x, y: group.y }, { x: 20, y: 0 });
  assert.equal(groups[1].type, "single");
  assert.equal(groups[1].key, "player:c");
});

test("a proximity chain cannot percolate beyond the cluster radius", () => {
  const groups = clusterMapMarkers([
    point("player:a", "player", "default", 0, 0),
    point("player:b", "player", "default", 40, 0),
    point("player:c", "player", "default", 80, 0),
    point("player:d", "player", "default", 120, 0),
  ], 48);
  assert.deepEqual(groups.map((group) =>
    group.type === "single" ? [group.member.key] : group.members.map((member) => member.key)), [
    ["player:a", "player:b"],
    ["player:c", "player:d"],
  ]);
});

test("clusters never cross player/base or tile-layer boundaries", () => {
  const groups = clusterMapMarkers([
    point("player:default", "player", "default", 10, 10),
    point("base:default", "base", "default", 10, 10),
    point("player:tree", "player", "tree", 10, 10),
  ], 48);
  assert.equal(groups.length, 3);
  assert.ok(groups.every((group) => group.type === "single"));
});

test("the selected marker stays exact and standalone while neighbors cluster", () => {
  const selected = point("player:selected", "player", "default", 100, 100, -353196.34375, 270687.59375);
  const groups = clusterMapMarkers([
    selected,
    point("player:b", "player", "default", 102, 101, -350000, 271000),
    point("player:c", "player", "default", 104, 102, -349000, 272000),
  ], 48, selected.key);
  const exact = groups.find((group) => group.key === selected.key);
  const cluster = groups.find((group) => group.type === "cluster");
  assert.equal(exact.type, "single");
  assert.deepEqual(exact.member.value.location, { x: -353196.34375, y: 270687.59375 });
  assert.equal(cluster.members.length, 2);
  assert.ok(cluster.members.every((member) => member.value.location !== selected.value.location));
});

test("invalid clustering radius leaves every original marker accessible", () => {
  const points = [
    point("player:a", "player", "default", 0, 0),
    point("player:b", "player", "default", 0, 0),
  ];
  assert.deepEqual(clusterMapMarkers(points, 0).map((group) => group.key), ["player:a", "player:b"]);
});

function worker(instanceId, x, y, over = {}) {
  return {
    key: `worker:${instanceId}`,
    kind: "worker",
    layerId: "default",
    x,
    y,
    value: { instanceId, name: instanceId, activity: "working", hpPercent: 80, location: { x, y, z: 0 }, ...over },
  };
}

test("nearby base workers collapse into one worker cluster, keeping a lone worker standalone", () => {
  const groups = clusterMapMarkers([
    worker("w-a", 0, 0),
    worker("w-b", 20, 0),
    worker("w-c", 400, 0),
  ], 48);
  const cluster = groups.find((group) => group.type === "cluster");
  const single = groups.find((group) => group.type === "single");
  assert.equal(cluster.members.length, 2);
  assert.ok(cluster.key.includes("worker"));
  assert.equal(single.member.value.instanceId, "w-c");
});

test("a worker is in danger only when knocked out or critically hurt", () => {
  const at = { location: { x: 0, y: 0, z: 0 } };
  assert.equal(isWorkerInDanger({ activity: "incapacitated", ...at }), true);
  assert.equal(isWorkerInDanger({ activity: "working", hpPercent: 10, ...at }), true);
  assert.equal(isWorkerInDanger({ activity: "working", hpPercent: 90, ...at }), false);
  assert.equal(isWorkerInDanger({ activity: "working", ...at }), false); // unknown HP is not danger
});

test("a worker cluster label names how many members are hurt and flags danger", () => {
  const at = { location: { x: 0, y: 0, z: 0 } };
  const summary = summarizeWorkerCluster([
    { activity: "working", hpPercent: 90, ...at },
    { activity: "incapacitated", ...at },
    { activity: "working", hpPercent: 12, ...at },
  ]);
  assert.equal(summary.label, "3 workers · 2 hurt");
  assert.equal(summary.hurt, 2);
  assert.equal(summary.danger, true);
});

test("a healthy worker cluster reads plainly with no hurt count", () => {
  const at = { location: { x: 0, y: 0, z: 0 } };
  const summary = summarizeWorkerCluster([
    { activity: "working", hpPercent: 90, ...at },
    { activity: "idle", hpPercent: 88, ...at },
  ]);
  assert.equal(summary.label, "2 workers");
  assert.equal(summary.danger, false);
});

test("map clusters expose a direct exact-coordinate chooser when zoom cannot separate members", async () => {
  const route = await readFile(new URL("../src/routes/map/Map.tsx", import.meta.url), "utf8");
  assert.match(route, /marker-cluster-menu/);
  assert.match(route, /worldToGame\(target\.location\.x, target\.location\.y\)/);
  assert.match(route, /onClick=\{\(\) => onTarget\(target\)\}/);
  assert.match(route, /next\.scale > view\.scale \* 1\.15/);
});
