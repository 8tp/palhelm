import { useEffect, useMemo, useState } from "react";
import type { PlayerPal } from "../api/types";
import { Dialog } from "./ConfirmDialog";
import { PalIcon } from "./PalIcon";
import { IconChevronLeft, IconChevronRight } from "./icons";
import { PalDetailPanel, PalInfoButton } from "./PalDetails";

const PARTY_SLOTS = 5;
const BOX_SLOTS = 30;

interface PalPage {
  key: string;
  label: string;
  /** Fixed slot grid; null entries render as empty cells like the in-game box. */
  slots: (PlayerPal | null)[];
  columns: number;
}

/** Lay the player's pals out into the party page, numbered box pages, and a
 * catch-all page for pals the save placed outside party/box (base, expeditions). */
function buildPages(pals: PlayerPal[]): PalPage[] {
  const pages: PalPage[] = [];

  const party = pals.filter((p) => p.inParty).sort((a, b) => (a.partySlot ?? 0) - (b.partySlot ?? 0));
  if (party.length > 0) {
    const slots = Array<PlayerPal | null>(PARTY_SLOTS).fill(null);
    party.forEach((pal, i) => {
      const slot = pal.partySlot ?? i;
      if (slot >= 0 && slot < PARTY_SLOTS) slots[slot] = pal;
    });
    pages.push({ key: "party", label: "Party", slots, columns: PARTY_SLOTS });
  }

  const boxed = pals.filter((p) => !p.inParty && p.boxPage !== null);
  const maxPage = boxed.reduce((max, p) => Math.max(max, p.boxPage ?? 0), -1);
  for (let page = 0; page <= maxPage; page++) {
    const slots = Array<PlayerPal | null>(BOX_SLOTS).fill(null);
    for (const pal of boxed) {
      if (pal.boxPage !== page) continue;
      const slot = pal.boxSlot ?? -1;
      if (slot >= 0 && slot < BOX_SLOTS) slots[slot] = pal;
    }
    pages.push({ key: `box-${page}`, label: `Box ${page + 1}`, slots, columns: 6 });
  }

  const elsewhere = pals.filter((p) => !p.inParty && p.boxPage === null);
  if (elsewhere.length > 0) {
    pages.push({ key: "other", label: "Base & expeditions", slots: elsewhere, columns: 6 });
  }

  return pages;
}

function PalCell({ pal, expanded, onInfo }: { pal: PlayerPal | null; expanded: boolean; onInfo: () => void }) {
  if (!pal) return <div className="pal-cell pal-cell-empty" aria-hidden="true" />;
  const detailsId = `box-pal-details-${pal.instanceId}`;
  return (
    <div className="pal-cell" title={`${pal.displayName} · Lv ${pal.level}`}>
      <span className="pal-cell-info">
        <PalInfoButton pal={pal} expanded={expanded} controls={detailsId} onClick={onInfo} />
      </span>
      <PalIcon characterId={pal.characterId} displayName={pal.displayName} />
      <span className="pal-cell-name">{pal.displayName}</span>
      <span className="pal-cell-meta">
        Lv {pal.level}
        {pal.isAlpha && <span className="pal-tag alpha">α</span>}
        {pal.isLucky && <span className="pal-tag lucky">✦</span>}
      </span>
    </div>
  );
}

export function PalBoxDialog({
  open,
  onClose,
  playerName,
  pals,
}: {
  open: boolean;
  onClose: () => void;
  playerName: string;
  pals: PlayerPal[];
}) {
  const pages = useMemo(() => buildPages(pals), [pals]);
  const [index, setIndex] = useState(0);
  const [expandedPalId, setExpandedPalId] = useState<string | null>(null);
  // Clamp when the roster shrinks between opens (page count can change).
  const current = pages.length === 0 ? null : pages[Math.min(index, pages.length - 1)];
  const safeIndex = Math.min(index, Math.max(0, pages.length - 1));
  const expandedPal = current?.slots.find((pal) => pal?.instanceId === expandedPalId) ?? null;

  useEffect(() => setExpandedPalId(null), [safeIndex, open]);

  return (
    <Dialog open={open} title={`${playerName} · Pals`} onClose={onClose} className="pal-box-dialog">
      {current === null ? (
        <div className="pal-box-empty">No Pals in the latest save.</div>
      ) : (
        <div className="pal-box">
          <div className="pal-box-nav">
            <button
              type="button"
              className="btn btn-ghost btn-sm"
              onClick={() => setIndex((i) => Math.max(0, i - 1))}
              disabled={safeIndex === 0}
              aria-label="Previous box"
            >
              <IconChevronLeft />
            </button>
            <span className="pal-box-title">
              {current.label}
              <span className="pal-box-count">
                {current.slots.filter(Boolean).length} pal{current.slots.filter(Boolean).length === 1 ? "" : "s"}
                {pages.length > 1 && ` · ${safeIndex + 1}/${pages.length}`}
              </span>
            </span>
            <button
              type="button"
              className="btn btn-ghost btn-sm"
              onClick={() => setIndex((i) => Math.min(pages.length - 1, i + 1))}
              disabled={safeIndex >= pages.length - 1}
              aria-label="Next box"
            >
              <IconChevronRight />
            </button>
          </div>
          <div className="pal-grid" style={{ gridTemplateColumns: `repeat(${current.columns}, 1fr)` }}>
            {current.slots.map((pal, i) => (
              <PalCell
                key={pal ? pal.instanceId : `empty-${i}`}
                pal={pal}
                expanded={pal?.instanceId === expandedPalId}
                onInfo={() => setExpandedPalId(pal?.instanceId === expandedPalId ? null : pal?.instanceId ?? null)}
              />
            ))}
          </div>
          {expandedPal && <PalDetailPanel pal={expandedPal} id={`box-pal-details-${expandedPal.instanceId}`} />}
          {pages.length > 1 && (
            <div className="pal-box-tabs">
              {pages.map((page, i) => (
                <button
                  type="button"
                  key={page.key}
                  className={["pal-box-tab", i === safeIndex ? "is-active" : ""].filter(Boolean).join(" ")}
                  onClick={() => setIndex(i)}
                >
                  {page.label}
                </button>
              ))}
            </div>
          )}
        </div>
      )}
    </Dialog>
  );
}
