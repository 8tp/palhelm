export interface MapViewport {
  scale: number;
  tx: number;
  ty: number;
}

export interface MapZoomBounds {
  min: number;
  max: number;
}

export interface MapAnchor {
  x: number;
  y: number;
}

export const DEFAULT_MAP_LAYERS = Object.freeze({
  Players: true,
  Bases: true,
  Workers: false,
  PalBoxes: false,
});

/** Zoom around a screen-space anchor while keeping the map point beneath it fixed. */
export function zoomMapView(
  view: MapViewport,
  factor: number,
  anchor: MapAnchor,
  bounds: MapZoomBounds,
): MapViewport {
  const next = Math.min(bounds.max, Math.max(bounds.min, view.scale * factor));
  const ratio = next / view.scale;
  return {
    scale: next,
    tx: anchor.x - (anchor.x - view.tx) * ratio,
    ty: anchor.y - (anchor.y - view.ty) * ratio,
  };
}

export function wheelZoomFactor(deltaY: number): number {
  return deltaY < 0 ? 1.25 : 0.8;
}

/**
 * Installs a non-passive listener so wheel and Ctrl/trackpad zoom are contained by the map.
 * The listener exists only on the map viewport, leaving normal page scrolling elsewhere alone.
 */
export function addContainedMapWheelListener(
  target: HTMLElement,
  onWheel: (event: WheelEvent) => void,
): () => void {
  const handleWheel = (event: WheelEvent) => {
    event.preventDefault();
    event.stopPropagation();
    onWheel(event);
  };
  target.addEventListener("wheel", handleWheel, { passive: false });
  return () => target.removeEventListener("wheel", handleWheel);
}
