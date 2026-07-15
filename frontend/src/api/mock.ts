// Mock adapter for api/client.ts. Active when VITE_MOCK=1 or ?mock is present.
// Returns realistic fixtures matching the design mockups (My Palworld Server, 2/16 players,
// Kestrel/VossR/mika_o/HaruQ/tessellate, 7 guilds, backups list, config groups) with
// believable latency and live-ish drift.
// Extensioned specifiers here (permitted by tsconfig's `allowImportingTsExtensions` +
// `moduleResolution: bundler`, and rewritten transparently by Vite) so this module's runtime
// (non `import type`) dependencies resolve unmodified under plain `node --test`, which — unlike
// Vite — requires an explicit extension. Every other value import in this file already
// resolves through the `./types` import below, which is `import type` and erased entirely by
// type stripping, so it never hits Node's resolver.
import { ApiRequestError } from "./types.ts";
import { gameToWorld } from "../app/mapTransform.ts";
import type {
  Backup,
  BackupContentEntry,
  BackupDryRun,
  BackupSchedule,
  ConfigDoc,
  ConfigSetting,
  ConfigValue,
  ConsoleLogEntry,
  Guild,
  IntegrationKey,
  IntegrationKeyCreated,
  LiveWorldSnapshot,
  MapDataset,
  MetricsCurrent,
  MetricsHistory,
  MetricsWindow,
  PalExplorerPage,
  PalExplorerPal,
  PalExplorerParams,
  PaldeckIconDataset,
  PalhelmEvent,
  Player,
  PlayerDetail,
  PlayerPal,
  Role,
  SavedCommand,
  ServerActivity,
  ServerActivityWindow,
  ServerHealth,
  ServerInfo,
  SessionInfo,
  WhitelistEntry,
  WorldInfo,
} from "./types";

// ---------- helpers ----------

function latency(min = 50, max = 200): Promise<void> {
  const ms = min + Math.random() * (max - min);
  return new Promise((resolve) => setTimeout(resolve, ms));
}

function wobble(base: number, amount: number): number {
  return base + (Math.random() - 0.5) * 2 * amount;
}

const SESSION_KEY = "palhelm.mock.session";

function readSession(): SessionInfo | null {
  try {
    const raw = sessionStorage.getItem(SESSION_KEY);
    return raw ? (JSON.parse(raw) as SessionInfo) : null;
  } catch {
    return null;
  }
}

function writeSession(session: SessionInfo | null) {
  if (session) sessionStorage.setItem(SESSION_KEY, JSON.stringify(session));
  else sessionStorage.removeItem(SESSION_KEY);
}

function requireSession(): SessionInfo {
  const s = readSession();
  if (!s) throw new ApiRequestError(401, "unauthorized", "Sign in to continue.");
  return s;
}

function requireAdmin(): SessionInfo {
  const s = requireSession();
  if (s.role !== "admin") {
    throw new ApiRequestError(403, "forbidden", "This action requires the admin role.");
  }
  return s;
}

// ---------- fixture state ----------

const BOOT_AT = Date.now() - 6 * 3600_000 - 12 * 60_000; // server up 6h12m

const players: Player[] = [
  {
    uid: "84C20A31-1234-4B7E-9A11-000000000001",
    steamId: "76561198012345678",
    name: "Kestrel",
    accountName: "kestrel",
    online: true,
    level: 31,
    guildId: "g-nightloom",
    guildName: "Nightloom",
    ping: 23,
    // API locations are UE world cm; fixtures pick spots via their in-game display coords.
    location: gameToWorld(-361, 292),
    firstSeenAt: "2026-07-04T09:12:00Z",
    lastSeenAt: new Date().toISOString(),
    playtimeSec: 21 * 3600 + 36 * 60,
    banned: false,
    whitelisted: true,
  },
  {
    uid: "1F60E842-1234-4B7E-9A11-000000000002",
    steamId: "76561198087654321",
    name: "VossR",
    accountName: "vossr",
    online: true,
    level: 29,
    guildId: "g-nightloom",
    guildName: "Nightloom",
    ping: 41,
    location: gameToWorld(118, -412),
    firstSeenAt: "2026-07-04T10:02:00Z",
    lastSeenAt: new Date().toISOString(),
    playtimeSec: 18 * 3600 + 5 * 60,
    banned: false,
    whitelisted: true,
  },
  {
    uid: "5A9C2E10-1234-4B7E-9A11-000000000003",
    steamId: "76561198055512345",
    name: "mika_o",
    accountName: "mika_o",
    online: false,
    level: 27,
    guildId: "g-nightloom",
    guildName: "Nightloom",
    ping: null,
    location: null,
    firstSeenAt: "2026-07-05T08:00:00Z",
    lastSeenAt: "2026-07-09T22:18:00Z",
    playtimeSec: 15 * 3600 + 51 * 60,
    banned: false,
    whitelisted: true,
  },
  {
    uid: "3B7D1F44-1234-4B7E-9A11-000000000004",
    steamId: "76561198033398765",
    name: "HaruQ",
    accountName: "haruq",
    online: false,
    level: 14,
    guildId: "g-driftbone",
    guildName: "Driftbone",
    ping: null,
    location: null,
    firstSeenAt: "2026-07-06T14:20:00Z",
    lastSeenAt: "2026-07-06T19:51:00Z",
    playtimeSec: 6 * 3600 + 12 * 60,
    banned: false,
    whitelisted: false,
  },
  {
    uid: "9E4A6C77-1234-4B7E-9A11-000000000005",
    steamId: "76561198099911223",
    name: "tessellate",
    accountName: "tessellate",
    online: false,
    level: 8,
    guildId: null,
    guildName: null,
    ping: null,
    location: null,
    firstSeenAt: "2026-07-05T02:00:00Z",
    lastSeenAt: "2026-07-05T03:12:00Z",
    playtimeSec: 2 * 3600 + 4 * 60,
    banned: true,
    whitelisted: false,
  },
];

