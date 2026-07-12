import { useEffect, useRef, useState, type KeyboardEvent } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { api } from "../../api/client";
import type { ConsoleLogEntry } from "../../api/types";
import { useAuth, useIsAdmin } from "../../app/AuthProvider";
import { usePaletteBridge } from "../../app/paletteBridge";
import { Card, CardBody, CardHead } from "../../components/Card";
import { Pill } from "../../components/Pill";
import { Banner } from "../../components/Banner";
import { EmptyState } from "../../components/EmptyState";
import { Dialog, ConfirmDialog } from "../../components/ConfirmDialog";
import { Field } from "../../components/Field";
import { Tooltip } from "../../components/Tooltip";
import { useToast } from "../../components/Toast";
import "./Console.css";

function ts(iso: string): string {
  return new Date(iso).toLocaleTimeString(undefined, { hour: "2-digit", minute: "2-digit", second: "2-digit", hour12: false });
}

export default function ConsoleRoute() {
  const isAdmin = useIsAdmin();
  const { username } = useAuth();
  const queryClient = useQueryClient();
  const toast = useToast();

  const healthQuery = useQuery({ queryKey: ["server", "health"], queryFn: () => api.server.health(), refetchInterval: 15000 });
  const logQuery = useQuery({ queryKey: ["console", "log"], queryFn: () => api.console.log(200) });
  const savedQuery = useQuery({ queryKey: ["console", "saved"], queryFn: () => api.console.savedList() });

  const [command, setCommand] = useState("");
  const [historyIdx, setHistoryIdx] = useState<number | null>(null);
  const [newOpen, setNewOpen] = useState(false);
  const [deleting, setDeleting] = useState<{ id: string; name: string } | null>(null);
  const logRef = useRef<HTMLDivElement>(null);
  const commandInputRef = useRef<HTMLInputElement>(null);

  // The ⌘K command palette inserts a saved command here (never executes it directly —
  // RCON commands can be destructive, so the operator always reviews before sending).
  const { consoleInsertRequest, clearConsoleInsertRequest } = usePaletteBridge();
  useEffect(() => {
    if (!consoleInsertRequest) return;
    setCommand(consoleInsertRequest.command);
    setHistoryIdx(null);
    commandInputRef.current?.focus();
    clearConsoleInsertRequest();
  }, [consoleInsertRequest, clearConsoleInsertRequest]);

  const execMutation = useMutation({
    mutationFn: (cmd: string) => api.console.exec(cmd),
    onSettled: () => queryClient.invalidateQueries({ queryKey: ["console", "log"] }),
    onError: () => toast.push("Command failed to send. Check the RCON connection.", "danger"),
  });

  const entries = logQuery.data ?? [];
  const commandHistory = entries.map((e) => e.command);

  // Pin the log to the bottom whenever entries change.
  useEffect(() => {
    const el = logRef.current;
    if (el) el.scrollTop = el.scrollHeight;
  }, [entries.length]);

  function send() {
    const cmd = command.trim();
    if (!cmd || execMutation.isPending) return;
    execMutation.mutate(cmd);
    setCommand("");
    setHistoryIdx(null);
  }

  function onKeyDown(e: KeyboardEvent<HTMLInputElement>) {
    if (e.key === "Enter") {
      e.preventDefault();
      send();
    } else if (e.key === "ArrowUp") {
      e.preventDefault();
      if (commandHistory.length === 0) return;
      const next = historyIdx === null ? commandHistory.length - 1 : Math.max(0, historyIdx - 1);
      setHistoryIdx(next);
      setCommand(commandHistory[next] ?? "");
    } else if (e.key === "ArrowDown") {
      e.preventDefault();
      if (historyIdx === null) return;
      const next = historyIdx + 1;
      if (next >= commandHistory.length) {
        setHistoryIdx(null);
        setCommand("");
      } else {
        setHistoryIdx(next);
        setCommand(commandHistory[next] ?? "");
      }
    }
  }

  const rcon = healthQuery.data?.rcon;

  return (
    <main className="content" style={{ minHeight: "calc(100vh - var(--helmstrip-h))" }}>
      <div className="page-head">
        <h1>Console</h1>
        <span className="sub">RCON session · audit-logged</span>
        <div className="spacer" />
        {rcon === "ok" ? <Pill tone="ok">Connected</Pill> : rcon === "error" ? <Pill tone="danger">Disconnected</Pill> : null}
      </div>

      <div className="console-layout">
        <Card className="console-card">
          {logQuery.isError ? (
            <CardBody>
              <Banner tone="warn">Couldn't load the console log. Check the panel's RCON connection.</Banner>
            </CardBody>
          ) : (
            <div className="console" role="log" aria-label="RCON session" ref={logRef}>
              <div className="line">
                <span className="ts"></span>
                <span className="sys">— session opened by {username} —</span>
              </div>
              {entries.map((e, i) => (
                <LogEntry key={i} entry={e} />
              ))}
              {entries.length === 0 && !logQuery.isLoading && (
                <div className="line">
                  <span className="ts"></span>
                  <span className="sys">no commands yet — type one below</span>
                </div>
              )}
            </div>
          )}
          <div className="prompt-row">
            <input
              ref={commandInputRef}
              className="input input-mono"
              type="text"
              placeholder={isAdmin ? "Type an RCON command — ↑ for history" : "Viewer role — console is read-only"}
              aria-label="RCON command"
              value={command}
              disabled={!isAdmin}
              onChange={(e) => {
                setCommand(e.target.value);
                setHistoryIdx(null);
              }}
              onKeyDown={onKeyDown}
            />
            <button type="button" className="btn btn-primary" onClick={send} disabled={!isAdmin || execMutation.isPending}>
              {execMutation.isPending ? "Sending…" : "Send"}
            </button>
          </div>
        </Card>

        <aside style={{ display: "flex", flexDirection: "column", gap: "var(--space-4)" }}>
          <Card>
            <CardHead title="Saved commands">
              {isAdmin && (
                <button type="button" className="btn btn-sm btn-ghost" onClick={() => setNewOpen(true)}>
                  + New
                </button>
              )}
            </CardHead>
            {savedQuery.isError ? (
              <CardBody>
                <Banner tone="warn">Couldn't load saved commands.</Banner>
              </CardBody>
            ) : (savedQuery.data ?? []).length === 0 ? (
              <CardBody>
                <EmptyState
                  title="No saved commands"
                  description={isAdmin ? "Save a command you run often and it appears here for one-click reuse." : "None saved yet."}
                />
              </CardBody>
            ) : (
              <CardBody flush>
                {(savedQuery.data ?? []).map((c) => (
                  <div className="saved-cmd" key={c.id}>
                    <div className="meta">
                      <div className="name">{c.name}</div>
                      <code>{c.command}</code>
                    </div>
                    {isAdmin && (
                      <>
                        <button
                          type="button"
                          className="btn btn-sm"
                          disabled={execMutation.isPending}
                          onClick={() => execMutation.mutate(c.command)}
                        >
                          Run
                        </button>
                        <Tooltip label="Delete">
                          <button
                            type="button"
                            className="btn btn-sm btn-ghost"
                            aria-label={`Delete saved command ${c.name}`}
                            onClick={() => setDeleting({ id: c.id, name: c.name })}
                          >
                            ✕
                          </button>
                        </Tooltip>
                      </>
                    )}
                  </div>
                ))}
              </CardBody>
            )}
          </Card>

          <Card>
            <CardHead title="Command reference" />
            <CardBody className="cmd-ref">
              <div>
                <code>ShowPlayers</code> — list connected players
              </div>
              <div>
                <code>Broadcast &lt;msg&gt;</code> — message all players (no spaces; use _)
              </div>
              <div>
                <code>KickPlayer &lt;steamid&gt;</code>
              </div>
              <div>
                <code>Shutdown &lt;sec&gt; &lt;msg&gt;</code> — graceful stop with warning
              </div>
              <div className="foot">
                Vanilla RCON is limited — most moderation actions in Palhelm use the REST API instead. <kbd>↑</kbd> cycles history.
              </div>
            </CardBody>
          </Card>
        </aside>
      </div>

      <NewSavedCommandDialog open={newOpen} onClose={() => setNewOpen(false)} />
      <ConfirmDialog
        open={deleting !== null}
        title={`Delete "${deleting?.name}"`}
        onClose={() => setDeleting(null)}
        danger
        confirmLabel="Delete saved command"
        onConfirm={async () => {
          if (!deleting) return;
          try {
            await api.console.savedDelete(deleting.id);
            queryClient.invalidateQueries({ queryKey: ["console", "saved"] });
            toast.push("Saved command deleted.", "ok");
          } catch {
            toast.push("Couldn't delete the saved command.", "danger");
          }
          setDeleting(null);
        }}
      >
        <p style={{ color: "var(--ink-2)", fontSize: "var(--text-sm)" }}>
          Removes the shortcut only — it doesn't run or undo anything on the server.
        </p>
      </ConfirmDialog>
    </main>
  );
}

