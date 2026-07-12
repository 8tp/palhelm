import { useEffect, useRef, useState, type ReactNode } from "react";
import { useLocation, useNavigate } from "react-router";
import { useQuery } from "@tanstack/react-query";
import { Command } from "cmdk";
import { api } from "../api/client";
import type { Player, SavedCommand } from "../api/types";
import { useAuth, useIsAdmin } from "../app/AuthProvider";
import { usePaletteBridge, type PlayerActionKind } from "../app/paletteBridge";
import { Dialog } from "./ConfirmDialog";
import { Pill, type PillTone } from "./Pill";
import { NAV_ITEMS } from "./Shell";
import { IconSearch } from "./icons";
import { loadPaletteRecents, savePaletteRecent, type RecentEntry } from "./paletteRecents";
import "../styles/command-palette.css";

function loadRecents(isAdmin: boolean): RecentEntry[] {
  return loadPaletteRecents(localStorage, isAdmin);
}

function saveRecent(entry: RecentEntry, isAdmin: boolean): RecentEntry[] {
  return savePaletteRecent(localStorage, entry, isAdmin);
}

const PLAYER_ACTION_LABEL: Record<PlayerActionKind, (name: string) => string> = {
  kick: (name) => `Kick ${name}…`,
  ban: (name) => `Ban ${name}…`,
  unban: (name) => `Unban ${name}`,
};

const STATUS_TONE = (p: Player): PillTone => (p.banned ? "danger" : p.online ? "ok" : "idle");
const STATUS_LABEL = (p: Player): string => (p.banned ? "Banned" : p.online ? "Online" : "Offline");

/**
 * The ⌘K / Ctrl+K command palette. cmdk drives fuzzy filtering, keyboard nav, and
 * grouped results (see docs/research/raw/shadcn-eval.md §4); the outer chrome is the
 * app's existing native-<dialog> `Dialog` so focus trap / Esc-to-close come free, just
 * like every other dialog in the app. Player-scoped actions and the saved-command
 * insert never touch the API directly — they hand off to app/paletteBridge.tsx, which
 * the existing Players.tsx / Console.tsx / HelmStrip already-built dialogs consume.
 * No new mutation logic lives here.
 */