const guildNames = ["Nightloom", "Driftbone", "Cinderwake", "Palisade", "Thornmere", "Greywatch", "Amberfen"];
// Base spots in in-game display coords (roughly matching the mockup marker layout).
const baseSpots: Record<string, { x: number; y: number }[]> = {
  "g-nightloom": [
    { x: -660, y: 490 },
    { x: -80, y: -430 },
  ],
  "g-driftbone": [
    { x: 430, y: 370 },
    { x: 610, y: -160 },
  ],
  "g-cinderwake": [{ x: -300, y: -640 }],
  "g-palisade": [{ x: 250, y: 720 }],
};
const guilds: Guild[] = guildNames.map((name, i) => {
  const id = `g-${name.toLowerCase()}`;
  const members = players.filter((p) => p.guildId === id).map((p) => ({ uid: p.uid, name: p.name }));
  const spots = baseSpots[id] ?? [];
  return {
    id,
    name,
    adminUid: members[0]?.uid ?? `synthetic-${i}`,
    memberCount: Math.max(members.length, i === 0 ? 3 : i === 1 ? 2 : 1),
    members,
    bases: spots.map((spot, b) => ({
      id: `${id}-base-${b}`,
      location: gameToWorld(spot.x, spot.y),
      level: 3 + ((i + b) % 5),
    })),
  };
});

let whitelist: WhitelistEntry[] = [
  { steamId: "76561198012345678", name: "Kestrel" },
  { steamId: "76561198087654321", name: "VossR" },
  { steamId: "76561198055512345", name: "mika_o" },
];

const backups: Backup[] = [
  { id: "b1", file: "world-2026-07-09-2342.tar.gz", createdAt: "2026-07-09T23:42:07Z", sizeBytes: 14_680_064, trigger: "scheduled", worldDay: 3 },
  { id: "b2", file: "world-2026-07-09-1942.tar.gz", createdAt: "2026-07-09T19:42:11Z", sizeBytes: 14_600_000, trigger: "scheduled", worldDay: 3 },
  { id: "b3", file: "world-2026-07-09-1811.tar.gz", createdAt: "2026-07-09T18:11:03Z", sizeBytes: 14_600_000, trigger: "pre-restore", worldDay: 3 },
  { id: "b4", file: "world-2026-07-09-1542.tar.gz", createdAt: "2026-07-09T15:42:09Z", sizeBytes: 14_500_000, trigger: "scheduled", worldDay: 3 },
  { id: "b5", file: "world-2026-07-09-1142.tar.gz", createdAt: "2026-07-09T11:42:02Z", sizeBytes: 14_400_000, trigger: "scheduled", worldDay: 3 },
  { id: "b6", file: "world-2026-07-08-2342.tar.gz", createdAt: "2026-07-08T23:42:05Z", sizeBytes: 14_200_000, trigger: "scheduled", worldDay: 2 },
  { id: "b7", file: "world-2026-07-08-1633.tar.gz", createdAt: "2026-07-08T16:33:41Z", sizeBytes: 13_900_000, trigger: "manual", worldDay: 2 },
  { id: "b8", file: "world-2026-07-07-2216.tar.gz", createdAt: "2026-07-07T22:16:18Z", sizeBytes: 12_800_000, trigger: "manual", worldDay: 1 },
];

let schedule: BackupSchedule = {
  enabled: true,
  everyMinutes: 240,
  keepDays: 30,
  nextRunAt: new Date(Date.now() + 78 * 60_000).toISOString(),
};

const configGroups: ConfigSetting[] = [
  { key: "SERVER_NAME", value: "My Palworld Server", effectiveValue: "My Palworld Server", type: "string", group: "general", default: "Palworld Server", pending: false, editable: true, readOnly: false },
  { key: "SERVER_DESCRIPTION", value: "1.0 server", effectiveValue: "1.0 server", type: "string", group: "general", default: "", pending: false, editable: true, readOnly: false },
  { key: "SERVER_PASSWORD", value: "•••", effectiveValue: "•••", type: "string", group: "general", default: "", pending: false, editable: true, readOnly: false },
  { key: "ADMIN_PASSWORD", value: "•••", effectiveValue: "•••", type: "string", group: "general", default: "", pending: false, editable: true, readOnly: false },
  { key: "PLAYERS", value: 16, effectiveValue: 16, type: "integer", group: "general", default: 32, pending: false, editable: true, readOnly: false },
  { key: "EXP_RATE", value: 1.5, effectiveValue: 1, type: "number", group: "gameplay", default: 1, pending: true, editable: true, readOnly: false },
  { key: "PAL_CAPTURE_RATE", value: 1, effectiveValue: 1, type: "number", group: "gameplay", default: 1, pending: false, editable: true, readOnly: false },
  { key: "DAY_TIME_SPEED_RATE", value: 1, effectiveValue: 1, type: "number", group: "gameplay", default: 1, pending: false, editable: true, readOnly: false },
  { key: "NIGHT_TIME_SPEED_RATE", value: 1, effectiveValue: 1, type: "number", group: "gameplay", default: 1, pending: false, editable: true, readOnly: false },
  { key: "DIFFICULTY", value: "None", effectiveValue: "None", type: "string", group: "gameplay", default: "None", pending: false, editable: true, readOnly: false },
  { key: "DEATH_PENALTY", value: "All", effectiveValue: "All", type: "string", group: "gameplay", default: "All", pending: false, editable: true, readOnly: false },
  { key: "PUBLIC_PORT", value: 8211, effectiveValue: 8211, type: "integer", group: "network", default: 8211, pending: false, editable: true, readOnly: false },
  { key: "RCON_ENABLED", value: true, effectiveValue: true, type: "boolean", group: "panel-managed", default: true, pending: false, editable: false, readOnly: true },
  { key: "REST_API_ENABLED", value: true, effectiveValue: true, type: "boolean", group: "panel-managed", default: true, pending: false, editable: false, readOnly: true },
];

let configVersion = "mock:1";

