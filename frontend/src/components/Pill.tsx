import type { ReactNode } from "react";

export type PillTone = "ok" | "warn" | "danger" | "idle";

export function Pill({ tone, children }: { tone: PillTone; children: ReactNode }) {
  return (
    <span className={`pill pill-${tone}`}>
      <span className="dot" aria-hidden="true" />
      {children}
    </span>
  );
}
