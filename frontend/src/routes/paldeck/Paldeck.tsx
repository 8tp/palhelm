import { useMemo, useState } from "react";
import { useQuery } from "@tanstack/react-query";
import { Link, useSearchParams } from "react-router";
import { api } from "../../api/client";
import type { PlayerPaldeck, ServerPaldeck } from "../../api/types";
import { formatRelativeToNow } from "../../app/format";
import { Banner } from "../../components/Banner";
import { Card, CardBody, CardHead } from "../../components/Card";
import { EmptyState } from "../../components/EmptyState";
import { SearchField } from "../../components/Field";
import { PalIcon } from "../../components/PalIcon";
import { palExplorerHref } from "../pals/palExplorer";
import { filterPaldeckSpecies, paldeckPercent, type PaldeckSpeciesFilter } from "./paldeckView";
import "./Paldeck.css";

type PaldeckSpecies = ServerPaldeck["species"][number] | PlayerPaldeck["species"][number];

export default function PaldeckRoute() {
  const [searchParams, setSearchParams] = useSearchParams();
  const selectedUid = searchParams.get("player") ?? "";
  const [search, setSearch] = useState("");
  const [filter, setFilter] = useState<PaldeckSpeciesFilter>("all");
  const playersQuery = useQuery({ queryKey: ["players"], queryFn: () => api.players.list() });
  const serverQuery = useQuery({ queryKey: ["paldeck", "server"], queryFn: () => api.paldeck.get(), enabled: !selectedUid });
  const playerQuery = useQuery({ queryKey: ["paldeck", "player", selectedUid], queryFn: () => api.paldeck.player(selectedUid), enabled: Boolean(selectedUid) });
  const data = selectedUid ? playerQuery.data : serverQuery.data;
  const query = selectedUid ? playerQuery : serverQuery;
  const captureConclusive = data ? "player" in data
    ? data.coverage.captureCountsAvailable && !data.coverage.captureCountsTruncated
    : data.coverage.playersTotal > 0 && data.coverage.playersWithCaptureCounts === data.coverage.playersTotal && !data.coverage.captureCountsTruncated
    : false;
  const effectiveFilter = !captureConclusive && filter === "unseen" ? "all" : filter;
  const species = useMemo(() => filterPaldeckSpecies<PaldeckSpecies>(data?.species ?? [], search, effectiveFilter), [data?.species, search, effectiveFilter]);

  function selectPlayer(uid: string) {
    const next = new URLSearchParams(searchParams);
    if (uid) next.set("player", uid);
    else next.delete("player");
    setSearchParams(next, { replace: true });
  }

  return (
    <main className="content paldeck-page">
      <div className="page-head paldeck-head">
        <div>
          <h1>Paldeck</h1>
          <span className="sub">capture progress from parsed saves · 1.0 catalog</span>
        </div>
        <label className="paldeck-player-select">
          <span>Progress view</span>
          <select className="input" value={selectedUid} onChange={(event) => selectPlayer(event.target.value)}>
            <option value="">All players</option>
            {(playersQuery.data ?? []).map((player) => <option key={player.uid} value={player.uid}>{player.name || "Unknown player"}</option>)}
          </select>
        </label>
      </div>

      {query.isError ? (
        <Banner tone="warn">Couldn't load Paldeck data from player saves.</Banner>
      ) : query.isPending || !data ? (
        <Card><CardBody><span className="skel skel-text paldeck-skeleton" /></CardBody></Card>
      ) : (
        <PaldeckContent data={data} search={search} setSearch={setSearch} filter={filter} setFilter={setFilter} species={species} />
      )}
    </main>
  );
}

