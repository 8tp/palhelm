import type { PlayerPal } from "../api/types";

export function humanizePalIdentifier(value: string): string {
  return value
    .replace(/^PassiveSkill_/i, "")
    .replace(/_PAL$/i, "")
    .replace(/_/g, " ")
    .replace(/([a-z])([A-Z])/g, "$1 $2")
    .replace(/([A-Za-z])(\d+)/g, "$1 $2")
    .replace(/\bup\b/gi, "Up")
    .replace(/\bdown\b/gi, "Down")
    .replace(/\s+/g, " ")
    .trim();
}

export function palPlacementLabel(pal: PlayerPal): string {
  if (pal.placement === "base" || pal.baseId) {
    return pal.baseId ? `Base worker · ${pal.baseId.slice(0, 8)}` : "Base worker";
  }
  if (pal.inParty) return `Party slot ${(pal.partySlot ?? 0) + 1}`;
  if (pal.boxPage !== null) return `Box ${pal.boxPage + 1} · slot ${(pal.boxSlot ?? 0) + 1}`;
  return "Placement unavailable";
}

export function palGenderLabel(gender: PlayerPal["gender"]): string {
  if (gender === "male") return "Male ♂";
  if (gender === "female") return "Female ♀";
  return "Unknown";
}
