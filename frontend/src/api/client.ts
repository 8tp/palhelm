// Typed fetch client for the Palhelm REST API (docs/API.md).
// When VITE_MOCK=1 or a `?mock` query param is present, every call routes to mock.ts instead,
// so `npm run dev` is fully browsable with no backend.
import * as mock from "./mock";
import { ApiRequestError } from "./types";
import { parseConfigDoc } from "./configContract";
import { notifyUnauthorized } from "./requestPolicy";
import type {
  Backup,
  BackupContentEntry,
  BackupDryRun,
  BackupSchedule,
  BackupStorage,
  ConfigDoc,
  ConfigValue,
  ConsoleLogEntry,
  Guild,
  GuildDetail,
  IntegrationKey,
  IntegrationKeyCreated,
  MapDataset,
  MetricsCurrent,
  MetricsHistory,
  MetricsWindow,
  PalExplorerPage,
  PalExplorerParams,
  PaldeckIconDataset,
  PlayerPaldeck,
  PalhelmEvent,
  Player,
  PlayerDetail,
  Role,
  SavedCommand,
  ServerActivity,
  ServerActivityWindow,
  ServerHealth,
  ServerPaldeck,
  ServerInfo,
  SessionInfo,
  WhitelistEntry,
  WorldInfo,
  LiveWorldSnapshot,
  LiveWorldActivityHistory,
} from "./types";

export const USE_MOCK =
  import.meta.env.VITE_MOCK === "1" ||
  (typeof window !== "undefined" && new URLSearchParams(window.location.search).has("mock"));

const BASE = "/api/v1";

async function responseError(res: Response): Promise<ApiRequestError> {
  let code = "unknown_error";
  let message = res.statusText || "Request failed";
  let extra: Record<string, unknown> = {};
  try {
    const data = (await res.json()) as Record<string, unknown> & { error?: Record<string, unknown> };
    if (data.error) {
      const { code: c, message: m, ...errorExtra } = data.error;
      if (typeof c === "string") code = c;
      if (typeof m === "string") message = m;
      const envelopeExtra = { ...data };
      delete envelopeExtra.error;
      extra = { ...envelopeExtra, ...errorExtra };
    }
  } catch {
    // response wasn't JSON — keep the defaults above
  }
  notifyUnauthorized(res.status);
  return new ApiRequestError(res.status, code, message, extra);
}

async function request<Res>(method: string, path: string, body?: unknown): Promise<Res> {
  const res = await fetch(BASE + path, {
    method,
    headers: body !== undefined ? { "Content-Type": "application/json" } : {},
    body: body !== undefined ? JSON.stringify(body) : undefined,
    credentials: "include",
  });
  if (!res.ok) {
    throw await responseError(res);
  }
  if (res.status === 204) return undefined as Res;
  const text = await res.text();
  return (text ? JSON.parse(text) : undefined) as Res;
}

async function requestText(method: string, path: string): Promise<string> {
  const res = await fetch(BASE + path, { method, credentials: "include" });
  if (!res.ok) throw await responseError(res);
  return res.text();
}

