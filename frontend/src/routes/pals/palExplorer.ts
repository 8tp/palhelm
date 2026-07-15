import type { PalExplorerPal, PalExplorerParams } from "../../api/types";

export const PAL_EXPLORER_PAGE_SIZE = 48;
export const PAL_EXPLORER_CLIENT_CAP = 480;

export interface PalExplorerFilterState {
  q: string;
  ownerSource: string;
  placement: string;
  specimen: string;
  minLevel: string;
  maxLevel: string;
}

/** Convert form strings to the API's narrow query contract without sending empty values. */
export function palExplorerParams(state: PalExplorerFilterState): PalExplorerParams {
  const params: PalExplorerParams = {};
  const q = state.q.trim();
  if (q) params.q = q;
  if (isOneOf(state.ownerSource, "save", "personal_container", "last_observed", "unresolved")) {
    params.ownerSource = state.ownerSource;
  }
  if (isOneOf(state.placement, "party", "box", "base", "unknown")) {
    params.placement = state.placement;
  }
  if (isOneOf(state.specimen, "standard", "alpha", "lucky", "boss")) {
    params.specimen = state.specimen;
  }
  const minLevel = boundedLevel(state.minLevel);
  const maxLevel = boundedLevel(state.maxLevel);
  if (minLevel !== undefined) params.minLevel = minLevel;
  if (maxLevel !== undefined) params.maxLevel = maxLevel;
  return params;
}

export function palOwnerSummary(pal: Pick<PalExplorerPal, "ownerName" | "ownerResolved" | "ownerSource">): string {
  if (!pal.ownerResolved || !pal.ownerName) return "Owner unavailable";
  switch (pal.ownerSource) {
    case "personal_container":
      return `${pal.ownerName} · current personal container`;
    case "last_observed":
      return `${pal.ownerName} · last observed owner`;
    case "save":
      return `${pal.ownerName} · save owner`;
    default:
      return pal.ownerName;
  }
}

export function palSpecimenLabels(pal: Pick<PalExplorerPal, "isAlpha" | "isLucky" | "isBoss">): string[] {
  const labels: string[] = [];
  if (pal.isBoss) labels.push("Boss");
  else if (pal.isAlpha) labels.push("Alpha");
  if (pal.isLucky) labels.push("Lucky");
  return labels;
}

function boundedLevel(raw: string): number | undefined {
  if (!/^\d{1,3}$/.test(raw)) return undefined;
  const value = Number(raw);
  return value <= 999 ? value : undefined;
}

function isOneOf<T extends string>(value: string, ...allowed: T[]): value is T {
  return allowed.some((candidate) => candidate === value);
}
