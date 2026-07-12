import type { ReactNode } from "react";
import { IconKebab } from "./icons";

/** Card-body wrapper for a `.table` — flush padding, horizontal scroll on overflow. */
export function TableWrap({ children }: { children: ReactNode }) {
  return (
    <div className="card-body flush" style={{ overflowX: "auto" }}>
      <table className="table">{children}</table>
    </div>
  );
}

/**
 * Row action cell: buttons are hover-revealed (`.actions .btn`) per ui.css, but a kebab button
 * stays keyboard-reachable — visible on focus — so hover-only actions are never focus-trapped.
 */
export function RowActions({ children, onMore }: { children: ReactNode; onMore?: () => void }) {
  return (
    <td className="actions">
      {children}
      {onMore && (
        <button type="button" className="btn btn-sm btn-ghost row-kebab" onClick={onMore} aria-label="More actions">
          <IconKebab />
        </button>
      )}
    </td>
  );
}

export function WhoCell({ initials, name, id }: { initials: string; name: string; id: string }) {
  return (
    <div className="who-cell">
      <span className="avatar">{initials}</span>
      <div>
        <div className="name">{name}</div>
        <div className="id">{id}</div>
      </div>
    </div>
  );
}
