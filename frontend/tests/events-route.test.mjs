import assert from "node:assert/strict";
import { readFile } from "node:fs/promises";
import test from "node:test";

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
