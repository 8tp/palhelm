import assert from "node:assert/strict";
import { readFile } from "node:fs/promises";
import test from "node:test";
import { activityCoverageNote, topPeakBuckets } from "../src/routes/activity/activityView.ts";

test("activity route is authenticated, lazy, navigable, and uses bounded server analytics", async () => {
  const [app, shell, route, client] = await Promise.all([
    readFile(new URL("../src/app/App.tsx", import.meta.url), "utf8"),
    readFile(new URL("../src/components/Shell.tsx", import.meta.url), "utf8"),
    readFile(new URL("../src/routes/activity/Activity.tsx", import.meta.url), "utf8"),
    readFile(new URL("../src/api/client.ts", import.meta.url), "utf8"),
  ]);
  assert.match(app, /lazy\(\(\) => import\("\.\.\/routes\/activity\/Activity"\)\)/);
  assert.match(app, /path="activity"/);
  assert.match(shell, /to: "\/activity"/);
  assert.match(shell, /queryKey: \["activity"\]/);
  assert.match(route, /panel-observed sessions/);
  assert.match(route, /current membership attribution/);
  assert.match(route, /not lifetime game history/);
  assert.match(client, /\/activity\?window=/);
});

test("peak hours sort by average concurrency without mutating the API order", () => {
  const buckets = [
    { at: "2026-07-15T00:00:00Z", peakPlayers: 3, averagePlayers: 1.2 },
    { at: "2026-07-15T01:00:00Z", peakPlayers: 2, averagePlayers: 1.8 },
    { at: "2026-07-15T02:00:00Z", peakPlayers: 4, averagePlayers: 1.8 },
  ];
  assert.deepEqual(topPeakBuckets(buckets, 2).map((bucket) => bucket.at), [buckets[2].at, buckets[1].at]);
  assert.equal(buckets[0].at, "2026-07-15T00:00:00Z");
});

test("coverage language exposes partial windows and defensive truncation", () => {
  assert.match(activityCoverageNote({ trackingSince: "2026-07-15T12:00:00Z", since: "2026-07-15T00:00:00Z", analysisTruncated: false }), /partway through/);
  assert.match(activityCoverageNote({ trackingSince: "2026-07-01T00:00:00Z", since: "2026-07-15T00:00:00Z", analysisTruncated: false }), /full selected window/);
  assert.match(activityCoverageNote({ trackingSince: null, since: "2026-07-15T00:00:00Z", analysisTruncated: true }), /defensive interval cap/);
});
