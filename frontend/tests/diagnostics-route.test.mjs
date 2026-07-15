import assert from "node:assert/strict";
import { readFile } from "node:fs/promises";
import test from "node:test";
import {
  classifyFreshness,
  gameDataStateDetail,
  gameDataStateTone,
  linkCoverage,
} from "../src/app/diagnostics.ts";

const source = async (path) => readFile(new URL(`../src/${path}`, import.meta.url), "utf8");

test("diagnostics route is wired into authenticated routing and shared navigation", async () => {
  const [app, shell, route] = await Promise.all([
    source("app/App.tsx"),
    source("components/Shell.tsx"),
    source("routes/diagnostics/Diagnostics.tsx"),
  ]);
  assert.match(app, /path="diagnostics"/);
  assert.match(shell, /to: "\/diagnostics"/);
  assert.match(route, /api\.server\.health\(\)/);
  assert.match(route, /api\.world\.get\(\)/);
  assert.match(route, /api\.world\.snapshot\(\)/);
  assert.match(route, /api\.backups\.list\(\)/);
  assert.match(route, /api\.backups\.schedule\(\)/);
});

test("diagnostics surface uses bounded rollout evidence and explicit contract gaps", async () => {
  const route = await source("routes/diagnostics/Diagnostics.tsx");
  for (const field of [
    "lastRequestDurationMs",
    "lastAcceptedActorCount",
    "lastErrorCategory",
    "linkedBasePals",
    "unresolvedBasePals",
    "linkLookupFailed",
    "scheduledDelayMs",
    "nextAttemptAt",
  ]) assert.match(route, new RegExp(field));
  assert.doesNotMatch(route, /snapshot\?*\.actors|snapshot\.actors/);
  assert.match(route, /Filesystem headroom/);
  assert.match(route, /Database schema/);
  assert.match(route, /not exposed by the authenticated API/i);
});

test("freshness classification treats absent and invalid timestamps as unavailable", () => {
  const now = Date.parse("2026-07-15T12:00:00Z");
  assert.deepEqual(classifyFreshness(null, 1_000, 2_000, now), { tone: "idle", label: "Unavailable", ageMs: null });
  assert.deepEqual(classifyFreshness("not-a-date", 1_000, 2_000, now), { tone: "idle", label: "Unavailable", ageMs: null });
  assert.equal(classifyFreshness("2026-07-15T11:59:59.500Z", 1_000, 2_000, now).label, "Current");
  assert.equal(classifyFreshness("2026-07-15T11:59:58.500Z", 1_000, 2_000, now).label, "Aging");
  assert.equal(classifyFreshness("2026-07-15T11:59:57Z", 1_000, 2_000, now).label, "Stale");
});

test("Game Data state and link coverage preserve unavailable and failure semantics", () => {
  assert.equal(gameDataStateTone("ready"), "ok");
  assert.equal(gameDataStateTone("stale"), "warn");
  assert.equal(gameDataStateTone("unauthorized"), "danger");
  assert.match(gameDataStateDetail("disabled"), /disabled/i);
  assert.deepEqual(linkCoverage(0, 0), { value: "Unavailable", detail: "No accepted base-Pal actors to link." });
  assert.deepEqual(linkCoverage(9, 1), { value: "90%", detail: "9 linked · 1 unresolved" });
});
