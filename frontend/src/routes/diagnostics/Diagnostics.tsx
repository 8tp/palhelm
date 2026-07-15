import { useState, type ReactNode } from "react";
import { useQuery } from "@tanstack/react-query";
import { api } from "../../api/client";
import { classifyFreshness, gameDataStateDetail, gameDataStateTone, linkCoverage } from "../../app/diagnostics";
import { formatBytes, formatDateTime, formatDuration, formatRelativeToNow } from "../../app/format";
import { Banner } from "../../components/Banner";
import { Card, CardBody, CardHead } from "../../components/Card";
import { DiagnosticRow, DiagnosticRows, UnavailableDiagnostic } from "../../components/DiagnosticRows";
import "./Diagnostics.css";

const FIFTEEN_SECONDS = 15_000;

function QueryCardState({ loading, error, children }: { loading: boolean; error: boolean; children: ReactNode }) {
  if (error) return <CardBody><Banner tone="warn">This diagnostic source could not be loaded.</Banner></CardBody>;
  if (loading) return <CardBody><span className="skel skel-text diagnostics-skeleton" /></CardBody>;
  return <>{children}</>;
}

function timestampDetail(value: string | null | undefined): string {
  if (!value) return "No successful observation has been recorded.";
  return `${formatRelativeToNow(value)} · ${formatDateTime(value)}`;
}

function deadlineLabel(value: string): string {
  const remainingSec = (new Date(value).getTime() - Date.now()) / 1000;
  return remainingSec > 0 ? `in ${formatDuration(remainingSec)}` : formatRelativeToNow(value);
}

