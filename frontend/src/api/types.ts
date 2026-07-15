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
  /** Aggregate player-save observations. Omitted by older parsers; absence is not zero. */
  captureTotal?: number;
  uniquePalsCaptured?: number;
  paldeckUnlocked?: number;
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
  durationSec: number;
}

export interface PlayerActivityWindow {
  durationSec: number;
  sessionCount: number;
}

export interface PlayerActivity {
  /** This is deliberately observation-scoped, not a claim about lifetime game history. */
  coverage: "panel_observed_sessions";
  trackingSince: string | null;
  currentSession: PlayerSession | null;
  windows: {
    last24Hours: PlayerActivityWindow;
    last7Days: PlayerActivityWindow;
    last30Days: PlayerActivityWindow;
  };
  recentSessions: PlayerSession[];
  recentSessionsTruncated: boolean;
}

export interface PlayerDetail extends Player {
  pals: PlayerPal[];
  sessions: PlayerSession[];
  activity: PlayerActivity;
}

export type ServerActivityWindow = "24h" | "7d" | "30d";

export interface ActivityConcurrencyBucket {
  at: string;
  peakPlayers: number;
  averagePlayers: number;
}

export interface ActivityPlayerRank {
  uid: string;
  name: string;
  guildId: string;
  guildName: string;
  durationSec: number;
  sessionCount: number;
  currentSession: boolean;
  /** First observed by this panel inside the selected window, not necessarily new to Palworld. */
  firstObserved: boolean;
}

export interface ActivityGuildRank {
  guildId: string;
  guildName: string;
  durationSec: number;
  sessionCount: number;
  activePlayers: number;
}

export interface ServerActivity {
  coverage: "panel_observed_sessions";
  trackingSince: string | null;
  window: ServerActivityWindow;
  since: string;
  through: string;
  bucketSec: number;
  analysisTruncated: boolean;
  activePlayers: number;
  newPlayers: number;
  returningPlayers: number;
  peakConcurrency: number;
  peakAt: string | null;
  concurrency: ActivityConcurrencyBucket[];
  players: ActivityPlayerRank[];
  guilds: ActivityGuildRank[];
  guildAttribution: "current_player_guild";
  unattributedPlayers: number;
  unattributedDurationSec: number;
}

export type PalOwnerSource = "save" | "personal_container" | "last_observed" | "unresolved";
export type PalPlacement = "party" | "box" | "base" | "unknown";
export type PalSpecimenFilter = "standard" | "alpha" | "lucky" | "boss";

export interface PalExplorerPal extends PlayerPal {
  isBoss: boolean;
  placement: PalPlacement;
  ownerUid: string;
  ownerName: string;
  ownerSource: PalOwnerSource;
  ownerResolved: boolean;
}

export interface PalExplorerPage {
  data: PalExplorerPal[];
  nextCursor: string | null;
}

export interface PalExplorerParams {
  cursor?: string;
  limit?: number;
  q?: string;
  ownerSource?: PalOwnerSource;
  placement?: PalPlacement;
  specimen?: PalSpecimenFilter;
  minLevel?: number;
  maxLevel?: number;
}

export interface WhitelistEntry {
  steamId: string;
  name?: string;
}

