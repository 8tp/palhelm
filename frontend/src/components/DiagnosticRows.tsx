import type { ReactNode } from "react";
import { Pill, type PillTone } from "./Pill";

export function DiagnosticRows({ children }: { children: ReactNode }) {
  return <dl className="diagnostic-rows">{children}</dl>;
}

export function DiagnosticRow({
  label,
  value,
  detail,
  tone,
}: {
  label: string;
  value: ReactNode;
  detail?: ReactNode;
  tone?: PillTone;
}) {
  return (
    <div className="diagnostic-row">
      <dt>{label}</dt>
      <dd>
        <span className="diagnostic-value">{tone ? <Pill tone={tone}>{value}</Pill> : value}</span>
        {detail && <span className="diagnostic-detail">{detail}</span>}
      </dd>
    </div>
  );
}

export function UnavailableDiagnostic({ label, reason }: { label: string; reason: string }) {
  return <DiagnosticRow label={label} value="Unavailable" detail={reason} tone="idle" />;
}
