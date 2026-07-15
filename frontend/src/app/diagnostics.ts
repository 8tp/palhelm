import type { GameDataState } from "../api/types";
import type { PillTone } from "../components/Pill";

export interface Freshness {
  tone: PillTone;
  label: "Current" | "Aging" | "Stale" | "Unavailable";
  ageMs: number | null;
}

/** Classify an API timestamp without treating a missing or malformed value as current. */
export function classifyFreshness(
  iso: string | null | undefined,
  warnAfterMs: number,
  staleAfterMs: number,
  nowMs = Date.now(),
): Freshness {
  if (!iso) return { tone: "idle", label: "Unavailable", ageMs: null };
  const timestamp = new Date(iso).getTime();
  if (!Number.isFinite(timestamp) || timestamp <= 0) {
    return { tone: "idle", label: "Unavailable", ageMs: null };
  }
  const ageMs = Math.max(0, nowMs - timestamp);
  if (ageMs >= staleAfterMs) return { tone: "danger", label: "Stale", ageMs };
  if (ageMs >= warnAfterMs) return { tone: "warn", label: "Aging", ageMs };
  return { tone: "ok", label: "Current", ageMs };
}

export function gameDataStateTone(state: GameDataState): PillTone {
  if (state === "ready") return "ok";
  if (state === "stale" || state === "pending") return "warn";
  if (state === "unavailable" || state === "unauthorized") return "danger";
  return "idle";
}

export function gameDataStateDetail(state: GameDataState): string {
  switch (state) {
    case "ready": return "The shared Game Data poller has a usable snapshot.";
    case "stale": return "The last accepted snapshot is retained while the poller retries.";
    case "pending": return "Enabled; waiting for the first accepted snapshot.";
    case "disabled": return "The optional Game Data capability is disabled.";
    case "unsupported": return "The game server does not expose the required endpoint.";
    case "unauthorized": return "The configured game API credentials were rejected.";
    case "unavailable": return "No accepted snapshot is currently available.";
  }
}

export function linkCoverage(linked: number, unresolved: number): { value: string; detail: string } {
  const total = Math.max(0, linked) + Math.max(0, unresolved);
  if (total === 0) return { value: "Unavailable", detail: "No accepted base-Pal actors to link." };
  const percent = Math.round((Math.max(0, linked) / total) * 100);
  return { value: `${percent}%`, detail: `${Math.max(0, linked)} linked · ${Math.max(0, unresolved)} unresolved` };
}
