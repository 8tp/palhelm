import {
  PAL_WORK_DATA_GENERATED_AT,
  PAL_WORK_DATA_SOURCES,
  PAL_WORK_SUITABILITY_ROWS,
} from "../data/palWorkSuitabilities.generated.ts";

export interface PalWorkSuitability {
  id: string;
  name: string;
  level: number;
}

export type WorkSuitabilityKind =
  | "kindling"
  | "watering"
  | "planting"
  | "electricity"
  | "handiwork"
  | "gathering"
  | "lumbering"
  | "mining"
  | "medicine"
  | "cooling"
  | "transporting"
  | "farming"
  | "unknown";

const displayOrder: WorkSuitabilityKind[] = [
  "kindling",
  "watering",
  "planting",
  "electricity",
  "handiwork",
  "gathering",
  "lumbering",
  "mining",
  "medicine",
  "cooling",
  "transporting",
  "farming",
  "unknown",
];

const workByCharacterId = new Map<string, readonly PalWorkSuitability[]>(
  Object.entries(PAL_WORK_SUITABILITY_ROWS).map(([characterId, rows]) => [
    characterId,
    rows.map(([id, name, level]) => ({ id, name, level })),
  ]),
);

export const PAL_WORK_DATA_PROVENANCE = [
  "Version-pinned species metadata",
  PAL_WORK_DATA_GENERATED_AT ? `generated ${PAL_WORK_DATA_GENERATED_AT}` : null,
  PAL_WORK_DATA_SOURCES.length > 0 ? `sources: ${PAL_WORK_DATA_SOURCES.map((source) => source.name).join(", ")}` : null,
].filter(Boolean).join(" · ");

/** Join save CharacterIDs to version-pinned species metadata. Boss prefixes
 * describe the instance variant and do not change the species' work levels. */
export function workSuitabilitiesFor(characterId: string): readonly PalWorkSuitability[] | undefined {
  const key = characterId.trim().replace(/^(?:BOSS_)+/i, "").toLocaleLowerCase("en-US");
  const found = workByCharacterId.get(key);
  if (!found) return undefined;
  return [...found].sort((a, b) => {
    const aOrder = displayOrder.indexOf(workSuitabilityKind(a.id, a.name));
    const bOrder = displayOrder.indexOf(workSuitabilityKind(b.id, b.name));
    return aOrder - bOrder || b.level - a.level || a.name.localeCompare(b.name);
  });
}

export function workSuitabilityKind(id: string, name: string): WorkSuitabilityKind {
  const key = `${id} ${name}`.toLocaleLowerCase("en-US").replace(/[^a-z]/g, "");
  if (key.includes("emitflame") || key.includes("kindling")) return "kindling";
  if (key.includes("watering")) return "watering";
  if (key.includes("seeding") || key.includes("planting")) return "planting";
  if (key.includes("generateelectricity") || key.includes("generatingelectricity")) return "electricity";
  if (key.includes("handcraft") || key.includes("handiwork")) return "handiwork";
  if (key.includes("collection") || key.includes("gathering")) return "gathering";
  if (key.includes("deforest") || key.includes("lumbering")) return "lumbering";
  if (key.includes("mining")) return "mining";
  if (key.includes("productmedicine") || key.includes("medicineproduction")) return "medicine";
  if (key.includes("cooling") || key.includes("cool")) return "cooling";
  if (key.includes("transport")) return "transporting";
  if (key.includes("monsterfarm") || key.includes("farming")) return "farming";
  return "unknown";
}
