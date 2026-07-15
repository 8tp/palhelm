import { Fragment, useMemo, useState } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { api } from "../../api/client";
import { ApiRequestError, type Backup, type BackupDryRun, type BackupTrigger } from "../../api/types";
import { useIsAdmin } from "../../app/AuthProvider";
import { formatBytes, formatDateTime, formatDuration } from "../../app/format";
import { Card, CardBody, CardHead } from "../../components/Card";
import { Pill, type PillTone } from "../../components/Pill";
import { Banner } from "../../components/Banner";
import { EmptyState } from "../../components/EmptyState";
import { Dialog, ConfirmDialog } from "../../components/ConfirmDialog";
import { DropdownMenu, DropdownMenuItem, DropdownMenuLinkItem } from "../../components/DropdownMenu";
import { SearchField } from "../../components/Field";
import { Meter } from "../../components/Meter";
import { DiffList, type DiffItem } from "../../components/DiffList";
import { CodeWell } from "../../components/CodeWell";
import { useToast } from "../../components/Toast";
import { IconArchive, IconWarn } from "../../components/icons";
import "./Backups.css";

const TRIGGER_TONE: Record<BackupTrigger, PillTone> = {
  scheduled: "idle",
  manual: "ok",
  "pre-restore": "warn",
  imported: "idle",
};

