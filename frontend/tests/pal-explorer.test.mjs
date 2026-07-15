import assert from "node:assert/strict";
import test from "node:test";
import { readFile } from "node:fs/promises";
import { palExplorerParams, palOwnerSummary, palSpecimenLabels } from "../src/routes/pals/palExplorer.ts";

test("Pal explorer narrows form strings to bounded API parameters", () => {
  assert.deepEqual(palExplorerParams({
    q: "  Mammorest  ", ownerSource: "last_observed", placement: "base", specimen: "boss", minLevel: "0", maxLevel: "35",
  }), {
    q: "Mammorest", ownerSource: "last_observed", placement: "base", specimen: "boss", minLevel: 0, maxLevel: 35,
  });
  assert.deepEqual(palExplorerParams({
    q: " ", ownerSource: "guessed", placement: "inventory", specimen: "shiny", minLevel: "-1", maxLevel: "1000",
  }), {});
});

test("owner evidence is explicit and unresolved owners are never guessed", () => {
  assert.equal(palOwnerSummary({ ownerName: "Kestrel", ownerResolved: true, ownerSource: "personal_container" }), "Kestrel · current personal container");
  assert.equal(palOwnerSummary({ ownerName: "Kestrel", ownerResolved: true, ownerSource: "last_observed" }), "Kestrel · last observed owner");
  assert.equal(palOwnerSummary({ ownerName: "", ownerResolved: false, ownerSource: "unresolved" }), "Owner unavailable");
});

test("boss variants receive a Boss emblem instead of a duplicate Alpha label", () => {
  assert.deepEqual(palSpecimenLabels({ isBoss: true, isAlpha: true, isLucky: false }), ["Boss"]);
  assert.deepEqual(palSpecimenLabels({ isBoss: false, isAlpha: true, isLucky: true }), ["Alpha", "Lucky"]);
});

test("standalone explorer uses bounded infinite loading and reusable Pal details", async () => {
  const source = await readFile(new URL("../src/routes/pals/Pals.tsx", import.meta.url), "utf8");
  assert.match(source, /useInfiniteQuery/);
  assert.match(source, /PAL_EXPLORER_CLIENT_CAP/);
  assert.match(source, /<PalIcon/);
  assert.match(source, /<PalDetailPanel/);
  assert.match(source, /ownerSource/);
});
