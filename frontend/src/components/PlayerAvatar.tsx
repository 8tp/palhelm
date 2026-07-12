// Player save data carries no gender field (see docs/API.md's Player shape — investigated,
// not present), so the identity chip can't be a gendered figure. This stands in with a neutral,
// license-clean "expedition trainer" mark: a head-and-shoulders silhouette with a satchel strap,
// drawn once as an original shape (not a stock person/user icon). It renders in the existing
// `.avatar` chip (28px in tables, 40px in the player detail head — see Players.css) and inherits
// `--ink-2`, so it reads correctly in both themes without its own color tokens. Online/offline/
// banned state styling on the chip's ring/background is unchanged — ui.css's `.avatar` has never
// varied by state, and this doesn't add any.
import { useState } from "react";
import { api, USE_MOCK } from "../api/client";

function TrainerMark() {
  return (
    <svg viewBox="0 0 24 24" width="60%" height="60%" fill="currentColor" aria-hidden="true">
      <circle cx="12" cy="8.4" r="3.3" />
      <path d="M12 12.9c-4.3 0-7.7 2.8-7.7 7.1a1 1 0 0 0 1 1h13.4a1 1 0 0 0 1-1c0-4.3-3.4-7.1-7.7-7.1z" />
      {/* satchel strap: a knockout line in the chip's own background — reads as an expedition
          pack strap crossing the chest, the one detail that sets this apart from a bare bust */}
      <path d="M8.4 13.5L14.7 20.6" stroke="var(--surface-3)" strokeWidth="1.3" strokeLinecap="round" fill="none" />
    </svg>
  );
}

// When a player has a resolvable Steam avatar the panel proxies it same-origin (see the
// backend's /players/:uid/avatar); a 404 or load error (private profile, console crossplay,
// avatars disabled) falls back to the neutral TrainerMark, the same contract PalIcon uses.
export function PlayerAvatar({ name, uid, className }: { name: string; uid?: string; className?: string }) {
  const [failed, setFailed] = useState(false);
  const showImage = !USE_MOCK && !failed && !!uid;
  return (
    <span className={["avatar", className].filter(Boolean).join(" ")} title={name}>
      {showImage ? (
        <img src={api.players.avatarUrl(uid)} alt="" loading="lazy" onError={() => setFailed(true)} />
      ) : (
        <TrainerMark />
      )}
    </span>
  );
}
