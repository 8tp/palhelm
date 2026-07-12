import assert from "node:assert/strict";
import test from "node:test";
import { readFile } from "node:fs/promises";
import { humanizePalIdentifier, palGenderLabel, palPlacementLabel } from "../src/components/palDetails.ts";

test("Pal detail labels humanize save identifiers and preserve unknown data honestly", () => {
  assert.equal(humanizePalIdentifier("CraftSpeed_up2"), "Craft Speed Up 2");
  assert.equal(humanizePalIdentifier("ElementBoost_Earth_2_PAL"), "Element Boost Earth 2");
  assert.equal(palGenderLabel("female"), "Female ♀");
  assert.equal(palGenderLabel(""), "Unknown");
});

test("Pal placement describes party, box, and exact base membership", () => {
  const base = { inParty: false, partySlot: null, boxPage: null, boxSlot: null, placement: "base", baseId: "1234567890abcdef" };
  assert.equal(palPlacementLabel(base), "Base worker · 12345678");
  assert.equal(palPlacementLabel({ ...base, placement: "party", baseId: null, inParty: true, partySlot: 2 }), "Party slot 3");
  assert.equal(palPlacementLabel({ ...base, placement: "box", baseId: null, boxPage: 1, boxSlot: 4 }), "Box 2 · slot 5");
});

test("both the team list and Palbox cells expose the reusable info control", async () => {
  const [players, box] = await Promise.all([
    readFile(new URL("../src/routes/players/Players.tsx", import.meta.url), "utf8"),
    readFile(new URL("../src/components/PalBoxDialog.tsx", import.meta.url), "utf8"),
  ]);
  assert.match(players, /<PalInfoButton/);
  assert.match(players, /<PalDetailPanel/);
  assert.match(box, /<PalInfoButton/);
  assert.match(box, /<PalDetailPanel/);
});
