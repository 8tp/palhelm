import type { LiveWorldSnapshot, Player } from "../api/types";

export interface PlayerMarker {
  key: string;
  name: string;
  location: { x: number; y: number };
}

export interface PlayerMarkerSelection {
  markers: PlayerMarker[];
  usedLive: boolean;
}

function finiteLocation(location: { x: number; y: number } | null | undefined): location is { x: number; y: number } {
  return location !== null && location !== undefined && Number.isFinite(location.x) && Number.isFinite(location.y);
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
