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

export const EMPTY_PAL_EXPLORER_FILTERS: PalExplorerFilterState = {
  q: "",
  ownerSource: "",
  placement: "",
  specimen: "",
  minLevel: "",
  maxLevel: "",
};

const PAL_EXPLORER_SEARCH_KEYS = ["q", "ownerSource", "placement", "specimen", "minLevel", "maxLevel"] as const;

/** Restore only the explorer's allowlisted filters from a shareable URL. */
export function palExplorerFiltersFromSearch(search: URLSearchParams | string): PalExplorerFilterState {
  const query = typeof search === "string" ? new URLSearchParams(search) : search;
  const params = palExplorerParams({
    q: query.get("q") ?? "",
    ownerSource: query.get("ownerSource") ?? "",
    placement: query.get("placement") ?? "",
    specimen: query.get("specimen") ?? "",
    minLevel: query.get("minLevel") ?? "",
    maxLevel: query.get("maxLevel") ?? "",
  });
  return {
    q: params.q ?? "",
    ownerSource: params.ownerSource ?? "",
    placement: params.placement ?? "",
    specimen: params.specimen ?? "",
    minLevel: params.minLevel === undefined ? "" : String(params.minLevel),
    maxLevel: params.maxLevel === undefined ? "" : String(params.maxLevel),
  };
}

/**
 * Write a canonical explorer query while retaining unrelated flags such as `mock`.
 * Cursors are intentionally excluded so a record link always starts at fresh results.
 */
export function palExplorerSearch(
  state: PalExplorerFilterState,
  current: URLSearchParams | string = "",
): URLSearchParams {
  const query = new URLSearchParams(typeof current === "string" ? current : current.toString());
  for (const key of PAL_EXPLORER_SEARCH_KEYS) query.delete(key);
  query.delete("cursor");
  const params = palExplorerParams(state);
  if (params.q) query.set("q", params.q);
  if (params.ownerSource) query.set("ownerSource", params.ownerSource);
  if (params.placement) query.set("placement", params.placement);
  if (params.specimen) query.set("specimen", params.specimen);
  if (params.minLevel !== undefined) query.set("minLevel", String(params.minLevel));
  if (params.maxLevel !== undefined) query.set("maxLevel", String(params.maxLevel));
  return query;
}

/** Build a stable panel deep link for records, history, guilds, or external integrations. */
export function palExplorerHref(filters: Partial<PalExplorerFilterState>): string {
  const query = palExplorerSearch({ ...EMPTY_PAL_EXPLORER_FILTERS, ...filters });
  const suffix = query.toString();
  return suffix ? `/pals?${suffix}` : "/pals";
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
      return pal.ownerName;
    case "last_observed":
      return `${pal.ownerName} · last known owner`;
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
