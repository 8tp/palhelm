/** A Pal is never condensed at rank 1 and gains one star per condense, up to four. */
export const MAX_CONDENSE_STARS = 4;

/**
 * Filled condenser stars for a raw Pal rank, or null when the rank is unavailable.
 *
 * Honesty rule: a null/undefined rank (the save carried no Rank property) returns
 * null so callers can say "Unavailable" or render nothing — it is never coerced to
 * zero stars. A present rank maps 1..5 to 0..4 filled stars, clamped so out-of-range
 * saves can't render a broken row.
 */
export function condensedStars(rank: number | null | undefined): number | null {
  if (rank === null || rank === undefined) return null;
  return Math.max(0, Math.min(MAX_CONDENSE_STARS, Math.round(rank) - 1));
}
