import assert from "node:assert/strict";
import { readFile } from "node:fs/promises";
import test from "node:test";
import {
  gameToWorld,
  layerMapToWorld,
  worldInBounds,
  worldToGame,
  worldToLayerMap,
} from "../src/app/mapTransform.ts";

const PALPAGOS = {
  a: 0.000353395913859746,
  b: 256,
  c: -0.000353395913859746,
  d: 123.47653230259525,
};
const BOUNDS = [[-1099399, -724399], [349399, 724399]];

const fixtures = JSON.parse(await readFile(
  new URL("./fixtures/map-transform-1.0.json", import.meta.url),
  "utf8",
));

test("live starting-area data coordinates convert to Palworld display coordinates", () => {
  const game = worldToGame(-353196.34375, 270687.59375);
  assert.deepEqual(game, { x: 246, y: -500 });
  const world = gameToWorld(game.x, game.y);
  assert.ok(Math.abs(world.x - -353196.34375) < 250);
  assert.ok(Math.abs(world.y - 270687.59375) < 250);
});

test("THGL transform consumes Palworld Y horizontally and X vertically", () => {
  const world = { x: -353196.34375, y: 270687.59375 };
  const map = worldToLayerMap(world.x, world.y, PALPAGOS, 512);
  // On the native z0 image this is about pixel (351.7, 248.3), not the old
  // top-left placement produced by feeding X horizontally and Y vertically.
  assert.ok(Math.abs(map.x * 2 - 351.7) < 0.2);
  assert.ok(Math.abs(map.y * 2 - 248.3) < 0.2);
  const roundTrip = layerMapToWorld(map.x, map.y, PALPAGOS, 512);
  assert.ok(Math.abs(roundTrip.x - world.x) < 0.01);
  assert.ok(Math.abs(roundTrip.y - world.y) < 0.01);
  assert.equal(worldInBounds(world.x, world.y, BOUNDS), true);
});

test("checked-in 1.0 layer anchors lock transform axes, offsets, and inverses", () => {
  assert.deepEqual(fixtures.layers.map((layer) => layer.id), ["default", "tree"]);
  for (const layer of fixtures.layers) {
    for (const anchor of layer.anchors) {
      const map = worldToLayerMap(anchor.world.x, anchor.world.y, layer.transform, layer.tileSize);
      const nativePixel = { x: map.x * layer.tileSize / 256, y: map.y * layer.tileSize / 256 };
      assert.ok(Math.abs(nativePixel.x - anchor.nativePixel.x) < 1e-9, `${layer.id}/${anchor.id} x offset`);
      assert.ok(Math.abs(nativePixel.y - anchor.nativePixel.y) < 1e-9, `${layer.id}/${anchor.id} y offset`);
      const inverse = layerMapToWorld(map.x, map.y, layer.transform, layer.tileSize);
      assert.ok(Math.abs(inverse.x - anchor.world.x) < 1e-6, `${layer.id}/${anchor.id} inverse x`);
      assert.ok(Math.abs(inverse.y - anchor.world.y) < 1e-6, `${layer.id}/${anchor.id} inverse y`);
      assert.equal(worldInBounds(anchor.world.x, anchor.world.y, layer.bounds), true, `${layer.id}/${anchor.id} bounds`);
      if (anchor.game) assert.deepEqual(worldToGame(anchor.world.x, anchor.world.y), anchor.game);
    }
  }
});

test("Palpagos and World Tree bounds use data-X/data-Y order and do not bleed layers", () => {
  const [palpagos, tree] = fixtures.layers;
  const palpagosPoint = palpagos.anchors[0].world;
  const treePoint = tree.anchors.find((anchor) => anchor.id === "dataset-bounds-center").world;
  assert.equal(worldInBounds(palpagosPoint.x, palpagosPoint.y, palpagos.bounds), true);
  assert.equal(worldInBounds(palpagosPoint.x, palpagosPoint.y, tree.bounds), false);
  assert.equal(worldInBounds(treePoint.x, treePoint.y, tree.bounds), true);
  assert.equal(worldInBounds(treePoint.x, treePoint.y, palpagos.bounds), false);

  for (const layer of fixtures.layers) {
    const [[minX, minY], [maxX, maxY]] = layer.bounds;
    const centerX = (minX + maxX) / 2;
    const centerY = (minY + maxY) / 2;
    assert.equal(worldInBounds(minX, minY, layer.bounds), true, `${layer.id} inclusive minimum`);
    assert.equal(worldInBounds(maxX, maxY, layer.bounds), true, `${layer.id} inclusive maximum`);
    assert.equal(worldInBounds(minX - 1, centerY, layer.bounds), false, `${layer.id} world-X minimum`);
    assert.equal(worldInBounds(centerX, minY - 1, layer.bounds), false, `${layer.id} world-Y minimum`);
  }
});

test("live-server survey positions stay on the layers where the players actually stood", () => {
  const [palpagos, tree] = fixtures.layers;
  // Read off the live 1.0 server on 2026-07-15: a player on Feybreak (far southwest,
  // world X beyond -724k) and a player northeast of the starting area (world Y beyond
  // +349k). The old axis-swapped bounds check filtered both off the Palpagos layer.
  const feybreak = { x: -757845, y: -61591 };
  const northeast = { x: 119362, y: 408511 };
  assert.equal(worldInBounds(feybreak.x, feybreak.y, palpagos.bounds), true, "Feybreak is on Palpagos");
  assert.equal(worldInBounds(northeast.x, northeast.y, palpagos.bounds), true, "northeast is on Palpagos");
  assert.equal(worldInBounds(feybreak.x, feybreak.y, tree.bounds), false);
  assert.equal(worldInBounds(northeast.x, northeast.y, tree.bounds), false);
  // A World Tree visitor reads game-x between about -2127 and -1382 on the in-game map
  // and must resolve to the tree layer, not Palpagos.
  const treeCenter = tree.anchors.find((anchor) => anchor.id === "dataset-bounds-center");
  assert.deepEqual(worldToGame(treeCenter.world.x, treeCenter.world.y), treeCenter.game);
  assert.equal(worldInBounds(treeCenter.world.x, treeCenter.world.y, palpagos.bounds), false);
});
