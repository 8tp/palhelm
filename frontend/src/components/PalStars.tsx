import { condensedStars, MAX_CONDENSE_STARS } from "./palStars";
import "./PalStars.css";

/**
 * Renders the 0–4 condenser stars for a Pal. Returns nothing when the rank is
 * unavailable, so compact contexts stay clean and never imply "zero stars" for
 * missing data; detail contexts should show their own "Unavailable" copy instead.
 */
export function PalStars({ rank, className }: { rank: number | null | undefined; className?: string }) {
  const filled = condensedStars(rank);
  if (filled === null) return null;
  const label = `Condensed ${filled} of ${MAX_CONDENSE_STARS} stars`;
  return (
    <span className={["pal-stars", className].filter(Boolean).join(" ")} role="img" aria-label={label} title={label}>
      {Array.from({ length: MAX_CONDENSE_STARS }, (_, i) => (
        <span key={i} className={i < filled ? "pal-star is-filled" : "pal-star"} aria-hidden="true">
          {i < filled ? "★" : "☆"}
        </span>
      ))}
    </span>
  );
}