export const api = {
  auth: {
    login: (password: string): Promise<{ role: Role }> =>
      USE_MOCK ? mock.login(password) : request("POST", "/auth/login", { password }),
    logout: (): Promise<void> => (USE_MOCK ? mock.logout() : request("POST", "/auth/logout")),
    session: (): Promise<SessionInfo> => (USE_MOCK ? mock.session() : request("GET", "/auth/session")),
  },
  server: {
    get: (): Promise<ServerInfo> => (USE_MOCK ? mock.getServer() : request("GET", "/server")),
    health: (): Promise<ServerHealth> => (USE_MOCK ? mock.getServerHealth() : request("GET", "/server/health")),
    announce: (message: string): Promise<void> =>
      USE_MOCK ? mock.announce(message) : request("POST", "/server/announce", { message }),
    save: (): Promise<void> => (USE_MOCK ? mock.save() : request("POST", "/server/save")),
    shutdown: (waitSec: number, message: string, countdown: boolean): Promise<void> =>
      USE_MOCK
        ? mock.shutdown(waitSec, message, countdown)
        : request("POST", "/server/shutdown", { waitSec, message, countdown }),
    cancelShutdown: (): Promise<void> =>
      USE_MOCK ? mock.cancelShutdown() : request("POST", "/server/shutdown/cancel"),
  },
  metrics: {
    current: (): Promise<MetricsCurrent> => (USE_MOCK ? mock.metricsCurrent() : request("GET", "/metrics/current")),
    history: (window: MetricsWindow): Promise<MetricsHistory> =>
      USE_MOCK ? mock.metricsHistory(window) : request("GET", `/metrics/history?window=${window}`),
  },
  activity: {
    get: (window: ServerActivityWindow = "7d"): Promise<ServerActivity> =>
      USE_MOCK ? mock.getServerActivity(window) : request("GET", `/activity?window=${window}`),
  },
  players: {
    list: (): Promise<Player[]> => (USE_MOCK ? mock.listPlayers() : request("GET", "/players")),
    detail: (uid: string): Promise<PlayerDetail> =>
      USE_MOCK ? mock.playerDetail(uid) : request("GET", `/players/${uid}`),
    kick: (uid: string, message?: string): Promise<void> =>
      USE_MOCK ? mock.kickPlayer(uid, message) : request("POST", `/players/${uid}/kick`, { message }),
    ban: (uid: string, message?: string): Promise<void> =>
      USE_MOCK ? mock.banPlayer(uid, message) : request("POST", `/players/${uid}/ban`, { message }),
    unban: (uid: string): Promise<void> =>
      USE_MOCK ? mock.unbanPlayer(uid) : request("POST", `/players/${uid}/unban`),
    // Builds the <img src> for a player's proxied Steam avatar (404 = none, handled by the
    // caller's onError fallback to the placeholder mark). Proxied same-origin for CSP.
    avatarUrl: (uid: string): string => `${BASE}/players/${encodeURIComponent(uid)}/avatar`,
  },
  pals: {
    list: (params: PalExplorerParams = {}): Promise<PalExplorerPage> => {
      if (USE_MOCK) return mock.listPals(params);
      const query = new URLSearchParams();
      if (params.cursor) query.set("cursor", params.cursor);
      if (params.limit !== undefined) query.set("limit", String(params.limit));
      if (params.q) query.set("q", params.q);
      if (params.ownerSource) query.set("ownerSource", params.ownerSource);
      if (params.placement) query.set("placement", params.placement);
      if (params.specimen) query.set("specimen", params.specimen);
      if (params.minLevel !== undefined) query.set("minLevel", String(params.minLevel));
      if (params.maxLevel !== undefined) query.set("maxLevel", String(params.maxLevel));
      const suffix = query.size > 0 ? `?${query}` : "";
      return request("GET", `/pals${suffix}`);
    },
  },
  whitelist: {
    get: (): Promise<WhitelistEntry[]> => (USE_MOCK ? mock.getWhitelist() : request("GET", "/whitelist")),
    put: (entries: WhitelistEntry[]): Promise<WhitelistEntry[]> =>
      USE_MOCK ? mock.putWhitelist(entries) : request("PUT", "/whitelist", entries),
  },
  guilds: {
    list: (): Promise<Guild[]> => (USE_MOCK ? mock.listGuilds() : request("GET", "/guilds")),
    detail: (id: string): Promise<GuildDetail> =>
      USE_MOCK ? mock.guildDetail(id) : request("GET", `/guilds/${encodeURIComponent(id)}`),
  },
  // Session-authenticated admin key management (docs/specs/integration-api.md §9) — not the
  // bearer-token Integration API surface itself, which this frontend never calls directly.
  integrationKeys: {
    list: (): Promise<IntegrationKey[]> =>
      USE_MOCK ? mock.listIntegrationKeys() : request("GET", "/integration-keys"),
    create: (label: string): Promise<IntegrationKeyCreated> =>
      USE_MOCK ? mock.createIntegrationKey(label) : request("POST", "/integration-keys", { label }),
    revoke: (id: string): Promise<IntegrationKey> =>
      USE_MOCK ? mock.revokeIntegrationKey(id) : request("DELETE", `/integration-keys/${id}`),
  },
  paldeck: {
    get: (): Promise<ServerPaldeck> => (USE_MOCK ? mock.getServerPaldeck() : request("GET", "/paldeck")),
    player: (uid: string): Promise<PlayerPaldeck> =>
      USE_MOCK ? mock.getPlayerPaldeck(uid) : request("GET", `/players/${encodeURIComponent(uid)}/paldeck`),
    iconDataset: (): Promise<PaldeckIconDataset> =>
      USE_MOCK ? mock.getPaldeckIconDataset() : request("GET", "/paldeck/icon-dataset"),
    // Not a JSON call — this builds the <img src> for a pal icon (404 = not installed, handled
    // by the caller's onError fallback). Lowercased to match the backend's case-insensitive lookup.
    iconUrl: (characterId: string): string => `${BASE}/paldeck/icon/${encodeURIComponent(characterId.toLowerCase())}`,
  },
  map: {
    dataset: (): Promise<MapDataset> => (USE_MOCK ? mock.getMapDataset() : request("GET", "/map/dataset")),
  },
  world: {
    get: (): Promise<WorldInfo> => (USE_MOCK ? mock.getWorld() : request("GET", "/world")),
    snapshot: (): Promise<LiveWorldSnapshot> =>
      USE_MOCK ? mock.getWorldSnapshot() : request("GET", "/world/snapshot"),
    activity: (window: MetricsWindow = "1h"): Promise<LiveWorldActivityHistory> =>
      request("GET", `/world/activity?window=${window}`),
    parse: (): Promise<void> => (USE_MOCK ? mock.parseWorld() : request("POST", "/world/parse")),
  },
  console: {
    exec: (command: string): Promise<{ output: string }> =>
      USE_MOCK ? mock.consoleExec(command) : request("POST", "/console/exec", { command }),
    log: (limit = 200): Promise<ConsoleLogEntry[]> =>
      USE_MOCK ? mock.consoleLogList(limit) : request("GET", `/console/log?limit=${limit}`),
    savedList: (): Promise<SavedCommand[]> =>
      USE_MOCK ? mock.savedCommandsList() : request("GET", "/console/saved"),
    savedCreate: (name: string, command: string): Promise<SavedCommand> =>
      USE_MOCK ? mock.savedCommandCreate(name, command) : request("POST", "/console/saved", { name, command }),
    savedDelete: (id: string): Promise<void> =>
      USE_MOCK ? mock.savedCommandDelete(id) : request("DELETE", `/console/saved/${id}`),
  },
  backups: {
    list: (): Promise<Backup[]> => (USE_MOCK ? mock.listBackups() : request("GET", "/backups")),
    create: (): Promise<Backup> => (USE_MOCK ? mock.createBackup() : request("POST", "/backups")),
    contents: (id: string): Promise<BackupContentEntry[]> =>
      USE_MOCK ? mock.backupContents(id) : request("GET", `/backups/${id}/contents`),
    dryRun: (id: string): Promise<BackupDryRun> =>
      USE_MOCK ? mock.restoreDryRun(id) : request("POST", `/backups/${id}/restore/dry-run`),
    restore: (id: string, confirm: string): Promise<void> =>
      USE_MOCK ? mock.restore(id, confirm) : request("POST", `/backups/${id}/restore`, { confirm }),
    remove: (id: string): Promise<void> => (USE_MOCK ? mock.deleteBackup(id) : request("DELETE", `/backups/${id}`)),
    schedule: (): Promise<BackupSchedule> =>
      USE_MOCK ? mock.getSchedule() : request("GET", "/backups/schedule"),
    storage: (): Promise<BackupStorage> =>
      USE_MOCK ? mock.getStorage() : request("GET", "/backups/storage"),
    setSchedule: (s: BackupSchedule): Promise<BackupSchedule> =>
      USE_MOCK ? mock.setSchedule(s) : request("PUT", "/backups/schedule", s),
  },
  config: {
    get: async (): Promise<ConfigDoc> => parseConfigDoc(USE_MOCK ? await mock.getConfig() : await request("GET", "/config")),
    put: async (version: string, changes: Record<string, ConfigValue>): Promise<ConfigDoc> =>
      parseConfigDoc(USE_MOCK ? await mock.putConfig(version, changes) : await request("PUT", "/config", { version, changes })),
    raw: (): Promise<string> => (USE_MOCK ? mock.getConfigRaw() : requestText("GET", "/config/raw")),
    apply: (): Promise<void> => (USE_MOCK ? mock.applyConfig() : request("POST", "/config/apply")),
  },
  events: {
    list: (limit = 100, kind?: string): Promise<PalhelmEvent[]> =>
      USE_MOCK ? mock.listEvents(limit, kind) : request("GET", `/events?limit=${limit}${kind ? `&kind=${kind}` : ""}`),
  },
};

export { ApiRequestError };
