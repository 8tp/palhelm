import test from "node:test";
import assert from "node:assert/strict";
import { readFile } from "node:fs/promises";

const players = await readFile(new URL("../src/routes/players/Players.tsx", import.meta.url), "utf8");
const styles = await readFile(new URL("../src/routes/players/Players.css", import.meta.url), "utf8");
const client = await readFile(new URL("../src/api/client.ts", import.meta.url), "utf8");

test("player detail exposes a reviewed, online-only item grant flow", () => {
  assert.match(players, /Give item…/);
  assert.match(players, /disabled=\{!d\.online\}/);
  assert.match(players, /Review grant/);
  assert.match(players, /Confirm grant/);
  assert.match(players, /This changes a live player inventory/);
  assert.match(players, /ItemRarityBadge/);
  assert.match(styles, /item-rarity-legendary/);
  assert.match(players, /selected\.category/);
});

test("grant retries reuse one idempotency key and poll only the audit status", () => {
  assert.match(players, /newIdempotencyKey\(\)/);
  assert.match(players, /api\.items\.grant\(player\.uid, selected\.id, quantity, reason\.trim\(\), idempotencyKey\)/);
  assert.match(players, /api\.items\.grantStatus\(grant\.requestId\)/);
  assert.doesNotMatch(players, /api\.items\.grant\([^\n]+crypto\.randomUUID/);
});

test("item mutation remains outside the read-only Integration API", () => {
  assert.match(client, /request\("POST", `\/players\/\$\{encodeURIComponent\(uid\)\}\/item-grants`/);
  assert.doesNotMatch(client, /integration.*item-grants/i);
});
