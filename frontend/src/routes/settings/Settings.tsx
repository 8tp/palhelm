import { useState } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { api } from "../../api/client";
import { ApiRequestError, type IntegrationKey, type IntegrationKeyCreated } from "../../api/types";
import { formatDateTime, formatRelativeToNow } from "../../app/format";
import { applyThemeChoice, getThemeChoice, type ThemeChoice } from "../../app/theme";
import { useIsAdmin } from "../../app/AuthProvider";
import { Card, CardBody, CardHead } from "../../components/Card";
import { Pill } from "../../components/Pill";
import { Banner } from "../../components/Banner";
import { EmptyState } from "../../components/EmptyState";
import { Dialog, ConfirmDialog } from "../../components/ConfirmDialog";
import { Field } from "../../components/Field";
import { CodeWell } from "../../components/CodeWell";
import { useToast } from "../../components/Toast";
import "./Settings.css";

export default function SettingsRoute() {
  const isAdmin = useIsAdmin();
  const serverQuery = useQuery({ queryKey: ["server"], queryFn: () => api.server.get() });
  const healthQuery = useQuery({ queryKey: ["server", "health"], queryFn: () => api.server.health(), refetchInterval: 15000 });
  const configQuery = useQuery({ queryKey: ["config"], queryFn: () => api.config.get() });

  const health = healthQuery.data;
  const panelVersion = serverQuery.data?.panelVersion;

  return (
    <main className="content">
      <div className="page-head">
        <h1>Panel settings</h1>
      </div>

      <div className="grid cols-2">
        {/* connections */}
        <Card>
          <CardHead title="Connections" />
          {healthQuery.isError ? (
            <CardBody>
              <Banner tone="warn">Couldn't reach the panel API for connection health.</Banner>
            </CardBody>
          ) : (
            <CardBody flush>
              <table className="table">
                <tbody>
                  <tr>
                    <td style={{ color: "var(--ink-2)" }}>REST API</td>
                    <td className="num">game server</td>
                    <td>
                      {health ? (
                        <Pill tone={health.rest === "ok" ? "ok" : "danger"}>{health.rest === "ok" ? "Connected" : "Error"}</Pill>
                      ) : (
                        "—"
                      )}
                    </td>
                  </tr>
                  <tr>
                    <td style={{ color: "var(--ink-2)" }}>RCON</td>
                    <td className="num">game server</td>
                    <td>
                      {health ? (
                        <Pill tone={health.rcon === "ok" ? "ok" : "danger"}>{health.rcon === "ok" ? "Connected" : "Error"}</Pill>
                      ) : (
                        "—"
                      )}
                    </td>
                  </tr>
                  <tr>
                    <td style={{ color: "var(--ink-2)" }}>Save data</td>
                    <td className="num">mounted volume</td>
                    <td>
                      {health ? (
                        <Pill tone={health.save.state === "ok" ? "ok" : "danger"}>
                          {health.save.state === "ok" ? `synced ${formatRelativeToNow(health.save.lastSyncAt)}` : "Error"}
                        </Pill>
                      ) : (
                        "—"
                      )}
                    </td>
                  </tr>
                  <tr>
                    <td style={{ color: "var(--ink-2)" }}>Compose file</td>
                    <td className="num" title={configQuery.data?.composeFile}>
                      {configQuery.data?.composeFile ?? "—"}
                    </td>
                    <td>
                      <Pill tone="idle">read-write</Pill>
                    </td>
                  </tr>
                </tbody>
              </table>
            </CardBody>
          )}
        </Card>

        {/* save sync */}
        <SaveSyncCard />
      </div>

      <div className="grid cols-2">
        <GameDataDiagnosticsCard />
      </div>

      <div className="grid cols-2">
        {/* authentication */}
        <Card>
          <CardHead title="Authentication" />
          <CardBody style={{ display: "flex", flexDirection: "column", gap: "var(--space-3)" }}>
            <div className="field">
              <label htmlFor="auth-admin">Admin password</label>
              <input id="auth-admin" className="input input-mono" type="text" value="PALHELM_ADMIN_PASSWORD" readOnly />
              <span className="field-hint">set via environment variable — not editable from the panel</span>
            </div>
            <div className="field">
              <label htmlFor="auth-viewer">Viewer role</label>
              <input id="auth-viewer" className="input input-mono" type="text" value="PALHELM_VIEWER_PASSWORD" readOnly />
              <span className="field-hint">optional read-only login, also configured via environment variable</span>
            </div>
            <div className="field">
              <label htmlFor="auth-session">Session duration</label>
              <select id="auth-session" className="input" style={{ width: 200 }} defaultValue="7" disabled>
                <option value="1">1 day</option>
                <option value="7">7 days</option>
                <option value="30">30 days</option>
              </select>
              <span className="field-hint">configured via PALHELM_SESSION_DAYS — shown here for reference</span>
            </div>
          </CardBody>
        </Card>

        {/* appearance */}
        <AppearanceCard />
      </div>

      {isAdmin && (
        <div className="grid cols-2">
          <IntegrationApiCard />
        </div>
      )}

      <div className="grid cols-2">
        <Card className="span-2">
          <CardHead title="About" />
          <CardBody style={{ display: "flex", flexDirection: "column", gap: "var(--space-2)" }}>
            <div className="kv-row">
              <span className="k">Palhelm</span>
              <span className="v">{panelVersion ? `v${panelVersion}` : "—"}</span>
            </div>
            <div className="kv-row">
              <span className="k">License</span>
              <span className="v">Apache-2.0</span>
            </div>
            <div className="about-links">
              <a href="https://github.com/" target="_blank" rel="noreferrer">
                Documentation
              </a>
              <a href="https://github.com/" target="_blank" rel="noreferrer">
                Source
              </a>
              <a href="https://github.com/" target="_blank" rel="noreferrer">
                Report an issue
              </a>
              <a href="https://github.com/" target="_blank" rel="noreferrer">
                Release notes
              </a>
            </div>
          </CardBody>
        </Card>
      </div>
    </main>
  );
}

