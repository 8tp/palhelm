const BASE_ICON_ALIASES: Readonly<Record<string, string>> = {
  plantslime_flower: "plantslime",
  grasspanda_electric_tower: "grasspanda_electric",
  lazydragon_electric_tower: "lazydragon_electric",
};

/** Resolve save-instance variants to the static base-species portrait key. */
export function palIconId(characterId: string): string {
  const raw = characterId.trim().toLowerCase();
  if (raw === "boss_hunter_rifle") return raw; // Named human bounty target Hawk.
  let base = raw;
  while (base.startsWith("boss_")) base = base.slice("boss_".length);
  return BASE_ICON_ALIASES[base] ?? base;
}
