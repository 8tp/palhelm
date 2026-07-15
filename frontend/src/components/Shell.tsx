import { useEffect, useRef, useState, type ComponentType } from "react";
import { NavLink, Outlet } from "react-router";
import { useQuery, useQueryClient } from "@tanstack/react-query";
import { api, USE_MOCK } from "../api/client";
import { useAuth, useIsAdmin } from "../app/AuthProvider";
import { useToast } from "./Toast";
import { usePaletteBridge } from "../app/paletteBridge";
import { formatDuration } from "../app/format";
import {
  HelmMark,
  IconActivity,
  IconBackups,
  IconConfig,
  IconConsole,
  IconMap,
  IconOverview,
  IconPlayers,
  IconSettings,
  IconEvents,
  IconInfo,
  IconPals,
  type IconProps,
} from "./icons";
import { Pill } from "./Pill";
import { Sparkline } from "./Sparkline";
import { Dialog } from "./ConfirmDialog";
import { Field } from "./Field";
import { useSSE } from "../app/useSSE";
import type { MetricsCurrent, PalhelmEvent } from "../api/types";

export interface NavItem {
  to: string;
  end?: boolean;
  label: string;
  icon: ComponentType<IconProps>;
  /** Rail section header this item falls under; omit for the top ungrouped items. */
  group?: string;
}

/**
 * Single source of truth for the app's routes: the rail below and the command palette's
 * Navigation group (components/CommandPalette.tsx) both render from this list so they
 * can never drift apart.
 */
export const NAV_ITEMS: NavItem[] = [
  { to: "/", end: true, label: "Overview", icon: IconOverview },
  { to: "/players", label: "Players", icon: IconPlayers },
  { to: "/activity", label: "Activity", icon: IconActivity },
  { to: "/pals", label: "Pal explorer", icon: IconPals },
  { to: "/map", label: "Live map", icon: IconMap },
  { to: "/events", label: "Events", icon: IconEvents },
  { to: "/console", label: "Console", icon: IconConsole },
  { to: "/backups", label: "Backups", icon: IconBackups, group: "World" },
  { to: "/config", label: "Configuration", icon: IconConfig, group: "World" },
  { to: "/diagnostics", label: "Diagnostics", icon: IconInfo, group: "Panel" },
  { to: "/settings", label: "Settings", icon: IconSettings, group: "Panel" },
];

export function Shell() {
  return (
    <div className="shell">
      <LiveQueryBridge />
      <Rail />
      <div className="main">
        <HelmStrip />
        <Outlet />
      </div>
    </div>
  );
}

function LiveQueryBridge() {
  const queryClient = useQueryClient();
  useSSE({
    url: "/api/v1/events/stream",
    // Mock mode has no backend to serve the stream; connecting only yields a 404 and
    // a failed EventSource. react-query's refetchInterval polling keeps the mock UI live.
    enabled: !USE_MOCK,
    onMessage: (eventName, data) => {
      if (eventName === "metrics") {
        queryClient.setQueryData<MetricsCurrent>(["metrics", "current"], data as MetricsCurrent);
        return;
      }
      if (eventName === "players") {
        void queryClient.invalidateQueries({ queryKey: ["players"] });
        void queryClient.invalidateQueries({ queryKey: ["activity"] });
        void queryClient.invalidateQueries({ queryKey: ["guilds"] });
        void queryClient.invalidateQueries({ queryKey: ["server", "health"] });
        return;
      }
      if (eventName === "event") {
        const event = data as Partial<PalhelmEvent>;
        void queryClient.invalidateQueries({ queryKey: ["events"] });
        if (event.kind === "backup") void queryClient.invalidateQueries({ queryKey: ["backups"] });
        if (event.kind === "system") {
          void queryClient.invalidateQueries({ queryKey: ["server"] });
          void queryClient.invalidateQueries({ queryKey: ["server", "health"] });
        }
      }
    },
  });
  return null;
}

