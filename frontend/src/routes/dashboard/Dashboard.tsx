import { useMemo, useState } from "react";
import { useQuery } from "@tanstack/react-query";
import { api } from "../../api/client";
import { Card, CardBody, CardHead } from "../../components/Card";
import { StatTile, StatTileSkeleton } from "../../components/StatTile";
import { Chart } from "../../components/Chart";
import { Pill } from "../../components/Pill";
import { Banner } from "../../components/Banner";
import { EventMessage } from "../../components/EventMessage";
import { formatBytes, formatDateTime, formatDuration, formatRelativeToNow, formatWorldGuid } from "../../app/format";
import type { MetricsWindow } from "../../api/types";
import { Link } from "react-router";
import "./Dashboard.css";

function clockLabel(unixSec: number): string {
  const d = new Date(unixSec * 1000);
  return `${String(d.getHours()).padStart(2, "0")}:${String(d.getMinutes()).padStart(2, "0")}`;
}

export default function Dashboard() {
  const [window_, setWindow] = useState<MetricsWindow>("1h");

  const serverQuery = useQuery({ queryKey: ["server"], queryFn: () => api.server.get() });
  const healthQuery = useQuery({ queryKey: ["server", "health"], queryFn: () => api.server.health(), refetchInterval: 15000 });
  const metricsQuery = useQuery({ queryKey: ["metrics", "current"], queryFn: () => api.metrics.current(), refetchInterval: 5000 });
  const perfHistoryQuery = useQuery({
    queryKey: ["metrics", "history", window_],
    queryFn: () => api.metrics.history(window_),
    refetchInterval: 30000,
  });
  const playersHistoryQuery = useQuery({
    queryKey: ["metrics", "history", "24h"],
    queryFn: () => api.metrics.history("24h"),
    refetchInterval: 60000,
  });
  const guildsQuery = useQuery({ queryKey: ["guilds"], queryFn: () => api.guilds.list() });
  const backupsQuery = useQuery({ queryKey: ["backups"], queryFn: () => api.backups.list() });
  const playersQuery = useQuery({ queryKey: ["players"], queryFn: () => api.players.list() });
  const eventsQuery = useQuery({ queryKey: ["events", 5], queryFn: () => api.events.list(5) });

  const fpsSeries = useMemo((): [number[], number[]] => {
    const s = perfHistoryQuery.data?.series;
    return [s?.t ?? [], s?.fps ?? []];
  }, [perfHistoryQuery.data]);

  const frameTimeSeries = useMemo((): [number[], number[]] => {
    const s = perfHistoryQuery.data?.series;
    return [s?.t ?? [], s?.frameTimeMs ?? []];
  }, [perfHistoryQuery.data]);

  const playersSeries = useMemo((): [number[], number[]] => {
    const s = playersHistoryQuery.data?.series;
    return [s?.t ?? [], s?.players ?? []];
  }, [playersHistoryQuery.data]);

  const fpsYRange = useMemo((): [number, number] | undefined => {
    const fps = perfHistoryQuery.data?.series.fps;
    if (!fps || fps.length === 0) return undefined;
    const min = Math.min(...fps);
    const max = Math.max(...fps);
    const floor = Math.max(0, Math.floor((min - 6) / 10) * 10);
    const ceil = Math.ceil((max + 2) / 10) * 10;
    return [floor, ceil];
  }, [perfHistoryQuery.data]);

  const dipAnnotation = useMemo(() => {
    const fps = perfHistoryQuery.data?.series.fps;
    if (!fps || fps.length === 0) return undefined;
    let minIdx = 0;
    for (let i = 1; i < fps.length; i++) {
      const v = fps[i];
      const cur = fps[minIdx];
      if (v !== undefined && cur !== undefined && v < cur) minIdx = i;
    }
    const minVal = fps[minIdx];
    if (minVal === undefined) return undefined;
    // Only annotate a meaningful dip: skip when the minimum sits within 10% of the median.
    const sorted = [...fps].sort((a, b) => a - b);
    const median = sorted[Math.floor(sorted.length / 2)] ?? 0;
    if (minVal >= median * 0.9) return undefined;
    return { index: minIdx, text: `${Math.round(minVal)} fps dip` };
  }, [perfHistoryQuery.data]);

  const seenLast24h = useMemo(() => {
    const players = playersQuery.data;
    if (!players) return null;
    const cutoff = Date.now() - 24 * 3600_000;
    return players.filter((p) => new Date(p.lastSeenAt).getTime() >= cutoff).length;
  }, [playersQuery.data]);

  const guildsWithBases = useMemo(() => guildsQuery.data?.filter((g) => g.bases.length > 0).length ?? null, [guildsQuery.data]);

  const latestBackup = backupsQuery.data?.[0];
  const totalBackupBytes = backupsQuery.data?.reduce((sum, b) => sum + b.sizeBytes, 0) ?? 0;

  const metrics = metricsQuery.data;
  const server = serverQuery.data;
  const health = healthQuery.data;

  return (
    <main className="content">
      <div className="page-head">
        <h1>Overview</h1>
        <span className="sub">
          {server ? `${server.name} · ${server.version}` : serverQuery.isError ? "server unreachable" : "loading…"}
        </span>
      </div>

      {/* stat row */}
      <div className="grid cols-4">
        {metricsQuery.isLoading ? (
          <StatTileSkeleton />
        ) : metricsQuery.isError ? (
          <Card className="stat">
            <Banner tone="warn">Couldn't load server metrics.</Banner>
          </Card>
        ) : (
          <StatTile label="Server FPS" value={metrics!.fps} delta={`frame time ${metrics!.frameTimeMs.toFixed(1)} ms`} />
        )}

        {metricsQuery.isLoading ? (
          <StatTileSkeleton />
        ) : metricsQuery.isError ? (
          <Card className="stat">
            <Banner tone="warn">Couldn't load player count.</Banner>
          </Card>
        ) : (
          <StatTile
            label="Players online"
            value={metrics!.players}
            unit={`of ${metrics!.maxPlayers}`}
            delta={seenLast24h !== null ? `${seenLast24h} seen in last 24 h` : undefined}
          />
        )}

        {metricsQuery.isLoading ? (
          <StatTileSkeleton />
        ) : metricsQuery.isError ? (
          <Card className="stat">
            <Banner tone="warn">Couldn't load base camps.</Banner>
          </Card>
        ) : (
          <StatTile
            label="Base camps"
            value={metrics!.baseCamps}
            delta={guildsWithBases !== null ? `across ${guildsWithBases} guilds` : undefined}
          />
        )}

        {backupsQuery.isLoading ? (
          <StatTileSkeleton />
        ) : backupsQuery.isError ? (
          <Card className="stat">
            <Banner tone="warn">Couldn't load backups.</Banner>
          </Card>
        ) : latestBackup ? (
          <StatTile
            label="Last backup"
            value={formatDuration((Date.now() - new Date(latestBackup.createdAt).getTime()) / 1000)}
            unit="ago"
            delta={`${backupsQuery.data!.length} kept · ${formatBytes(totalBackupBytes)} total`}
            deltaTone="up"
          />
        ) : (
          <StatTile label="Last backup" value="—" delta="no backups yet" />
        )}
      </div>

      <div className="grid cols-3">
        {/* performance chart */}
        <Card span2>
          <CardHead title="Server performance" hint="sampled every 5 s">
            <div className="legend-row" role="tablist" aria-label="Time range">
              <button
                type="button"
                className={window_ === "24h" ? "btn btn-sm" : "btn btn-ghost btn-sm"}
                aria-selected={window_ === "24h"}
                onClick={() => setWindow("24h")}
              >
                24 h
              </button>
              <button
                type="button"
                className={window_ === "1h" ? "btn btn-sm" : "btn btn-ghost btn-sm"}
                aria-selected={window_ === "1h"}
                onClick={() => setWindow("1h")}
              >
                60 min
              </button>
            </div>
          </CardHead>
          {perfHistoryQuery.isError ? (
            <CardBody>
              <Banner tone="warn">Couldn't load performance history.</Banner>
            </CardBody>
          ) : (
            <CardBody chart>
              <Chart
                data={fpsSeries}
                height={180}
                ariaLabel={`Server FPS over the last ${window_ === "1h" ? "60 minutes" : "24 hours"}`}
                xFormat={clockLabel}
                yFormat={(v) => String(Math.round(v))}
                yRange={fpsYRange}
                annotation={dipAnnotation}
              />
              <div className="frame-time">
                <div className="frame-time-head">
                  <span>Frame time</span>
                  <span>ms</span>
                </div>
                <Chart
                  data={frameTimeSeries}
                  height={48}
                  ariaLabel="Frame time over the same window"
                  yFormat={(v) => v.toFixed(0)}
                  xAxis={false}
                />
              </div>
            </CardBody>
          )}
        </Card>

        {/* server info */}
        <Card>
          <CardHead title="Server" />
          {serverQuery.isError ? (
            <CardBody>
              <Banner tone="warn">Server is unreachable.</Banner>
            </CardBody>
          ) : (
            <CardBody flush>
              <table className="table">
                <tbody>
                  <tr>
                    <td style={{ color: "var(--ink-2)" }}>Name</td>
                    <td className="num">{server?.name ?? "—"}</td>
                  </tr>
                  <tr>
                    <td style={{ color: "var(--ink-2)" }}>Version</td>
                    <td className="num">{server?.version ?? "—"}</td>
                  </tr>
                  <tr>
                    <td style={{ color: "var(--ink-2)" }}>Panel</td>
                    <td className="num">v{server?.panelVersion ?? "—"}</td>
                  </tr>
                  <tr>
                    <td style={{ color: "var(--ink-2)" }}>World</td>
                    <td className="num" title={server?.worldGuid}>
                      {server ? formatWorldGuid(server.worldGuid) : "—"}
                    </td>
                  </tr>
                  <tr>
                    <td style={{ color: "var(--ink-2)" }}>RCON</td>
                    <td>
                      {health ? (
                        <Pill tone={health.rcon === "ok" ? "ok" : "danger"}>{health.rcon === "ok" ? "Connected" : "Error"}</Pill>
                      ) : (
                        "—"
                      )}
                    </td>
                  </tr>
                  <tr>
                    <td style={{ color: "var(--ink-2)" }}>REST API</td>
                    <td>
                      {health ? (
                        <Pill tone={health.rest === "ok" ? "ok" : "danger"}>{health.rest === "ok" ? "Connected" : "Error"}</Pill>
                      ) : (
                        "—"
                      )}
                    </td>
                  </tr>
                  <tr>
                    <td style={{ color: "var(--ink-2)" }}>Save sync</td>
                    <td>
                      {health ? <Pill tone="idle">{formatRelativeToNow(health.save.lastSyncAt)}</Pill> : "—"}
                    </td>
                  </tr>
                </tbody>
              </table>
            </CardBody>
          )}
        </Card>
      </div>

      <div className="grid cols-3">
        {/* players online chart */}
        <Card>
          <CardHead title="Players online" hint="last 24 h" />
          {playersHistoryQuery.isError ? (
            <CardBody>
              <Banner tone="warn">Couldn't load player history.</Banner>
            </CardBody>
          ) : (
            <CardBody chart>
              <Chart
                data={playersSeries}
                height={120}
                ariaLabel="Players online over the last 24 hours"
                xFormat={clockLabel}
                yFormat={(v) => String(Math.round(v))}
              />
            </CardBody>
          )}
        </Card>

        {/* recent events */}
        <Card span2>
          <CardHead title="Recent events">
            <Link to="/events" className="hint">
              view all
            </Link>
          </CardHead>
          {eventsQuery.isError ? (
            <CardBody>
              <Banner tone="warn">Couldn't load recent events.</Banner>
            </CardBody>
          ) : (
            <CardBody flush>
              <table className="table">
                <tbody>
                  {(eventsQuery.data ?? []).map((e, i) => (
                    <tr key={i}>
                      <td style={{ width: 90 }} className="num">
                        {formatDateTime(e.at).slice(11, 16)}
                      </td>
                      <td>
                        <Pill tone={e.kind === "join" ? "ok" : e.kind === "system" ? "warn" : "idle"}>{e.kind}</Pill>
                      </td>
                      <td>
                        <EventMessage text={e.message} />
                      </td>
                    </tr>
                  ))}
                  {eventsQuery.data && eventsQuery.data.length === 0 && (
                    <tr>
                      <td colSpan={3} style={{ color: "var(--ink-3)" }}>
                        No events yet.
                      </td>
                    </tr>
                  )}
                </tbody>
              </table>
            </CardBody>
          )}
        </Card>
      </div>
    </main>
  );
}