export default function DiagnosticsRoute() {
  const [refreshing, setRefreshing] = useState(false);
  const healthQuery = useQuery({ queryKey: ["server", "health"], queryFn: () => api.server.health(), refetchInterval: FIFTEEN_SECONDS });
  const worldQuery = useQuery({ queryKey: ["world"], queryFn: () => api.world.get(), refetchInterval: FIFTEEN_SECONDS });
  const snapshotQuery = useQuery({ queryKey: ["world", "snapshot"], queryFn: () => api.world.snapshot(), refetchInterval: FIFTEEN_SECONDS });
  const backupsQuery = useQuery({ queryKey: ["backups"], queryFn: () => api.backups.list(), refetchInterval: 60_000 });
  const scheduleQuery = useQuery({ queryKey: ["backups", "schedule"], queryFn: () => api.backups.schedule(), refetchInterval: 60_000 });
  const storageQuery = useQuery({ queryKey: ["backups", "storage"], queryFn: () => api.backups.storage(), refetchInterval: 60_000, retry: false });

  async function refresh() {
    setRefreshing(true);
    try {
      await Promise.all([
        healthQuery.refetch(), worldQuery.refetch(), snapshotQuery.refetch(), backupsQuery.refetch(), scheduleQuery.refetch(), storageQuery.refetch(),
      ]);
    } finally {
      setRefreshing(false);
    }
  }

  const health = healthQuery.data;
  const world = worldQuery.data;
  const snapshot = snapshotQuery.data;
  const backups = backupsQuery.data;
  const schedule = scheduleQuery.data;
  const storage = storageQuery.data;
  const latestBackup = backups?.[0];
  const totalBackupBytes = backups?.reduce((total, item) => total + item.sizeBytes, 0) ?? 0;
  const coverage = snapshot ? linkCoverage(snapshot.diagnostics.linkedBasePals, snapshot.diagnostics.unresolvedBasePals) : null;
  const cadence = Math.max(snapshot?.diagnostics.scheduledDelayMs ?? 0, 30_000);
  const snapshotFreshness = classifyFreshness(snapshot?.capturedAt, cadence * 2, cadence * 4);

  return (
    <main className="content diagnostics-page">
      <div className="page-head diagnostics-head">
        <div>
          <h1>Diagnostics</h1>
          <span className="sub">read-only health evidence · cached pollers</span>
        </div>
        <button type="button" className="btn btn-sm" disabled={refreshing} onClick={refresh}>
          {refreshing ? "Refreshing…" : "Refresh now"}
        </button>
      </div>

      <div className="diagnostics-grid">
        <Card>
          <CardHead title="Core pollers" hint="15 s refresh" />
          <QueryCardState loading={healthQuery.isLoading} error={healthQuery.isError}>
            <CardBody flush>
              <DiagnosticRows>
                <DiagnosticRow label="Palworld REST" value={health?.rest ?? "Unavailable"} tone={health?.rest === "ok" ? "ok" : "danger"} detail="Shared metrics, players, and server-info source." />
                <DiagnosticRow label="RCON probe" value={health?.rcon ?? "Unavailable"} tone={health?.rcon === "ok" ? "ok" : "danger"} detail="Background command-channel health probe." />
                <DiagnosticRow label="Save discovery" value={health?.save.state ?? "Unavailable"} tone={health?.save.state === "ok" ? "ok" : health?.save.state === "error" ? "danger" : "idle"} detail={timestampDetail(health?.save.lastSyncAt)} />
                <UnavailableDiagnostic label="Metrics sample time" reason="The current-metrics contract does not expose a sample timestamp." />
              </DiagnosticRows>
            </CardBody>
          </QueryCardState>
        </Card>

        <Card>
          <CardHead title="Save parser" hint="decoded world snapshot" />
          <QueryCardState loading={worldQuery.isLoading} error={worldQuery.isError}>
            <CardBody flush>
              <DiagnosticRows>
                <DiagnosticRow label="Last completed parse" value={world?.lastParseAt ? formatRelativeToNow(world.lastParseAt) : "Never"} tone={world?.lastParseAt ? "ok" : "idle"} detail={world?.lastParseAt ? formatDateTime(world.lastParseAt) : "No completed parser snapshot is available."} />
                <DiagnosticRow label="Parse duration" value={world?.lastParseAt ? `${world.parseDurationMs.toLocaleString()} ms` : "Unavailable"} />
                <DiagnosticRow label="Format coverage" value={world?.formatDrift ? "Drift detected" : "Complete"} tone={world?.formatDrift ? "warn" : "ok"} detail={world ? `${world.stats.skippedProps} skipped properties` : undefined} />
                <DiagnosticRow label="Decoded records" value={world ? `${world.stats.players} players · ${world.stats.pals} Pals · ${world.stats.guilds} guilds` : "Unavailable"} detail={world ? `World day ${world.day}` : undefined} />
              </DiagnosticRows>
            </CardBody>
          </QueryCardState>
        </Card>

        <Card className="diagnostics-wide">
          <CardHead title="Game Data" hint="optional Palworld 1.0 poller" />
          <QueryCardState loading={snapshotQuery.isLoading} error={snapshotQuery.isError}>
            <CardBody flush>
              <div className="diagnostics-columns">
                <DiagnosticRows>
                  <DiagnosticRow label="Capability state" value={snapshot?.state ?? "Unavailable"} tone={snapshot ? gameDataStateTone(snapshot.state) : "idle"} detail={snapshot ? gameDataStateDetail(snapshot.state) : undefined} />
                  <DiagnosticRow label="Snapshot freshness" value={snapshotFreshness.label} tone={snapshotFreshness.tone} detail={timestampDetail(snapshot?.capturedAt)} />
                  <DiagnosticRow label="Last attempt" value={snapshot?.lastAttemptAt ? formatRelativeToNow(snapshot.lastAttemptAt) : "Never"} detail={snapshot?.lastAttemptAt ? formatDateTime(snapshot.lastAttemptAt) : "The poller has not attempted a request."} />
                  <DiagnosticRow label="Request latency" value={snapshot ? `${snapshot.diagnostics.lastRequestDurationMs.toLocaleString()} ms` : "Unavailable"} detail="Last completed upstream request." />
                  <DiagnosticRow label="Accepted actors" value={snapshot ? snapshot.diagnostics.lastAcceptedActorCount.toLocaleString() : "Unavailable"} detail={snapshot?.truncated ? "The browser projection was capped." : "Bounded count only; raw actor data is not shown here."} />
                </DiagnosticRows>
                <DiagnosticRows>
                  <DiagnosticRow label="Base-Pal link coverage" value={coverage?.value ?? "Unavailable"} tone={snapshot?.diagnostics.linkLookupFailed ? "danger" : coverage?.value === "100%" ? "ok" : "warn"} detail={snapshot?.diagnostics.linkLookupFailed ? "Save-link lookup failed for this attempt." : coverage?.detail} />
                  <DiagnosticRow label="Last error category" value={snapshot?.diagnostics.lastErrorCategory || "none"} tone={snapshot?.diagnostics.lastErrorCategory && snapshot.diagnostics.lastErrorCategory !== "none" ? "warn" : "ok"} detail="Bounded category; upstream messages and bodies are never exposed." />
                  <DiagnosticRow label="Retry delay" value={snapshot ? formatDuration(snapshot.diagnostics.scheduledDelayMs / 1000) : "Unavailable"} detail="Backoff reported by the shared poller." />
                  <DiagnosticRow label="Next attempt" value={snapshot?.diagnostics.nextAttemptAt ? deadlineLabel(snapshot.diagnostics.nextAttemptAt) : "Not scheduled"} detail={snapshot?.diagnostics.nextAttemptAt ? formatDateTime(snapshot.diagnostics.nextAttemptAt) : "Disabled or no retry deadline is available."} />
                  <DiagnosticRow label="Accepted totals" value={snapshot ? `${snapshot.counts.players} players · ${snapshot.counts.basePals} base Pals · ${snapshot.counts.palBoxes} PalBoxes` : "Unavailable"} detail="Aggregate counts from the accepted snapshot." />
                </DiagnosticRows>
              </div>
            </CardBody>
          </QueryCardState>
        </Card>

        <Card>
          <CardHead title="Backups" hint="indexed archives" />
          <QueryCardState loading={backupsQuery.isLoading || scheduleQuery.isLoading} error={backupsQuery.isError || scheduleQuery.isError}>
            <CardBody flush>
              <DiagnosticRows>
                <DiagnosticRow label="Latest archive" value={latestBackup ? formatRelativeToNow(latestBackup.createdAt) : "None"} tone={latestBackup ? "ok" : "warn"} detail={latestBackup ? `${formatDateTime(latestBackup.createdAt)} · ${latestBackup.trigger} · ${formatBytes(latestBackup.sizeBytes)}` : "No indexed backup is available."} />
                <DiagnosticRow label="Schedule" value={schedule?.enabled ? `Every ${formatDuration(schedule.everyMinutes * 60)}` : "Disabled"} tone={schedule?.enabled ? "ok" : "idle"} detail={schedule?.enabled ? `${schedule.keepDays} day retention` : "Automatic backups are not scheduled."} />
                <DiagnosticRow label="Next scheduled run" value={schedule?.nextRunAt ? deadlineLabel(schedule.nextRunAt) : "Not scheduled"} detail={schedule?.nextRunAt ? formatDateTime(schedule.nextRunAt) : undefined} />
              </DiagnosticRows>
            </CardBody>
          </QueryCardState>
        </Card>

        <Card>
          <CardHead title="Local storage" hint="safe aggregate facts" />
          <QueryCardState loading={backupsQuery.isLoading} error={backupsQuery.isError}>
            <CardBody flush>
              <DiagnosticRows>
                <DiagnosticRow label="Indexed archives" value={backups ? backups.length.toLocaleString() : "Unavailable"} detail={backups ? `${formatBytes(totalBackupBytes)} indexed in total` : undefined} />
                {storage && storage.totalBytes !== null && storage.freeBytes !== null ? (
                  <DiagnosticRow
                    label="Filesystem headroom"
                    value={`${formatBytes(storage.freeBytes)} free`}
                    tone={storage.freeBytes / storage.totalBytes < 0.1 ? "warn" : "ok"}
                    detail={`${formatBytes(storage.totalBytes)} volume capacity · host path is not exposed`}
                  />
                ) : (
                  <UnavailableDiagnostic label="Filesystem headroom" reason="This panel build does not report backup-volume disk capacity." />
                )}
                <UnavailableDiagnostic label="Database schema" reason="SQLite internals and migration state are not exposed by the authenticated API." />
              </DiagnosticRows>
            </CardBody>
          </QueryCardState>
        </Card>
      </div>
    </main>
  );
}
