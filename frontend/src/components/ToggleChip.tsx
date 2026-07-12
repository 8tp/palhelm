import type { ReactNode } from "react";

export function ToggleChip({
  pressed,
  onClick,
  children,
  count,
}: {
  pressed: boolean;
  onClick: () => void;
  children: ReactNode;
  count?: number;
}) {
  return (
    <button type="button" className="chip-toggle" aria-pressed={pressed} onClick={onClick}>
      {children}
      {count !== undefined && <span className="n">{count}</span>}
    </button>
  );
}