function GameDataDiagnosticsCard() {
  const query = useQuery({
    queryKey: ["world", "snapshot"],
    queryFn: () => api.world.snapshot(),
    refetchInterval: 15000,
  });
  const snapshot = query.data;
  const state = snapshot?.state ?? "pending";
  const tone = state === "ready" ? "ok" : state === "pending" ? "idle" : "warn";
  const diagnostics = snapshot?.diagnostics;
  const activity = snapshot?.activity;
  return (
    <Card className="span-2">
      <CardHead title="Game Data API diagnostics">
        <Pill tone={tone}>{state}</Pill>
      </CardHead>
      <CardBody>
        {query.isError ? (
          <Banner tone="warn">Couldn't load Game Data API diagnostics.</Banner>
        ) : (
          <>
            <div className="grid cols-2">
              <div>
                <div className="kv-row"><span className="k">Snapshot freshness</span><span className="v">{snapshot?.capturedAt ? formatRelativeToNow(snapshot.capturedAt) : "no accepted snapshot"}</span></div>
                <div className="kv-row"><span className="k">Upstream request</span><span className="v mono">{diagnostics ? `${diagnostics.lastRequestDurationMs} ms` : "—"}</span></div>
                <div className="kv-row"><span className="k">Loaded actors</span><span className="v mono">{diagnostics?.lastAcceptedActorCount ?? "—"}</span></div>
                <div className="kv-row"><span className="k">FPS</span><span className="v mono">{snapshot ? `${snapshot.fps.toFixed(1)} · avg ${snapshot.fpsAvg.toFixed(1)}` : "—"}</span></div>
              </div>
              <div>
                <div className="kv-row"><span className="k">Exact worker links</span><span className="v mono">{diagnostics ? `${diagnostics.linkedBasePals}/${snapshot?.counts.basePals ?? 0}` : "—"}</span></div>
                <div className="kv-row"><span className="k">Unresolved workers</span><span className="v mono">{diagnostics?.unresolvedBasePals ?? "—"}</span></div>
                <div className="kv-row"><span className="k">Last poll result</span><span className="v">{diagnostics?.lastErrorCategory ?? "—"}</span></div>
                <div className="kv-row"><span className="k">Next attempt</span><span className="v">{diagnostics?.nextAttemptAt ? formatRelativeToNow(diagnostics.nextAttemptAt) : "not scheduled"}</span></div>
              </div>
            </div>
            {diagnostics?.linkLookupFailed && <Banner tone="warn">The snapshot loaded, but save-derived worker identity linkage failed.</Banner>}
            {activity && (
              <p className="field-hint mono" style={{ marginTop: "var(--space-3)" }}>
                workers · {activity.working} working · {activity.transporting} transporting · {activity.eating} eating · {activity.sleeping} sleeping · {activity.idle} idle · {activity.incapacitated} incapacitated · {activity.unknown} unknown
              </p>
            )}
          </>
        )}
      </CardBody>
    </Card>
  );
}