export function CommandPalette({ open, onClose }: { open: boolean; onClose: () => void }) {
  const navigate = useNavigate();
  const location = useLocation();
  const isAdmin = useIsAdmin();
  const bridge = usePaletteBridge();
  const [query, setQuery] = useState("");
  const [recents, setRecents] = useState<RecentEntry[]>([]);
  const inputRef = useRef<HTMLInputElement>(null);

  useEffect(() => {
    if (!isAdmin) setRecents(loadRecents(false));
  }, [isAdmin]);

  useEffect(() => {
    if (open) {
      setQuery("");
      setRecents(loadRecents(isAdmin));
    }
  }, [open, isAdmin]);

  // cmdk's own `autoFocus` on Command.Input fires once at mount time, while the palette's
  // <dialog> is still closed (display: none) — the call silently no-ops and never refires on
  // later opens. Focus explicitly instead, once per open. This effect lives in the parent
  // (CommandPalette), so it commits after Dialog's own effect has already called showModal() —
  // otherwise the dialog's native initial-focus behavior can steal focus back to itself.
  useEffect(() => {
    if (open) inputRef.current?.focus();
  }, [open]);

  const playersQuery = useQuery({ queryKey: ["players"], queryFn: () => api.players.list(), enabled: open });
  const savedQuery = useQuery({ queryKey: ["console", "saved"], queryFn: () => api.console.savedList(), enabled: open });
  const players = playersQuery.data ?? [];
  const saved = savedQuery.data ?? [];

  function close() {
    onClose();
  }

  function remember(entry: RecentEntry) {
    setRecents(saveRecent(entry, isAdmin));
  }

  function goTo(path: string) {
    if (location.pathname !== path) navigate(path);
  }

  function selectNav(item: (typeof NAV_ITEMS)[number]) {
    navigate(item.to);
    remember({ kind: "nav", key: item.to, label: item.label });
    close();
  }

  function selectPlayer(p: Player) {
    goTo("/players");
    remember({ kind: "player", key: p.uid, label: p.name });
    close();
  }

  function runPlayerAction(kind: PlayerActionKind, p: Player) {
    bridge.requestPlayerAction(kind, p.uid);
    goTo("/players");
    remember({ kind, key: p.uid, label: PLAYER_ACTION_LABEL[kind](p.name) });
    close();
  }

  function runBroadcast() {
    bridge.openBroadcast();
    remember({ kind: "broadcast", key: "", label: "Broadcast a message…" });
    close();
  }

  function insertSaved(c: SavedCommand) {
    bridge.requestConsoleInsert(c.command);
    goTo("/console");
    remember({ kind: "saved", key: c.id, label: c.name });
    close();
  }

  function runRecent(r: RecentEntry) {
    switch (r.kind) {
      case "nav":
        navigate(r.key);
        remember(r);
        close();
        return;
      case "player": {
        const p = players.find((pl) => pl.uid === r.key);
        if (p) selectPlayer(p);
        return;
      }
      case "kick":
      case "ban":
      case "unban": {
        const p = players.find((pl) => pl.uid === r.key);
        if (p) runPlayerAction(r.kind, p);
        return;
      }
      case "broadcast":
        runBroadcast();
        return;
      case "saved": {
        const c = saved.find((sc) => sc.id === r.key);
        if (c) insertSaved(c);
        return;
      }
    }
  }

  // Non-admins never see mutating entries (kick/ban/unban/broadcast) — mirrors the
  // existing role-gating pattern (viewer role hides destructive controls app-wide).
  const showActions = isAdmin && players.length > 0;

  return (
    <Dialog open={open} title="Command palette" onClose={close} className="dialog-command" hideHead>
      <Command shouldFilter loop label="Command palette" className="palette-command">
        <div className="palette-input-row">
          <IconSearch />
          <Command.Input
            ref={inputRef}
            value={query}
            onValueChange={setQuery}
            placeholder="Search players, navigate, run a saved command…"
          />
          <kbd className="palette-kbd">Esc</kbd>
        </div>
        <Command.List>
          <Command.Empty>No results.</Command.Empty>

          {query.trim() === "" && recents.length > 0 && (
            <Command.Group heading="Recent">
              {recents.map((r) => (
                <PaletteItem key={`recent-${r.kind}-${r.key}`} value={`recent ${r.label}`} onSelect={() => runRecent(r)}>
                  {r.label}
                </PaletteItem>
              ))}
            </Command.Group>
          )}

          <Command.Group heading="Navigation">
            {NAV_ITEMS.map((item) => (
              <PaletteItem key={item.to} value={`navigate go to ${item.label}`} onSelect={() => selectNav(item)}>
                <item.icon />
                <span className="palette-row">
                  <span>{item.label}</span>
                  <span className="palette-hint">{item.to}</span>
                </span>
              </PaletteItem>
            ))}
          </Command.Group>

          <Command.Group heading="Players">
            {players.map((p) => (
              <PaletteItem key={p.uid} value={`player ${p.name} ${p.steamId}`} onSelect={() => selectPlayer(p)}>
                <span className="palette-row">
                  <span>{p.name}</span>
                  <Pill tone={STATUS_TONE(p)}>{STATUS_LABEL(p)}</Pill>
                </span>
              </PaletteItem>
            ))}
          </Command.Group>

          {showActions && (
            <Command.Group heading="Actions">
              <PaletteItem value="broadcast message announce server" onSelect={runBroadcast}>
                Broadcast a message…
              </PaletteItem>
              {players.map((p) =>
                p.banned ? (
                  <PaletteItem key={`unban-${p.uid}`} value={`unban ${p.name}`} onSelect={() => runPlayerAction("unban", p)}>
                    {PLAYER_ACTION_LABEL.unban(p.name)}
                  </PaletteItem>
                ) : (
                  <PaletteItem
                    key={`kick-${p.uid}`}
                    value={`kick ${p.name}`}
                    disabled={!p.online}
                    onSelect={() => runPlayerAction("kick", p)}
                  >
                    {PLAYER_ACTION_LABEL.kick(p.name)}
                  </PaletteItem>
                ),
              )}
              {players
                .filter((p) => !p.banned)
                .map((p) => (
                  <PaletteItem key={`ban-${p.uid}`} value={`ban ${p.name}`} onSelect={() => runPlayerAction("ban", p)} danger>
                    {PLAYER_ACTION_LABEL.ban(p.name)}
                  </PaletteItem>
                ))}
            </Command.Group>
          )}

          {saved.length > 0 && (
            <Command.Group heading="Saved RCON commands">
              {saved.map((c) => (
                <PaletteItem key={c.id} value={`saved command ${c.name} ${c.command}`} onSelect={() => insertSaved(c)}>
                  <span className="palette-row">
                    <span>{c.name}</span>
                    <code className="palette-hint">{c.command}</code>
                  </span>
                </PaletteItem>
              ))}
            </Command.Group>
          )}
        </Command.List>
        <div className="palette-foot">
          <span>
            <kbd className="palette-kbd">↑↓</kbd> navigate
          </span>
          <span>
            <kbd className="palette-kbd">↵</kbd> select
          </span>
          <span>
            <kbd className="palette-kbd">esc</kbd> close
          </span>
        </div>
      </Command>
    </Dialog>
  );
}

function PaletteItem({
  children,
  danger = false,
  ...props
}: {
  children: ReactNode;
  value: string;
  disabled?: boolean;
  danger?: boolean;
  onSelect: () => void;
}) {
  return (
    <Command.Item className={danger ? "palette-item-danger" : undefined} {...props}>
      {children}
    </Command.Item>
  );
}

/**
 * Mounts the palette once near the app root and owns the global ⌘K / Ctrl+K listener.
 * Sibling to ToastProvider/TooltipProvider per the eval's recommendation (§4). Only
 * live once authenticated — nothing it offers (nav, players, actions, saved commands)
 * means anything on the login screen.
 */
export function CommandPaletteProvider({ children }: { children: ReactNode }) {
  const { status } = useAuth();
  const authenticated = status === "authenticated";
  const [open, setOpen] = useState(false);

  useEffect(() => {
    if (!authenticated) return;
    function onKeyDown(e: KeyboardEvent) {
      if ((e.metaKey || e.ctrlKey) && e.key.toLowerCase() === "k") {
        e.preventDefault();
        setOpen((o) => !o);
      }
    }
    window.addEventListener("keydown", onKeyDown);
    return () => window.removeEventListener("keydown", onKeyDown);
  }, [authenticated]);

  return (
    <>
      {children}
      {authenticated && <CommandPalette open={open} onClose={() => setOpen(false)} />}
    </>
  );
}