const consoleLog: ConsoleLogEntry[] = [
  { at: "2026-07-09T22:30:11Z", user: "admin", command: "Info", output: "Welcome to Pal Server[v1.0.0.100427] My Palworld Server", isError: false },
  { at: "2026-07-09T22:31:40Z", user: "admin", command: "ShowPlayers", output: "name,playeruid,steamid\nKestrel,84C20A31,76561198012345678\nVossR,1F60E842,76561198087654321", isError: false },
  { at: "2026-07-09T22:33:02Z", user: "admin", command: "Broadcast Server_restarting_at_midnight", output: "Broadcasted: Server_restarting_at_midnight", isError: false },
  { at: "2026-07-09T22:33:20Z", user: "admin", command: "TeleportToPlayer 76561198012345678", output: "Error: this command is only available in-game.", isError: true },
  { at: "2026-07-09T22:36:48Z", user: "admin", command: "Save", output: "Complete Save", isError: false },
];

let savedCommands: SavedCommand[] = [
  { id: "s1", name: "Who's on", command: "ShowPlayers" },
  { id: "s2", name: "Save world", command: "Save" },
  { id: "s3", name: "Restart in 5 min", command: "Shutdown 300 Restarting_in_5_minutes" },
];

const events: PalhelmEvent[] = [
  { at: "2026-07-09T23:42:00Z", kind: "backup", message: "Scheduled backup completed — `14.6 MB` in 1.2 s" },
  { at: "2026-07-09T22:31:00Z", kind: "system", message: "Server started (world `A1B2C3D4…5678`)" },
  { at: "2026-07-09T22:30:00Z", kind: "panel", message: "Palhelm connected to RCON and REST API" },
  { at: "2026-07-09T18:04:00Z", kind: "leave", message: "**Kestrel** left after 2h 41m" },
  { at: "2026-07-09T15:23:00Z", kind: "join", message: "**Kestrel** joined ~steam_76561198012345678~" },
];

let fpsWobbleSeed = 0;

// ---------- auth ----------

export async function login(password: string): Promise<{ role: Role }> {
  await latency(150, 350);
  // Mock credentials: "admin" -> admin role, "viewer" -> viewer role, anything else fails.
  if (password === "admin") {
    const session: SessionInfo = { role: "admin", username: "admin" };
    writeSession(session);
    return { role: "admin" };
  }
  if (password === "viewer") {
    const session: SessionInfo = { role: "viewer", username: "viewer" };
    writeSession(session);
    return { role: "viewer" };
  }
  throw new ApiRequestError(401, "invalid_credentials", "Incorrect password.");
}

export async function logout(): Promise<void> {
  await latency();
  writeSession(null);
}

export async function session(): Promise<SessionInfo> {
  await latency(30, 120);
  return requireSession();
}

// ---------- server ----------

export async function getServer(): Promise<ServerInfo> {
  requireSession();
  await latency();
  return {
    name: "My Palworld Server",
    description: "1.0 server",
    version: "v1.0.0.100427",
    worldGuid: "A1B2C3D4E5F6478090ABCDEF12345678",
    state: "running",
    uptimeSec: Math.floor((Date.now() - BOOT_AT) / 1000),
    panelVersion: "0.8.0",
  };
}

export async function getServerHealth(): Promise<ServerHealth> {
  requireSession();
  await latency();
  return {
    rest: "ok",
    rcon: "ok",
    save: { state: "ok", lastSyncAt: new Date(Date.now() - 4 * 60_000).toISOString() },
  };
}

export async function announce(_message: string): Promise<void> {
  requireAdmin();
  await latency(150, 300);
}

export async function save(): Promise<void> {
  requireAdmin();
  await latency(200, 500);
}

export async function shutdown(_waitSec: number, _message: string, _countdown: boolean): Promise<void> {
  requireAdmin();
  await latency(200, 400);
}

export async function cancelShutdown(): Promise<void> {
  requireAdmin();
  await latency();
}

// ---------- metrics ----------

export async function metricsCurrent(): Promise<MetricsCurrent> {
  requireSession();
  await latency(40, 150);
  fpsWobbleSeed += 1;
  const fps = Math.round(wobble(59, 1.5));
  return {
    fps: Math.max(1, fps),
    fpsAvg: 59.3,
    frameTimeMs: Math.round((1000 / Math.max(1, fps)) * 10) / 10,
    players: players.filter((p) => p.online).length,
    maxPlayers: 16,
    day: 3,
    uptimeSec: Math.floor((Date.now() - BOOT_AT) / 1000),
    baseCamps: guilds.reduce((sum, g) => sum + g.bases.length, 0),
  };
}

function buildSeries(points: number, stepSec: number, dipAt?: number): MetricsHistory {
  const now = Math.floor(Date.now() / 1000);
  const t: number[] = [];
  const fps: number[] = [];
  const frameTimeMs: number[] = [];
  const playersSeries: number[] = [];
  for (let i = 0; i < points; i++) {
    const ts = now - (points - 1 - i) * stepSec;
    t.push(ts);
    let f = 59 + Math.sin(i / 6) * 1.2 + (Math.random() - 0.5) * 1.4;
    if (dipAt !== undefined && Math.abs(i - dipAt) <= 2) {
      const depth = 1 - Math.abs(i - dipAt) / 2.4;
      f -= 11 * depth;
    }
    f = Math.max(20, Math.round(f * 10) / 10);
    fps.push(f);
    frameTimeMs.push(Math.round((1000 / f) * 10) / 10);
    const p = Math.max(0, Math.round(1.4 + Math.sin(i / 20) * 1.3 + (Math.random() - 0.5) * 0.6));
    playersSeries.push(Math.min(16, p));
  }
  return { series: { t, fps, frameTimeMs, players: playersSeries } };
}

export async function metricsHistory(window: MetricsWindow): Promise<MetricsHistory> {
  requireSession();
  await latency(80, 200);
  if (window === "1h") return buildSeries(60, 60, 40);
  if (window === "24h") return buildSeries(288, 300, 210);
  return buildSeries(168, 3600, 140);
}

// ---------- players ----------