export default function BackupsRoute() {
  const isAdmin = useIsAdmin();
  const queryClient = useQueryClient();
  const toast = useToast();

  const backupsQuery = useQuery({ queryKey: ["backups"], queryFn: () => api.backups.list() });
  const scheduleQuery = useQuery({ queryKey: ["backups", "schedule"], queryFn: () => api.backups.schedule() });
  const storageQuery = useQuery({ queryKey: ["backups", "storage"], queryFn: () => api.backups.storage() });

  const [search, setSearch] = useState("");
  const [triggerFilter, setTriggerFilter] = useState<"all" | BackupTrigger>("all");
  const [browsingId, setBrowsingId] = useState<string | null>(null);
  const [restoring, setRestoring] = useState<Backup | null>(null);
  const [deleting, setDeleting] = useState<Backup | null>(null);

  const createMutation = useMutation({
    mutationFn: () => api.backups.create(),
    onSuccess: (b) => {
      queryClient.invalidateQueries({ queryKey: ["backups"] });
      toast.push(`Backup ${b.file} created.`, "ok");
    },
    onError: () => toast.push("Backup failed to start. Check the panel logs.", "danger"),
  });

  const backups = useMemo(() => backupsQuery.data ?? [], [backupsQuery.data]);
  const filtered = useMemo(() => {
    const q = search.trim().toLowerCase();
    return backups.filter((b) => {
      if (triggerFilter !== "all" && b.trigger !== triggerFilter) return false;
      if (q && !b.file.toLowerCase().includes(q)) return false;
      return true;
    });
  }, [backups, search, triggerFilter]);

  const totalBytes = backups.reduce((sum, b) => sum + b.sizeBytes, 0);
  const oldest = backups[backups.length - 1];
  const nextRun = scheduleQuery.data?.nextRunAt ? new Date(scheduleQuery.data.nextRunAt).getTime() - Date.now() : null;

  return (
    <main className="content">
      <div className="page-head">
        <h1>Backups</h1>
        <span className="sub">
          {backupsQuery.data ? `${backups.length} snapshots · ${formatBytes(totalBytes)}` : "loading…"}
        </span>
      </div>

      <div className="backups-layout">
        <div className="backups-main">
          <div className="toolbar">
            {isAdmin && (
              <button type="button" className="btn btn-primary" disabled={createMutation.isPending} onClick={() => createMutation.mutate()}>
                {createMutation.isPending ? "Backing up…" : "Back up now"}
              </button>
            )}
            <SearchField placeholder="Search snapshots…" value={search} onChange={(e) => setSearch(e.target.value)} aria-label="Search snapshots" />
            <select
              className="input"
              style={{ width: "auto" }}
              value={triggerFilter}
              onChange={(e) => setTriggerFilter(e.target.value as "all" | BackupTrigger)}
              aria-label="Filter by trigger"
            >
              <option value="all">All triggers</option>
              <option value="scheduled">Scheduled</option>
              <option value="manual">Manual</option>
              <option value="pre-restore">Pre-restore</option>
              <option value="imported">Imported</option>
            </select>
            <div className="spacer" />
            {scheduleQuery.data?.enabled && nextRun !== null && nextRun > 0 && (
              <span className="next-hint">next scheduled backup in {formatDuration(nextRun / 1000)}</span>
            )}
          </div>

          <Card>
            {backupsQuery.isError ? (
              <CardBody>
                <Banner tone="warn">Couldn't load backups. Check the panel's data volume.</Banner>
              </CardBody>
            ) : backupsQuery.isLoading ? (
              <CardBody>
                <span className="skel skel-text" style={{ width: "100%", height: 120 }} />
              </CardBody>
            ) : filtered.length === 0 ? (
              <CardBody>
                <EmptyState
                  icon={<IconArchive />}
                  title={backups.length === 0 ? "No backups yet" : "No snapshots match"}
                  description={
                    backups.length === 0
                      ? "Create one now or enable the schedule — snapshots of the world save appear here."
                      : "Try a different search or trigger filter."
                  }
                />
              </CardBody>
            ) : (
              <CardBody flush style={{ overflowX: "auto" }}>
                <table className="table">
                  <thead>
                    <tr>
                      <th>Snapshot</th>
                      <th>Created</th>
                      <th>Size</th>
                      <th>Trigger</th>
                      <th className="actions"></th>
                    </tr>
                  </thead>
                  <tbody>
                    {filtered.map((b) => (
                      <Fragment key={b.id}>
                        <tr className={browsingId === b.id ? "row-selected" : undefined}>
                          <td>
                            <div className="snap-name">{b.file}</div>
                            {b.worldDay !== undefined && <div className="snap-day">Day {b.worldDay}</div>}
                          </td>
                          <td className="num">{formatDateTime(b.createdAt)}</td>
                          <td className="num">{formatBytes(b.sizeBytes)}</td>
                          <td>
                            <Pill tone={TRIGGER_TONE[b.trigger]}>{b.trigger}</Pill>
                          </td>
                          <td className="actions">
                            <DropdownMenu triggerLabel={`Actions for ${b.file}`}>
                              <DropdownMenuItem onClick={() => setBrowsingId(browsingId === b.id ? null : b.id)}>
                                {browsingId === b.id ? "Hide contents" : "Browse contents"}
                              </DropdownMenuItem>
                              {isAdmin && <DropdownMenuItem onClick={() => setRestoring(b)}>Restore…</DropdownMenuItem>}
                              <DropdownMenuLinkItem href={`/api/v1/backups/${b.id}/download`} download>
                                Download
                              </DropdownMenuLinkItem>
                              {isAdmin && (
                                <DropdownMenuItem danger onClick={() => setDeleting(b)}>
                                  Delete
                                </DropdownMenuItem>
                              )}
                            </DropdownMenu>
                          </td>
                        </tr>
                        {browsingId === b.id && (
                          <tr className="row-selected">
                            <td colSpan={5} style={{ padding: 0 }}>
                              <ContentsDrawer backup={b} onClose={() => setBrowsingId(null)} />
                            </td>
                          </tr>
                        )}
                      </Fragment>
                    ))}
                  </tbody>
                </table>
              </CardBody>
            )}
          </Card>
        </div>

        <aside style={{ display: "flex", flexDirection: "column", gap: "var(--space-4)" }}>
          <ScheduleCard />

          <Card>
            <CardHead title="Storage" />
            <CardBody>
              {(() => {
                const capacity = storageQuery.data?.totalBytes ?? null;
                const free = storageQuery.data?.freeBytes ?? null;
                const kept = `${backups.length} snapshots kept${
                  oldest ? ` · oldest ${formatDuration((Date.now() - new Date(oldest.createdAt).getTime()) / 1000)} ago` : ""
                }`;
                return (
                  <div className="stat" style={{ padding: 0 }}>
                    <span className="label">Used by backups</span>
                    <div className="value">
                      {formatBytes(totalBytes)}
                      {capacity !== null && <small> of {formatBytes(capacity)}</small>}
                    </div>
                    {capacity !== null ? (
                      <>
                        <div style={{ marginTop: 10 }}>
                          <Meter value={totalBytes} max={capacity} />
                        </div>
                        <div className="delta">
                          {kept}
                          {free !== null ? ` · ${formatBytes(free)} free on volume` : ""}
                        </div>
                      </>
                    ) : (
                      <div className="delta">
                        {kept}
                        <div style={{ marginTop: 4, color: "var(--ink-3)" }}>
                          Total volume capacity isn't reported by this panel build.
                        </div>
                      </div>
                    )}
                  </div>
                );
              })()}
            </CardBody>
          </Card>
        </aside>
      </div>

      <RestoreDialog backup={restoring} onClose={() => setRestoring(null)} />

      <ConfirmDialog
        open={deleting !== null}
        title={`Delete ${deleting?.file ?? ""}`}
        onClose={() => setDeleting(null)}
        danger
        confirmLabel="Delete snapshot"
        onConfirm={async () => {
          if (!deleting) return;
          try {
            await api.backups.remove(deleting.id);
            queryClient.invalidateQueries({ queryKey: ["backups"] });
            toast.push("Snapshot deleted.", "ok");
          } catch {
            toast.push("Couldn't delete the snapshot.", "danger");
          }
          setDeleting(null);
        }}
      >
        <p style={{ color: "var(--ink-2)", fontSize: "var(--text-sm)" }}>
          Permanently removes this snapshot from the backup volume. This can't be undone.
        </p>
      </ConfirmDialog>
    </main>
  );
}

