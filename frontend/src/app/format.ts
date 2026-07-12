// Shared formatting helpers. Timestamps from the API are RFC 3339 UTC; sizes are bytes.

export function formatDuration(totalSec: number): string {
  const sec = Math.max(0, Math.floor(totalSec));
  const d = Math.floor(sec / 86400);
  const h = Math.floor((sec % 86400) / 3600);
  const m = Math.floor((sec % 3600) / 60);
  if (d > 0) return `${d}d ${h}h`;
  if (h > 0) return `${h}h ${m}m`;
  const s = sec % 60;
  if (m > 0) return `${m}m ${s}s`;
  return `${s}s`;
}

export function formatRelativeToNow(iso: string | null | undefined): string {
  if (!iso) return "never";
  const t = new Date(iso).getTime();
  // Null/zero-value timestamps from the API mean "hasn't happened yet".
  if (!Number.isFinite(t) || t <= 0) return "never";
  const sec = Math.max(0, Math.floor((Date.now() - t) / 1000));
  if (sec < 60) return "just now";
  return `${formatDuration(sec)} ago`;
}

export function formatClock(iso: string): string {
  const d = new Date(iso);
  return d.toLocaleTimeString(undefined, { hour: "2-digit", minute: "2-digit", hour12: false });
}

export function formatDateTime(iso: string): string {
  const d = new Date(iso);
  const pad = (n: number) => String(n).padStart(2, "0");
  return `${d.getFullYear()}-${pad(d.getMonth() + 1)}-${pad(d.getDate())} ${pad(d.getHours())}:${pad(
    d.getMinutes(),
  )}:${pad(d.getSeconds())}`;
}

export function formatBytes(bytes: number): string {
  if (bytes < 1000) return `${bytes} B`;
  const units = ["KB", "MB", "GB", "TB"];
  let value = bytes / 1000;
  let unitIdx = 0;
  while (value >= 1000 && unitIdx < units.length - 1) {
    value /= 1000;
    unitIdx += 1;
  }
  const text = value.toFixed(value >= 100 ? 0 : 1).replace(/\.0$/, "");
  return `${text} ${units[unitIdx]}`;
}

export function formatWorldGuid(guid: string): string {
  if (guid.length <= 12) return guid;
  return `${guid.slice(0, 8)}…${guid.slice(-4)}`;
}

export function truncateMiddle(text: string, keepStart = 8, keepEnd = 4): string {
  if (text.length <= keepStart + keepEnd + 1) return text;
  const tail = keepEnd > 0 ? text.slice(-keepEnd) : "";
  return `${text.slice(0, keepStart)}…${tail}`;
}
