import { useMemo, useState } from "react";
import { useQuery } from "@tanstack/react-query";
import { api, USE_MOCK } from "../api/client";
import { palIconId } from "../lib/palIconId";

function initials(name: string): string {
  return name.slice(0, 2).toUpperCase();
}

/**
 * The installed-icon roster (GET /paldeck/icon-dataset) — fetched once and shared by every
 * <PalIcon> on the page via TanStack Query's cache (same query key, `staleTime: Infinity`
 * since the roster only changes when an operator re-runs scripts/fetch-pal-icons.sh).
 */
function useKnownPalIconIds(): Set<string> {
  const { data } = useQuery({
    queryKey: ["paldeck", "icon-dataset"],
    queryFn: () => api.paldeck.iconDataset(),
    staleTime: Infinity,
  });
  return useMemo(() => new Set((data?.characterIds ?? []).map((id) => id.toLowerCase())), [data]);
}

/**
 * Fills the existing `.pal-chip` slot (24px, 2px inked border) with the pal's preview icon
 * when one is installed, falling back to the two-letter initials chip otherwise — on a 404,
 * on a load error, or under mock mode (where no icon files exist to serve, so we never issue
 * the request in the first place).
 */
export function PalIcon({ characterId, displayName }: { characterId: string; displayName: string }) {
  const known = useKnownPalIconIds();
  const [failed, setFailed] = useState(false);
  const iconId = palIconId(characterId);
  const showImage = !USE_MOCK && !failed && known.has(iconId);

  return (
    <span className="pal-chip">
      {showImage ? (
        <img src={api.paldeck.iconUrl(iconId)} alt="" loading="lazy" onError={() => setFailed(true)} />
      ) : (
        initials(displayName)
      )}
    </span>
  );
}