function LogEntry({ entry }: { entry: ConsoleLogEntry }) {
  const outputLines = entry.output.split("\n");
  return (
    <>
      <div className="line">
        <span className="ts">{ts(entry.at)}</span>
        <span className="cmd">{entry.command}</span>
      </div>
      <div className="line">
        <span className="ts">{ts(entry.at)}</span>
        <span className={entry.isError ? "err" : undefined}>
          {outputLines.map((l, i) => (
            <span key={i}>
              {l}
              {i < outputLines.length - 1 && <br />}
            </span>
          ))}
        </span>
      </div>
    </>
  );
}

function NewSavedCommandDialog({ open, onClose }: { open: boolean; onClose: () => void }) {
  const [name, setName] = useState("");
  const [command, setCommand] = useState("");
  const [busy, setBusy] = useState(false);
  const queryClient = useQueryClient();
  const toast = useToast();

  async function create() {
    if (!name.trim() || !command.trim()) return;
    setBusy(true);
    try {
      await api.console.savedCreate(name.trim(), command.trim());
      queryClient.invalidateQueries({ queryKey: ["console", "saved"] });
      toast.push("Saved command added.", "ok");
      setName("");
      setCommand("");
      onClose();
    } catch {
      toast.push("Couldn't save the command.", "danger");
    } finally {
      setBusy(false);
    }
  }

  return (
    <Dialog
      open={open}
      title="New saved command"
      onClose={onClose}
      footer={
        <>
          <button type="button" className="btn btn-ghost" onClick={onClose}>
            Cancel
          </button>
          <button type="button" className="btn btn-primary" onClick={create} disabled={busy || !name.trim() || !command.trim()}>
            {busy ? "Saving…" : "Save command"}
          </button>
        </>
      }
    >
      <Field label="Name" placeholder="Who's on" value={name} onChange={(e) => setName(e.target.value)} autoFocus />
      <Field label="Command" mono placeholder="ShowPlayers" value={command} onChange={(e) => setCommand(e.target.value)} />
    </Dialog>
  );
}
