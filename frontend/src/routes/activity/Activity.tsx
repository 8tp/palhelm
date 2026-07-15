import { useState } from "react";
import { useQuery } from "@tanstack/react-query";
import { api } from "../../api/client";
import type { ServerActivityWindow } from "../../api/types";
import { formatDuration } from "../../app/format";
import { Banner } from "../../components/Banner";
import { Card, CardBody, CardHead } from "../../components/Card";
import { EmptyState } from "../../components/EmptyState";
import { activityCoverageNote, localBucketLabel, topPeakBuckets } from "./activityView";
import "./Activity.css";

const WINDOWS: Array<{ value: ServerActivityWindow; label: string }> = [
  { value: "24h", label: "24 hours" },
  { value: "7d", label: "7 days" },
  { value: "30d", label: "30 days" },
];

export default function ActivityRoute() {
  const [window, setWindow] = useState<ServerActivityWindow>("7d");
  const query = useQuery({
    queryKey: ["activity", window],
    queryFn: () => api.activity.get(window),
    refetchInterval: 60_000,
  });
  const activity = query.data;
  const maxConcurrency = Math.max(1, ...(activity?.concurrency.map((bucket) => bucket.peakPlayers) ?? [1]));
  const peakBuckets = topPeakBuckets(activity?.concurrency ?? []);

  return (
    <main className="content activity-page">
      <div className="page-head activity-head">
        <div>
          <h1>Player activity</h1>
          <span className="sub">panel-observed sessions · rolling windows · no lifetime claims</span>
        </div>
        <div className="activity-window-tabs" aria-label="Activity window">
          {WINDOWS.map((item) => (
            <button key={item.value} type="button" className={window === item.value ? "is-active" : ""} onClick={() => setWindow(item.value)}>{item.label}</button>
          ))}
        </div>
      </div>

      {query.isError ? (
        <Banner tone="warn">Couldn't load observed player activity.</Banner>
      ) : query.isPending || !activity ? (
        <Card><CardBody><span className="skel skel-text activity-skeleton" /></CardBody></Card>
      ) : activity.activePlayers === 0 ? (
        <Card><CardBody><EmptyState title="No observed sessions" description="Activity appears after this panel observes player join and leave transitions." /></CardBody></Card>
      ) : (
        <>
          <Banner tone={activity.analysisTruncated ? "warn" : "info"}>{activityCoverageNote(activity)} These are panel observations, not lifetime game history.</Banner>

          <div className="activity-kpis">
            <KPI label="Active players" value={String(activity.activePlayers)} detail={`${activity.newPlayers} first observed · ${activity.returningPlayers} returning`} />
            <KPI label="Peak concurrency" value={String(activity.peakConcurrency)} detail={activity.peakAt ? new Date(activity.peakAt).toLocaleString() : "No peak observed"} />
            <KPI label="Guild attribution" value={String(activity.activePlayers - activity.unattributedPlayers)} detail={`${activity.unattributedPlayers} without a current guild`} />
          </div>

          <div className="activity-layout">
            <Card className="activity-concurrency-card">
              <CardHead title="Concurrency" hint={`${formatDuration(activity.bucketSec)} buckets`} />
              <CardBody>
                <div className="activity-bars" aria-label="Observed concurrency timeline">
                  {activity.concurrency.map((bucket) => (
                    <div key={bucket.at} className="activity-bar-slot" title={`${localBucketLabel(bucket.at, activity.bucketSec)} · avg ${bucket.averagePlayers.toFixed(1)} · peak ${bucket.peakPlayers}`}>
                      <span style={{ height: `${Math.max(2, (bucket.averagePlayers / maxConcurrency) * 100)}%` }} />
                    </div>
                  ))}
                </div>
                <div className="activity-chart-foot"><span>{new Date(activity.since).toLocaleString()}</span><span>{new Date(activity.through).toLocaleString()}</span></div>
              </CardBody>
            </Card>

            <Card>
              <CardHead title="Peak hours" hint="browser local time" />
              <CardBody flush>
                <div className="activity-peak-list">
                  {peakBuckets.map((bucket, index) => (
                    <div key={bucket.at}><b>#{index + 1}</b><span>{localBucketLabel(bucket.at, activity.bucketSec)}</span><strong>{bucket.averagePlayers.toFixed(1)} avg · {bucket.peakPlayers} peak</strong></div>
                  ))}
                </div>
              </CardBody>
            </Card>
          </div>

          <div className="activity-layout">
            <Card>
              <CardHead title="Most active players" hint={`top ${activity.players.length} · selected window`} />
              <CardBody flush className="activity-table-wrap">
                <table className="table activity-table"><thead><tr><th>Player</th><th>Observed</th><th>Sessions</th></tr></thead><tbody>
                  {activity.players.map((player) => <tr key={player.uid}><td><strong>{player.name || "Unknown player"}</strong><small>{player.firstObserved ? "First observed in window" : player.currentSession ? "Current session open" : player.guildName || "No current guild"}</small></td><td className="num">{formatDuration(player.durationSec)}</td><td className="num">{player.sessionCount}</td></tr>)}
                </tbody></table>
              </CardBody>
            </Card>

            <Card>
              <CardHead title="Guild activity" hint="current membership attribution" />
              {activity.guilds.length === 0 ? <CardBody><EmptyState title="No attributable guild activity" description="Observed players do not currently have guild evidence." /></CardBody> : (
                <CardBody flush className="activity-table-wrap">
                  <table className="table activity-table"><thead><tr><th>Guild</th><th>Observed</th><th>Players</th></tr></thead><tbody>
                    {activity.guilds.map((guild) => <tr key={guild.guildId}><td><strong>{guild.guildName || "Unnamed guild"}</strong><small>{guild.sessionCount} observed sessions</small></td><td className="num">{formatDuration(guild.durationSec)}</td><td className="num">{guild.activePlayers}</td></tr>)}
                  </tbody></table>
                  <p className="activity-attribution-note">Sessions are attributed to each player's current save-derived guild. Historical guild membership is not stored.</p>
                </CardBody>
              )}
            </Card>
          </div>
        </>
      )}
    </main>
  );
}

function KPI({ label, value, detail }: { label: string; value: string; detail: string }) {
  return <Card><CardBody className="activity-kpi"><span>{label}</span><strong>{value}</strong><small>{detail}</small></CardBody></Card>;
}
