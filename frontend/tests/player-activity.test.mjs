import assert from "node:assert/strict";
import test from "node:test";
import { readFile } from "node:fs/promises";

test("player detail presents bounded observed activity separately from total tracked playtime", async () => {
  const [route, types, mock] = await Promise.all([
    readFile(new URL("../src/routes/players/Players.tsx", import.meta.url), "utf8"),
    readFile(new URL("../src/api/types.ts", import.meta.url), "utf8"),
    readFile(new URL("../src/api/mock.ts", import.meta.url), "utf8"),
  ]);
  assert.match(route, /Observed activity/);
  assert.match(route, /panel tracking only/);
  assert.match(route, /This is not lifetime game history/);
  assert.match(route, /last24Hours/);
  assert.match(route, /last7Days/);
  assert.match(route, /last30Days/);
  assert.match(types, /panel_observed_sessions/);
  assert.match(types, /recentSessionsTruncated/);
  assert.match(mock, /coverage: "panel_observed_sessions"/);
});