// ---------------- save sync ----------------

function SaveSyncCard() {
  const isAdmin = useIsAdmin();
  const queryClient = useQueryClient();
  const toast = useToast();
  const worldQuery = useQuery({ queryKey: ["world"], queryFn: () => api.world.get() });

  const parseMutation = useMutation({
    mutationFn: () => api.world.parse(),
    onSuccess: () => {
      toast.push("Save re-parse started.", "ok");
      queryClient.invalidateQueries({ queryKey: ["world"] });
      queryClient.invalidateQueries({ queryKey: ["players"] });
      queryClient.invalidateQueries({ queryKey: ["guilds"] });
    },
    onError: () => toast.push("Parse is already running or failed to start.", "danger"),
  });

  const w = worldQuery.data;

  return (
    <Card>
      <CardHead title="Save sync">{w?.formatDrift && <Pill tone="warn">format drift</Pill>}</CardHead>
      {worldQuery.isError ? (
        <CardBody>
          <Banner tone="warn">Couldn't load save-sync status.</Banner>
        </CardBody>
      ) : (
        <CardBody style={{ display: "flex", flexDirection: "column", gap: "var(--space-3)" }}>
          <div className="field">
            <label htmlFor="sync-interval">Interval</label>
            <select id="sync-interval" className="input" style={{ width: 200 }} defaultValue="10" disabled>
              <option value="1">1 minute</option>
              <option value="5">5 minutes</option>
              <option value="10">10 minutes</option>
              <option value="30">30 minutes</option>
            </select>
            <span className="field-hint">configured via PALHELM_SYNC_MINUTES — shown here for reference</span>
          </div>
          {isAdmin && (
            <div>
              <button type="button" className="btn" disabled={parseMutation.isPending} onClick={() => parseMutation.mutate()}>
                {parseMutation.isPending ? "Parsing…" : "Parse now"}
              </button>
            </div>
          )}
          {w && (
            <span className="field-hint mono">
              last parse {(w.parseDurationMs / 1000).toFixed(1)} s · {w.stats.guilds} guilds · {w.stats.players} players ·{" "}
              {w.stats.skippedProps} skipped properties
            </span>
          )}
        </CardBody>
      )}
    </Card>
  );
}

// ---------------- appearance ----------------

function AppearanceCard() {
  const [choice, setChoice] = useState<ThemeChoice>(() => getThemeChoice());

  function pick(next: ThemeChoice) {
    applyThemeChoice(next);
    setChoice(next);
  }

  const options: { key: ThemeChoice; label: string }[] = [
    { key: "system", label: "System" },
    { key: "dark", label: "Dark" },
    { key: "light", label: "Light" },
  ];

  return (
    <Card>
      <CardHead title="Appearance" />
      <CardBody>
        <div className="field">
          <label>Theme</label>
          <div className="theme-row" role="radiogroup" aria-label="Theme">
            {options.map((o) => (
              <button
                key={o.key}
                type="button"
                className="btn"
                role="radio"
                aria-pressed={choice === o.key}
                aria-checked={choice === o.key}
                onClick={() => pick(o.key)}
              >
                {o.label}
              </button>
            ))}
          </div>
        </div>
      </CardBody>
    </Card>
  );
}