// ---------- Guilds ----------
export interface GuildBase {
  id: string;
  // null when the base's world transform was never decoded (a pre-decoding
  // save). Consumers must treat this as "location unavailable", never (0,0).
  location: { x: number; y: number } | null;
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

export interface GuildDetailMember {
  uid: string;
  name: string;
  level: number;
  online: boolean;
  lastSeenAt: string | null;
  playtimeSec: number;
  captureTotal: number | null;
  uniquePalsCaptured: number | null;
  paldeckUnlocked: number | null;
  observedDurationSec: number;
  observedSessionCount: number;
  currentSession: boolean;
}

export interface GuildDetailBase {
  id: string;
  location: PlayerLocation | null;
  level: number;
  palCount: number;
}

export interface GuildDetailPal {
  instanceId: string;
  characterId: string;
  displayName: string;
  level: number;
  isAlpha: boolean;
  isLucky: boolean;
  isBoss: boolean;
  placement: PalPlacement;
  baseId: string | null;
  ownerUid: string;
  ownerName: string;
  ownerSource: PalOwnerSource;
  ownerResolved: boolean;
  association: "guild_base" | "current_member_owner";
}

export interface GuildDetail {
  id: string;
  name: string;
  adminUid: string;
  memberCount: number;
  members: GuildDetailMember[];
  bases: GuildDetailBase[];
  palCount: number;
  palsTruncated: boolean;
  pals: GuildDetailPal[];
  activity: {
    coverage: "panel_observed_sessions";
    attribution: "current_guild_membership";
    window: "30d";
    since: string;
    through: string;
    trackingSince: string | null;
    analysisTruncated: boolean;
    durationSec: number;
    sessionCount: number;
    activePlayers: number;
  };
}

// ---------- Paldeck progression ----------
export interface PaldeckCatalogCoverage {
  version: "palworld_1.0_pinned";
  knownSpecies: number;
  observedUnknownSpecies: number;
}

export interface ServerPaldeckSpecies {
  characterId: string;
  displayName: string;
  known: boolean;
  captureCount: number | null;
  capturedByPlayers: number | null;
  unlockedByPlayers: number | null;
}

export interface ServerPaldeck {
  coverage: {
    source: "player_save_record_data";
    playersTotal: number;
    playersWithCaptureCounts: number;
    playersWithUnlockFlags: number;
    captureCountsTruncated: boolean;
    unlockFlagsTruncated: boolean;
    oldestObservedAt: string | null;
    latestObservedAt: string | null;
  };
  catalog: PaldeckCatalogCoverage;
  captureTotal: number | null;
  uniqueSpeciesCaptured: number | null;
  speciesUnlocked: number | null;
  species: ServerPaldeckSpecies[];
}

export interface PlayerPaldeck {
  player: { uid: string; name: string };
  coverage: {
    source: "player_save_record_data";
    captureCountsAvailable: boolean;
    unlockFlagsAvailable: boolean;
    captureCountsTruncated: boolean;
    unlockFlagsTruncated: boolean;
    captureObservedAt: string | null;
    unlockObservedAt: string | null;
  };
  catalog: PaldeckCatalogCoverage;
  captureTotal: number | null;
  uniquePalsCaptured: number | null;
  paldeckUnlocked: number | null;
  species: Array<{
    characterId: string;
    displayName: string;
    known: boolean;
    captureCount: number | null;
    unlocked: boolean | null;
  }>;
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
  instanceId?: string;
  baseId?: string;
  ownerUid?: string;
  ownerName?: string;
  ownerSource?: string;
  linked?: boolean;
  location: { x: number; y: number; z: number };
}

export interface LiveWorldActivityCounts {
  working: number;
  transporting: number;
  eating: number;
  sleeping: number;
  idle: number;
  inactive: number;
  combat: number;
  incapacitated: number;
  moving: number;
  unknown: number;
}

export interface LiveWorldDiagnostics {
  lastRequestDurationMs: number;
  lastAcceptedActorCount: number;
  lastErrorCategory: string;
  linkedBasePals: number;
  unresolvedBasePals: number;
  linkLookupFailed: boolean;
  scheduledDelayMs: number;
  nextAttemptAt: string | null;
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
  activity: LiveWorldActivityCounts;
  actors: LiveWorldActor[];
  truncated: boolean;
  diagnostics: LiveWorldDiagnostics;
}

export interface LiveWorldActivitySample extends LiveWorldActivityCounts {
  at: string;
  fps: number;
  fpsAvg: number;
  players: number;
  basePals: number;
  linkedBasePals: number;
}

export interface LiveWorldActivityHistory {
  window: "1h" | "24h" | "7d";
  bucketSec: number;
  samples: LiveWorldActivitySample[];
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