export async function listPlayers(): Promise<Player[]> {
  requireSession();
  await latency();
  return players.map((p) => ({ ...p, banned: p.banned }));
}

type MockPalBase = Omit<PlayerPal, "inParty" | "partySlot" | "boxPage" | "boxSlot">;

// Give mock pals a realistic placement: the first five fill the party, the rest
// flow into 30-slot box pages, so the party view and the box popup have data.
function withPlacement(pals: MockPalBase[]): PlayerPal[] {
  return pals.map((pal, i) =>
    i < 5
      ? { ...pal, inParty: true, partySlot: i, boxPage: null, boxSlot: null, placement: "party" as const, baseId: null }
      : { ...pal, inParty: false, partySlot: null, boxPage: Math.floor((i - 5) / 30), boxSlot: (i - 5) % 30, placement: "box" as const, baseId: null },
  );
}

const palsByPlayer: Record<string, MockPalBase[]> = {
  Kestrel: [
    {
      instanceId: "pal-k1", characterId: "Anubis", displayName: "Anubis", level: 34, isAlpha: true, isLucky: false,
      hp: 1240.5, gender: "male", talents: { hp: 87, melee: 73, shot: 92, defense: 81 },
      passiveSkillIds: ["CraftSpeed_up2", "ElementBoost_Earth_2_PAL"], equippedSkillIds: ["RockLance", "StoneShotgun", "GroundWave"],
    },
    { instanceId: "pal-k2", characterId: "Grizzbolt", displayName: "Grizzbolt", level: 31, isAlpha: false, isLucky: false },
    { instanceId: "pal-k3", characterId: "Faleris", displayName: "Faleris", level: 30, isAlpha: false, isLucky: false },
    { instanceId: "pal-k4", characterId: "Digtoise", displayName: "Digtoise", level: 27, isAlpha: false, isLucky: false },
    { instanceId: "pal-k5", characterId: "Penking", displayName: "Penking", level: 25, isAlpha: false, isLucky: true },
    { instanceId: "pal-k6", characterId: "Rayhound", displayName: "Rayhound", level: 24, isAlpha: false, isLucky: false },
    { instanceId: "pal-k7", characterId: "Tombat", displayName: "Tombat", level: 22, isAlpha: false, isLucky: false },
    { instanceId: "pal-k8", characterId: "Foxparks", displayName: "Foxparks", level: 19, isAlpha: false, isLucky: false },
    { instanceId: "pal-k9", characterId: "Lamball", displayName: "Lamball", level: 12, isAlpha: false, isLucky: false },
    { instanceId: "pal-k10", characterId: "Cattiva", displayName: "Cattiva", level: 11, isAlpha: false, isLucky: false },
    { instanceId: "pal-k11", characterId: "Chikipi", displayName: "Chikipi", level: 8, isAlpha: false, isLucky: false },
    { instanceId: "pal-k12", characterId: "Pengullet", displayName: "Pengullet", level: 7, isAlpha: false, isLucky: false },
  ],
  VossR: [
    { instanceId: "pal-v1", characterId: "Frostallion", displayName: "Frostallion", level: 32, isAlpha: false, isLucky: false },
    { instanceId: "pal-v2", characterId: "Ragnahawk", displayName: "Ragnahawk", level: 28, isAlpha: false, isLucky: false },
    { instanceId: "pal-v3", characterId: "Surfent", displayName: "Surfent", level: 26, isAlpha: false, isLucky: false },
    { instanceId: "pal-v4", characterId: "Direhowl", displayName: "Direhowl", level: 20, isAlpha: false, isLucky: false },
  ],
  mika_o: [
    { instanceId: "pal-m1", characterId: "Mossanda", displayName: "Mossanda", level: 27, isAlpha: false, isLucky: false },
    { instanceId: "pal-m2", characterId: "Bristla", displayName: "Bristla", level: 21, isAlpha: false, isLucky: false },
    { instanceId: "pal-m3", characterId: "Petallia", displayName: "Petallia", level: 18, isAlpha: false, isLucky: false },
  ],
  HaruQ: [
    { instanceId: "pal-h1", characterId: "Eikthyrdeer", displayName: "Eikthyrdeer", level: 13, isAlpha: false, isLucky: false },
    { instanceId: "pal-h2", characterId: "Fuack", displayName: "Fuack", level: 9, isAlpha: false, isLucky: false },
  ],
  tessellate: [{ instanceId: "pal-t1", characterId: "Depresso", displayName: "Depresso", level: 6, isAlpha: false, isLucky: false }],
};

export async function listPals(params: PalExplorerParams = {}): Promise<PalExplorerPage> {
  requireSession();
  await latency();
  const q = params.q?.trim().toLowerCase() ?? "";
  const all: PalExplorerPal[] = players
    .flatMap((owner) => withPlacement(palsByPlayer[owner.name] ?? []).map((pal) => ({
      ...pal,
      isBoss: pal.characterId.toLowerCase().startsWith("boss_"),
      placement: pal.placement ?? "unknown",
      ownerUid: owner.uid,
      ownerName: owner.name,
      ownerSource: "personal_container" as const,
      ownerResolved: true,
    })))
    .sort((a, b) => a.instanceId.localeCompare(b.instanceId));
  const filtered = all.filter((pal) => {
    if (params.cursor && pal.instanceId <= params.cursor) return false;
    if (q && !`${pal.displayName} ${pal.characterId} ${pal.ownerName}`.toLowerCase().includes(q)) return false;
    if (params.ownerSource && pal.ownerSource !== params.ownerSource) return false;
    if (params.placement && pal.placement !== params.placement) return false;
    if (params.minLevel !== undefined && pal.level < params.minLevel) return false;
    if (params.maxLevel !== undefined && pal.level > params.maxLevel) return false;
    if (params.specimen === "standard" && (pal.isAlpha || pal.isLucky || pal.isBoss)) return false;
    if (params.specimen === "alpha" && (!pal.isAlpha || pal.isBoss)) return false;
    if (params.specimen === "lucky" && !pal.isLucky) return false;
    if (params.specimen === "boss" && !pal.isBoss) return false;
    return true;
  });
  const limit = Math.max(1, Math.min(params.limit ?? 48, 100));
  const data = filtered.slice(0, limit);
  return { data, nextCursor: filtered.length > limit ? data.at(-1)?.instanceId ?? null : null };
}

