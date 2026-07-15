import type { ActivityConcurrencyBucket, ServerActivity } from "../../api/types";

export function topPeakBuckets(buckets: ActivityConcurrencyBucket[], limit = 3): ActivityConcurrencyBucket[] {
  return [...buckets]
    .sort((a, b) => b.averagePlayers - a.averagePlayers || b.peakPlayers - a.peakPlayers || a.at.localeCompare(b.at))
    .slice(0, Math.max(0, limit));
}

export function activityCoverageNote(activity: Pick<ServerActivity, "trackingSince" | "since" | "analysisTruncated">): string {
  if (activity.analysisTruncated) return "Analysis hit its cap; rankings and concurrency may be incomplete.";
  if (!activity.trackingSince) return "No player sessions observed yet.";
  if (new Date(activity.trackingSince) > new Date(activity.since)) {
    return `Tracking began ${new Date(activity.trackingSince).toLocaleString()}, partway through this window.`;
  }
  return `Tracking since ${new Date(activity.trackingSince).toLocaleString()} — the full window is covered.`;
}

export function localBucketLabel(at: string, bucketSec: number): string {
  const date = new Date(at);
  if (bucketSec >= 86_400) return date.toLocaleDateString(undefined, { month: "short", day: "numeric" });
  return date.toLocaleString(undefined, { weekday: "short", hour: "numeric" });
}
