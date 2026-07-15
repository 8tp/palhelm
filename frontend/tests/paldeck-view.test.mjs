import assert from "node:assert/strict";
import test from "node:test";
import { filterPaldeckSpecies, paldeckPercent } from "../src/routes/paldeck/paldeckView.ts";

const species = [
  { characterId: "Anubis", displayName: "Anubis", known: true, captureCount: 3 },
  { characterId: "Mammorest", displayName: "Mammorest", known: true, captureCount: 0 },
  { characterId: "NewPal", displayName: "New Pal", known: false, captureCount: null },
];

test("Paldeck filters distinguish observed zero from unavailable", () => {
  assert.deepEqual(filterPaldeckSpecies(species, "", "captured").map((item) => item.characterId), ["Anubis"]);
  assert.deepEqual(filterPaldeckSpecies(species, "", "unseen").map((item) => item.characterId), ["Mammorest"]);
  assert.deepEqual(filterPaldeckSpecies(species, "", "unavailable").map((item) => item.characterId), ["NewPal"]);
  assert.deepEqual(filterPaldeckSpecies(species, "new pal", "all").map((item) => item.characterId), ["NewPal"]);
});

test("Paldeck percentages remain unavailable without a value or catalog", () => {
  assert.equal(paldeckPercent(null, 100), null);
  assert.equal(paldeckPercent(20, 0), null);
  assert.equal(paldeckPercent(51, 200), 25.5);
  assert.equal(paldeckPercent(999, 200), 100);
});
