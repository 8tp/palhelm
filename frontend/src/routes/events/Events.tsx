import { useState } from "react";
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
import {
  countEventLanes,
  EVENT_LANES,
  filterEvents,
  kindsForLane,
  type EventKindFilter,
  type EventLane,
} from "./eventLanes";
import "./Events.css";

const PAGE_SIZE = 25;
const FETCH_LIMIT = 500;

const KIND_LABELS: Record<EventKindFilter, string> = {
  all: "All kinds",
  join: "Joins",
  leave: "Leaves",
  backup: "Backups",
  system: "System",
  panel: "Panel audit",
  config: "Configuration",
};

function tone(kind: EventKind): "ok" | "warn" | "idle" | "danger" {
  if (kind === "join") return "ok";
  if (kind === "system" || kind === "config") return "warn";
  if (kind === "panel") return "danger";
  return "idle";
}

export default function EventsRoute() {
  const [lane, setLane] = useState<EventLane>("all");
  const [kind, setKind] = useState<EventKindFilter>("all");
  const [search, setSearch] = useState("");
  const [page, setPage] = useState(0);
  const eventsQuery = useQuery({
    queryKey: ["events", FETCH_LIMIT],
    queryFn: () => api.events.list(FETCH_LIMIT),
    refetchInterval: 30_000,
  });
  const events = eventsQuery.data ?? [];
  const laneCounts = countEventLanes(events);
  const filtered = filterEvents(events, lane, kind, search);
  const availableKinds = kindsForLane(lane);
  const pageCount = Math.max(1, Math.ceil(filtered.length / PAGE_SIZE));
  const safePage = Math.min(page, pageCount - 1);
  const shown = filtered.slice(safePage * PAGE_SIZE, (safePage + 1) * PAGE_SIZE);

  function changeLane(next: EventLane) {
    setLane(next);
    setKind("all");
    setPage(0);
  }

  function changeKind(next: EventKindFilter) {
    setKind(next);
    setPage(0);
  }

  return (
    <main className="content">
      <div className="page-head">
        <h1>Events & audit</h1>
        <span className="sub">player activity · operations & audit · health incidents</span>
      </div>

      <div className="event-lanes" role="group" aria-label="Event lanes">
        {EVENT_LANES.map((item) => (
          <button
            type="button"
            aria-pressed={lane === item.id}
            className={`event-lane ${lane === item.id ? "is-active" : ""}`}
            key={item.id}
            onClick={() => changeLane(item.id)}
          >
            <span className="event-lane-name">{item.label}</span>
            <span className="event-lane-count">{laneCounts[item.id].toLocaleString()}</span>
            <span className="event-lane-description">{item.description}</span>
          </button>
        ))}
      </div>
      <p className="events-scope">
        Lane counts cover the newest {events.length.toLocaleString()} events returned by the panel (bounded to {FETCH_LIMIT}).
      </p>

      <div className="events-toolbar">
        <SearchField
          value={search}
          onChange={(event) => { setSearch(event.target.value); setPage(0); }}
          placeholder="Search event messages…"
          aria-label="Search events"
        />
        <select className="input" value={kind} onChange={(event) => changeKind(event.target.value as EventKindFilter)} aria-label="Filter exact event kind">
          <option value="all">{KIND_LABELS.all}</option>
          {availableKinds.map((item) => <option key={item} value={item}>{KIND_LABELS[item]}</option>)}
        </select>
        <span className="events-count">{filtered.length.toLocaleString()} matching · page {safePage + 1} of {pageCount}</span>
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
