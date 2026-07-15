import type { EventKind, PalhelmEvent } from "../../api/types";

export type EventLane = "all" | "player" | "operations" | "health";
export type EventKindFilter = "all" | EventKind;

export interface EventLaneDefinition {
  id: EventLane;
  label: string;
  description: string;
  kinds: readonly EventKind[];
}

export const EVENT_LANES: readonly EventLaneDefinition[] = [
  { id: "all", label: "All events", description: "Combined timeline", kinds: ["join", "leave", "backup", "panel", "config", "system"] },
  { id: "player", label: "Player activity", description: "Joins and leaves", kinds: ["join", "leave"] },
  { id: "operations", label: "Operations & audit", description: "Backups and panel changes", kinds: ["backup", "panel", "config"] },
  { id: "health", label: "Health incidents", description: "System health transitions", kinds: ["system"] },
];

const laneByKind = new Map<EventKind, EventLane>([
  ["join", "player"],
  ["leave", "player"],
  ["backup", "operations"],
  ["panel", "operations"],
  ["config", "operations"],
  ["system", "health"],
]);

export function eventLaneFor(kind: EventKind): EventLane {
  return laneByKind.get(kind) ?? "all";
}

export function kindsForLane(lane: EventLane): readonly EventKind[] {
  return EVENT_LANES.find((item) => item.id === lane)?.kinds ?? [];
}

export function countEventLanes(events: readonly PalhelmEvent[]): Record<EventLane, number> {
  const counts: Record<EventLane, number> = { all: events.length, player: 0, operations: 0, health: 0 };
  for (const event of events) {
    const lane = eventLaneFor(event.kind);
    if (lane !== "all") counts[lane] += 1;
  }
  return counts;
}

export function filterEvents(
  events: readonly PalhelmEvent[],
  lane: EventLane,
  kind: EventKindFilter,
  search: string,
): PalhelmEvent[] {
  const needle = search.trim().toLocaleLowerCase();
  return events.filter((event) => {
    if (lane !== "all" && eventLaneFor(event.kind) !== lane) return false;
    if (kind !== "all" && event.kind !== kind) return false;
    if (!needle) return true;
    return event.message.toLocaleLowerCase().includes(needle) || event.kind.includes(needle);
  });
}
