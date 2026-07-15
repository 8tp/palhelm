import { useCallback, useEffect, useMemo, useRef, useState } from "react";
import { useQuery } from "@tanstack/react-query";
import { api, USE_MOCK } from "../../api/client";
import type { MapDataset, MapDatasetLayer } from "../../api/types";
import {
  layerMapToWorld,
  MAP_SIZE,
  mapToWorld,
  worldInBounds,
  worldToGame,
  worldToLayerMap,
  worldToMap,
  type LayerTransform,
  type MapPoint,
} from "../../app/mapTransform";
import { formatRelativeToNow, formatWorldGuid } from "../../app/format";
import { tileZoomForScale } from "../../app/mapTiles";
import { addContainedMapWheelListener, DEFAULT_MAP_LAYERS, wheelZoomFactor, zoomMapView } from "../../app/mapInteraction";
import { selectLiveMapActors, selectPlayerMarkers } from "../../app/liveWorld";
import { Card, CardBody, CardHead } from "../../components/Card";
import { EmptyState } from "../../components/EmptyState";
import { ToggleChip } from "../../components/ToggleChip";
import { CodeWell } from "../../components/CodeWell";
import { Tooltip } from "../../components/Tooltip";
import {
  IconFitView,
  IconMapBase,
  IconMapEmpty,
  IconMapPalBox,
  IconMapPlayer,
  IconMapWorker,
  IconZoomIn,
  IconZoomOut,
} from "../../components/icons";
import "./Map.css";

type TileState = "checking" | "tiles" | "mockgrid" | "missing";

// Resolved, UI-ready view of one tile pyramid — either the legacy flat pre-1.0 pyramid (the
// fallback when dataset.json is absent, matching today's live data dir) or a layer reported by
// GET /api/v1/map/dataset (e.g. THGL's "default"/Palpagos, "tree"/World Tree).
interface ResolvedLayer {
  id: string;
  label: string;
  format: "png" | "webp";
  path: string; // "" for the legacy flat layout: /map-tiles/{z}/{x}/{y}.ext
  tileSize: number;
  minZoom: number;
  maxZoom: number;
  transform: LayerTransform | null;
  bounds: [[number, number], [number, number]] | null;
}

const LEGACY_LAYER: ResolvedLayer = {
  id: "legacy",
  label: "Map",
  format: "png",
  path: "",
  tileSize: 256,
  minZoom: 0,
  maxZoom: 6,
  transform: null,
  bounds: null,
};

function resolveLayers(dataset: MapDataset | undefined): ResolvedLayer[] {
  const layers = dataset?.layers ?? [];
  if (layers.length === 0) return [LEGACY_LAYER];
  return layers.map((l: MapDatasetLayer) => ({
    id: l.id,
    label: l.label || l.id,
    format: l.format ?? "png",
    path: l.path,
    tileSize: l.tile_size ?? 256,
    minZoom: l.min_zoom,
    maxZoom: l.max_zoom,
    transform: l.transform ?? null,
    bounds: l.bounds ?? null,
  }));
}

function tileUrl(layer: ResolvedLayer, z: number, x: number, y: number) {
  const prefix = layer.path ? `/map-tiles/${layer.path}` : "/map-tiles";
  return `${prefix}/${z}/${x}/${y}.${layer.format}`;
}

/** World cm → map units on the active layer's MAP_SIZE-square (per-layer transform when the
 * dataset supplies one — see docs/ROADMAP-v2.md for THGL's coordinate semantics — falling
 * back to the legacy hand-tuned palworld.gg transform otherwise). */
function layerWorldToMap(layer: ResolvedLayer, worldX: number, worldY: number): MapPoint {
  return layer.transform ? worldToLayerMap(worldX, worldY, layer.transform, layer.tileSize) : worldToMap(worldX, worldY);
}