// ---------------- contents drawer ----------------

function ContentsDrawer({ backup, onClose }: { backup: Backup; onClose: () => void }) {
  const contentsQuery = useQuery({
    queryKey: ["backups", backup.id, "contents"],
    queryFn: () => api.backups.contents(backup.id),
  });

  return (
    <div className="contents-drawer">
      <h3>Contents — {backup.file}</h3>
      {contentsQuery.isError ? (
        <Banner tone="warn">Couldn't read the archive contents.</Banner>
      ) : contentsQuery.isLoading ? (
        <span className="skel skel-text" style={{ width: "60%" }} />
      ) : (
        <div className="contents-list">
          {(contentsQuery.data ?? []).map((f) => (
            <div className="row" key={f.path}>
              <span className="path">{f.path}</span>
              <span className="size">{formatBytes(f.sizeBytes)}</span>
            </div>
          ))}
        </div>
      )}
      <div>
        <button type="button" className="btn btn-ghost btn-sm" onClick={onClose}>
          Close
        </button>
      </div>
    </div>
  );
}

// ---------------- restore dialog (dry-run → typed confirmation → 409-aware) ----------------

function RestoreDialog({ backup, onClose }: { backup: Backup | null; onClose: () => void }) {
  const queryClient = useQueryClient();
  const toast = useToast();
  const [confirmWord, setConfirmWord] = useState("");
  const [error, setError] = useState<ApiRequestError | null>(null);

  const dryRunQuery = useQuery({
    queryKey: ["backups", backup?.id, "dry-run"],
    queryFn: () => api.backups.dryRun(backup!.id),
    enabled: backup !== null,
    staleTime: 0,
  });

  const restoreMutation = useMutation({
    mutationFn: () => api.backups.restore(backup!.id, confirmWord),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ["backups"] });
      toast.push("Restore started. A pre-restore backup was taken first.", "ok");
      close();
    },
    onError: (err) => {
      setError(err instanceof ApiRequestError ? err : new ApiRequestError(0, "unknown", "Restore failed. Try again."));
    },
  });

  function close() {
    setConfirmWord("");
    setError(null);
    onClose();
  }

  const diffItems: DiffItem[] = (dryRunQuery.data ?? ({ changes: [] } as unknown as BackupDryRun)).changes.map((c) => ({
    kind: c.kind,
    text:
      c.kind === "modify" && c.fromSize !== undefined && c.toSize !== undefined
        ? `${c.path} (${formatBytes(c.fromSize)} → ${formatBytes(c.toSize)})`
        : c.kind === "add" && c.toSize !== undefined
          ? `${c.path} (new)`
          : c.path,
  }));

  const manualCommand = typeof error?.extra.manualCommand === "string" ? error.extra.manualCommand : null;

  return (
    <Dialog
      open={backup !== null}
      title={`Restore ${backup?.file ?? ""}`}
      onClose={close}
      footer={
        <>
          <button type="button" className="btn btn-ghost" onClick={close}>
            Cancel
          </button>
          <button
            type="button"
            className="btn btn-danger-solid"
            disabled={confirmWord !== "RESTORE" || restoreMutation.isPending || dryRunQuery.isLoading || dryRunQuery.isError}
            onClick={() => restoreMutation.mutate()}
          >
            {restoreMutation.isPending ? "Restoring…" : "Restore this snapshot"}
          </button>
        </>
      }
    >
      {dryRunQuery.isError ? (
        <Banner tone="warn">Couldn't compute the restore dry-run. Try again.</Banner>
      ) : dryRunQuery.isLoading ? (
        <>
          <p style={{ color: "var(--ink-2)", fontSize: "var(--text-sm)" }}>Comparing the snapshot against the live save…</p>
          <span className="skel skel-text" style={{ width: "100%", height: 48 }} />
        </>
      ) : (
        <>
          <div className="dryrun">
            <h3>Restore dry-run — changes vs the live save</h3>
            <DiffList items={diffItems} />
            <div className="banner banner-warn">
              <IconWarn />
              Restoring requires stopping the server. A pre-restore backup is always taken first.
            </div>

            {error && (
              <>
                <div className="banner" style={{ background: "var(--danger-soft)", borderColor: "transparent", color: "var(--danger-ink)" }}>
                  <IconWarn />
                  {error.message}
                </div>
                {manualCommand && <CodeWell>{manualCommand}</CodeWell>}
              </>
            )}
          </div>

          <div className="field confirm-word">
            <label htmlFor="confirm-restore">
              Type <b style={{ fontFamily: "var(--font-mono)" }}>RESTORE</b> to confirm
            </label>
            <input
              id="confirm-restore"
              className="input"
              value={confirmWord}
              onChange={(e) => {
                setConfirmWord(e.target.value);
                if (error) setError(null);
              }}
              autoComplete="off"
              spellCheck={false}
            />
          </div>
        </>
      )}
    </Dialog>
  );
}

