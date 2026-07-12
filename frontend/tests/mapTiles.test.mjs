import assert from "node:assert/strict";
import test from "node:test";
import { tileZoomForScale } from "../src/app/mapTiles.ts";

test("tile zoom accounts for the active layer tile size", () => {
  // A 256-unit map displayed at 512 px needs four 256px z1 tiles, but one
  // 512px z0 tile. The old hard-coded divisor selected z1 for both layers.
  assert.equal(tileZoomForScale(2, 256, 256, 0, 6), 1);
  assert.equal(tileZoomForScale(2, 256, 512, 0, 4), 0);
});

test("tile zoom respects layer bounds", () => {
  assert.equal(tileZoomForScale(0.01, 256, 512, 0, 4), 0);
  assert.equal(tileZoomForScale(128, 256, 512, 0, 4), 4);
});