/** Inverse of layerWorldToMap, for the cursor coordinate readout. */
function layerMapToWorldFor(layer: ResolvedLayer, mapX: number, mapY: number): MapPoint {
  return layer.transform ? layerMapToWorld(mapX, mapY, layer.transform, layer.tileSize) : mapToWorld(mapX, mapY);
}

/** True if a world-cm point should render on this layer — always true for the legacy layer
 * (no bounds recorded) or a layer whose dataset entry didn't publish bounds; otherwise markers
 * outside a layer's mapped extent (e.g. players on Palpagos while viewing the World Tree) are
 * hidden rather than drawn in the wrong place. */
function onLayer(layer: ResolvedLayer, worldX: number, worldY: number): boolean {
  return !layer.bounds || worldInBounds(worldX, worldY, layer.bounds);
}

interface View {
  scale: number; // screen px per map unit
  tx: number; // screen-space translation
  ty: number;
}

export default function MapRoute() {
  const [layers, setLayers] = useState<Record<string, boolean>>(() => ({ ...DEFAULT_MAP_LAYERS }));
  const [tileState, setTileState] = useState<TileState>("checking");
  const [view, setView] = useState<View | null>(null);
  const [cursorGame, setCursorGame] = useState<{ x: number; y: number } | null>(null);
  const [activeLayerId, setActiveLayerId] = useState<string | null>(null);
  const wellRef = useRef<HTMLDivElement>(null);
  const dragRef = useRef<{ startX: number; startY: number; tx: number; ty: number } | null>(null);

  const serverQuery = useQuery({ queryKey: ["server"], queryFn: () => api.server.get() });
  const healthQuery = useQuery({ queryKey: ["server", "health"], queryFn: () => api.server.health() });
  const playersQuery = useQuery({ queryKey: ["players"], queryFn: () => api.players.list(), refetchInterval: 30000 });
  const guildsQuery = useQuery({ queryKey: ["guilds"], queryFn: () => api.guilds.list() });
  const datasetQuery = useQuery({ queryKey: ["map", "dataset"], queryFn: () => api.map.dataset() });
  const worldSnapshotQuery = useQuery({
    queryKey: ["world", "snapshot"],
    queryFn: () => api.world.snapshot(),
    refetchInterval: 30000,
  });

  const mockTiles = typeof window !== "undefined" && new URLSearchParams(window.location.search).has("mocktiles");

  // The tile pyramid(s) available to render: whatever GET /api/v1/map/dataset reports, or a
  // single legacy flat layer when the dataset is pre-1.0 / not yet fetched. React Query keeps
  // datasetQuery.data referentially stable across unchanged refetches, so this only recomputes
  // when the dataset actually changes.
  const availableLayers = useMemo(() => resolveLayers(datasetQuery.data), [datasetQuery.data]);
  const isPreV1 = (datasetQuery.data?.game_version ?? "pre-1.0") !== "1.0";

  // Default (or repair) the active layer selection whenever the available layer set changes.
  useEffect(() => {
    if (availableLayers.length === 0) return;
    if (activeLayerId === null || !availableLayers.some((l) => l.id === activeLayerId)) {
      setActiveLayerId(availableLayers[0].id);
    }
  }, [availableLayers, activeLayerId]);

  const activeLayer = availableLayers.find((l) => l.id === activeLayerId) ?? availableLayers[0] ?? LEGACY_LAYER;

  // Decide whether tiles exist: mock mode renders the designed empty state unless ?mocktiles;
  // real mode probes the active layer's z0 tile and falls back to the empty state on 404. Waits
  // for the dataset fetch to settle first so it probes the real layer, not a legacy guess.
  useEffect(() => {
    if (USE_MOCK) {
      setTileState(mockTiles ? "mockgrid" : "missing");
      return;
    }
    if (datasetQuery.isLoading) return;
    let cancelled = false;
    const img = new Image();
    img.onload = () => !cancelled && setTileState("tiles");
    img.onerror = () => !cancelled && setTileState(mockTiles ? "mockgrid" : "missing");
    img.src = tileUrl(activeLayer, 0, 0, 0);
    return () => {
      cancelled = true;
    };
  }, [mockTiles, datasetQuery.isLoading, activeLayer]);

  // Fit the 256-square to the well on first layout.
  const fitView = useCallback((): View | null => {
    const el = wellRef.current;
    if (!el) return null;
    const { clientWidth: w, clientHeight: h } = el;
    if (w === 0 || h === 0) return null;
    const scale = Math.min(w, h) / MAP_SIZE;
    return { scale, tx: (w - MAP_SIZE * scale) / 2, ty: (h - MAP_SIZE * scale) / 2 };
  }, []);

  useEffect(() => {
    if (tileState !== "tiles" && tileState !== "mockgrid") return;
    if (view === null) {
      const v = fitView();
      if (v) setView(v);
    }
  }, [tileState, view, fitView]);

  const scaleBounds = useCallback((): { min: number; max: number } => {
    const el = wellRef.current;
    const min = el ? (Math.min(el.clientWidth, el.clientHeight) / MAP_SIZE) * Math.pow(2, activeLayer.minZoom) : 1;
    return { min, max: min * Math.pow(2, activeLayer.maxZoom) };
  }, [activeLayer.minZoom, activeLayer.maxZoom]);

  const zoomAt = useCallback((factor: number, cx?: number, cy?: number) => {
    setView((v) => {
      if (!v) return v;
      const el = wellRef.current;
      const px = cx ?? (el ? el.clientWidth / 2 : 0);
      const py = cy ?? (el ? el.clientHeight / 2 : 0);
      return zoomMapView(v, factor, { x: px, y: py }, scaleBounds());
    });
  }, [scaleBounds]);

  const resetView = useCallback(() => {
    const next = fitView();
    if (next) setView(next);
  }, [fitView]);

  useEffect(() => {
    const el = wellRef.current;
    const hasInteractiveMap = tileState === "tiles" || tileState === "mockgrid";
    if (!el || !hasInteractiveMap) return;
    return addContainedMapWheelListener(el, (event) => {
      const rect = el.getBoundingClientRect();
      zoomAt(wheelZoomFactor(event.deltaY), event.clientX - rect.left, event.clientY - rect.top);
    });
  }, [tileState, zoomAt]);

  function onPointerDown(e: React.PointerEvent) {
    if (!view) return;
    if ((e.target as Element).closest("button, a, input, select, textarea")) return;
    (e.target as Element).setPointerCapture?.(e.pointerId);
    dragRef.current = { startX: e.clientX, startY: e.clientY, tx: view.tx, ty: view.ty };
  }
  function onPointerMove(e: React.PointerEvent) {
    const el = wellRef.current;
    if (el && view) {
      const rect = el.getBoundingClientRect();
      const mx = (e.clientX - rect.left - view.tx) / view.scale;
      const my = (e.clientY - rect.top - view.ty) / view.scale;
      if (mx >= 0 && my >= 0 && mx <= MAP_SIZE && my <= MAP_SIZE) {
        // Cursor readout is always Palworld's own in-game display coordinate (tile-imagery
        // independent), reached by inverting whichever transform placed this pixel — legacy or
        // per-layer — into UE world cm and then applying the fixed world->game-display formula.
        const w = layerMapToWorldFor(activeLayer, mx, my);
        setCursorGame(worldToGame(w.x, w.y));
      }
    }
    const d = dragRef.current;
    if (d) {
      setView((v) => (v ? { ...v, tx: d.tx + (e.clientX - d.startX), ty: d.ty + (e.clientY - d.startY) } : v));
    }
  }
  function onPointerUp() {
    dragRef.current = null;
  }
  // Tile zoom level for the current scale: enough resolution that one tile pixel ≥ one screen pixel.
  const tileZ = view
    ? tileZoomForScale(view.scale, MAP_SIZE, activeLayer.tileSize, activeLayer.minZoom, activeLayer.maxZoom)
    : 0;

  // TanStack deliberately retains the previous successful data on a refetch failure. Exact
  // coordinates must fail back to REST instead of presenting that retained `ready` document as
  // current when Palhelm could not refresh its freshness state.
  const liveSnapshot = worldSnapshotQuery.isError || worldSnapshotQuery.isRefetchError ? undefined : worldSnapshotQuery.data;
  const playerMarkerSelection = selectPlayerMarkers(playersQuery.data ?? [], liveSnapshot);
  const playerMarkers = playerMarkerSelection.markers;
  const bases = (guildsQuery.data ?? []).flatMap((g) => g.bases.map((b) => ({ ...b, guildName: g.name })));
  const liveMapActors = selectLiveMapActors(liveSnapshot);
  const workers = liveMapActors.workers;
  const palBoxes = liveMapActors.palBoxes;
  const baseHealth = useMemo(() => {
    const grouped = new Map<string, typeof workers>();
    for (const worker of workers) {
      const current = grouped.get(worker.baseId!) ?? [];
      current.push(worker);
      grouped.set(worker.baseId!, current);
    }
    return [...grouped.entries()].map(([baseId, members]) => ({
      baseId,
      name: bases.find((base) => base.id === baseId)?.guildName ?? `Base ${baseId.slice(0, 8)}`,
      members,
      lowHP: members.filter((worker) => worker.hpPercent !== undefined && worker.hpPercent < 25).length,
      incapacitated: members.filter((worker) => worker.activity === "incapacitated").length,
      idle: members.filter((worker) => worker.activity === "idle" || worker.activity === "inactive").length,
    }));
  }, [workers, bases]);

  const toScreen = useCallback(
    (mapX: number, mapY: number) => {
      if (!view) return { x: 0, y: 0 };
      return { x: view.tx + mapX * view.scale, y: view.ty + mapY * view.scale };
    },
    [view],
  );

  const hasMap = tileState === "tiles" || tileState === "mockgrid";

  return (
    <main className="content">
      <div className="page-head">
        <h1>Live map</h1>
        <span className="sub" title={serverQuery.data?.worldGuid}>
          {serverQuery.data ? `world ${formatWorldGuid(serverQuery.data.worldGuid)}` : "world positions from save data"}
        </span>
      </div>

      <Card className="map-card">
        <CardHead title="World map" hint={playerMarkerSelection.usedLive ? "live positions from Palworld game data" : "positions from REST/save data"}>
          {playerMarkerSelection.usedLive && liveSnapshot?.capturedAt ? (
            <span className="hint">live snapshot {formatRelativeToNow(liveSnapshot.capturedAt)}</span>
          ) : healthQuery.data ? (
            <span className="hint">synced {formatRelativeToNow(healthQuery.data.save.lastSyncAt)}</span>
          ) : null}
        </CardHead>

        <div
          ref={wellRef}
          className={`map-well${hasMap ? " pannable" : ""}`}
          aria-label="World map"
          onPointerDown={hasMap ? onPointerDown : undefined}
          onPointerMove={hasMap ? onPointerMove : undefined}
          onPointerUp={hasMap ? onPointerUp : undefined}
          onPointerLeave={hasMap ? onPointerUp : undefined}
        >
          {hasMap && (
            <div className="map-toggles" role="group" aria-label="Map layers">
              <ToggleChip
                pressed={layers.Players ?? false}
                onClick={() => setLayers((l) => ({ ...l, Players: !l.Players }))}
                count={playerMarkers.length}
              >
                Players
              </ToggleChip>
              <ToggleChip
                pressed={layers.Bases ?? false}
                onClick={() => setLayers((l) => ({ ...l, Bases: !l.Bases }))}
                count={bases.length}
              >
                Bases
              </ToggleChip>
              <ToggleChip
                pressed={layers.Workers ?? false}
                onClick={() => setLayers((l) => ({ ...l, Workers: !l.Workers }))}
                count={workers.length}
              >
                Workers
              </ToggleChip>
              <ToggleChip
                pressed={layers.PalBoxes ?? false}
                onClick={() => setLayers((l) => ({ ...l, PalBoxes: !l.PalBoxes }))}
                count={palBoxes.length}
              >
                PalBoxes
              </ToggleChip>
              {isPreV1 && <span className="stamp stamp-warn stamp-tilt">Map data: pre-1.0</span>}
              {liveSnapshot?.state === "stale" && <span className="stamp stamp-warn">Live data stale</span>}
              {liveSnapshot?.state === "unsupported" && <span className="stamp stamp-warn">Game data unavailable</span>}
              {liveSnapshot?.state === "unauthorized" && <span className="stamp stamp-warn">Game data unauthorized</span>}
              {liveSnapshot?.state === "unavailable" && <span className="stamp stamp-warn">Game data unavailable</span>}
              {liveSnapshot?.truncated && <span className="stamp stamp-warn">Live data incomplete</span>}
            </div>
          )}

          {hasMap && availableLayers.length > 1 && (
            <div className="map-toggles map-layer-toggles" role="group" aria-label="Map tile layer">
              {availableLayers.map((l) => (
                <ToggleChip key={l.id} pressed={activeLayer.id === l.id} onClick={() => setActiveLayerId(l.id)}>
                  {l.label}
                </ToggleChip>
              ))}
            </div>
          )}

          {tileState === "missing" && (
            <div className="map-empty-fill">
              <EmptyState icon={<IconMapEmpty />} title="Map tiles not installed">
                <p>
                  Live map rendering needs terrain tiles derived from the game's own assets. These are copyrighted by
                  Pocketpair and are not shipped with Palhelm — generate them once from your server's install.
                </p>
                <CodeWell>docker exec palhelm palhelm fetch-map-tiles</CodeWell>
                <div style={{ marginTop: "var(--space-2)" }}>
                  <button type="button" className="btn btn-ghost btn-sm">
                    Learn more
                  </button>
                </div>
              </EmptyState>
            </div>
          )}

          {hasMap && view && (
            <>
              {/* map-space layer: tiles or the plain mock grid */}
              <div
                className="map-layer"
                style={{ transform: `translate(${view.tx}px, ${view.ty}px) scale(${view.scale})` }}
                aria-hidden="true"
              >
                {tileState === "mockgrid" ? (
                  <div className="map-grid" style={{ width: MAP_SIZE, height: MAP_SIZE }} />
                ) : (
                  <TileGrid layer={activeLayer} z={tileZ} onTileError={() => setTileState("missing")} />
                )}
              </div>

              {/* screen-space markers (chips stay crisp and unscaled) */}
              {layers.Bases &&
                bases
                  .filter((b) => onLayer(activeLayer, b.location.x, b.location.y))
                  .map((b) => {
                    const m = layerWorldToMap(activeLayer, b.location.x, b.location.y);
                    const s = toScreen(m.x, m.y);
                    return (
                      <div key={b.id} className="marker marker-base" style={{ left: s.x, top: s.y }}>
                        <span className="marker-symbol"><IconMapBase /></span>
                        <span className="chip">{b.guildName}</span>
                      </div>
                    );
                  })}
              {layers.Players &&
                playerMarkers
                  .filter((p) => onLayer(activeLayer, p.location.x, p.location.y))
                  .map((p) => {
                    const m = layerWorldToMap(activeLayer, p.location.x, p.location.y);
                    const s = toScreen(m.x, m.y);
                    return (
                      <div key={p.key} className="marker marker-player" style={{ left: s.x, top: s.y }}>
                        <span className="marker-symbol"><IconMapPlayer /></span>
                        <span className="chip">{p.name}</span>
                      </div>
                    );
                  })}
              {layers.Workers &&
                workers
                  .filter((worker) => onLayer(activeLayer, worker.location.x, worker.location.y))
                  .map((worker) => {
                    const m = layerWorldToMap(activeLayer, worker.location.x, worker.location.y);
                    const s = toScreen(m.x, m.y);
                    const danger = worker.activity === "incapacitated" || (worker.hpPercent !== undefined && worker.hpPercent < 25);
                    return (
                      <div key={worker.instanceId} className={`marker marker-worker${danger ? " danger" : ""}`} style={{ left: s.x, top: s.y }}>
                        <span className="marker-symbol"><IconMapWorker /></span>
                        <span className="chip">{worker.name || worker.characterId || "Pal"} · {worker.activity}</span>
                      </div>
                    );
                  })}
              {layers.PalBoxes &&
                palBoxes
                  .filter((box) => onLayer(activeLayer, box.location.x, box.location.y))
                  .map((box, index) => {
                    const m = layerWorldToMap(activeLayer, box.location.x, box.location.y);
                    const s = toScreen(m.x, m.y);
                    return (
                      <div key={`${box.guildName ?? "palbox"}-${index}`} className="marker marker-palbox" style={{ left: s.x, top: s.y }}>
                        <span className="marker-symbol"><IconMapPalBox /></span>
                        <span className="chip">{box.guildName || "Palbox"}</span>
                      </div>
                    );
                  })}

              <div className="map-zoom">
                <Tooltip label="Zoom in" side="right">
                  <button type="button" aria-label="Zoom in" onClick={() => zoomAt(1.5)}>
                    <IconZoomIn />
                  </button>
                </Tooltip>
                <Tooltip label="Zoom out" side="right">
                  <button type="button" aria-label="Zoom out" onClick={() => zoomAt(1 / 1.5)}>
                    <IconZoomOut />
                  </button>
                </Tooltip>
                <Tooltip label="Fit map" side="right">
                  <button type="button" aria-label="Fit map" onClick={resetView}>
                    <IconFitView />
                  </button>
                </Tooltip>
              </div>

              <div className="map-coord">{cursorGame ? `${cursorGame.x}, ${cursorGame.y}` : "—, —"}</div>
            </>
          )}
        </div>
      </Card>
      {liveMapActors.available && liveSnapshot && (
        <Card>
          <CardHead title="Live base health" hint="exact save-linked workers only">
            <span className="hint">{liveSnapshot.diagnostics.unresolvedBasePals} unresolved</span>
          </CardHead>
          <CardBody>
            {baseHealth.length === 0 ? (
              <p className="hint">No exact-linked live base workers are currently loaded.</p>
            ) : (
              <div className="base-health-grid">
                {baseHealth.map((base) => (
                  <div className="base-health-item" key={base.baseId}>
                    <strong>{base.name}</strong>
                    <span>{base.members.length} loaded · {base.idle} idle · {base.lowHP} low HP · {base.incapacitated} incapacitated</span>
                  </div>
                ))}
              </div>
            )}
          </CardBody>
        </Card>
      )}
    </main>
  );
}

/** Renders the full tile pyramid level `z` of `layer` in map space (each level covers the
 * 256-square). */
function TileGrid({ layer, z, onTileError }: { layer: ResolvedLayer; z: number; onTileError: () => void }) {
  const n = Math.pow(2, z);
  const size = MAP_SIZE / n;
  const tiles = [];
  for (let x = 0; x < n; x++) {
    for (let y = 0; y < n; y++) {
      tiles.push(
        <img
          key={`${layer.id}/${z}/${x}/${y}`}
          src={tileUrl(layer, z, x, y)}
          alt=""
          width={size}
          height={size}
          style={{ left: x * size, top: y * size, width: size, height: size }}
          onError={onTileError}
          draggable={false}
        />,
      );
    }
  }
  return <>{tiles}</>;
}
