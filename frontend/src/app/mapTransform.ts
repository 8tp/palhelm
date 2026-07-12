// World → map transform (docs/ARCHITECTURE.md "Map"): linear interpolation of UE world
// coordinates (cm) over the extents below onto a 256-unit square (PST-compatible, Leaflet
// CRS.Simple convention: map y grows downward / southward). Zoom levels 0–6.

export const WORLD_MIN_X = -999940;
export const WORLD_MIN_Y = -738920;
export const WORLD_MAX_X = 447900;
export const WORLD_MAX_Y = 708920;
export const MAP_SIZE = 256;
export const MIN_ZOOM = 0;
export const MAX_ZOOM = 6;

export interface MapPoint {
  x: number;
  y: number;
}

/** UE world cm → legacy map units on the 256-square. */
export function worldToMap(worldX: number, worldY: number): MapPoint {
  const u = (worldX - WORLD_MIN_X) / (WORLD_MAX_X - WORLD_MIN_X);
  const v = (worldY - WORLD_MIN_Y) / (WORLD_MAX_Y - WORLD_MIN_Y);
  return { x: v * MAP_SIZE, y: (1 - u) * MAP_SIZE };
}

/** Inverse of worldToMap — used by fixtures to place mock players at chosen map spots. */
export function mapToWorld(mapX: number, mapY: number): MapPoint {
  const v = mapX / MAP_SIZE;
  const u = 1 - mapY / MAP_SIZE;
  return {
    x: WORLD_MIN_X + u * (WORLD_MAX_X - WORLD_MIN_X),
    y: WORLD_MIN_Y + v * (WORLD_MAX_Y - WORLD_MIN_Y),
  };
}

/**
 * UE data coordinates → the coordinates displayed on Palworld's map. Palworld
 * deliberately flips the axes: display X comes from data Y, display Y from data X.
 * Constants come from WorldMapUIData/DT_WorldMapUIData.
 */
export function worldToGame(worldX: number, worldY: number): MapPoint {
  return { x: Math.round((worldY - 158000) / 459), y: Math.round((worldX + 123888) / 459) };
}

/** In-game display coords → UE world cm (fixture helper). */
export function gameToWorld(gameX: number, gameY: number): MapPoint {
  return { x: gameY * 459 - 123888, y: gameX * 459 + 158000 };
}

// ---------- per-layer transform (THGL tile datasets, Palworld 1.0+) ----------
//
// THGL's own tile config (cdn.th.gl/palworld/config/tiles.json) embeds a Leaflet-style
// L.Transformation(a, b, c, d) per layer: tilePixel(zoom) = 2^zoom * (a*dataY + b),
// 2^zoom * (c*dataX + d), where the canvas at native zoom 0 is exactly `tileSize` square.
// THGL follows Leaflet's longitude/latitude ordering: horizontal input is Palworld data Y and
// vertical input is data X. The upstream b/d offsets are authoritative and must not be
// re-derived from bounds. This is a *different, per-layer*
// replacement for worldToMap, not a variant of it: the legacy worldToMap was hand-tuned to the
// (now-stale) palworld.gg pyramid's orientation, while this one is read directly from the new
// provider's own config, so it needs no rotation/mirroring to line up with that provider's tile
// images.
export interface LayerTransform {
  a: number;
  b: number;
  c: number;
  d: number;
}

/** UE world cm → map units on the MAP_SIZE-square, for a THGL-style per-layer transform. */
export function worldToLayerMap(worldX: number, worldY: number, t: LayerTransform, tileSize: number): MapPoint {
  return { x: ((t.a * worldY + t.b) / tileSize) * MAP_SIZE, y: ((t.c * worldX + t.d) / tileSize) * MAP_SIZE };
}

/** Inverse of worldToLayerMap — used for the cursor-position readout on THGL-transformed layers. */
export function layerMapToWorld(mapX: number, mapY: number, t: LayerTransform, tileSize: number): MapPoint {
  const pixelX = (mapX / MAP_SIZE) * tileSize;
  const pixelY = (mapY / MAP_SIZE) * tileSize;
  return { x: (pixelY - t.d) / t.c, y: (pixelX - t.b) / t.a };
}

/** Whether a world-cm point falls within a layer's published bounds (with a small margin). */
export function worldInBounds(worldX: number, worldY: number, bounds: [[number, number], [number, number]]): boolean {
  const [[x0, y0], [x1, y1]] = bounds;
  const minX = Math.min(x0, x1);
  const maxX = Math.max(x0, x1);
  const minY = Math.min(y0, y1);
  const maxY = Math.max(y0, y1);
  return worldY >= minX && worldY <= maxX && worldX >= minY && worldX <= maxY;
}
