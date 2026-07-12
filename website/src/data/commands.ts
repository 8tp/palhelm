/**
 * The bot's full slash-command ledger: 32 commands (31 + /help), 5 admin-gated.
 * Source of truth: bot/src/commands/index.ts (verified against the bot report).
 * Ordered by category so the unfiltered "All" view reads as grouped sections.
 */
export type CommandCategory =
  | "Server"
  | "Players"
  | "Pals"
  | "Breeding"
  | "History"
  | "AI"
  | "Admin";

export type Filter = "All" | CommandCategory;

export interface Command {
  /** the slash name, e.g. "/status" */
  command: string;
  /** option signature, e.g. "<name>" or "[layer]"; omitted when the command takes none */
  args?: string;
  /** one honest sentence, sentence case */
  description: string;
  category: CommandCategory;
  /** gated behind a Discord role AND the panel session API */
  admin: boolean;
}

/** filter tabs in display order */
export const FILTERS: Filter[] = [
  "All",
  "Server",
  "Players",
  "Pals",
  "Breeding",
  "History",
  "AI",
  "Admin",
];

export const commands: Command[] = [
  // ── Server ──────────────────────────────────────────────
  {
    command: "/status",
    description:
      "Server name, state, version, uptime, online count, world day, and FPS.",
    category: "Server",
    admin: false,
  },
  {
    command: "/metrics",
    description:
      "FPS, frame time, player count, world day, uptime, and base-camp counts.",
    category: "Server",
    admin: false,
  },
  {
    command: "/map",
    args: "[layer]",
    description: "World map image with guild bases and live players plotted.",
    category: "Server",
    admin: false,
  },
  {
    command: "/guilds",
    description: "Every guild with its member and base counts.",
    category: "Server",
    admin: false,
  },
  {
    command: "/help",
    description: "A categorized directory of every command.",
    category: "Server",
    admin: false,
  },

  // ── Players ─────────────────────────────────────────────
  {
    command: "/players",
    description: "Who is online right now.",
    category: "Players",
    admin: false,
  },
  {
    command: "/player",
    args: "<name>",
    description: "A player profile card plus their top pals, with autocomplete.",
    category: "Players",
    admin: false,
  },
  {
    command: "/compare",
    args: "<a> <b>",
    description: "Two players side by side.",
    category: "Players",
    admin: false,
  },
  {
    command: "/leaderboard",
    args: "[category]",
    description:
      "Rankings by level, playtime, pal count, rare pals, or guild; switchable inline.",
    category: "Players",
    admin: false,
  },
  {
    command: "/profile",
    args: "link|status|unlink",
    description: "Link a Discord user to a Palworld player for self queries.",
    category: "Players",
    admin: false,
  },

  // ── Pals ────────────────────────────────────────────────
  {
    command: "/pal",
    args: "<pal> [player]",
    description:
      "Inspect one owned pal: work suitability, stats, skills, and placement.",
    category: "Pals",
    admin: false,
  },
  {
    command: "/pals",
    args: "<name>",
    description: "A player's pals rendered as an icon-grid image.",
    category: "Pals",
    admin: false,
  },
  {
    command: "/box",
    args: "<name> [page]",
    description: "Browse a player's pal storage box with prev/next buttons.",
    category: "Pals",
    admin: false,
  },
  {
    command: "/whohas",
    args: "<pal>",
    description: "Find the current owners of a species.",
    category: "Pals",
    admin: false,
  },
  {
    command: "/rare",
    args: "[player]",
    description: "A gallery of boss, alpha, and lucky pals.",
    category: "Pals",
    admin: false,
  },
  {
    command: "/collection",
    args: "[player]",
    description: "306-pal completion, missing species, and rare variants.",
    category: "Pals",
    admin: false,
  },
  {
    command: "/dex",
    args: "<pal>",
    description:
      "1.0 mechanics, learnset, work, ownership, and icon for a species.",
    category: "Pals",
    admin: false,
  },
  {
    command: "/workers",
    args: "<job> [player]",
    description: "Rank worker pals for a base job.",
    category: "Pals",
    admin: false,
  },
  {
    command: "/team",
    args: "<purpose> [player]",
    description: "Recommend a combat party or a base-role roster.",
    category: "Pals",
    admin: false,
  },
  {
    command: "/progress",
    args: "[player]",
    description: "Lifetime captures, unique captures, and paldeck unlocks.",
    category: "Pals",
    admin: false,
  },
  {
    command: "/goal",
    args: "add|list|remove",
    description: "Restart-safe collection goals that notify on completion.",
    category: "Pals",
    admin: false,
  },

  // ── Breeding ────────────────────────────────────────────
  {
    command: "/breed",
    args: "<child> [player]",
    description: "Rank parent pairs for a target child by what's owned.",
    category: "Breeding",
    admin: false,
  },
  {
    command: "/breedpath",
    args: "<target> [scope] [player]",
    description: "The shortest breeding chain from an owned roster to a target.",
    category: "Breeding",
    admin: false,
  },

  // ── History ─────────────────────────────────────────────
  {
    command: "/history",
    args: "[filter]",
    description: "A paginated feed of joins, leaves, backups, and panel events.",
    category: "History",
    admin: false,
  },
  {
    command: "/trends",
    args: "[window]",
    description: "Level, playtime, and roster movement over time.",
    category: "History",
    admin: false,
  },
  {
    command: "/records",
    description: "Current server records for players, pals, and guilds.",
    category: "History",
    admin: false,
  },

  // ── AI ──────────────────────────────────────────────────
  {
    command: "/ask",
    args: "<question> [private]",
    description:
      "A read-only Palworld assistant with live server tools, a built-in game guide, and cited web search.",
    category: "AI",
    admin: false,
  },

  // ── Admin (Discord role + panel session API) ────────────
  {
    command: "/backup",
    description: "Trigger a world backup now.",
    category: "Admin",
    admin: true,
  },
  {
    command: "/backups",
    description: "Recent backups plus the schedule.",
    category: "Admin",
    admin: true,
  },
  {
    command: "/announce",
    args: "<message>",
    description: "Broadcast an in-game message.",
    category: "Admin",
    admin: true,
  },
  {
    command: "/diagnostics",
    description: "Cache, knowledge, history, AI, and automation status.",
    category: "Admin",
    admin: true,
  },
  {
    command: "/profileadmin",
    args: "assign|clear",
    description: "Manage other members' player links.",
    category: "Admin",
    admin: true,
  },
];
