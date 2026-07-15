import assert from "node:assert/strict";
import test from "node:test";
import { readFile } from "node:fs/promises";
import { condensedStars, MAX_CONDENSE_STARS } from "../src/components/palStars.ts";

test("condensed stars map rank 1..5 to 0..4 filled stars", () => {
  assert.equal(MAX_CONDENSE_STARS, 4);
  assert.equal(condensedStars(1), 0);
  assert.equal(condensedStars(2), 1);
  assert.equal(condensedStars(3), 2);
  assert.equal(condensedStars(5), 4);
});

test("unavailable rank stays null and is never coerced to zero stars", () => {
  // The honesty rule: missing data is null, not 0 — callers show "Unavailable"
  // or render nothing rather than an all-empty star row.
  assert.equal(condensedStars(null), null);
  assert.equal(condensedStars(undefined), null);
  // A present rank of 1 (never condensed) is a real 0 stars, distinct from null.
  assert.equal(condensedStars(1), 0);
  assert.notEqual(condensedStars(1), condensedStars(null));
});

test("out-of-range ranks clamp instead of rendering a broken row", () => {
  assert.equal(condensedStars(0), 0);
  assert.equal(condensedStars(9), 4);
  assert.equal(condensedStars(2.4), 1);
});

test("the reusable stars component is shared across every per-Pal surface", async () => {
  const [details, pals, box] = await Promise.all([
    readFile(new URL("../src/components/PalDetails.tsx", import.meta.url), "utf8"),
    readFile(new URL("../src/routes/pals/Pals.tsx", import.meta.url), "utf8"),
    readFile(new URL("../src/components/PalBoxDialog.tsx", import.meta.url), "utf8"),
  ]);
  // Detail panel adds a Condensed fact that stays honest about missing ranks.
  assert.match(details, /Condensed/);
  assert.match(details, /condensedStars\(pal\.rank\) === null \? "Unavailable"/);
  assert.match(details, /<PalStars/);
  // Compact contexts reuse the same component and only show it for condensed pals.
  assert.match(pals, /<PalStars rank=\{pal\.rank\}/);
  assert.match(box, /pal\.rank > 1 && <PalStars/);
});
