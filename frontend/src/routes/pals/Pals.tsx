import { useEffect, useMemo, useState, type ReactNode } from "react";
import { useInfiniteQuery } from "@tanstack/react-query";
import { useSearchParams } from "react-router";
import { api } from "../../api/client";
import type { PalExplorerPal } from "../../api/types";
import { Banner } from "../../components/Banner";
import { Button } from "../../components/Button";
import { Card, CardBody, CardHead } from "../../components/Card";
import { EmptyState } from "../../components/EmptyState";
import { PalDetailPanel, PalInfoButton } from "../../components/PalDetails";
import { PalIcon } from "../../components/PalIcon";
import { palPlacementLabel } from "../../components/palDetails";
import { SearchField } from "../../components/Field";
import {
  PAL_EXPLORER_CLIENT_CAP,
  PAL_EXPLORER_PAGE_SIZE,
  EMPTY_PAL_EXPLORER_FILTERS,
  palExplorerFiltersFromSearch,
  palExplorerParams,
  palExplorerSearch,
  palOwnerSummary,
  palSpecimenLabels,
  type PalExplorerFilterState,
} from "./palExplorer";
import "./Pals.css";

export default function PalsRoute() {
  const [searchParams, setSearchParams] = useSearchParams();
  const filters = useMemo(() => palExplorerFiltersFromSearch(searchParams), [searchParams]);
  const [debouncedSearch, setDebouncedSearch] = useState("");
  const [expanded, setExpanded] = useState<string | null>(null);

  useEffect(() => {
    const timer = window.setTimeout(() => setDebouncedSearch(filters.q), 250);
    return () => window.clearTimeout(timer);
  }, [filters.q]);

  const params = palExplorerParams({ ...filters, q: debouncedSearch });
  const rangeInvalid = params.minLevel !== undefined && params.maxLevel !== undefined && params.minLevel > params.maxLevel;
  const palsQuery = useInfiniteQuery({
    queryKey: ["pals", "explorer", params],
    initialPageParam: "",
    enabled: !rangeInvalid,
    queryFn: ({ pageParam }) => api.pals.list({ ...params, cursor: pageParam || undefined, limit: PAL_EXPLORER_PAGE_SIZE }),
    getNextPageParam: (lastPage, pages) => {
      const loaded = pages.reduce((total, page) => total + page.data.length, 0);
      if (loaded >= PAL_EXPLORER_CLIENT_CAP) return undefined;
      return lastPage.nextCursor ?? undefined;
    },
  });
  const pals = palsQuery.data?.pages.flatMap((page) => page.data) ?? [];
  const capped = pals.length >= PAL_EXPLORER_CLIENT_CAP && palsQuery.data?.pages.at(-1)?.nextCursor !== null;

  function update<K extends keyof PalExplorerFilterState>(key: K, value: PalExplorerFilterState[K]) {
    setSearchParams(palExplorerSearch({ ...filters, [key]: value }, searchParams), { replace: true });
    setExpanded(null);
  }

  return (
    <main className="content pals-explorer">
      <div className="page-head">
        <h1>Pal explorer</h1>
        <span className="sub">Search every save-derived Pal without opening players one at a time</span>
      </div>

      <Card className="pals-filter-card">
        <CardHead title="Roster filters" hint="Viewer-safe save data" />
        <CardBody>
          <div className="pals-filter-grid">
            <label className="pals-search-filter">
              <span>Pal or owner</span>
              <SearchField
                value={filters.q}
                placeholder="Mammorest, Anubis, Kestrel…"
                aria-label="Search Pals or owners"
                onChange={(event) => update("q", event.target.value)}
              />
            </label>
            <ExplorerSelect label="Placement" value={filters.placement} onChange={(value) => update("placement", value)}>
              <option value="">Everywhere</option>
              <option value="party">Party</option>
              <option value="box">Palbox</option>
              <option value="base">Base workers</option>
              <option value="unknown">Unknown</option>
            </ExplorerSelect>
            <ExplorerSelect label="Specimen" value={filters.specimen} onChange={(value) => update("specimen", value)}>
              <option value="">All specimens</option>
              <option value="standard">Standard</option>
              <option value="alpha">Alpha</option>
              <option value="lucky">Lucky</option>
              <option value="boss">Boss</option>
            </ExplorerSelect>
            <ExplorerSelect label="Owner evidence" value={filters.ownerSource} onChange={(value) => update("ownerSource", value)}>
              <option value="">Any evidence</option>
              <option value="personal_container">Current container</option>
              <option value="save">Save owner</option>
              <option value="last_observed">Last observed</option>
              <option value="unresolved">Unresolved</option>
            </ExplorerSelect>
            <label>
              <span>Minimum level</span>
              <input className="input" type="number" min="0" max="999" inputMode="numeric" value={filters.minLevel} onChange={(event) => update("minLevel", event.target.value)} placeholder="0" />
            </label>
            <label>
              <span>Maximum level</span>
              <input className="input" type="number" min="0" max="999" inputMode="numeric" value={filters.maxLevel} onChange={(event) => update("maxLevel", event.target.value)} placeholder="Any" />
            </label>
            <Button sm variant="ghost" className="pals-clear" onClick={() => { setSearchParams(palExplorerSearch(EMPTY_PAL_EXPLORER_FILTERS, searchParams), { replace: true }); setExpanded(null); }}>
              Clear filters
            </Button>
          </div>
        </CardBody>
      </Card>

      {rangeInvalid ? (
        <Banner tone="warn">Minimum level cannot be higher than maximum level.</Banner>
      ) : palsQuery.isError ? (
        <Banner tone="warn">Couldn't load the Pal roster. Save data may not have been parsed yet.</Banner>
      ) : palsQuery.isPending ? (
        <Card><CardBody><span className="skel skel-text" style={{ width: "100%", height: 180 }} /></CardBody></Card>
      ) : pals.length === 0 ? (
        <Card><CardBody><EmptyState title="No Pals match" description="Try a wider level range or clear one of the roster filters." /></CardBody></Card>
      ) : (
        <>
          <div className="pals-results-head">
            <span>{pals.length} loaded</span>
            <span>Results are ordered by stable save instance</span>
          </div>
          <div className="pals-card-grid">
            {pals.map((pal) => (
              <PalExplorerCard key={pal.instanceId} pal={pal} expanded={expanded === pal.instanceId} onToggle={() => setExpanded((current) => current === pal.instanceId ? null : pal.instanceId)} />
            ))}
          </div>
          <div className="pals-load-more">
            {capped ? (
              <Banner tone="info">Showing the first {PAL_EXPLORER_CLIENT_CAP} matches. Narrow the filters to inspect the rest.</Banner>
            ) : palsQuery.hasNextPage ? (
              <Button disabled={palsQuery.isFetchingNextPage} onClick={() => palsQuery.fetchNextPage()}>
                {palsQuery.isFetchingNextPage ? "Loading…" : `Load ${PAL_EXPLORER_PAGE_SIZE} more`}
              </Button>
            ) : (
              <span>End of matching roster</span>
            )}
          </div>
        </>
      )}
    </main>
  );
}

