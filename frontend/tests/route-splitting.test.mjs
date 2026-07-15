import assert from "node:assert/strict";
import { readFile } from "node:fs/promises";
import test from "node:test";

const app = await readFile(new URL("../src/app/App.tsx", import.meta.url), "utf8");

test("authenticated routes are lazy while auth and Shell stay eager", () => {
  const authenticatedModules = [
    "dashboard/Dashboard",
    "players/Players",
    "activity/Activity",
    "pals/Pals",
    "map/Map",
    "events/Events",
    "console/Console",
    "backups/Backups",
    "config/Config",
    "diagnostics/Diagnostics",
    "settings/Settings",
  ];

  assert.match(app, /import \{ lazy, Suspense, type ReactNode \} from "react"/);
  assert.match(app, /import \{ Shell \} from "\.\.\/components\/Shell"/);
  assert.match(app, /import Login from "\.\.\/routes\/login\/Login"/);
  for (const module of authenticatedModules) {
    assert.match(app, new RegExp(`lazy\\(\\(\\) => import\\("\\.\\.\\/routes\\/${module.replaceAll("/", "\\/")}"\\)\\)`));
    assert.doesNotMatch(app, new RegExp(`import [^\\n]+ from "\\.\\.\\/routes\\/${module.replaceAll("/", "\\/")}"`));
  }
});

test("one shared route fallback preserves the authenticated Shell", () => {
  assert.match(app, /function RouteLoader\(\)/);
  assert.match(app, /role="status" aria-busy="true" aria-label="Loading page"/);
  assert.match(app, /function lazyRoute\(element: ReactNode\)/);
  assert.match(app, /<Suspense fallback=\{<RouteLoader \/>\}>\{element\}<\/Suspense>/);
  assert.match(app, /<RequireAuth>\s*<Shell \/>\s*<\/RequireAuth>/);
  assert.match(app, /<Route index element=\{lazyRoute\(<Dashboard \/>\)\} \/>/);
});
