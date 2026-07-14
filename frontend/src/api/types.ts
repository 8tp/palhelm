// Types mirroring docs/API.md. Keep in sync with the backend OpenAPI doc.

export type Role = "admin" | "viewer";

export interface ApiError {
  error: { code: string; message: string; [detail: string]: unknown };
}

export class ApiRequestError extends Error {
  code: string;
  status: number;
  /** Extra fields the backend attaches to the error object (e.g. `manualCommand` on 501). */
  extra: Record<string, unknown>;
  constructor(status: number, code: string, message: string, extra: Record<string, unknown> = {}) {
    super(message);
    this.name = "ApiRequestError";
    this.status = status;
    this.code = code;
    this.extra = extra;
  }
}

// ---------- Auth ----------
export interface SessionInfo {
  role: Role;
  username: string;
}

// ---------- Server ----------
export type ServerState = "running" | "starting" | "stopping" | "stopped";

export interface ServerInfo {
  name: string;
  description: string;
  version: string;
  worldGuid: string;
  state: ServerState;
  uptimeSec: number;
  panelVersion: string;
}

export type HealthState = "ok" | "error";

export interface ServerHealth {
  rest: HealthState;
  rcon: HealthState;
  save: { state: HealthState; lastSyncAt: string };
}

// ---------- Metrics ----------
export interface MetricsCurrent {
  fps: number;
  fpsAvg: number;
  frameTimeMs: number;
  players: number;
  maxPlayers: number;
  day: number;
  uptimeSec: number;
  baseCamps: number;
}

export type MetricsWindow = "1h" | "24h" | "7d";

export interface MetricsHistory {
  series: {
    t: number[];
    fps: number[];
    frameTimeMs: number[];
    players: number[];
  };
}

// ---------- Players ----------
export interface PlayerLocation {
  x: number;
  y: number;
}

export interface Player {
  uid: string;
  steamId: string;
  name: string;
  accountName: string;
  online: boolean;
  level: number;
  guildId: string | null;
  guildName: string | null;
  ping: number | null;
  location: PlayerLocation | null;
  firstSeenAt: string;
  lastSeenAt: string;
  playtimeSec: number;
  banned: boolean;
  whitelisted: boolean;
}

export interface PlayerPal {
  instanceId: string;
  characterId: string;
  displayName: string;
  level: number;
  isAlpha: boolean;
  isLucky: boolean;
  /** In the player's active party (Otomo container). */
  inParty: boolean;
  /** Slot within the party (0-4) when inParty, else null. */
  partySlot: number | null;
  /** Pal-box page index (0-based, 30 slots per page) when in storage, else null. */
  boxPage: number | null;
  /** Slot within the box page (0-29) when in storage, else null. */
  boxSlot: number | null;
  /** Safe derived placement; unknown is never assumed to mean a base. */
  placement?: "party" | "box" | "base" | "unknown";
  /** Public base join key when placement is base. */
  baseId?: string | null;
  /** Individual save observations. Null means unavailable, not zero. */
  hp?: number | null;
  gender?: "male" | "female" | "unknown" | "";
  talents?: {
    hp: number | null;
    melee: number | null;
    shot: number | null;
    defense: number | null;
  };
  passiveSkillIds?: string[];
  equippedSkillIds?: string[];
}

export interface PlayerSession {
  joinedAt: string;
  leftAt: string | null;
  durationSec: number | null;
}

export interface PlayerDetail extends Player {
  pals: PlayerPal[];
  sessions: PlayerSession[];
}

export interface WhitelistEntry {
  steamId: string;
  name?: string;
}

// ---------- Guilds ----------
export interface GuildBase {
  id: string;
  location: { x: number; y: number };
  level: number;
}

export interface GuildMember {
  uid: string;
  name: string;
}

export interface Guild {
  id: string;
  name: string;
  adminUid: string;
  memberCount: number;
  members: GuildMember[];
  bases: GuildBase[];
}

// ---------- Map ----------
// Mirrors backend/internal/server/tiles.go's mapDatasetInfo/mapDatasetLayer JSON shape.
export interface MapDatasetTransform {
  a: number;
  b: number;
  c: number;
  d: number;
}

export interface MapDatasetLayer {
  id: string;
  label?: string;
  path: string;
  format?: "png" | "webp";
  tile_size?: number;
  min_zoom: number;
  max_zoom: number;
  transform?: MapDatasetTransform | null;
  bounds?: [[number, number], [number, number]] | null;
}