// ---------------- schedule card ----------------

function ScheduleCard() {
  const isAdmin = useIsAdmin();
  const queryClient = useQueryClient();
  const toast = useToast();
  const scheduleQuery = useQuery({ queryKey: ["backups", "schedule"], queryFn: () => api.backups.schedule() });

  const saveMutation = useMutation({
    mutationFn: (patch: { everyMinutes?: number; keepDays?: number; enabled?: boolean }) =>
      api.backups.setSchedule({ ...scheduleQuery.data!, ...patch }),
    onSuccess: (s) => {
      queryClient.setQueryData(["backups", "schedule"], s);
      toast.push("Backup schedule updated.", "ok");
    },
    onError: () => toast.push("Couldn't update the schedule.", "danger"),
  });

  const s = scheduleQuery.data;
  const nextIn = s?.enabled && s.nextRunAt ? new Date(s.nextRunAt).getTime() - Date.now() : null;

  return (
    <Card>
      <CardHead title="Schedule">
        {s && <Pill tone={s.enabled ? "ok" : "idle"}>{s.enabled ? "Enabled" : "Off"}</Pill>}
      </CardHead>
      {scheduleQuery.isError ? (
        <CardBody>
          <Banner tone="warn">Couldn't load the schedule.</Banner>
        </CardBody>
      ) : !s ? (
        <CardBody>
          <span className="skel skel-text" style={{ width: "100%", height: 60 }} />
        </CardBody>
      ) : (
        <CardBody style={{ display: "flex", flexDirection: "column", gap: "var(--space-3)" }}>
          <div className="field">
            <label htmlFor="sched-every">Every</label>
            <select
              id="sched-every"
              className="input"
              value={s.everyMinutes}
              disabled={!isAdmin || saveMutation.isPending}
              onChange={(e) => saveMutation.mutate({ everyMinutes: Number(e.target.value) })}
            >
              <option value={60}>1 hour</option>
              <option value={120}>2 hours</option>
              <option value={240}>4 hours</option>
              <option value={360}>6 hours</option>
              <option value={720}>12 hours</option>
            </select>
          </div>
          <div className="field">
            <label htmlFor="sched-keep">Keep</label>
            <select
              id="sched-keep"
              className="input"
              value={s.keepDays}
              disabled={!isAdmin || saveMutation.isPending}
              onChange={(e) => saveMutation.mutate({ keepDays: Number(e.target.value) })}
            >
              <option value={7}>7 days</option>
              <option value={14}>14 days</option>
              <option value={30}>30 days</option>
              <option value={60}>60 days</option>
            </select>
          </div>
          <div className="sched-next">
            <span className="label">Next run</span>
            <span className="num">
              {s.enabled && s.nextRunAt
                ? new Date(s.nextRunAt).toLocaleTimeString(undefined, { hour: "2-digit", minute: "2-digit", hour12: false })
                : "Not scheduled"}{" "}
              {nextIn !== null && nextIn > 0 && <span style={{ color: "var(--ink-3)" }}>· in {formatDuration(nextIn / 1000)}</span>}
            </span>
          </div>
        </CardBody>
      )}
    </Card>
  );
}