export async function playerDetail(uid: string): Promise<PlayerDetail> {
  requireSession();
  await latency();
  const p = players.find((x) => x.uid === uid);
  if (!p) throw new ApiRequestError(404, "not_found", "Player not found.");
  const currentSession = p.online
    ? { joinedAt: new Date(Date.now() - 104 * 60_000).toISOString(), leftAt: null, durationSec: 104 * 60 }
    : null;
  const recentSessions = currentSession
    ? [currentSession]
    : [{ joinedAt: p.firstSeenAt, leftAt: p.lastSeenAt, durationSec: Math.min(p.playtimeSec, 9700) }];
  return {
    ...p,
    pals: withPlacement(palsByPlayer[p.name] ?? []),
    sessions: recentSessions,
    activity: {
      coverage: "panel_observed_sessions",
      trackingSince: p.firstSeenAt,
      currentSession,
      windows: {
        last24Hours: { durationSec: currentSession?.durationSec ?? 0, sessionCount: currentSession ? 1 : 0 },
        last7Days: { durationSec: Math.min(p.playtimeSec, 12 * 3600), sessionCount: Math.min(4, Math.max(1, Math.ceil(p.playtimeSec / 7200))) },
        last30Days: { durationSec: p.playtimeSec, sessionCount: Math.min(12, Math.max(1, Math.ceil(p.playtimeSec / 7200))) },
      },
      recentSessions,
      recentSessionsTruncated: false,
    },
  };
}

export async function getServerActivity(window: ServerActivityWindow = "7d"): Promise<ServerActivity> {
  requireSession();
  await latency();
  const now = new Date();
  const durationMs = window === "24h" ? 86_400_000 : window === "7d" ? 7 * 86_400_000 : 30 * 86_400_000;
  const bucketMs = window === "24h" ? 3_600_000 : window === "7d" ? 6 * 3_600_000 : 86_400_000;
  const since = new Date(now.getTime() - durationMs);
  const concurrency = Array.from({ length: durationMs / bucketMs }, (_, index) => {
    const wave = Math.max(0, Math.sin((index / 4) * Math.PI));
    const averagePlayers = Number((wave * 2.2).toFixed(2));
    return {
      at: new Date(since.getTime() + index * bucketMs).toISOString(),
      peakPlayers: Math.ceil(averagePlayers),
      averagePlayers,
    };
  });
  const rankedPlayers = [...players]
    .map((player, index) => ({
      uid: player.uid, name: player.name, guildId: player.guildId ?? "", guildName: player.guildName ?? "",
      durationSec: Math.min(player.playtimeSec, Math.floor(durationMs / 1000 / (index + 3))),
      sessionCount: Math.max(1, 6 - index), currentSession: player.online,
      firstObserved: new Date(player.firstSeenAt) >= since,
    }))
    .sort((a, b) => b.durationSec - a.durationSec);
  const guilds = guildNames.slice(0, 3).map((name) => {
    const members = rankedPlayers.filter((player) => player.guildName === name);
    return {
      guildId: `g-${name.toLowerCase()}`, guildName: name,
      durationSec: members.reduce((total, player) => total + player.durationSec, 0),
      sessionCount: members.reduce((total, player) => total + player.sessionCount, 0), activePlayers: members.length,
    };
  }).filter((guild) => guild.activePlayers > 0);
  const peakConcurrency = Math.max(0, ...concurrency.map((bucket) => bucket.peakPlayers));
  return {
    coverage: "panel_observed_sessions", trackingSince: players.map((player) => player.firstSeenAt).sort()[0] ?? null,
    window, since: since.toISOString(), through: now.toISOString(), bucketSec: bucketMs / 1000,
    analysisTruncated: false, activePlayers: rankedPlayers.length,
    newPlayers: rankedPlayers.filter((player) => player.firstObserved).length,
    returningPlayers: rankedPlayers.filter((player) => !player.firstObserved).length,
    peakConcurrency,
    peakAt: concurrency.find((bucket) => bucket.peakPlayers === peakConcurrency)?.at ?? null,
    concurrency, players: rankedPlayers, guilds, guildAttribution: "current_player_guild",
    unattributedPlayers: rankedPlayers.filter((player) => !player.guildId).length,
    unattributedDurationSec: rankedPlayers.filter((player) => !player.guildId).reduce((total, player) => total + player.durationSec, 0),
  };
}

export async function kickPlayer(uid: string, _message?: string): Promise<void> {
  requireAdmin();
  await latency(150, 300);
  const p = players.find((x) => x.uid === uid);
  if (p) {
    p.online = false;
    p.ping = null;
    p.location = null;
    p.lastSeenAt = new Date().toISOString();
    events.unshift({ at: new Date().toISOString(), kind: "leave", message: `**${p.name}** was kicked by admin` });
  }
}

export async function banPlayer(uid: string, _message?: string): Promise<void> {
  requireAdmin();
  await latency(150, 300);
  const p = players.find((x) => x.uid === uid);
  if (p) {
    p.banned = true;
    if (p.online) {
      p.online = false;
      p.ping = null;
      p.location = null;
      p.lastSeenAt = new Date().toISOString();
    }
    events.unshift({ at: new Date().toISOString(), kind: "panel", message: `**${p.name}** was banned by admin` });
  }
}

export async function unbanPlayer(uid: string): Promise<void> {
  requireAdmin();
  await latency(150, 300);
  const p = players.find((x) => x.uid === uid);
  if (p) p.banned = false;
}

export async function getWhitelist(): Promise<WhitelistEntry[]> {
  requireSession();
  await latency();
  return whitelist;
}

