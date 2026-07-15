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

export interface MapViewportSize {
  width: number;
  height: number;
}

export interface MapSearchTarget {
  key: string;
  kind: "player" | "base";
  label: string;
  detail: string;
  location: MapAnchor;
}

export interface SharedMapCoordinates {
  x: number;
  y: number;
  layerId: string | null;
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

function clampScale(scale: number, bounds: MapZoomBounds): number {
  return Math.min(bounds.max, Math.max(bounds.min, scale));
}

/** Center one map-space point without changing corrected coordinate transforms. */
export function centerMapPoint(
  point: MapAnchor,
  viewport: MapViewportSize,
  scale: number,
  bounds: MapZoomBounds,
): MapViewport {
  const nextScale = clampScale(scale, bounds);
  return {
    scale: nextScale,
    tx: viewport.width / 2 - point.x * nextScale,
    ty: viewport.height / 2 - point.y * nextScale,
  };
}

/** Fit finite map-space points into the viewport. A single point gets useful surrounding context. */
export function fitMapPoints(
  points: readonly MapAnchor[],
  viewport: MapViewportSize,
  bounds: MapZoomBounds,
  padding = 48,
): MapViewport | null {
  const finite = points.filter((point) => Number.isFinite(point.x) && Number.isFinite(point.y));
  if (finite.length === 0 || viewport.width <= 0 || viewport.height <= 0) return null;
  const minX = Math.min(...finite.map((point) => point.x));
  const maxX = Math.max(...finite.map((point) => point.x));
  const minY = Math.min(...finite.map((point) => point.y));
  const maxY = Math.max(...finite.map((point) => point.y));
  const spanX = maxX - minX;
  const spanY = maxY - minY;
  const usableWidth = Math.max(1, viewport.width - padding * 2);
  const usableHeight = Math.max(1, viewport.height - padding * 2);
  const fittedScale = spanX === 0 && spanY === 0
    ? bounds.min * 4
    : Math.min(spanX > 0 ? usableWidth / spanX : Number.POSITIVE_INFINITY, spanY > 0 ? usableHeight / spanY : Number.POSITIVE_INFINITY);
  return centerMapPoint(
    { x: (minX + maxX) / 2, y: (minY + maxY) / 2 },
    viewport,
    fittedScale,
    bounds,
  );
}

export function filterMapSearchTargets(targets: readonly MapSearchTarget[], search: string, limit = 8): MapSearchTarget[] {
  const needle = search.trim().toLocaleLowerCase();
  if (!needle) return [];
  return targets
    .filter((target) => `${target.label} ${target.detail}`.toLocaleLowerCase().includes(needle))
    .sort((a, b) => {
      const aLabel = a.label.toLocaleLowerCase();
      const bLabel = b.label.toLocaleLowerCase();
      const aRank = aLabel === needle ? 0 : aLabel.startsWith(needle) ? 1 : 2;
      const bRank = bLabel === needle ? 0 : bLabel.startsWith(needle) ? 1 : 2;
      return aRank - bRank || a.label.localeCompare(b.label) || a.key.localeCompare(b.key);
    })
    .slice(0, Math.max(0, limit));
}

/** Parse only display coordinates and a bounded layer id; shared links never carry identity data. */
export function parseSharedMapCoordinates(search: string): SharedMapCoordinates | null {
  const params = new URLSearchParams(search);
  const rawX = params.get("x");
  const rawY = params.get("y");
  if (rawX === null || rawY === null) return null;
  const x = Number(rawX);
  const y = Number(rawY);
  if (!Number.isFinite(x) || !Number.isFinite(y)) return null;
  const rawLayer = params.get("layer");
  const layerId = rawLayer && /^[a-zA-Z0-9_-]{1,64}$/.test(rawLayer) ? rawLayer : null;
  return { x: Math.round(x), y: Math.round(y), layerId };
}

export function buildSharedMapURL(href: string, coordinates: MapAnchor, layerId: string): string {
  const url = new URL(href);
  url.searchParams.set("x", String(Math.round(coordinates.x)));
  url.searchParams.set("y", String(Math.round(coordinates.y)));
  url.searchParams.set("layer", layerId);
  return url.toString();
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
