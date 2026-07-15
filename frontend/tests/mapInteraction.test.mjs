import assert from "node:assert/strict";
import test from "node:test";
import {
  addContainedMapWheelListener,
  DEFAULT_MAP_LAYERS,
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
