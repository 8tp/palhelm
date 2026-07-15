import assert from "node:assert/strict";
import test from "node:test";
import { palIconId } from "../src/lib/palIconId.ts";

test("Pal icons use base species art for save variants", () => {
  assert.equal(palIconId("BOSS_GrassMammoth"), "grassmammoth");
  assert.equal(palIconId("BOSS_BOSS_GrassMammoth"), "grassmammoth");
  assert.equal(palIconId("PlantSlime_Flower"), "plantslime");
  assert.equal(palIconId("GrassPanda_Electric_Tower"), "grasspanda_electric");
});

test("Pal icons retain the exact named-human portrait when one exists", () => {
  assert.equal(palIconId("BOSS_Hunter_Rifle"), "boss_hunter_rifle");
});