export async function putWhitelist(entries: WhitelistEntry[]): Promise<WhitelistEntry[]> {
  requireAdmin();
  await latency(150, 300);
  whitelist = entries;
  for (const p of players) {
    p.whitelisted = whitelist.some((w) => w.steamId === p.steamId);
  }
  return whitelist;
}

// ---------- guilds ----------

export async function listGuilds(): Promise<Guild[]> {
  requireSession();
  await latency();
  return guilds;
}

// ---------- integration keys ----------
// Admin key management (docs/specs/integration-api.md §9). This mock never validates a bearer
// token against `/api/integration/v1` — that surface isn't part of this frontend's scope — it
// only backs the Settings "Integration API" card: create/list/revoke.

const MAX_ACTIVE_INTEGRATION_KEYS = 100;
const INTEGRATION_KEY_LABEL_MAX = 64;

let integrationKeys: IntegrationKey[] = [];

const HEX_CHARS = "0123456789abcdef";
const BASE64URL_CHARS = "ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789-_";

function randomFrom(chars: string, len: number): string {
  let out = "";
  for (let i = 0; i < len; i++) out += chars[Math.floor(Math.random() * chars.length)];
  return out;
}

function generateIntegrationKeyId(): string {
  let id = randomFrom(HEX_CHARS, 8);
  while (integrationKeys.some((k) => k.id === id)) id = randomFrom(HEX_CHARS, 8);
  return id;
}

// Matches the real `phk_<8 hex>_<43 base64url>` shape (docs/specs/integration-api.md §2.1,
// 56 chars total) so the UI's copy/format handling is exercised honestly, but the secret half
// always carries an obviously-fake marker — this is generated in the browser, is never checked
// against anything, and is not a real credential.
function fakeIntegrationKeySecret(): string {
  const marker = "MOCKNOTAREALKEY-";
  return marker + randomFrom(BASE64URL_CHARS, 43 - marker.length);
}

export async function listIntegrationKeys(): Promise<IntegrationKey[]> {
  requireAdmin();
  await latency();
  return [...integrationKeys].sort((a, b) => b.createdAt.localeCompare(a.createdAt));
}

export async function createIntegrationKey(label: string): Promise<IntegrationKeyCreated> {
  requireAdmin();
  await latency(150, 300);
  const trimmed = label.trim();
  // Matches the backend's per-rune unicode.IsControl check (integration_keys.go), which
  // rejects C0 (U+0000–U+001F), DEL (U+007F), and C1 (U+0080–U+009F) controls.
  // eslint-disable-next-line no-control-regex -- rejecting control characters is the point
  const hasControlChars = /[\u0000-\u001f\u007f-\u009f]/.test(trimmed);
  if (!trimmed || trimmed.length > INTEGRATION_KEY_LABEL_MAX || hasControlChars) {
    throw new ApiRequestError(400, "invalid_request", "Label must be 1–64 characters with no control characters.");
  }
  const activeCount = integrationKeys.filter((k) => k.revokedAt === null).length;
  if (activeCount >= MAX_ACTIVE_INTEGRATION_KEYS) {
    throw new ApiRequestError(409, "too_many_keys", "The 100 active integration key limit has been reached.");
  }
  const id = generateIntegrationKeyId();
  const record: IntegrationKey = {
    id,
    label: trimmed,
    createdAt: new Date().toISOString(),
    lastUsedAt: null,
    revokedAt: null,
  };
  integrationKeys.push(record);
  events.unshift({ at: record.createdAt, kind: "panel", message: `Integration key "${trimmed}" created` });
  return { ...record, key: `phk_${id}_${fakeIntegrationKeySecret()}` };
}

export async function revokeIntegrationKey(id: string): Promise<IntegrationKey> {
  requireAdmin();
  await latency(150, 300);
  const record = integrationKeys.find((k) => k.id === id);
  if (!record) throw new ApiRequestError(404, "not_found", "Integration key not found.");
  if (record.revokedAt === null) {
    record.revokedAt = new Date().toISOString();
    events.unshift({ at: record.revokedAt, kind: "panel", message: `Integration key "${record.label}" revoked` });
  }
  return { ...record };
}

/** Test-only: node:test runs every test in this file in one process, so module state
 *  otherwise leaks between tests. Not imported by any app code. */
export function __resetIntegrationKeysForTests(): void {
  integrationKeys = [];
}

/** Test-only: seeds active (non-revoked) keys directly, bypassing `latency()`, so cap-boundary
 *  tests don't pay for 100 real create round-trips. Not imported by any app code. */
export function __seedActiveIntegrationKeysForTests(count: number): IntegrationKey[] {
  const seeded: IntegrationKey[] = [];
  for (let i = 0; i < count; i++) {
    const record: IntegrationKey = {
      id: generateIntegrationKeyId(),
      label: `seed-${i}`,
      createdAt: new Date(Date.now() - (count - i) * 1000).toISOString(),
      lastUsedAt: null,
      revokedAt: null,
    };
    integrationKeys.push(record);
    seeded.push(record);
  }
  return seeded;
}

// ---------- paldeck icons ----------

// A couple of entries so the "known id" branch in <PalIcon> is exercised in mock mode — the
// component itself still skips the actual <img> fetch under USE_MOCK (see components/PalIcon.tsx),
// since no icon files exist without a real fetch-pal-icons.sh run against a backend.
export async function getPaldeckIconDataset(): Promise<PaldeckIconDataset> {
  requireSession();
  await latency(30, 100);
  return {
    source: "palworld.gg",
    fetchedAt: new Date(Date.now() - 2 * 24 * 3600_000).toISOString(),
    count: 2,
    characterIds: ["anubis", "grizzbolt"],
  };
}

// ---------- map ----------