// NB: fields here are snake_case (not this file's usual camelCase) because they mirror
// backend/internal/server/tiles.go's mapDatasetInfo verbatim — that struct's JSON tags match
// the fetch-tooling-authored dataset.json sidecar exactly, since the same struct both reads
// that file and re-serves it here with no field renaming in between.
export interface MapDataset {
  fetched_at: string | null;
  game_version: string;
  source: string;
  notes?: string;
  layers: MapDatasetLayer[];
}

// ---------- World ----------
export interface WorldInfo {
  day: number;
  lastParseAt: string;
  parseDurationMs: number;
  stats: { players: number; pals: number; guilds: number; skippedProps: number };
  formatDrift: boolean;
}

export type GameDataState = "disabled" | "pending" | "ready" | "stale" | "unsupported" | "unauthorized" | "unavailable";

export interface LiveWorldCounts {
  players: number;
  partyPals: number;
  basePals: number;
  wildPals: number;
  npcs: number;
  palBoxes: number;
  unknown: number;
}

export interface LiveWorldActor {
  kind: "Player" | "OtomoPal" | "BaseCampPal" | "PalBox" | string;
  characterId?: string;
  isBoss?: boolean;
  name?: string;
  trainerName?: string;
  guildName?: string;
  level?: number;
  hpPercent?: number;
  active?: boolean;
  activity: "working" | "transporting" | "eating" | "sleeping" | "idle" | "inactive" | "combat" | "incapacitated" | "moving" | "unknown";
  location: { x: number; y: number; z: number };
}

/** Sanitized session-only projection of the optional Palworld 1.0 game-data snapshot. */
export interface LiveWorldSnapshot {
  state: GameDataState;
  capturedAt: string | null;
  lastAttemptAt: string | null;
  sourceTime?: string;
  fps: number;
  fpsAvg: number;
  counts: LiveWorldCounts;
  actors: LiveWorldActor[];
  truncated: boolean;
}

// ---------- Console ----------
export interface ConsoleLogEntry {
  at: string;
  user: string;
  command: string;
  output: string;
  isError: boolean;
}

export interface SavedCommand {
  id: string;
  name: string;
  command: string;
}

// ---------- Backups ----------
export type BackupTrigger = "scheduled" | "manual" | "pre-restore" | "imported";

export interface Backup {
  id: string;
  file: string;
  createdAt: string;
  sizeBytes: number;
  trigger: BackupTrigger;
  worldDay?: number;
}

export interface BackupContentEntry {
  path: string;
  sizeBytes: number;
  modifiedAt: string;
}

export type DiffKind = "add" | "modify" | "delete";

export interface BackupDiffChange {
  path: string;
  kind: DiffKind;
  fromSize?: number;
  toSize?: number;
}

export interface BackupDryRun {
  changes: BackupDiffChange[];
  requiresStop: true;
}

export interface BackupSchedule {
  enabled: boolean;
  everyMinutes: number;
  keepDays: number;
  nextRunAt: string | null;
}

// ---------- Config ----------
export type ConfigValue = string | number | boolean;

export interface ConfigSetting {
  key: string;
  value: ConfigValue;
  effectiveValue: ConfigValue;
  type: "string" | "integer" | "number" | "boolean";
  group: string;
  default: ConfigValue;
  pending: boolean;
  editable: boolean;
  readOnly: boolean;
}

export interface ConfigCapability {
  available: boolean;
  reason?: string;
}

export interface ConfigDoc {
  source: "compose" | "ini";
  composeFile?: string;
  service: string;
  version?: string;
  capabilities: {
    write: ConfigCapability;
    apply: ConfigCapability;
  };
  manualCommand: string;
  settings: ConfigSetting[];
}

// ---------- Integration API keys ----------
// Mirrors docs/specs/integration-api.md §9 (admin key-management routes). These are
// session-authenticated admin routes distinct from the bearer-token integration surface
// itself — the frontend never talks to /api/integration/v1 directly.
export interface IntegrationKey {
  id: string;
  label: string;
  createdAt: string;
  lastUsedAt: string | null;
  revokedAt: string | null;
}

/** The `201 POST /integration-keys` response — the only shape that ever carries `key`. */
export interface IntegrationKeyCreated extends IntegrationKey {
  key: string;
}

// ---------- Paldeck icons ----------
export interface PaldeckIconDataset {
  source: string;
  fetchedAt: string | null;
  count: number;
  characterIds: string[];
}

// ---------- Events ----------
export type EventKind = "join" | "leave" | "backup" | "system" | "panel" | "config";

export interface PalhelmEvent {
  at: string;
  kind: EventKind;
  message: string;
  meta?: Record<string, unknown>;
}
