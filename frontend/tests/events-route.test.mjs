import assert from "node:assert/strict";
import { readFile } from "node:fs/promises";
import test from "node:test";
import { countEventLanes, eventLaneFor, filterEvents, kindsForLane } from "../src/routes/events/eventLanes.ts";

const source = async (path) => readFile(new URL(`../src/${path}`, import.meta.url), "utf8");

test("events route replaces the dead hash link and is part of primary navigation", async () => {
  const [app, shell, dashboard, events] = await Promise.all([
    source("app/App.tsx"),
    source("components/Shell.tsx"),
    source("routes/dashboard/Dashboard.tsx"),
    source("routes/events/Events.tsx"),
  ]);
  assert.match(app, /path="events"/);
  assert.match(shell, /to: "\/events"/);
  assert.doesNotMatch(dashboard, /#\/events/);
  assert.match(dashboard, /to="\/events"/);
  assert.match(events, /Events & audit/);
  assert.match(events, /PAGE_SIZE = 25/);
  assert.match(events, /Counts cover the newest/);
  assert.match(events, /Filter event kind/);
  assert.match(events, /api\.events\.list\(FETCH_LIMIT\)/);
});

test("semantic lanes partition every exact event kind", () => {
  assert.equal(eventLaneFor("join"), "player");
  assert.equal(eventLaneFor("leave"), "player");
  assert.equal(eventLaneFor("backup"), "operations");
  assert.equal(eventLaneFor("panel"), "operations");
  assert.equal(eventLaneFor("config"), "operations");
  assert.equal(eventLaneFor("system"), "health");
  assert.deepEqual(kindsForLane("player"), ["join", "leave"]);
  assert.deepEqual(kindsForLane("operations"), ["backup", "panel", "config"]);
  assert.deepEqual(kindsForLane("health"), ["system"]);
});

test("lane counts and filters operate on one bounded corpus", () => {
  const events = [
    { at: "2026-07-15T12:00:00Z", kind: "join", message: "Player One joined" },
    { at: "2026-07-15T11:59:00Z", kind: "leave", message: "Player Two left" },
    { at: "2026-07-15T11:58:00Z", kind: "backup", message: "Backup completed" },
    { at: "2026-07-15T11:57:00Z", kind: "panel", message: "Operator ran Save" },
    { at: "2026-07-15T11:56:00Z", kind: "config", message: "Configuration updated" },
    { at: "2026-07-15T11:55:00Z", kind: "system", message: "Palworld REST API is unreachable" },
  ];
  assert.deepEqual(countEventLanes(events), { all: 6, player: 2, operations: 3, health: 1 });
  assert.deepEqual(filterEvents(events, "operations", "panel", "operator").map((event) => event.kind), ["panel"]);
  assert.deepEqual(filterEvents(events, "player", "all", "player one").map((event) => event.kind), ["join"]);
  assert.deepEqual(filterEvents(events, "health", "all", "").map((event) => event.kind), ["system"]);
});

test("panel version is backend-derived and SSE drives query cache updates", async () => {
  const [login, shell, sse] = await Promise.all([
    source("routes/login/Login.tsx"),
    source("components/Shell.tsx"),
    source("app/useSSE.ts"),
  ]);
  assert.doesNotMatch(login, /PANEL_VERSION|Palhelm v0\./);
  assert.doesNotMatch(shell, /PANEL_VERSION/);
  assert.match(shell, /panelVersion/);
  assert.match(shell, /useSSE/);
  assert.match(shell, /invalidateQueries\(\{ queryKey: \["events"\]/);
  assert.doesNotMatch(sse, /es\?\.close\(\);\s*es = null/);
});
