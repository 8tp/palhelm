import test from "node:test";
import assert from "node:assert/strict";
import { readFile } from "node:fs/promises";

const players = await readFile(new URL("../src/routes/players/Players.tsx", import.meta.url), "utf8");
const client = await readFile(new URL("../src/api/client.ts", import.meta.url), "utf8");
const styles = await readFile(new URL("../src/routes/players/Players.css", import.meta.url), "utf8");

test("player detail scopes a reviewed Palbox-first Give Pal workflow", () => {
  assert.match(players, /Give Pal…/);
  assert.match(players, /Natural roll|natural roll/i);
  assert.match(players, /Customize specimen/);
  assert.match(players, /first safe Palbox slot/);
  assert.match(players, /Confirm Pal grant/);
  assert.match(players, /Alpha/);
  assert.match(players, /Lucky/);
  assert.match(players, /HP IV/);
  assert.match(players, /Melee attack IV/);
  assert.match(players, /Ranged attack IV/);
  assert.match(players, /Defense IV/);
  assert.match(players, /Passive skills/);
  assert.match(players, /Rainbow and World Tree traits/);
  assert.match(players, /selectedPassives\.map\(\(passive\) => passive\.id\)/);
  assert.match(players, /api\.palGrants\.passives\(passiveSearch, 200\)/);
  assert.match(players, /<ToggleChip pressed=\{!advanced\}/);
  assert.match(players, /aria-live="polite"/);
  assert.match(players, /Preview only:/);
  assert.match(players, /enabled: open, staleTime/);
  assert.match(client, /\/pal-grants\/passives/);
});

test("Give Pal uses field-guide tokens and responsive bounded dialog layout", () => {
  assert.doesNotMatch(styles, /#[0-9a-f]{3,8}\b/i);
  assert.match(styles, /item-grant-dialog \{ width: min\(900px, 94vw\); max-height: 92vh;/);
  assert.match(styles, /pal-grant-ivs[^}]*repeat\(4,/);
  assert.match(styles, /pal-passive-results[^}]*repeat\(2,/);
  assert.match(styles, /@media \(max-width: 560px\)/);
});

test("Pal creation reuses one idempotency key and polls the audit record", () => {
  assert.match(players, /api\.palGrants\.grant\(player\.uid, selected\.characterId, spec, reason\.trim\(\), idempotencyKey\)/);
  assert.match(players, /api\.palGrants\.grantStatus\(grant\.requestId\)/);
  assert.match(players, /Never resubmit a creation request/);
});

test("Give Pal remains a session-admin mutation outside the Integration API", () => {
  assert.match(client, /request\("POST", `\/players\/\$\{encodeURIComponent\(uid\)\}\/pal-grants`/);
  assert.doesNotMatch(client, /integration.*pal-grants/i);
});