// The real THGL-sourced dataset.json written by scripts/fetch-map-tiles.sh into
// map-tiles-1.0/ (see docs/ROADMAP-v2.md) — reused verbatim here so `?mock&mocktiles` exercises
// the exact same per-layer transform the real deploy will serve, without needing a live backend.
const THGL_1_0_MAP_DATASET: MapDataset = {
  source: "thgl",
  fetched_at: "2026-07-10T13:05:22Z",
  game_version: "1.0",
  notes:
    "THGL maintainer notes the redrawn Palpagos offset still needs fixing upstream; treat pixel alignment as best-effort.",
  layers: [
    {
      id: "default",
      label: "Palpagos",
      path: "default",
      format: "webp",
      tile_size: 512,
      min_zoom: 0,
      max_zoom: 4,
      transform: { a: 0.000353395913859746, b: 256, c: -0.000353395913859746, d: 123.47653230259525 },
      bounds: [
        [-1099399, -724399],
        [349399, 724399],
      ],
    },
    {
      id: "tree",
      label: "World Tree",
      path: "tree",
      format: "webp",
      tile_size: 512,
      min_zoom: 0,
      max_zoom: 4,
      transform: { a: 0.0014979651664584533, b: 1225.6306053008072, c: -0.0014979651664584533, d: 1032.3204475170935 },
      bounds: [
        [347352.5, -818196],
        [689147.5, -476401],
      ],
    },
  ],
};

const PRE_1_0_MAP_DATASET: MapDataset = { fetched_at: null, game_version: "pre-1.0", source: "palworld.gg", layers: [] };

export async function getMapDataset(): Promise<MapDataset> {
  requireSession();
  await latency();
  // Mirrors Map.tsx's own ?mocktiles check: without it, mock mode simulates today's live data
  // dir (pre-1.0, no tiles); with it, mock mode simulates the post-deploy THGL 1.0 dataset.
  const mocktiles = typeof window !== "undefined" && new URLSearchParams(window.location.search).has("mocktiles");
  return mocktiles ? THGL_1_0_MAP_DATASET : PRE_1_0_MAP_DATASET;
}

// ---------- world ----------

export async function getWorld(): Promise<WorldInfo> {
  requireSession();
  await latency();
  return {
    day: 3,
    lastParseAt: new Date(Date.now() - 4 * 60_000).toISOString(),
    parseDurationMs: 1200,
    stats: { players: players.length, pals: 46, guilds: guilds.length, skippedProps: 0 },
    formatDrift: false,
  };
}

export async function getWorldSnapshot(): Promise<LiveWorldSnapshot> {
  requireSession();
  await latency();
  const online = players.filter((player) => player.online && player.location);
  return {
    state: "ready",
    capturedAt: new Date(Date.now() - 12_000).toISOString(),
    lastAttemptAt: new Date(Date.now() - 12_000).toISOString(),
    sourceTime: "2026-07-14 13:00:00",
    fps: 57,
    fpsAvg: 55.4,
    counts: { players: online.length, partyPals: online.length * 2, basePals: 18, wildPals: 84, npcs: 11, palBoxes: 2, unknown: 0 },
    activity: { working: 9, transporting: 2, eating: 1, sleeping: 2, idle: 2, inactive: 1, combat: 0, incapacitated: 1, moving: 0, unknown: 0 },
    actors: [
      ...online.map((player) => ({
        kind: "Player",
        name: player.name,
        guildName: player.guildName ?? undefined,
        level: player.level,
        activity: "idle" as const,
        active: true,
        location: { x: player.location!.x, y: player.location!.y, z: 0 },
      })),
      { kind: "BaseCampPal", characterId: "Anubis", name: "Anubis", level: 35, hpPercent: 88, active: true, activity: "working", linked: true, instanceId: "mock-pal-1", baseId: guilds[0]?.bases[0]?.id, ownerName: players[0]?.name, location: { x: guilds[0]?.bases[0]?.location.x ?? 0, y: guilds[0]?.bases[0]?.location.y ?? 0, z: 0 } },
    ],
    truncated: false,
    diagnostics: { lastRequestDurationMs: 184, lastAcceptedActorCount: 118, lastErrorCategory: "none", linkedBasePals: 18, unresolvedBasePals: 0, linkLookupFailed: false, scheduledDelayMs: 30000, nextAttemptAt: new Date(Date.now() + 18_000).toISOString() },
  };
}

export async function parseWorld(): Promise<void> {
  requireAdmin();
  await latency(400, 900);
}

// ---------- console ----------

function mockRconOutput(command: string): { output: string; isError: boolean } {
  const [verb, ...rest] = command.trim().split(/\s+/);
  switch ((verb ?? "").toLowerCase()) {
    case "info":
      return { output: "Welcome to Pal Server[v1.0.0.100427] My Palworld Server", isError: false };
    case "showplayers": {
      const online = players.filter((p) => p.online);
      const rows = online.map((p) => `${p.name},${p.uid.slice(0, 8)},${p.steamId}`);
      return { output: ["name,playeruid,steamid", ...rows].join("\n"), isError: false };
    }
    case "save":
      return { output: "Complete Save", isError: false };
    case "broadcast":
      return rest.length
        ? { output: `Broadcasted: ${rest.join(" ")}`, isError: false }
        : { output: "Error: Broadcast requires a message.", isError: true };
    case "shutdown":
      return { output: `Shutdown scheduled: ${rest.join(" ") || "now"}`, isError: false };
    case "kickplayer":
      return rest.length
        ? { output: `Kicked: ${rest[0]}`, isError: false }
        : { output: "Error: KickPlayer requires a steamid.", isError: true };
    default:
      return { output: `Unknown command: ${command}`, isError: true };
  }
}

export async function consoleExec(command: string): Promise<{ output: string }> {
  const s = requireAdmin();
  await latency(100, 300);
  const { output, isError } = mockRconOutput(command);
  consoleLog.push({ at: new Date().toISOString(), user: s.username, command, output, isError });
  return { output };
}

export async function consoleLogList(_limit: number): Promise<ConsoleLogEntry[]> {
  requireSession();
  await latency();
  return consoleLog;
}

export async function savedCommandsList(): Promise<SavedCommand[]> {
  requireSession();
  await latency();
  return savedCommands;
}

