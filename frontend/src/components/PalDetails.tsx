import type { PlayerPal } from "../api/types";
import { truncateMiddle } from "../app/format";
import { IconInfo } from "./icons";
import { humanizePalIdentifier, palGenderLabel, palPlacementLabel } from "./palDetails";
import { WorkSuitabilityBadges } from "./WorkSuitabilityBadges";
import { PAL_WORK_DATA_PROVENANCE } from "./workSuitabilities";
import "./PalDetails.css";

export function PalInfoButton({
  pal,
  expanded,
  controls,
  onClick,
}: {
  pal: PlayerPal;
  expanded: boolean;
  controls: string;
  onClick: () => void;
}) {
  return (
    <button
      type="button"
      className={["pal-info-button", expanded ? "is-active" : ""].filter(Boolean).join(" ")}
      aria-label={`${expanded ? "Hide" : "Show"} details for ${pal.displayName}`}
      aria-expanded={expanded}
      aria-controls={controls}
      title={`${expanded ? "Hide" : "Show"} Pal details`}
      onClick={(event) => {
        event.stopPropagation();
        onClick();
      }}
    >
      <IconInfo width={14} height={14} />
    </button>
  );
}

export function PalDetailPanel({ pal, id }: { pal: PlayerPal; id: string }) {
  const talents = pal.talents;
  const passives = pal.passiveSkillIds;
  const equipped = pal.equippedSkillIds;
  return (
    <section id={id} className="pal-detail-panel" aria-label={`${pal.displayName} details`}>
      <div className="pal-detail-facts">
        <Fact label="Gender" value={palGenderLabel(pal.gender)} />
        <Fact label="Current HP" value={pal.hp === undefined || pal.hp === null ? "Unavailable" : formatNumber(pal.hp)} />
        <Fact label="Placement" value={palPlacementLabel(pal)} />
        <Fact label="Specimen" value={[pal.isAlpha ? "Alpha" : null, pal.isLucky ? "Lucky" : null].filter(Boolean).join(" · ") || "Standard"} />
      </div>

      <div className="pal-detail-section">
        <h4>Individual talents</h4>
        <div className="pal-talent-grid">
          <Talent label="HP" value={talents?.hp} />
          <Talent label="Melee" value={talents?.melee} />
          <Talent label="Ranged" value={talents?.shot} />
          <Talent label="Defense" value={talents?.defense} />
        </div>
      </div>

      <div className="pal-detail-section">
        <h4>Work suitability <span className="pal-detail-section-source" title={PAL_WORK_DATA_PROVENANCE}>pinned species data</span></h4>
        <WorkSuitabilityBadges characterId={pal.characterId} />
      </div>

      <SkillList title="Passive skills" values={passives} />
      <SkillList title="Equipped attacks" values={equipped} />

      <div className="pal-detail-identity">
        <span title={pal.characterId}>Species ID {truncateMiddle(pal.characterId, 14, 6)}</span>
        <span title={pal.instanceId}>Instance {truncateMiddle(pal.instanceId, 10, 6)}</span>
      </div>
    </section>
  );
}

function Fact({ label, value }: { label: string; value: string }) {
  return (
    <div className="pal-detail-fact">
      <span>{label}</span>
      <strong>{value}</strong>
    </div>
  );
}

function Talent({ label, value }: { label: string; value: number | null | undefined }) {
  return (
    <div className="pal-talent-box">
      <span>{label}</span>
      <strong>{value === undefined || value === null ? "—" : value}</strong>
    </div>
  );
}

function SkillList({ title, values }: { title: string; values: string[] | undefined }) {
  return (
    <div className="pal-detail-section">
      <h4>{title}</h4>
      {values === undefined ? (
        <span className="pal-detail-muted">Unavailable from this save parse</span>
      ) : values.length === 0 ? (
        <span className="pal-detail-muted">None observed</span>
      ) : (
        <div className="pal-skill-list">
          {values.map((value) => <span key={value} title={value}>{humanizePalIdentifier(value)}</span>)}
        </div>
      )}
    </div>
  );
}

function formatNumber(value: number): string {
  return Number.isInteger(value) ? String(value) : value.toFixed(1);
}
