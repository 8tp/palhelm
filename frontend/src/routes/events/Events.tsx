import { useMemo, useState } from "react";
import { useQuery } from "@tanstack/react-query";
import { api } from "../../api/client";
import type { EventKind } from "../../api/types";
import { Banner } from "../../components/Banner";
import { Card, CardBody } from "../../components/Card";
import { EmptyState } from "../../components/EmptyState";
import { EventMessage } from "../../components/EventMessage";
import { SearchField } from "../../components/Field";
import { Pill } from "../../components/Pill";
import { formatDateTime } from "../../app/format";
import "./Events.css";

const PAGE_SIZE = 25;
const FETCH_LIMIT = 500;
type Filter = "all" | EventKind;

const FILTERS: Array<{ value: Filter; label: string }> = [
  { value: "all", label: "All events" },
  { value: "join", label: "Joins" },
  { value: "leave", label: "Leaves" },
  { value: "backup", label: "Backups" },
  { value: "system", label: "System" },
  { value: "panel", label: "Panel audit" },
  { value: "config", label: "Configuration" },
];

function tone(kind: EventKind): "ok" | "warn" | "idle" | "danger" {
  if (kind === "join") return "ok";
  if (kind === "system" || kind === "config") return "warn";
  if (kind === "panel") return "danger";
  return "idle";
}

export default function EventsRoute() {
  const [filter, setFilter] = useState<Filter>("all");
  const [search, setSearch] = useState("");
  const [page, setPage] = useState(0);
  const eventsQuery = useQuery({
    queryKey: ["events", FETCH_LIMIT, filter],
    queryFn: () => api.events.list(FETCH_LIMIT, filter === "all" ? undefined : filter),
    refetchInterval: 30_000,
  });
  const filtered = useMemo(() => {
    const needle = search.trim().toLocaleLowerCase();
    if (!needle) return eventsQuery.data ?? [];
    return (eventsQuery.data ?? []).filter((event) =>
      event.message.toLocaleLowerCase().includes(needle) || event.kind.includes(needle),
    );
  }, [eventsQuery.data, search]);
  const pageCount = Math.max(1, Math.ceil(filtered.length / PAGE_SIZE));
  const safePage = Math.min(page, pageCount - 1);
  const shown = filtered.slice(safePage * PAGE_SIZE, (safePage + 1) * PAGE_SIZE);

  function changeFilter(next: Filter) {
    setFilter(next);
    setPage(0);
  }

  return (
    <main className="content">
      <div className="page-head">
        <h1>Events & audit</h1>
        <span className="sub">player activity · operations · panel changes</span>
      </div>

      <div className="events-toolbar">
        <SearchField
          value={search}
          onChange={(event) => { setSearch(event.target.value); setPage(0); }}
          placeholder="Search event messages…"
          aria-label="Search events"
        />
        <select className="input" value={filter} onChange={(event) => changeFilter(event.target.value as Filter)} aria-label="Filter event type">
          {FILTERS.map((item) => <option key={item.value} value={item.value}>{item.label}</option>)}
        </select>
        <span className="events-count">{filtered.length} shown · newest {FETCH_LIMIT} scanned</span>
      </div>

      <Card>
        {eventsQuery.isError ? (
          <CardBody><Banner tone="warn">Couldn't load panel events.</Banner></CardBody>
        ) : eventsQuery.isLoading ? (
          <CardBody><span className="skel skel-text" style={{ width: "100%", height: 100 }} /></CardBody>
        ) : shown.length === 0 ? (
          <CardBody><EmptyState title="No matching events" description="Try a different event type or search phrase." /></CardBody>
        ) : (
          <CardBody flush className="events-table-wrap">
            <table className="table events-table">
              <thead><tr><th>Time</th><th>Type</th><th>Event</th><th>Actor</th></tr></thead>
              <tbody>
                {shown.map((event, index) => {
                  const actor = typeof event.meta?.actor === "string" ? event.meta.actor : "—";
                  return (
                    <tr key={`${event.at}-${event.kind}-${safePage * PAGE_SIZE + index}`}>
                      <td className="num events-time">{formatDateTime(event.at)}</td>
                      <td><Pill tone={tone(event.kind)}>{event.kind}</Pill></td>
                      <td><EventMessage text={event.message} /></td>
                      <td className="num">{actor}</td>
                    </tr>
                  );
                })}
              </tbody>
            </table>
          </CardBody>
        )}
      </Card>

      {filtered.length > PAGE_SIZE && (
        <div className="events-pagination" aria-label="Event pages">
          <button type="button" className="btn btn-sm" disabled={safePage === 0} onClick={() => setPage(safePage - 1)}>Previous</button>
          <span className="num">Page {safePage + 1} of {pageCount}</span>
          <button type="button" className="btn btn-sm" disabled={safePage >= pageCount - 1} onClick={() => setPage(safePage + 1)}>Next</button>
        </div>
      )}
    </main>
  );
}
