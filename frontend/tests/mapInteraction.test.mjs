import assert from "node:assert/strict";
import { readFile } from "node:fs/promises";
import test from "node:test";
import {
  addContainedMapWheelListener,
  buildSharedMapURL,
  centerMapPoint,
  DEFAULT_MAP_LAYERS,
  filterMapSearchTargets,
  fitMapPoints,
  parseSharedMapCoordinates,
  wheelZoomFactor,
  zoomMapView,
} from "../src/app/mapInteraction.ts";

test("dense live map layers start hidden", () => {
  assert.equal(DEFAULT_MAP_LAYERS.Workers, false);
  assert.equal(DEFAULT_MAP_LAYERS.PalBoxes, false);
  assert.equal(DEFAULT_MAP_LAYERS.Players, true);
  assert.equal(DEFAULT_MAP_LAYERS.Bases, true);
});

test("map zoom keeps its screen-space anchor fixed and clamps scale", () => {
  const view = { scale: 2, tx: 10, ty: 20 };
  const anchor = { x: 110, y: 70 };
  const zoomed = zoomMapView(view, 1.5, anchor, { min: 1, max: 4 });
  assert.deepEqual(zoomed, { scale: 3, tx: -40, ty: -5 });

  const clamped = zoomMapView(view, 10, anchor, { min: 1, max: 4 });
  assert.equal(clamped.scale, 4);
  assert.deepEqual(zoomMapView(view, 0.01, anchor, { min: 1, max: 4 }), {
    scale: 1,
    tx: 60,
    ty: 45,
  });
});

test("wheel direction maps to bounded zoom steps", () => {
  assert.equal(wheelZoomFactor(-1), 1.25);
  assert.equal(wheelZoomFactor(1), 0.8);
  assert.equal(wheelZoomFactor(0), 0.8);
});

test("map wheel listener is non-passive and contains the event", () => {
  let listener;
  let options;
  let removed;
  const target = {
    addEventListener(type, next, nextOptions) {
      assert.equal(type, "wheel");
      listener = next;
      options = nextOptions;
    },
    removeEventListener(type, next) {
      assert.equal(type, "wheel");
      removed = next;
    },
  };
  let prevented = 0;
  let stopped = 0;
  let handled = 0;
  const cleanup = addContainedMapWheelListener(target, () => handled++);
  assert.deepEqual(options, { passive: false });

  listener({
    preventDefault: () => prevented++,
    stopPropagation: () => stopped++,
  });
  assert.equal(prevented, 1);
  assert.equal(stopped, 1);
  assert.equal(handled, 1);

  cleanup();
  assert.equal(removed, listener);
});

test("map focus and fit operate only in map space", () => {
  assert.deepEqual(centerMapPoint({ x: 25, y: 50 }, { width: 500, height: 300 }, 4, { min: 1, max: 8 }), {
    scale: 4,
    tx: 150,
    ty: -50,
  });
  assert.deepEqual(fitMapPoints([{ x: 0, y: 0 }, { x: 100, y: 100 }], { width: 1000, height: 500 }, { min: 1, max: 10 }, 50), {
    scale: 4,
    tx: 300,
    ty: 50,
  });
  assert.equal(fitMapPoints([], { width: 1000, height: 500 }, { min: 1, max: 10 }), null);
});

test("map search prioritizes exact and prefix matches", () => {
  const targets = [
    { key: "base-1", kind: "base", label: "Nightloom", detail: "Base", location: { x: 1, y: 1 } },
    { key: "player-1", kind: "player", label: "Night", detail: "Online player", location: { x: 2, y: 2 } },
    { key: "player-2", kind: "player", label: "Player Two", detail: "Nightloom", location: { x: 3, y: 3 } },
  ];
  assert.deepEqual(filterMapSearchTargets(targets, "night").map((target) => target.key), ["player-1", "base-1", "player-2"]);
  assert.deepEqual(filterMapSearchTargets(targets, "missing"), []);
});

test("shared map links carry coordinates and layer but no identity", () => {
  assert.deepEqual(parseSharedMapCoordinates("?x=-361&y=292&layer=default"), { x: -361, y: 292, layerId: "default" });
  assert.equal(parseSharedMapCoordinates("?x=nope&y=292&layer=default"), null);
  assert.deepEqual(parseSharedMapCoordinates("?x=1.4&y=2.6&layer=../../bad"), { x: 1, y: 3, layerId: null });
  const shared = new URL(buildSharedMapURL("https://panel.example/map?mocktiles=1", { x: -361, y: 292 }, "default"));
  assert.equal(shared.searchParams.get("x"), "-361");
  assert.equal(shared.searchParams.get("y"), "292");
  assert.equal(shared.searchParams.get("layer"), "default");
  assert.equal(shared.searchParams.get("mocktiles"), "1");
  assert.equal(shared.searchParams.has("player"), false);
});

test("map route wires search, focus, fit, sharing, and mobile-safe controls", async () => {
  const [route, css] = await Promise.all([
    readFile(new URL("../src/routes/map/Map.tsx", import.meta.url), "utf8"),
    readFile(new URL("../src/routes/map/Map.css", import.meta.url), "utf8"),
  ]);
  for (const label of ["Focus selected", "Fit online", "Fit bases", "Copy coordinate link"]) {
    assert.match(route, new RegExp(label));
  }
  assert.match(route, /Search online players or bases/);
  assert.match(route, /parseSharedMapCoordinates/);
  assert.match(route, /onPointerLeave=\{hasMap \? cancelPointer/);
  assert.match(css, /@media \(max-width: 600px\)/);
  assert.match(css, /touch-action: none/);
});
