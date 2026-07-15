export type PaldeckSpeciesFilter = "all" | "captured" | "unseen" | "unavailable";

export interface PaldeckSpeciesView {
  characterId: string;
  displayName: string;
  known: boolean;
  captureCount: number | null;
}

export function paldeckPercent(value: number | null, knownSpecies: number): number | null {
  if (value === null || knownSpecies <= 0) return null;
  return Math.max(0, Math.min(100, Math.round((value / knownSpecies) * 1000) / 10));
}

export function filterPaldeckSpecies<T extends PaldeckSpeciesView>(
  species: readonly T[],
  search: string,
  filter: PaldeckSpeciesFilter,
): T[] {
  const needle = search.trim().toLocaleLowerCase();
  return species.filter((item) => {
    if (needle && !`${item.displayName} ${item.characterId}`.toLocaleLowerCase().includes(needle)) return false;
    if (filter === "captured") return item.captureCount !== null && item.captureCount > 0;
    if (filter === "unseen") return item.captureCount === 0;
    if (filter === "unavailable") return item.captureCount === null;
    return true;
  });
}