// ---------------- integration api ----------------
// Admin-only key management for the read-only bearer-token Integration API
// (docs/specs/integration-api.md §9). Rendered only when isAdmin — viewers never see this
// card and the underlying API returns 403 to them regardless.

const INTEGRATION_KEY_LABEL_MAX = 64;
const INTEGRATION_KEY_CAP = 100;

function IntegrationApiCard() {
  const queryClient = useQueryClient();
  const toast = useToast();
  const keysQuery = useQuery({ queryKey: ["integration-keys"], queryFn: () => api.integrationKeys.list() });

  const [creating, setCreating] = useState(false);
  const [revoking, setRevoking] = useState<IntegrationKey | null>(null);
  // Held only in local component state, never in the query cache — discarded on dismiss and
  // never re-derivable afterward (see RevealIntegrationKeyDialog).
  const [revealedKey, setRevealedKey] = useState<IntegrationKeyCreated | null>(null);

  const revokeMutation = useMutation({
    mutationFn: (id: string) => api.integrationKeys.revoke(id),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ["integration-keys"] });
      toast.push("Integration key revoked.", "ok");
      setRevoking(null);
    },
    onError: () => toast.push("Couldn't revoke the key. Try again.", "danger"),
  });

  const keys = keysQuery.data ?? [];
  const activeCount = keys.filter((k) => k.revokedAt === null).length;
  const atCap = activeCount >= INTEGRATION_KEY_CAP;

  return (
    <Card className="span-2">
      <CardHead title="Integration API" hint={`${activeCount} active key${activeCount === 1 ? "" : "s"}`}>
        <button type="button" className="btn btn-sm" disabled={atCap} onClick={() => setCreating(true)}>
          + New key
        </button>
      </CardHead>

      {keysQuery.isError ? (
        <CardBody>
          <Banner tone="warn">Couldn't load integration keys.</Banner>
        </CardBody>
      ) : keysQuery.isLoading ? (
        <CardBody>
          <span className="skel skel-text" style={{ width: "100%", height: 80 }} />
        </CardBody>
      ) : keys.length === 0 ? (
        <CardBody>
          <EmptyState
            title="No integration keys yet"
            description="Create a key to let a bot or dashboard read player, pal, and guild data over the read-only Integration API."
          />
        </CardBody>
      ) : (
        <CardBody flush style={{ overflowX: "auto" }}>
          <table className="table">
            <thead>
              <tr>
                <th>Label</th>
                <th>Key ID</th>
                <th>Created</th>
                <th>Last used</th>
                <th>Status</th>
                <th></th>
              </tr>
            </thead>
            <tbody>
              {keys.map((k) => (
                <tr key={k.id}>
                  <td>{k.label}</td>
                  <td className="num">{k.id}</td>
                  <td className="num">{formatDateTime(k.createdAt)}</td>
                  <td className="num">{formatRelativeToNow(k.lastUsedAt)}</td>
                  <td>
                    <Pill tone={k.revokedAt ? "idle" : "ok"}>{k.revokedAt ? "revoked" : "active"}</Pill>
                  </td>
                  <td style={{ textAlign: "right" }}>
                    {!k.revokedAt && (
                      <button type="button" className="btn btn-sm btn-ghost" onClick={() => setRevoking(k)}>
                        Revoke
                      </button>
                    )}
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
        </CardBody>
      )}

      {atCap && (
        <CardBody style={{ paddingTop: 0 }}>
          <Banner tone="warn">Active key limit reached ({INTEGRATION_KEY_CAP}/{INTEGRATION_KEY_CAP}) — revoke a key before creating another.</Banner>
        </CardBody>
      )}

      <CardBody style={{ display: "flex", flexDirection: "column", gap: "var(--space-2)", paddingTop: 0 }}>
        <span className="field-hint">Read-only bearer-token access for bots and dashboards:</span>
        <CodeWell>curl -H &quot;Authorization: Bearer phk_...&quot; https://host/api/integration/v1/players</CodeWell>
      </CardBody>

      <NewIntegrationKeyDialog
        open={creating}
        onClose={() => setCreating(false)}
        onCreated={(created) => {
          setCreating(false);
          setRevealedKey(created);
          queryClient.invalidateQueries({ queryKey: ["integration-keys"] });
        }}
      />

      <RevealIntegrationKeyDialog keyRecord={revealedKey} onClose={() => setRevealedKey(null)} />

      <ConfirmDialog
        open={revoking !== null}
        title={`Revoke "${revoking?.label ?? ""}"`}
        onClose={() => setRevoking(null)}
        danger
        confirmLabel="Revoke key"
        busy={revokeMutation.isPending}
        onConfirm={() => {
          if (revoking) revokeMutation.mutate(revoking.id);
        }}
      >
        <p style={{ color: "var(--ink-2)", fontSize: "var(--text-sm)" }}>
          Any bot or script using this key immediately loses access. This can't be undone — issue a new key to replace it.
        </p>
      </ConfirmDialog>
    </Card>
  );
}

function NewIntegrationKeyDialog({
  open,
  onClose,
  onCreated,
}: {
  open: boolean;
  onClose: () => void;
  onCreated: (key: IntegrationKeyCreated) => void;
}) {
  const [label, setLabel] = useState("");

  const createMutation = useMutation({
    mutationFn: (l: string) => api.integrationKeys.create(l),
    onSuccess: (created) => {
      setLabel("");
      createMutation.reset();
      onCreated(created);
    },
  });

  function close() {
    setLabel("");
    createMutation.reset();
    onClose();
  }

  const trimmed = label.trim();
  const error =
    createMutation.isError && createMutation.error instanceof ApiRequestError
      ? createMutation.error.message
      : createMutation.isError
        ? "Couldn't create the key. Try again."
        : null;

  return (
    <Dialog
      open={open}
      title="New integration key"
      onClose={close}
      footer={
        <>
          <button type="button" className="btn btn-ghost" onClick={close}>
            Cancel
          </button>
          <button
            type="button"
            className="btn btn-primary"
            disabled={!trimmed || trimmed.length > INTEGRATION_KEY_LABEL_MAX || createMutation.isPending}
            onClick={() => createMutation.mutate(trimmed)}
          >
            {createMutation.isPending ? "Creating…" : "Create key"}
          </button>
        </>
      }
    >
      <Field
        label="Label"
        placeholder="discord-bot"
        value={label}
        maxLength={INTEGRATION_KEY_LABEL_MAX}
        autoFocus
        onChange={(e) => setLabel(e.target.value)}
        hint={
          <span className="field-hint">
            {trimmed.length}/{INTEGRATION_KEY_LABEL_MAX} — identifies this key in the list below; never sent to bots
          </span>
        }
      />
      {error && <Banner tone="warn">{error}</Banner>}
    </Dialog>
  );
}

function RevealIntegrationKeyDialog({ keyRecord, onClose }: { keyRecord: IntegrationKeyCreated | null; onClose: () => void }) {
  const toast = useToast();

  async function copy() {
    if (!keyRecord) return;
    try {
      await navigator.clipboard.writeText(keyRecord.key);
      toast.push("Key copied to clipboard.", "ok");
    } catch {
      toast.push("Couldn't copy automatically — select and copy the key manually.", "danger");
    }
  }

  return (
    <Dialog
      open={keyRecord !== null}
      title="Integration key created"
      onClose={onClose}
      footer={
        <button type="button" className="btn btn-primary" onClick={onClose}>
          Done — I've saved it
        </button>
      }
    >
      {keyRecord && (
        <div style={{ display: "flex", flexDirection: "column", gap: "var(--space-3)" }}>
          <Banner tone="warn">
            This key is shown once and will not be shown again — Palhelm never stores the plaintext. Copy it now.
          </Banner>
          <div className="field">
            <span style={{ fontSize: "var(--text-sm)", fontWeight: 500, color: "var(--ink-2)" }}>{keyRecord.label}</span>
            <div className="key-reveal-row">
              <CodeWell>
                <span className="key-reveal-value">{keyRecord.key}</span>
              </CodeWell>
              <button type="button" className="btn btn-sm" onClick={copy}>
                Copy
              </button>
            </div>
          </div>
        </div>
      )}
    </Dialog>
  );
}