function Rail() {
  const { username, role } = useAuth();
  const serverQuery = useQuery({ queryKey: ["server"], queryFn: () => api.server.get() });
  return (
    <nav className="rail" aria-label="Primary">
      <div className="rail-brand">
        <HelmMark size={26} className="mark" />
        <span className="word">
          pal<b>helm</b>
        </span>
      </div>

      <div className="rail-nav">
        {NAV_ITEMS.map((item, i) => (
          <div key={item.to} style={{ display: "contents" }}>
            {item.group && item.group !== NAV_ITEMS[i - 1]?.group && <div className="rail-group">{item.group}</div>}
            <NavLink className="rail-item" to={item.to} end={item.end}>
              <item.icon />
              {item.label}
            </NavLink>
          </div>
        ))}
      </div>

      <div className="rail-foot">
        <span className="who">
          <b>{username ?? "—"}</b>
          <span>{role ?? ""}</span>
        </span>
        <span>{serverQuery.data?.panelVersion ? `v${serverQuery.data.panelVersion}` : ""}</span>
      </div>
    </nav>
  );
}

function HelmStrip() {
  const isAdmin = useIsAdmin();
  const fpsHistory = useRef<number[]>([]);
  const [sparkTick, setSparkTick] = useState(0);
  const [dialog, setDialog] = useState<"broadcast" | "save" | "shutdown" | null>(null);
  const bridge = usePaletteBridge();

  // HelmStrip is mounted for the whole authenticated app, so the ⌘K palette can open
  // this exact Broadcast dialog from any route without owning its own copy.
  useEffect(() => {
    return bridge.registerOpenBroadcast(() => setDialog("broadcast"));
  }, [bridge]);

  const metricsQuery = useQuery({
    queryKey: ["metrics", "current"],
    queryFn: () => api.metrics.current(),
    refetchInterval: 5000,
  });
  const serverQuery = useQuery({
    queryKey: ["server"],
    queryFn: () => api.server.get(),
    refetchInterval: 15000,
  });

  useEffect(() => {
    if (metricsQuery.data == null) return;
    const buf = fpsHistory.current;
    buf.push(metricsQuery.data.fps);
    if (buf.length > 14) buf.shift();
    setSparkTick((t) => t + 1);
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [metricsQuery.data]);

  const metrics = metricsQuery.data;
  const server = serverQuery.data;
  const online = server?.state === "running";

  return (
    <header className="helmstrip" aria-label="Server status">
      <div className="instrument">
        {serverQuery.isLoading ? (
          <span className="skel skel-text" style={{ width: 64 }} />
        ) : serverQuery.isError ? (
          <Pill tone="danger">Unreachable</Pill>
        ) : (
          <Pill tone={online ? "ok" : "idle"}>{online ? "Online" : (server?.state ?? "Unknown")}</Pill>
        )}
      </div>

      <div className="instrument">
        <div>
          <span className="label">Server FPS</span>
          <span className="value">
            {metrics ? metrics.fps : "—"} <small>avg {metrics ? metrics.fpsAvg.toFixed(1) : "—"}</small>
          </span>
        </div>
        <Sparkline key={sparkTick} values={fpsHistory.current.length ? fpsHistory.current : [1, 1]} />
      </div>

      <div className="instrument">
        <div>
          <span className="label">Players</span>
          <span className="value">
            {metrics ? metrics.players : "—"}
            <small>/{metrics ? metrics.maxPlayers : "—"}</small>
          </span>
        </div>
      </div>

      <div className="instrument">
        <div>
          <span className="label">In-game</span>
          <span className="value">Day {metrics ? metrics.day : "—"}</span>
        </div>
      </div>

      <div className="instrument">
        <div>
          <span className="label">Uptime</span>
          <span className="value">{metrics ? formatDuration(metrics.uptimeSec) : "—"}</span>
        </div>
      </div>

      <div className="grow" />

      {isAdmin && (
        <div className="actions">
          <button type="button" className="btn btn-sm" onClick={() => setDialog("broadcast")}>
            Broadcast
          </button>
          <button type="button" className="btn btn-sm" onClick={() => setDialog("save")}>
            Save world
          </button>
          <button type="button" className="btn btn-sm btn-danger" onClick={() => setDialog("shutdown")}>
            Shut down…
          </button>
        </div>
      )}

      <BroadcastDialog open={dialog === "broadcast"} onClose={() => setDialog(null)} />
      <SaveDialog open={dialog === "save"} onClose={() => setDialog(null)} />
      <ShutdownDialog open={dialog === "shutdown"} onClose={() => setDialog(null)} />
    </header>
  );
}

function BroadcastDialog({ open, onClose }: { open: boolean; onClose: () => void }) {
  const [message, setMessage] = useState("");
  const [busy, setBusy] = useState(false);
  const toast = useToast();

  async function send() {
    if (!message.trim()) return;
    setBusy(true);
    try {
      await api.server.announce(message.trim());
      toast.push("Broadcast sent to all players.", "ok");
      setMessage("");
      onClose();
    } catch {
      toast.push("Broadcast failed. Try again.", "danger");
    } finally {
      setBusy(false);
    }
  }

  return (
    <Dialog
      open={open}
      title="Broadcast a message"
      onClose={onClose}
      footer={
        <>
          <button type="button" className="btn btn-ghost" onClick={onClose}>
            Cancel
          </button>
          <button type="button" className="btn btn-primary" onClick={send} disabled={busy || !message.trim()}>
            {busy ? "Sending…" : "Send broadcast"}
          </button>
        </>
      }
    >
      <Field
        label="Message"
        placeholder="Server maintenance begins in 10 minutes"
        value={message}
        onChange={(e) => setMessage(e.target.value)}
        autoFocus
      />
    </Dialog>
  );
}

function SaveDialog({ open, onClose }: { open: boolean; onClose: () => void }) {
  const [busy, setBusy] = useState(false);
  const toast = useToast();
  const queryClient = useQueryClient();

  async function run() {
    setBusy(true);
    try {
      await api.server.save();
      toast.push("World save triggered.", "ok");
      queryClient.invalidateQueries({ queryKey: ["server"] });
      onClose();
    } catch {
      toast.push("Save failed. Try again.", "danger");
    } finally {
      setBusy(false);
    }
  }

  return (
    <Dialog
      open={open}
      title="Save world"
      onClose={onClose}
      footer={
        <>
          <button type="button" className="btn btn-ghost" onClick={onClose}>
            Cancel
          </button>
          <button type="button" className="btn btn-primary" onClick={run} disabled={busy}>
            {busy ? "Saving…" : "Save now"}
          </button>
        </>
      }
    >
      <p style={{ color: "var(--ink-2)", fontSize: "var(--text-sm)" }}>
        Triggers an immediate world save on the running server.
      </p>
    </Dialog>
  );
}

function ShutdownDialog({ open, onClose }: { open: boolean; onClose: () => void }) {
  const [waitSec, setWaitSec] = useState(60);
  const [message, setMessage] = useState("Server shutting down for maintenance");
  const [countdown, setCountdown] = useState(true);
  const [busy, setBusy] = useState(false);
  const toast = useToast();

  async function run() {
    setBusy(true);
    try {
      await api.server.shutdown(waitSec, message, countdown);
      toast.push(`Graceful shutdown scheduled in ${waitSec}s.`, "ok");
      onClose();
    } catch {
      toast.push("Shutdown failed to schedule.", "danger");
    } finally {
      setBusy(false);
    }
  }

  return (
    <Dialog
      open={open}
      title="Schedule graceful shutdown"
      onClose={onClose}
      footer={
        <>
          <button type="button" className="btn btn-ghost" onClick={onClose}>
            Cancel
          </button>
          <button type="button" className="btn btn-danger-solid" onClick={run} disabled={busy}>
            {busy ? "Scheduling…" : "Shut down server"}
          </button>
        </>
      }
    >
      <Field
        label="Wait time (seconds)"
        type="number"
        min={0}
        value={waitSec}
        onChange={(e) => setWaitSec(Number(e.target.value))}
      />
      <Field label="Message" value={message} onChange={(e) => setMessage(e.target.value)} />
      <label className="dialog-checkbox">
        <input type="checkbox" checked={countdown} onChange={(e) => setCountdown(e.target.checked)} />
        Send staged warnings (10/5/1 min, 30/10 s)
      </label>
      <p style={{ color: "var(--ink-2)", fontSize: "var(--text-sm)" }}>
        Palhelm stops the game server gracefully but cannot start it again. Any restart depends on your external service manager.
      </p>
    </Dialog>
  );
}