function PaldeckContent({ data, search, setSearch, filter, setFilter, species }: {
  data: ServerPaldeck | PlayerPaldeck;
  search: string;
  setSearch: (value: string) => void;
  filter: PaldeckSpeciesFilter;
  setFilter: (value: PaldeckSpeciesFilter) => void;
  species: PaldeckSpecies[];
}) {
  const isPlayer = "player" in data;
  const captureAvailable = isPlayer ? data.coverage.captureCountsAvailable : data.coverage.playersWithCaptureCounts > 0;
  const captureTruncated = data.coverage.captureCountsTruncated;
  const captureConclusive = isPlayer
    ? data.coverage.captureCountsAvailable && !captureTruncated
    : data.coverage.playersTotal > 0 && data.coverage.playersWithCaptureCounts === data.coverage.playersTotal && !captureTruncated;
  const unlockConclusive = isPlayer
    ? data.coverage.unlockFlagsAvailable && !data.coverage.unlockFlagsTruncated
    : data.coverage.playersTotal > 0 && data.coverage.playersWithUnlockFlags === data.coverage.playersTotal && !data.coverage.unlockFlagsTruncated;
  const observedAt = isPlayer ? data.coverage.captureObservedAt : data.coverage.latestObservedAt;
  const rawUnique = isPlayer ? data.uniquePalsCaptured : data.uniqueSpeciesCaptured;
  const pinnedCaptured = captureConclusive ? data.species.filter((item) => item.known && item.captureCount !== null && item.captureCount > 0).length : null;
  const pinnedUnlocked = unlockConclusive ? data.species.filter((item) => item.known && (isPlayer ? (item as PlayerPaldeck["species"][number]).unlocked === true : ((item as ServerPaldeck["species"][number]).unlockedByPlayers ?? 0) > 0)).length : null;

  return (
    <>
      <Banner tone={!captureConclusive ? "warn" : "info"}>
        {isPlayer
          ? captureAvailable ? `Capture data from ${formatRelativeToNow(observedAt)}.` : "No capture data decoded for this player yet."
          : `Capture data covers ${data.coverage.playersWithCaptureCounts} of ${data.coverage.playersTotal} players.`}
        {captureTruncated ? " The capture map was truncated, so “unseen” is not conclusive." : " Missing data is never counted as zero."}
      </Banner>

      <div className="paldeck-stats" aria-label="Paldeck summary">
        <ProgressStat label="Species captured" value={pinnedCaptured} denominator={data.catalog.knownSpecies} percent={paldeckPercent(pinnedCaptured, data.catalog.knownSpecies)} />
        <ProgressStat label="Entries unlocked" value={pinnedUnlocked} denominator={data.catalog.knownSpecies} percent={paldeckPercent(pinnedUnlocked, data.catalog.knownSpecies)} />
        <Card><CardBody className="paldeck-stat"><span>Total captures</span><strong>{data.captureTotal ?? "Unavailable"}</strong><small>sum of save counters</small></CardBody></Card>
        <Card><CardBody className="paldeck-stat"><span>Unique species counter</span><strong>{rawUnique ?? "Unavailable"}</strong><small>from the save · may include unlisted IDs</small></CardBody></Card>
      </div>

      <Card>
        <CardHead title="Species" hint={`${species.length} shown of ${data.catalog.knownSpecies}`} />
        <CardBody>
          <div className="paldeck-tools">
            <SearchField value={search} onChange={(event) => setSearch(event.target.value)} placeholder="Search Pal name…" aria-label="Search Paldeck species" />
            <select className="input" aria-label="Capture filter" value={!captureConclusive && filter === "unseen" ? "all" : filter} onChange={(event) => setFilter(event.target.value as PaldeckSpeciesFilter)}>
              <option value="all">All species</option><option value="captured">Captured</option><option value="unseen" disabled={!captureConclusive}>Unseen (needs full data)</option><option value="unavailable">No data</option>
            </select>
          </div>
        </CardBody>
        {species.length === 0 ? <CardBody><EmptyState title="No species match" description="Clear the search or choose another filter." /></CardBody> : (
          <CardBody className="paldeck-grid">
            {species.map((item) => {
              const count = item.captureCount;
              const unsafeZero = !captureConclusive && count === 0;
              const serverItem = !isPlayer ? item as ServerPaldeck["species"][number] : null;
              const playerItem = isPlayer ? item as PlayerPaldeck["species"][number] : null;
              return (
                <article className="paldeck-species" key={item.characterId}>
                  <PalIcon characterId={item.characterId} displayName={item.displayName} />
                  <div><strong>{item.displayName}</strong><small>{!item.known ? `Unlisted ID · ${item.characterId}` : count === null ? "No capture data" : unsafeZero ? "None seen in partial data" : count === 0 ? "Not captured" : `${count} captured`}</small></div>
                  <div className="paldeck-species-meta">
                    {serverItem && serverItem.capturedByPlayers !== null && <span>{serverItem.capturedByPlayers} players</span>}
                    {playerItem?.unlocked !== null && playerItem?.unlocked !== undefined && <span>{playerItem.unlocked ? "Unlocked" : "Locked"}</span>}
                    {count !== null && count > 0 && <Link to={palExplorerHref({ q: item.displayName })}>View roster</Link>}
                  </div>
                </article>
              );
            })}
          </CardBody>
        )}
      </Card>
      <p className="paldeck-footnote">Catalog {data.catalog.version} · {data.catalog.observedUnknownSpecies} IDs outside the catalog. Counts reflect the latest parsed save.</p>
    </>
  );
}

function ProgressStat({ label, value, denominator, percent }: { label: string; value: number | null; denominator: number; percent: number | null }) {
  return <Card><CardBody className="paldeck-stat"><span>{label}</span><strong>{value === null ? "Unavailable" : `${value} / ${denominator}`}</strong><small>{percent === null ? "needs full capture data" : `${percent}% of catalog`}</small>{percent !== null && <progress max="100" value={percent} aria-label={`${label}: ${percent}%`} />}</CardBody></Card>;
}
