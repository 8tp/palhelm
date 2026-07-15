import type { SVGProps } from "react";
import { workSuitabilitiesFor, workSuitabilityKind, type PalWorkSuitability, type WorkSuitabilityKind } from "./workSuitabilities";

export function WorkSuitabilityBadges({ characterId }: { characterId: string }) {
  const work = workSuitabilitiesFor(characterId);
  if (work === undefined) {
    return <span className="pal-detail-muted">Unavailable in the pinned Paldeck snapshot</span>;
  }
  if (work.length === 0) {
    return <span className="pal-detail-muted">No work suitability</span>;
  }
  return (
    <div className="pal-work-grid">
      {work.map((item) => <WorkBadge key={item.id} work={item} />)}
    </div>
  );
}

function WorkBadge({ work }: { work: PalWorkSuitability }) {
  const kind = workSuitabilityKind(work.id, work.name);
  return (
    <div className={`pal-work-badge is-${kind}`} title={`${work.name} level ${work.level}`}>
      <span className="pal-work-icon"><WorkIcon kind={kind} /></span>
      <span className="pal-work-name">{work.name}</span>
      <strong className="pal-work-level" aria-label={`level ${work.level}`}>Lv {work.level}</strong>
    </div>
  );
}

function WorkIcon({ kind }: { kind: WorkSuitabilityKind }) {
  const common: SVGProps<SVGSVGElement> = {
    width: 18,
    height: 18,
    viewBox: "0 0 18 18",
    fill: "none",
    stroke: "currentColor",
    strokeWidth: 1.55,
    strokeLinecap: "round",
    strokeLinejoin: "round",
    "aria-hidden": true,
  };
  switch (kind) {
    case "kindling":
      return <svg {...common}><path d="M10.4 1.7c.5 2.8-1.8 3.8-.9 5.7.6 1.3 2 1.4 2.5 2.8.5-1.1.6-2.2.2-3.4 2.3 1.8 3.1 4.2 1.9 6.6-1.4 2.9-5.8 3.7-8.5 1.4-2.2-1.8-2-4.9-.8-6.9 1.2-2 3.5-3.3 5.6-6.2Z" /><path d="M9.1 10.1c1.6 1.3 1.5 3.5-.2 4.5-1.2-.5-1.8-1.5-1.4-2.6.3-.8 1.1-1.2 1.6-1.9Z" /></svg>;
    case "watering":
      return <svg {...common}><path d="M9 1.8C7.1 4.5 4.8 7 4.8 10.3a4.2 4.2 0 0 0 8.4 0C13.2 7 10.9 4.5 9 1.8Z" /><path d="M7 11.2c.2 1.1.9 1.8 2 2" /></svg>;
    case "planting":
      return <svg {...common}><path d="M9 15.8V8.6" /><path d="M9 9C5.6 9 3.5 7.2 3.3 3.7 6.7 3.5 8.7 5.3 9 9Z" /><path d="M9 11.7c3.2 0 5.2-1.7 5.4-4.9-3.2-.2-5.1 1.5-5.4 4.9Z" /><path d="M4.2 15.8h9.6" /></svg>;
    case "electricity":
      return <svg {...common}><path d="m10.4 1.6-6 8.2h4l-.8 6.6 6-8.6H9.7l.7-6.2Z" /></svg>;
    case "handiwork":
      return <svg {...common}><path d="m3.1 14.9 6.2-6.2" /><path d="m7.7 3.2 2.1-1.4 5 5-1.4 2.1-2.1-2.1-6.5 6.5" /><path d="m2.5 13.6 1.9 1.9" /></svg>;
    case "gathering":
      return <svg {...common}><path d="M3.1 7.7h11.8l-1.1 7.5H4.2L3.1 7.7Z" /><path d="M6 7.7a3 3 0 0 1 6 0M6.5 10.6v2.1M9 10.6v2.1M11.5 10.6v2.1" /></svg>;
    case "lumbering":
      return <svg {...common}><path d="m4 15 7.4-7.4" /><path d="m8.8 4.1 2.3-2.3 4.8 4.8-2.3 2.3c-2.2-.8-4-2.6-4.8-4.8Z" /><path d="m2.8 13.8 1.4 1.4" /></svg>;
    case "mining":
      return <svg {...common}><path d="m5 15 6.7-10.7" /><path d="M2.1 5.8c3.7-2.9 8.9-2.8 13.8.4-3.4-1-6.6-.4-9.1 1.7L2.1 5.8Z" /></svg>;
    case "medicine":
      return <svg {...common}><path d="M7 1.8h4M8 1.8v4.1l-4 7a2 2 0 0 0 1.7 3h6.6a2 2 0 0 0 1.7-3l-4-7V1.8" /><path d="M5.5 11h7M7.5 13.3h3" /></svg>;
    case "cooling":
      return <svg {...common}><path d="M9 1.5v15M2.5 5.2l13 7.6M2.5 12.8l13-7.6" /><path d="m7 3.4 2 1.2 2-1.2M7 14.6 9 13.4l2 1.2M4.2 6.6l.1 2.3-2 1.1M13.8 11.4l-.1-2.3 2-1.1" /></svg>;
    case "transporting":
      return <svg {...common}><path d="M2.2 5.2 9 1.8l6.8 3.4v7.6L9 16.2l-6.8-3.4V5.2Z" /><path d="m2.2 5.2 6.8 3.4 6.8-3.4M9 8.6v7.6M5.6 3.5l6.8 3.4" /></svg>;
    case "farming":
      return <svg {...common}><path d="M9 16V5.4M9 7.3C6.4 7.3 4.7 6 4.5 3.5 7.1 3.3 8.7 4.6 9 7.3ZM9 10.7c2.6 0 4.3-1.3 4.5-3.8-2.6-.2-4.2 1.1-4.5 3.8ZM9 13.8c-2.2 0-3.6-1.1-3.8-3.2 2.2-.2 3.6.9 3.8 3.2Z" /></svg>;
    default:
      return <svg {...common}><circle cx="9" cy="9" r="6.5" /><path d="M9 5.4v4.2M9 12.7v.1" /></svg>;
  }
}