export async function savedCommandCreate(name: string, command: string): Promise<SavedCommand> {
  requireAdmin();
  await latency();
  const entry: SavedCommand = { id: `s${savedCommands.length + 1}`, name, command };
  savedCommands.push(entry);
  return entry;
}

export async function savedCommandDelete(id: string): Promise<void> {
  requireAdmin();
  await latency();
  savedCommands = savedCommands.filter((c) => c.id !== id);
}

// ---------- backups ----------

export async function listBackups(): Promise<Backup[]> {
  requireSession();
  await latency();
  return backups;
}

export async function createBackup(): Promise<Backup> {
  requireAdmin();
  await latency(300, 700);
  const b: Backup = {
    id: `b${backups.length + 1}`,
    file: `world-${new Date().toISOString().slice(0, 16).replace(/[-:T]/g, "").slice(0, 12)}.tar.gz`,
    createdAt: new Date().toISOString(),
    sizeBytes: 14_700_000,
    trigger: "manual",
    worldDay: 3,
  };
  backups.unshift(b);
  return b;
}

export async function backupContents(id: string): Promise<BackupContentEntry[]> {
  requireSession();
  await latency();
  const b = backups.find((x) => x.id === id);
  if (!b) throw new ApiRequestError(404, "not_found", "Backup not found.");
  return [
    { path: "Level.sav", sizeBytes: 13_900_000, modifiedAt: b.createdAt },
    { path: "LevelMeta.sav", sizeBytes: 4_096, modifiedAt: b.createdAt },
    { path: "Players/84C20A31.sav", sizeBytes: 210_000, modifiedAt: b.createdAt },
  ];
}

export async function restoreDryRun(id: string): Promise<BackupDryRun> {
  requireAdmin();
  await latency(300, 600);
  const b = backups.find((x) => x.id === id);
  if (!b) throw new ApiRequestError(404, "not_found", "Backup not found.");
  return {
    changes: [
      { path: "Players/84C20A31….sav", kind: "add", toSize: 210_000 },
      { path: "Level.sav", kind: "modify", fromSize: 14_600_000, toSize: 13_900_000 },
      { path: "1 base camp", kind: "delete" },
    ],
    requiresStop: true,
  };
}

export async function restore(id: string, confirm: string): Promise<void> {
  requireAdmin();
  if (confirm !== "RESTORE") throw new ApiRequestError(400, "bad_request", 'Type "RESTORE" to confirm.');
  const b = backups.find((x) => x.id === id);
  if (!b) throw new ApiRequestError(404, "not_found", "Backup not found.");
  await latency(500, 1200);
  // Docker control is disabled in this fixture and the game container is running, so the
  // API refuses per docs/API.md — this exercises the designed 409 state in the restore dialog.
  throw new ApiRequestError(
    409,
    "server_running",
    "The game server is still running, and Palhelm has no docker.sock access to stop it. Stop the container, then retry the restore.",
    { manualCommand: "docker compose stop palworld" },
  );
}

export async function deleteBackup(id: string): Promise<void> {
  requireAdmin();
  await latency(150, 300);
  const idx = backups.findIndex((x) => x.id === id);
  if (idx >= 0) backups.splice(idx, 1);
}

export async function getSchedule(): Promise<BackupSchedule> {
  requireSession();
  await latency();
  return schedule;
}

export async function setSchedule(next: BackupSchedule): Promise<BackupSchedule> {
  requireAdmin();
  await latency(150, 300);
  schedule = {
    ...next,
    nextRunAt: next.enabled ? new Date(Date.now() + next.everyMinutes * 60_000).toISOString() : null,
  };
  return schedule;
}

// ---------- config ----------

export async function getConfig(): Promise<ConfigDoc> {
  requireSession();
  await latency();
  return {
    source: "compose",
    composeFile: "/compose/docker-compose.yml",
    service: "palworld",
    version: configVersion,
    capabilities: {
      write: { available: true },
      apply: { available: false, reason: "One-click apply is intentionally disabled." },
    },
    manualCommand: "docker compose up -d palworld",
    settings: configGroups,
  };
}

export async function putConfig(version: string, changes: Record<string, ConfigValue>): Promise<ConfigDoc> {
  requireAdmin();
  await latency(200, 450);
  if (version !== configVersion) {
    throw new ApiRequestError(409, "config_conflict", "The compose file changed after it was loaded.");
  }
  for (const [key, value] of Object.entries(changes)) {
    const s = configGroups.find((c) => c.key === key);
    if (s) {
      s.value = value;
      s.pending = value !== s.effectiveValue;
    }
  }
  configVersion = `mock:${Number(configVersion.split(":")[1]) + 1}`;
  return getConfig();
}

export async function getConfigRaw(): Promise<string> {
  requireSession();
  await latency();
  // The live PalWorldSettings.ini reflects *effective* values (what the running server booted with).
  const opts = configGroups
    .filter((c) => c.key !== "ADMIN_PASSWORD" && c.key !== "SERVER_PASSWORD")
    .map((c) => {
      const v = c.type === "string" ? `"${c.effectiveValue}"` : c.effectiveValue;
      return `${c.key}=${v}`;
    })
    .join(",");
  return [
    "; This file is generated by the container entrypoint from compose environment variables.",
    "; Edits made here are overwritten on every boot — use the Palhelm settings editor instead.",
    "[/Script/Pal.PalGameWorldSettings]",
    `OptionSettings=(${opts},AdminPassword="***")`,
    "",
  ].join("\n");
}

export async function applyConfig(): Promise<void> {
  requireAdmin();
  await latency(400, 900);
  throw new ApiRequestError(
    501,
    "docker_apply_disabled",
    "One-click Docker apply is intentionally disabled; run the manual command from the host directory containing the compose file.",
    { manualCommand: "docker compose up -d palworld" },
  );
}

// ---------- events ----------

export async function listEvents(limit: number, kind?: string): Promise<PalhelmEvent[]> {
  requireSession();
  await latency();
  const filtered = kind ? events.filter((e) => e.kind === kind) : events;
  return filtered.slice(0, limit);
}
