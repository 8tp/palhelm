import assert from "node:assert/strict";
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
