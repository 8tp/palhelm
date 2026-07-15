import assert from "node:assert/strict";
import { readFile } from "node:fs/promises";
import test from "node:test";
import { clusterMapMarkers } from "../src/app/mapClustering.ts";

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

test("map clusters expose a direct exact-coordinate chooser when zoom cannot separate members", async () => {
  const route = await readFile(new URL("../src/routes/map/Map.tsx", import.meta.url), "utf8");
  assert.match(route, /marker-cluster-menu/);
  assert.match(route, /worldToGame\(target\.location\.x, target\.location\.y\)/);
  assert.match(route, /onClick=\{\(\) => onTarget\(target\)\}/);
  assert.match(route, /next\.scale > view\.scale \* 1\.15/);
});
