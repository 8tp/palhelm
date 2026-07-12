export const RECENTS_KEY = "palhelm.palette.recents";
const RECENTS_MAX = 5;

export type RecentKind = "nav" | "player" | "kick" | "ban" | "unban" | "broadcast" | "saved";
export interface RecentEntry {
  kind: RecentKind;
  key: string;
  label: string;
}

export interface RecentStorage {
  getItem(key: string): string | null;
  setItem(key: string, value: string): void;
}

const ADMIN_RECENT_KINDS = new Set<RecentKind>(["kick", "ban", "unban", "broadcast"]);

export function loadPaletteRecents(storage: RecentStorage, isAdmin: boolean): RecentEntry[] {
  try {
    const raw = storage.getItem(RECENTS_KEY);
    if (!raw) return [];
    const parsed = JSON.parse(raw) as RecentEntry[];
    if (!Array.isArray(parsed)) return [];
    const filtered = parsed.filter((entry) => isAdmin || !ADMIN_RECENT_KINDS.has(entry.kind)).slice(0, RECENTS_MAX);
    if (!isAdmin) storage.setItem(RECENTS_KEY, JSON.stringify(filtered));
    return filtered;
  } catch {
    return [];
  }
}

export function savePaletteRecent(storage: RecentStorage, entry: RecentEntry, isAdmin: boolean): RecentEntry[] {
  if (!isAdmin && ADMIN_RECENT_KINDS.has(entry.kind)) return loadPaletteRecents(storage, false);
  const next = [
    entry,
    ...loadPaletteRecents(storage, isAdmin).filter((recent) => !(recent.kind === entry.kind && recent.key === entry.key)),
  ].slice(0, RECENTS_MAX);
  try {
    storage.setItem(RECENTS_KEY, JSON.stringify(next));
  } catch {
    // Storage unavailable (private mode, quota) — recents remain in memory only.
  }
  return next;
}
