/** Selects the least tile-pyramid zoom whose native pixels cover the displayed map. */
export function tileZoomForScale(scale: number, mapSize: number, tileSize: number, minZoom: number, maxZoom: number): number {
  const nativeZoom = Math.ceil(Math.log2((scale * mapSize) / tileSize));
  return Math.max(minZoom, Math.min(maxZoom, nativeZoom));
}
