import type { ReactNode } from "react";

export function StatTile({
  label,
  value,
  unit,
  delta,
  deltaTone,
}: {
  label: string;
  value: ReactNode;
  unit?: ReactNode;
  delta?: ReactNode;
  deltaTone?: "up" | "down";
}) {
  return (
    <div className="card stat">
      <span className="label">{label}</span>
      <div className="value">
        {value}
        {unit !== undefined && <small> {unit}</small>}
      </div>
      {delta !== undefined && (
        <div className={["delta", deltaTone ?? ""].filter(Boolean).join(" ")}>{delta}</div>
      )}
    </div>
  );
}

export function StatTileSkeleton() {
  return (
    <div className="card stat">
      <span className="skel skel-text" style={{ width: "60%" }} />
      <div className="value" style={{ marginTop: 6 }}>
        <span className="skel skel-text" style={{ width: "40%", height: 22 }} />
      </div>
      <div className="delta" style={{ marginTop: 6 }}>
        <span className="skel skel-text" style={{ width: "70%" }} />
      </div>
    </div>
  );
}