function ExplorerSelect({ label, value, onChange, children }: { label: string; value: string; onChange: (value: string) => void; children: ReactNode }) {
  return (
    <label>
      <span>{label}</span>
      <select className="input" value={value} onChange={(event) => onChange(event.target.value)}>{children}</select>
    </label>
  );
}

function PalExplorerCard({ pal, expanded, onToggle }: { pal: PalExplorerPal; expanded: boolean; onToggle: () => void }) {
  const controls = `pal-explorer-${pal.instanceId}`;
  const specimen = palSpecimenLabels(pal);
  return (
    <article className={["pal-explorer-card", expanded ? "is-expanded" : ""].filter(Boolean).join(" ")}>
      <div className="pal-explorer-card-main">
        <PalIcon characterId={pal.characterId} displayName={pal.displayName} />
        <div className="pal-explorer-card-copy">
          <div className="pal-explorer-name-line">
            <h2>{pal.displayName}</h2>
            <span className="pal-explorer-level">Lv {pal.level}</span>
          </div>
          <div className="pal-explorer-tags">
            {specimen.map((label) => <span key={label} className={`pal-explorer-tag is-${label.toLowerCase()}`}>{label === "Boss" ? "◆ Boss" : label}</span>)}
            {specimen.length === 0 && <span className="pal-explorer-tag">Standard</span>}
          </div>
          <strong className={pal.ownerResolved ? "" : "is-muted"}>{palOwnerSummary(pal)}</strong>
          <span>{palPlacementLabel(pal)}</span>
        </div>
        <PalInfoButton pal={pal} expanded={expanded} controls={controls} onClick={onToggle} />
      </div>
      {expanded && <PalDetailPanel pal={pal} id={controls} />}
    </article>
  );
}
