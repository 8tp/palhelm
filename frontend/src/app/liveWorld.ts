import type { LiveWorldActor, LiveWorldSnapshot, Player } from "../api/types";

export interface PlayerMarker {
  key: string;
  name: string;
  location: { x: number; y: number };
}

export interface PlayerMarkerSelection {
  markers: PlayerMarker[];
  usedLive: boolean;
}

export interface LiveMapActorSelection {
  available: boolean;
  workers: LiveWorldActor[];
  palBoxes: LiveWorldActor[];
}

function finiteLocation(location: { x: number; y: number } | null | undefined): location is { x: number; y: number } {
  return location !== null && location !== undefined && Number.isFinite(location.x) && Number.isFinite(location.y);
}

/**
 * Selects exact live-only map layers from a complete current snapshot.
 *
 * Worker and PalBox positions have no REST/save coordinate fallback, so stale or truncated
 * snapshots must hide the entire layer instead of presenting partial or retained positions as
 * current. Workers additionally require the exact save-derived instance/base join.
 */
export function selectLiveMapActors(snapshot: LiveWorldSnapshot | undefined): LiveMapActorSelection {
  if (snapshot?.state !== "ready" || snapshot.truncated) {
    return { available: false, workers: [], palBoxes: [] };
  }

  return {
    available: true,
    workers: snapshot.actors.filter(
      (actor) =>
        actor.kind === "BaseCampPal" &&
        actor.linked === true &&
        Boolean(actor.instanceId) &&
        Boolean(actor.baseId) &&
        finiteLocation(actor.location),
    ),
    palBoxes: snapshot.actors.filter((actor) => actor.kind === "PalBox" && finiteLocation(actor.location)),
  };
}

/** A base worker counts as "in danger" when it is knocked out or critically low on HP. The map
 * keeps this visible on the worker's own chip and propagates it to any cluster it collapses into,
 * so an overview never hides a base that needs attention. Unknown HP is not treated as danger. */
export function isWorkerInDanger(worker: LiveWorldActor): boolean {
  return worker.activity === "incapacitated" || (worker.hpPercent !== undefined && worker.hpPercent < 25);
}

/** Plain-English summary for a group of clustered base workers, e.g. "12 workers · 2 hurt". The
 * hurt count is only appended when at least one worker is in danger, keeping healthy bases quiet. */
export function summarizeWorkerCluster(workers: readonly LiveWorldActor[]): { label: string; hurt: number; danger: boolean } {
  const hurt = workers.filter(isWorkerInDanger).length;
  const label = hurt > 0 ? `${workers.length} workers · ${hurt} hurt` : `${workers.length} workers`;
  return { label, hurt, danger: hurt > 0 };
}

/**
 * Reconciles transient game-data coordinates onto the authoritative REST roster.
 *
 * Game data is intentionally never a roster: the endpoint reports only loaded actors and can
 * be partial during streaming. We therefore preserve every online REST player, accept only a
 * complete current snapshot, and replace a coordinate only when one active actor has an exact,
 * unique name match. Extra or ambiguous actors are ignored.
 */
export function selectPlayerMarkers(players: Player[], snapshot: LiveWorldSnapshot | undefined): PlayerMarkerSelection {
  const markers = players
    .filter((player) => player.online && finiteLocation(player.location))
    .map((player) => ({ key: player.uid, name: player.name, location: player.location! }));

  if (snapshot?.state !== "ready" || snapshot.truncated) return { markers, usedLive: false };

  const byName = new Map<string, Array<{ x: number; y: number }>>();
  for (const actor of snapshot.actors) {
    if (actor.kind !== "Player" || actor.active !== true || !actor.name || !finiteLocation(actor.location)) continue;
    const locations = byName.get(actor.name) ?? [];
    locations.push(actor.location);
    byName.set(actor.name, locations);
  }

  const rosterNameCounts = new Map<string, number>();
  for (const marker of markers) rosterNameCounts.set(marker.name, (rosterNameCounts.get(marker.name) ?? 0) + 1);

  let usedLive = false;
  const reconciled = markers.map((marker) => {
    const matches = byName.get(marker.name);
    if (rosterNameCounts.get(marker.name) !== 1 || matches?.length !== 1) return marker;
    usedLive = true;
    return { ...marker, location: { x: matches[0].x, y: matches[0].y } };
  });
  return { markers: reconciled, usedLive };
}
