import { useEffect, useMemo, useState } from "react";
import { Link, useNavigate, useSearchParams } from "react-router";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { api } from "../../api/client";
import type { GrantableItem, Player, PlayerActivity, PlayerActivityWindow, WhitelistEntry } from "../../api/types";
import { useIsAdmin } from "../../app/AuthProvider";
import { usePaletteBridge } from "../../app/paletteBridge";
import { formatDuration, formatRelativeToNow, truncateMiddle } from "../../app/format";
import { worldToGame } from "../../app/mapTransform";
import { guildDisplayName } from "../../app/guildDisplay";
import { Card, CardBody, CardHead } from "../../components/Card";
import { Tabs } from "../../components/Tabs";
import { Pill } from "../../components/Pill";
import { Banner } from "../../components/Banner";
import { EmptyState } from "../../components/EmptyState";
import { Dialog, ConfirmDialog } from "../../components/ConfirmDialog";
import { DropdownMenu, DropdownMenuItem } from "../../components/DropdownMenu";
import { Field, SearchField } from "../../components/Field";
import { useToast } from "../../components/Toast";
import { IconPlayers } from "../../components/icons";
import { PlayerAvatar } from "../../components/PlayerAvatar";
import { PalIcon } from "../../components/PalIcon";
import { PalBoxDialog } from "../../components/PalBoxDialog";
import { PalDetailPanel, PalInfoButton } from "../../components/PalDetails";
import "./Players.css";

type TabKey = "players" | "guilds" | "whitelist" | "bans";
type Filter = "all" | "online" | "offline";
type ActionKind = "kick" | "ban" | "unban";
type PendingAction = { kind: ActionKind; player: Player } | null;

function initials(name: string): string {
  return name.slice(0, 2).toUpperCase();
}

function newIdempotencyKey(): string {
  const bytes = new Uint8Array(16);
  globalThis.crypto.getRandomValues(bytes);
  return `panel-${Array.from(bytes, (value) => value.toString(16).padStart(2, "0")).join("")}`;
}

const knownItemRarities = new Set(["common", "uncommon", "rare", "epic", "legendary"]);

function ItemRarityBadge({ rarity }: { rarity?: string }) {
  const label = rarity?.trim();
  if (!label) return null;
  const normalized = label.toLowerCase();
  const tone = knownItemRarities.has(normalized) ? normalized : "other";
  return <span className={`item-rarity item-rarity-${tone}`}>{label}</span>;
}

function lastSeenLabel(p: Player): string {
  if (p.online) return "now";
  const d = new Date(p.lastSeenAt);
  const sameYear = d.getFullYear() === new Date().getFullYear();
  const dayDiff = Math.floor((Date.now() - d.getTime()) / 86_400_000);
  const hm = d.toLocaleTimeString(undefined, { hour: "2-digit", minute: "2-digit", hour12: false });
  if (dayDiff < 1) return `today ${hm}`;
  if (dayDiff < 2) return `yesterday ${hm}`;
  const md = d.toLocaleDateString(undefined, { month: "short", day: "numeric", year: sameYear ? undefined : "numeric" });
  return `${md}, ${hm}`;
}

function StatusPill({ p }: { p: Player }) {
  if (p.banned) return <Pill tone="danger">Banned</Pill>;
  if (p.online) return <Pill tone="ok">Online</Pill>;
  return <Pill tone="idle">Offline</Pill>;
}

export default function PlayersRoute() {
  const [tab, setTab] = useState<TabKey>("players");
  const [searchParams, setSearchParams] = useSearchParams();
  const selectedUid = searchParams.get("player");
  const setSelectedUid = (uid: string | null) => {
    const next = new URLSearchParams(searchParams);
    if (uid) next.set("player", uid);
    else next.delete("player");
    setSearchParams(next, { replace: true });
  };

  const playersQuery = useQuery({ queryKey: ["players"], queryFn: () => api.players.list(), refetchInterval: 15000 });
  const guildsQuery = useQuery({ queryKey: ["guilds"], queryFn: () => api.guilds.list() });
  const whitelistQuery = useQuery({ queryKey: ["whitelist"], queryFn: () => api.whitelist.get() });
  const healthQuery = useQuery({ queryKey: ["server", "health"], queryFn: () => api.server.health() });

  const players = playersQuery.data ?? [];
  const online = players.filter((p) => p.online).length;
  const bannedCount = players.filter((p) => p.banned).length;
  const effectiveSelected = selectedUid ?? players.find((p) => p.online)?.uid ?? players[0]?.uid ?? null;

  return (
    <main className="content">
      <div className="page-head">
        <h1>Players</h1>
        <span className="sub">
          {playersQuery.data ? `${online} online · ${players.length} known` : "loading…"}
        </span>
      </div>

      <Tabs
        items={[
          { key: "players", label: "Players", count: playersQuery.data ? players.length : undefined },
          { key: "guilds", label: "Guilds", count: guildsQuery.data?.length },
          { key: "whitelist", label: "Player notes", count: whitelistQuery.data?.length },
          { key: "bans", label: "Bans", count: playersQuery.data ? bannedCount : undefined },
        ]}
        active={tab}
        onChange={(k) => setTab(k as TabKey)}
      />

      {tab === "players" && (
        <PlayersTab
          players={players}
          loading={playersQuery.isLoading}
          error={playersQuery.isError}
          selectedUid={effectiveSelected}
          onSelect={setSelectedUid}
          lastSyncAt={healthQuery.data?.save.lastSyncAt}
        />
      )}
      {tab === "guilds" && <GuildsTab />}
      {tab === "whitelist" && <WhitelistTab />}
      {tab === "bans" && <BansTab players={players} error={playersQuery.isError} />}
    </main>
  );
}

// ---------------- players tab ----------------

function PlayersTab({
  players,
  loading,
  error,
  selectedUid,
  onSelect,
  lastSyncAt,
}: {
  players: Player[];
  loading: boolean;
  error: boolean;
  selectedUid: string | null;
  onSelect: (uid: string) => void;
  lastSyncAt?: string;
}) {
  const isAdmin = useIsAdmin();
  const navigate = useNavigate();
  const [search, setSearch] = useState("");
  const [filter, setFilter] = useState<Filter>("all");
  const [action, setAction] = useState<PendingAction>(null);
  const [messaging, setMessaging] = useState<Player | null>(null);

  // The ⌘K command palette can request a kick/ban/unban from any route (see
  // app/paletteBridge.tsx) — it never opens a dialog or mutates itself, it just asks
  // this exact existing ConfirmDialog flow to open once the player list is available.
  const { playerActionRequest, clearPlayerActionRequest } = usePaletteBridge();
  useEffect(() => {
    if (!playerActionRequest) return;
    const player = players.find((p) => p.uid === playerActionRequest.uid);
    if (!player) return; // player list still loading — effect re-runs once it arrives
    setAction({ kind: playerActionRequest.kind, player });
    clearPlayerActionRequest();
  }, [playerActionRequest, players, clearPlayerActionRequest]);

  const filtered = useMemo(() => {
    const q = search.trim().toLowerCase();
    return players.filter((p) => {
      if (filter === "online" && !p.online) return false;
      if (filter === "offline" && p.online) return false;
      if (q && !p.name.toLowerCase().includes(q) && !p.steamId.includes(q)) return false;
      return true;
    });
  }, [players, search, filter]);

  return (
    <div className="players-layout">
      <div className="players-main">
        <div className="toolbar">
          <SearchField
            placeholder="Search name or Steam ID…"
            value={search}
            onChange={(e) => setSearch(e.target.value)}
            aria-label="Search players"
          />
          <select
            className="input"
            style={{ width: "auto" }}
            value={filter}
            onChange={(e) => setFilter(e.target.value as Filter)}
            aria-label="Filter players"
          >
            <option value="all">All players</option>
            <option value="online">Online now</option>
            <option value="offline">Offline</option>
          </select>
          <div className="spacer" />
          {lastSyncAt && <span className="sync-hint">save data synced {formatRelativeToNow(lastSyncAt)}</span>}
        </div>

        <Card>
          {error ? (
            <CardBody>
              <Banner tone="warn">Couldn't load players. The save data may not be parsed yet.</Banner>
            </CardBody>
          ) : loading ? (
            <CardBody>
              <span className="skel skel-text" style={{ width: "100%", height: 120 }} />
            </CardBody>
          ) : filtered.length === 0 ? (
            <CardBody>
              <EmptyState
                icon={<IconPlayers width={40} height={40} />}
                title="No players match"
                description="Try a different search or filter — players appear here once they've joined the server at least once."
              />
            </CardBody>
          ) : (
            <CardBody flush style={{ overflowX: "auto" }}>
              <table className="table">
                <thead>
                  <tr>
                    <th>Player</th>
                    <th>Status</th>
                    <th>Level</th>
                    <th>Guild</th>
                    <th>Ping</th>
                    <th>Last seen</th>
                    <th className="actions"></th>
                  </tr>
                </thead>
                <tbody>
                  {filtered.map((p) => (
                    <tr
                      key={p.uid}
                      className={p.uid === selectedUid ? "row-selected" : undefined}
                      onClick={() => onSelect(p.uid)}
                      style={{ cursor: "pointer" }}
                    >
                      <td>
                        <div className="who-cell">
                          <PlayerAvatar name={p.name} uid={p.uid} />
                          <div>
                            <div className="name">{p.name}</div>
                            <div className="id">steam_{p.steamId}</div>
                          </div>
                        </div>
                      </td>
                      <td>
                        <StatusPill p={p} />
                      </td>
                      <td className="num">{p.level}</td>
                      <td onClick={(event) => event.stopPropagation()}>{p.guildId && p.guildName ? <Link to={`/guilds/${encodeURIComponent(p.guildId)}`}>{p.guildName}</Link> : "—"}</td>
                      <td className="num">{p.ping !== null ? `${p.ping} ms` : "—"}</td>
                      <td className="num">{lastSeenLabel(p)}</td>
                      <td className="actions" onClick={(e) => e.stopPropagation()}>
                        {isAdmin && (
                          <DropdownMenu triggerLabel={`Actions for ${p.name}`}>
                            <DropdownMenuItem onClick={() => setMessaging(p)}>Message…</DropdownMenuItem>
                            <DropdownMenuItem
                              disabled={!p.location}
                              onClick={() => {
                                onSelect(p.uid);
                                if (!p.location) return;
                                const spot = worldToGame(p.location.x, p.location.y);
                                navigate(`/map?x=${spot.x}&y=${spot.y}`);
                              }}
                            >
                              Show on map
                            </DropdownMenuItem>
                            {p.banned ? (
                              <DropdownMenuItem onClick={() => setAction({ kind: "unban", player: p })}>Unban</DropdownMenuItem>
                            ) : (
                              <>
                                <DropdownMenuItem disabled={!p.online} onClick={() => setAction({ kind: "kick", player: p })}>
                                  Kick…
                                </DropdownMenuItem>
                                <DropdownMenuItem danger onClick={() => setAction({ kind: "ban", player: p })}>
                                  Ban…
                                </DropdownMenuItem>
                              </>
                            )}
                          </DropdownMenu>
                        )}
                      </td>
                    </tr>
                  ))}
                </tbody>
              </table>
            </CardBody>
          )}
        </Card>
      </div>

      <PlayerDetailPanel uid={selectedUid} onAction={(kind, player) => setAction({ kind, player })} />

      <PlayerActionDialogs action={action} onClose={() => setAction(null)} />
      <MessageDialog open={messaging !== null} onClose={() => setMessaging(null)} playerName={messaging?.name ?? ""} />
    </div>
  );
}

// ---------------- detail panel ----------------

function PlayerDetailPanel({ uid, onAction }: { uid: string | null; onAction: (kind: ActionKind, player: Player) => void }) {
  const isAdmin = useIsAdmin();
  const navigate = useNavigate();
  const [showAllPals, setShowAllPals] = useState(false);
  const [messageOpen, setMessageOpen] = useState(false);
  const [itemGrantOpen, setItemGrantOpen] = useState(false);
  const [expandedPalId, setExpandedPalId] = useState<string | null>(null);

  useEffect(() => setExpandedPalId(null), [uid]);

  const detailQuery = useQuery({
    queryKey: ["players", uid],
    queryFn: () => api.players.detail(uid!),
    enabled: uid !== null,
    refetchInterval: 60_000,
  });
  const grantsQuery = useQuery({
    queryKey: ["item-grants"],
    queryFn: () => api.items.grants(),
    enabled: isAdmin && uid !== null,
    refetchInterval: 30_000,
  });

  if (!uid) {
    return (
      <aside className="card" aria-label="Player detail">
        <CardBody>
          <EmptyState title="No player selected" description="Select a row to see save-data detail here." />
        </CardBody>
      </aside>
    );
  }

  const d = detailQuery.data;
  const pals = d?.pals ?? [];
  // The active party (Otomo container, up to 5) leads the panel; the full box is
  // one click away in the popup. Fall back to level order if a save carried no
  // party placement (older parses, base-only rosters).
  const partyPals = pals.filter((p) => p.inParty).sort((a, b) => (a.partySlot ?? 0) - (b.partySlot ?? 0));
  const shownPals = partyPals.length > 0 ? partyPals : [...pals].sort((a, b) => b.level - a.level).slice(0, 5);
  const gamePos = d?.location ? worldToGame(d.location.x, d.location.y) : null;

  return (
    <aside className="card" aria-label="Player detail">
      {detailQuery.isError ? (
        <CardBody>
          <Banner tone="warn">Couldn't load player detail.</Banner>
        </CardBody>
      ) : !d ? (
        <CardBody>
          <span className="skel skel-text" style={{ width: "100%", height: 60 }} />
        </CardBody>
      ) : (
        <>
          <div className="detail-head">
            <PlayerAvatar name={d.name} uid={d.uid} />
            <div style={{ minWidth: 0 }}>
              <h2>{d.name}</h2>
              <div className="id">
                steam_{d.steamId} · uid {truncateMiddle(d.uid, 8, 0)}
              </div>
            </div>
            <span style={{ marginLeft: "auto" }}>
              <StatusPill p={d} />
            </span>
          </div>

          <div className="kv">
            <div>
              <span className="label">Level</span>
              <span className="val">{d.level}</span>
            </div>
            <div>
              <span className="label">Guild</span>
              <span className="val">{d.guildId && d.guildName ? <Link to={`/guilds/${encodeURIComponent(d.guildId)}`}>{d.guildName}</Link> : "—"}</span>
            </div>
            <div>
              <span className="label">Position <span className="label-note">last save</span></span>
              <span className="val">{gamePos ? `${gamePos.x}, ${gamePos.y}` : "—"}</span>
            </div>
            <div>
              <span className="label">Ping</span>
              <span className="val">{d.ping !== null ? `${d.ping} ms` : "—"}</span>
            </div>
            <div>
              <span className="label">First seen</span>
              <span className="val">
                {new Date(d.firstSeenAt).toLocaleDateString(undefined, { month: "short", day: "numeric", year: "numeric" })}
              </span>
            </div>
            <div>
              <span className="label">Total tracked</span>
              <span className="val">{formatDuration(d.playtimeSec)}</span>
            </div>
          </div>

          <PlayerActivitySummary activity={d.activity} />

          <div className="card-head" style={{ borderTop: "1px solid var(--line)" }}>
            <h2>Pals</h2>
            <span className="hint">
              {partyPals.length > 0 ? `party of ${partyPals.length} · ${pals.length} owned` : `${pals.length} owned`}
            </span>
          </div>
          <div>
            {pals.length === 0 && (
              <div className="pal-row" style={{ color: "var(--ink-3)" }}>
                No Pals in the latest save.
              </div>
            )}
            {shownPals.map((pal) => {
              const detailsId = `player-pal-details-${pal.instanceId}`;
              const expanded = expandedPalId === pal.instanceId;
              return (
              <div key={pal.instanceId} className="pal-entry">
                <div className="pal-row">
                  <PalIcon characterId={pal.characterId} displayName={pal.displayName} /> {pal.displayName}
                  {pal.isAlpha && <span className="pal-tag alpha">α</span>}
                  {pal.isLucky && <span className="pal-tag lucky">✦</span>}
                  <span className="lvl">Lv {pal.level}</span>
                  <PalInfoButton
                    pal={pal}
                    expanded={expanded}
                    controls={detailsId}
                    onClick={() => setExpandedPalId(expanded ? null : pal.instanceId)}
                  />
                </div>
                {expanded && <PalDetailPanel pal={pal} id={detailsId} />}
              </div>
              );
            })}
            {pals.length > shownPals.length && (
              <button type="button" className="pal-more" onClick={() => setShowAllPals(true)}>
                show all {pals.length}
              </button>
            )}
          </div>
          <PalBoxDialog open={showAllPals} onClose={() => setShowAllPals(false)} playerName={d.name} pals={pals} />


          {isAdmin && (
            <>
              <div className="card-head" style={{ borderTop: "1px solid var(--line)" }}>
                <h2>Actions</h2>
              </div>
              <CardBody style={{ display: "flex", gap: "var(--space-2)", flexWrap: "wrap" }}>
                <button type="button" className="btn btn-sm" onClick={() => setMessageOpen(true)}>
                  Message
                </button>
                <button type="button" className="btn btn-sm" disabled={!d.online} onClick={() => setItemGrantOpen(true)}>
                  Give item…
                </button>
                <button
                  type="button"
                  className="btn btn-sm"
                  disabled={!gamePos}
                  onClick={() => gamePos && navigate(`/map?x=${gamePos.x}&y=${gamePos.y}`)}
                >
                  Show on map
                </button>
                {d.banned ? (
                  <button type="button" className="btn btn-sm" onClick={() => onAction("unban", d)}>
                    Unban
                  </button>
                ) : (
                  <>
                    <button type="button" className="btn btn-sm btn-ghost" disabled={!d.online} onClick={() => onAction("kick", d)}>
                      Kick…
                    </button>
                    <button type="button" className="btn btn-sm btn-danger" onClick={() => onAction("ban", d)}>
                      Ban…
                    </button>
                  </>
                )}
              </CardBody>
              {(grantsQuery.data ?? []).filter((grant) => grant.playerUid === d.uid).slice(0, 5).length > 0 && (
                <div className="item-grant-history" aria-label={`Recent item grants for ${d.name}`}>
                  {(grantsQuery.data ?? []).filter((grant) => grant.playerUid === d.uid).slice(0, 5).map((grant) => (
                    <div key={grant.requestId}>
                      <span><strong>{grant.quantity} × {grant.itemName}</strong><small>{grant.reason}</small></span>
                      <Pill tone={grant.status === "succeeded" ? "ok" : grant.status === "failed" ? "danger" : "idle"}>{grant.status}</Pill>
                    </div>
                  ))}
                </div>
              )}
            </>
          )}

          <MessageDialog open={messageOpen} onClose={() => setMessageOpen(false)} playerName={d.name} />
          <ItemGrantDialog open={itemGrantOpen} onClose={() => setItemGrantOpen(false)} player={d} />
        </>
      )}
    </aside>
  );
}

function ItemGrantDialog({ open, onClose, player }: { open: boolean; onClose: () => void; player: Player }) {
  const [search, setSearch] = useState("");
  const [selected, setSelected] = useState<GrantableItem | null>(null);
  const [quantity, setQuantity] = useState(1);
  const [reason, setReason] = useState("");
  const [reviewing, setReviewing] = useState(false);
  const [idempotencyKey, setIdempotencyKey] = useState("");
  const toast = useToast();
  const queryClient = useQueryClient();

  const capability = useQuery({
    queryKey: ["items", "capability"],
    queryFn: () => api.items.capability(),
    enabled: open,
    refetchInterval: open ? 15_000 : false,
  });
  const catalog = useQuery({
    queryKey: ["items", "search", search],
    queryFn: () => api.items.search(search, 50),
    enabled: open && capability.data?.ready === true,
    staleTime: 30_000,
  });
  const mutation = useMutation({
    mutationFn: () => {
      if (!selected) throw new Error("No item selected");
      if (!idempotencyKey) throw new Error("Grant confirmation expired; review it again.");
      return api.items.grant(player.uid, selected.id, quantity, reason.trim(), idempotencyKey);
    },
    onSuccess: (grant) => {
      toast.push(`${grant.quantity} × ${grant.itemName} queued for ${grant.playerName}.`, "ok");
      void queryClient.invalidateQueries({ queryKey: ["item-grants"] });
      void (async () => {
        for (let attempt = 0; attempt < 15; attempt += 1) {
          await new Promise((resolve) => window.setTimeout(resolve, 2_000));
          try {
            const status = await api.items.grantStatus(grant.requestId);
            if (status.status === "succeeded") {
              void queryClient.invalidateQueries({ queryKey: ["item-grants"] });
              toast.push(`${status.grantedQuantity ?? status.quantity} × ${status.itemName} delivered to ${status.playerName}.`, "ok");
              return;
            }
            if (status.status === "failed") {
              void queryClient.invalidateQueries({ queryKey: ["item-grants"] });
              toast.push(status.errorMessage || `Item grant for ${status.playerName} failed.`, "danger");
              return;
            }
          } catch {
            // The durable audit record remains authoritative; a transient status
            // read must never cause a second mutation submission.
          }
        }
      })();
      setSelected(null);
      setQuantity(1);
      setReason("");
      setReviewing(false);
      setIdempotencyKey("");
      onClose();
    },
    onError: (error) => toast.push(error instanceof Error ? error.message : "Item grant failed.", "danger"),
  });

  useEffect(() => {
    if (!open) {
      setReviewing(false);
      setIdempotencyKey("");
    }
  }, [open]);

  const canReview = selected !== null && quantity >= 1 && quantity <= selected.maxQuantity && reason.trim().length >= 3;
  const close = () => {
    if (!mutation.isPending) onClose();
  };

  return (
    <Dialog
      open={open}
      title={reviewing ? "Confirm item grant" : `Give item to ${player.name}`}
      onClose={close}
      className="item-grant-dialog"
      footer={
        <>
          <button type="button" className="btn btn-ghost" onClick={reviewing ? () => { setReviewing(false); setIdempotencyKey(""); } : close} disabled={mutation.isPending}>
            {reviewing ? "Back" : "Cancel"}
          </button>
          {reviewing ? (
            <button type="button" className="btn btn-primary" onClick={() => mutation.mutate()} disabled={mutation.isPending || capability.data?.ready !== true}>
              {mutation.isPending ? "Queuing…" : "Confirm grant"}
            </button>
          ) : (
            <button type="button" className="btn btn-primary" onClick={() => { setIdempotencyKey(newIdempotencyKey()); setReviewing(true); }} disabled={!canReview || capability.data?.ready !== true}>
              Review grant
            </button>
          )}
        </>
      }
    >
      {capability.isLoading ? (
        <p className="item-grant-note">Checking the server bridge…</p>
      ) : capability.data?.ready !== true ? (
        <Banner tone="warn">{capability.data?.reason ?? "The item grant bridge is unavailable."}</Banner>
      ) : reviewing && selected ? (
        <div className="item-grant-review">
          <img src={api.items.iconUrl(selected.id)} alt="" />
          <div>
            <div className="item-grant-review-title">
              <strong>{quantity} × {selected.name}</strong>
              <ItemRarityBadge rarity={selected.rarity} />
            </div>
            <span>{selected.category || "Item"} · <code>{selected.id}</code></span>
            <span>Recipient: {player.name}</span>
            <span>Reason: {reason.trim()}</span>
          </div>
          <Banner tone="warn">This changes a live player inventory. Palhelm will submit it once and keep an audit record.</Banner>
        </div>
      ) : (
        <div className="item-grant-form">
          <p className="item-grant-note">
            Online players only · catalogue {capability.data.catalogVersion} · {capability.data.catalogItems.toLocaleString()} validated items
          </p>
          <SearchField value={search} onChange={(event) => setSearch(event.target.value)} placeholder="Search item name, ID, or category…" autoFocus />
          <div className="item-grant-results" role="listbox" aria-label="Grantable items">
            {catalog.isLoading && <span className="item-grant-note">Searching catalogue…</span>}
            {catalog.data?.items.map((item) => (
              <button
                type="button"
                key={item.id}
                role="option"
                aria-selected={selected?.id === item.id}
                className={selected?.id === item.id ? "item-grant-item is-selected" : "item-grant-item"}
                onClick={() => { setSelected(item); setQuantity(Math.min(quantity, item.maxQuantity)); }}
              >
                <img src={api.items.iconUrl(item.id)} alt="" onError={(event) => { event.currentTarget.style.visibility = "hidden"; }} />
                <span className="item-grant-summary">
                  <strong>{item.name}</strong>
                  <span className="item-grant-meta">
                    <small>{item.category || "Item"}</small>
                    <ItemRarityBadge rarity={item.rarity} />
                  </span>
                </span>
                <code>{item.id}</code>
              </button>
            ))}
            {catalog.data && catalog.data.items.length === 0 && <span className="item-grant-note">No grantable items match that search.</span>}
          </div>
          {selected && (
            <div className="item-grant-fields">
              <Field label="Quantity" type="number" min={1} max={selected.maxQuantity} value={quantity} onChange={(event) => setQuantity(Number(event.target.value))} hint={<small>Maximum {selected.maxQuantity.toLocaleString()}</small>} />
              <Field label="Reason" value={reason} maxLength={200} onChange={(event) => setReason(event.target.value)} placeholder="Why is this being granted?" />
            </div>
          )}
        </div>
      )}
    </Dialog>
  );
}

function PlayerActivitySummary({ activity }: { activity: PlayerActivity }) {
  return (
    <section className="player-activity" aria-label="Observed player activity">
      <div className="card-head">
        <h2>Observed activity</h2>
        <span className="hint">panel tracking only</span>
      </div>
      {activity.trackingSince === null ? (
        <div className="player-activity-empty">No sessions observed yet.</div>
      ) : (
        <>
          <div className="player-activity-current">
            <span>Current session</span>
            <strong>{activity.currentSession ? formatDuration(activity.currentSession.durationSec) : "Offline"}</strong>
          </div>
          <div className="player-activity-windows">
            <ActivityWindow label="24 hours" value={activity.windows.last24Hours} />
            <ActivityWindow label="7 days" value={activity.windows.last7Days} />
            <ActivityWindow label="30 days" value={activity.windows.last30Days} />
          </div>
          <p className="player-activity-coverage">
            Tracked since {new Date(activity.trackingSince).toLocaleString()}.
            {activity.recentSessionsTruncated ? " Showing the 20 most recent sessions." : ""}
          </p>
        </>
      )}
    </section>
  );
}

function ActivityWindow({ label, value }: { label: string; value: PlayerActivityWindow }) {
  return (
    <div>
      <span>{label}</span>
      <strong>{formatDuration(value.durationSec)}</strong>
      <small>{value.sessionCount} {value.sessionCount === 1 ? "session" : "sessions"}</small>
    </div>
  );
}

function MessageDialog({ open, onClose, playerName }: { open: boolean; onClose: () => void; playerName: string }) {
  const [message, setMessage] = useState("");
  const [busy, setBusy] = useState(false);
  const toast = useToast();

  async function send() {
    if (!message.trim()) return;
    setBusy(true);
    try {
      await api.server.announce(`@${playerName}: ${message.trim()}`);
      toast.push("Broadcast sent.", "ok");
      setMessage("");
      onClose();
    } catch {
      toast.push("Broadcast failed. Try again.", "danger");
    } finally {
      setBusy(false);
    }
  }

  return (
    <Dialog
      open={open}
      title={`Message ${playerName}`}
      onClose={onClose}
      footer={
        <>
          <button type="button" className="btn btn-ghost" onClick={onClose}>
            Cancel
          </button>
          <button type="button" className="btn btn-primary" onClick={send} disabled={busy || !message.trim()}>
            {busy ? "Sending…" : "Send broadcast"}
          </button>
        </>
      }
    >
      <p style={{ color: "var(--ink-2)", fontSize: "var(--text-sm)" }}>
        The vanilla server has no private messages — this broadcasts to all players, prefixed with @{playerName}.
      </p>
      <Field label="Message" value={message} onChange={(e) => setMessage(e.target.value)} autoFocus />
    </Dialog>
  );
}

// ---------------- kick / ban / unban dialogs ----------------

function PlayerActionDialogs({ action, onClose }: { action: PendingAction; onClose: () => void }) {
  const queryClient = useQueryClient();
  const toast = useToast();
  const [message, setMessage] = useState("");

  const mutation = useMutation({
    mutationFn: async (a: NonNullable<PendingAction>) => {
      const msg = message.trim() || undefined;
      if (a.kind === "kick") await api.players.kick(a.player.uid, msg);
      else if (a.kind === "ban") await api.players.ban(a.player.uid, msg);
      else await api.players.unban(a.player.uid);
      return a;
    },
    onSuccess: (a) => {
      const verb = a.kind === "kick" ? "kicked" : a.kind === "ban" ? "banned" : "unbanned";
      toast.push(`${a.player.name} ${verb}.`, "ok");
      queryClient.invalidateQueries({ queryKey: ["players"] });
      queryClient.invalidateQueries({ queryKey: ["events"] });
      setMessage("");
      onClose();
    },
    onError: () => {
      toast.push("Action failed. Check the server connection and try again.", "danger");
    },
  });

  const kind = action?.kind;
  const player = action?.player;
  const title = !action
    ? ""
    : kind === "kick"
      ? `Kick ${player!.name}…`
      : kind === "ban"
        ? `Ban ${player!.name}…`
        : `Unban ${player!.name}`;
  const confirmLabel = kind === "kick" ? "Kick player" : kind === "ban" ? "Ban player" : "Unban player";

  return (
    <ConfirmDialog
      open={action !== null}
      title={title}
      onClose={() => {
        setMessage("");
        onClose();
      }}
      onConfirm={() => action && mutation.mutate(action)}
      confirmLabel={confirmLabel}
      danger={kind !== "unban"}
      busy={mutation.isPending}
    >
      {kind === "kick" && player && (
        <>
          <p style={{ color: "var(--ink-2)", fontSize: "var(--text-sm)" }}>
            Disconnects {player.name} from the server. They can rejoin immediately.
          </p>
          <Field label="Message shown to the player (optional)" value={message} onChange={(e) => setMessage(e.target.value)} autoFocus />
        </>
      )}
      {kind === "ban" && player && (
        <>
          <p style={{ color: "var(--ink-2)", fontSize: "var(--text-sm)" }}>
            Bans steam_{player.steamId} from the server until unbanned. If online, they are disconnected now.
          </p>
          <Field label="Message shown to the player (optional)" value={message} onChange={(e) => setMessage(e.target.value)} autoFocus />
        </>
      )}
      {kind === "unban" && player && (
        <p style={{ color: "var(--ink-2)", fontSize: "var(--text-sm)" }}>
          Removes the ban on steam_{player.steamId}. They can rejoin the server.
        </p>
      )}
    </ConfirmDialog>
  );
}

// ---------------- guilds tab ----------------

function GuildsTab() {
  const guildsQuery = useQuery({ queryKey: ["guilds"], queryFn: () => api.guilds.list() });

  return (
    <Card>
      {guildsQuery.isError ? (
        <CardBody>
          <Banner tone="warn">Couldn't load guilds from save data.</Banner>
        </CardBody>
      ) : guildsQuery.isLoading ? (
        <CardBody>
          <span className="skel skel-text" style={{ width: "100%", height: 120 }} />
        </CardBody>
      ) : (
        <CardBody flush style={{ overflowX: "auto" }}>
          <table className="table">
            <thead>
              <tr>
                <th>Guild</th>
                <th>Members</th>
                <th>Bases</th>
                <th>Roster</th>
              </tr>
            </thead>
            <tbody>
              {(guildsQuery.data ?? []).map((g) => (
                <tr key={g.id}>
                  <td>
                    <div className="who-cell">
                      <span className="avatar">{initials(guildDisplayName(g))}</span>
                      <div>
                        <div className="name"><Link to={`/guilds/${encodeURIComponent(g.id)}`}>{guildDisplayName(g)}</Link></div>
                        <div className="id">{g.id}</div>
                      </div>
                    </div>
                  </td>
                  <td className="num">{g.memberCount}</td>
                  <td className="num">{g.bases.length}</td>
                  <td style={{ color: "var(--ink-2)" }}>
                    {g.members.length > 0 ? (
                      g.members.map((m) => m.name).join(", ")
                    ) : (
                      <span style={{ color: "var(--ink-3)" }}>no known players</span>
                    )}
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
        </CardBody>
      )}
    </Card>
  );
}

// ---------------- whitelist tab ----------------

function WhitelistTab() {
  const isAdmin = useIsAdmin();
  const queryClient = useQueryClient();
  const toast = useToast();
  const whitelistQuery = useQuery({ queryKey: ["whitelist"], queryFn: () => api.whitelist.get() });
  const [draft, setDraft] = useState<WhitelistEntry[] | null>(null);

  const entries = draft ?? whitelistQuery.data ?? [];
  const dirty = draft !== null;

  const saveMutation = useMutation({
    mutationFn: (next: WhitelistEntry[]) => api.whitelist.put(next.filter((e) => e.steamId.trim() !== "")),
    onSuccess: (saved) => {
      queryClient.setQueryData(["whitelist"], saved);
      queryClient.invalidateQueries({ queryKey: ["players"] });
      setDraft(null);
      toast.push("Player notes saved.", "ok");
    },
    onError: () => toast.push("Couldn't save player notes. Try again.", "danger"),
  });

  function update(i: number, patch: Partial<WhitelistEntry>) {
    setDraft(entries.map((e, idx) => (idx === i ? { ...e, ...patch } : e)));
  }

  return (
    <Card>
      <CardHead title="Player notes" hint="local labels only — does not control who can join" />
      {whitelistQuery.isError ? (
        <CardBody>
          <Banner tone="warn">Couldn't load player notes.</Banner>
        </CardBody>
      ) : whitelistQuery.isLoading ? (
        <CardBody>
          <span className="skel skel-text" style={{ width: "100%", height: 80 }} />
        </CardBody>
      ) : (
        <CardBody flush>
          {entries.length === 0 && (
            <EmptyState
              title="No player notes"
              description={
                isAdmin
                  ? "Add Steam IDs to label players in Palhelm. These notes are not enforced by Palworld."
                  : "No local player annotations yet."
              }
            />
          )}
          {entries.map((e, i) => (
            <div className="whitelist-row" key={i}>
              <input
                className="input input-mono"
                value={e.steamId}
                placeholder="7656119…"
                aria-label="Steam ID"
                readOnly={!isAdmin}
                onChange={(ev) => update(i, { steamId: ev.target.value })}
              />
              <input
                className="input"
                value={e.name ?? ""}
                placeholder="name (optional)"
                aria-label="Player name"
                readOnly={!isAdmin}
                onChange={(ev) => update(i, { name: ev.target.value })}
              />
              {isAdmin && (
                <button
                  type="button"
                  className="btn btn-sm btn-ghost"
                  aria-label={`Remove ${e.name || e.steamId}`}
                  onClick={() => setDraft(entries.filter((_, idx) => idx !== i))}
                >
                  Remove
                </button>
              )}
            </div>
          ))}
          {isAdmin && (
            <div className="whitelist-foot">
              <button type="button" className="btn btn-sm" onClick={() => setDraft([...entries, { steamId: "", name: "" }])}>
                + Add entry
              </button>
              <div style={{ flex: 1 }} />
              {dirty && (
                <>
                  <button type="button" className="btn btn-ghost btn-sm" onClick={() => setDraft(null)}>
                    Discard
                  </button>
                  <button
                    type="button"
                    className="btn btn-primary btn-sm"
                    disabled={saveMutation.isPending}
                    onClick={() => saveMutation.mutate(entries)}
                  >
                    {saveMutation.isPending ? "Saving…" : "Save player notes"}
                  </button>
                </>
              )}
            </div>
          )}
        </CardBody>
      )}
    </Card>
  );
}

// ---------------- bans tab ----------------

function BansTab({ players, error }: { players: Player[]; error: boolean }) {
  const isAdmin = useIsAdmin();
  const [action, setAction] = useState<PendingAction>(null);
  const banned = players.filter((p) => p.banned);

  return (
    <Card>
      {error ? (
        <CardBody>
          <Banner tone="warn">Couldn't load players.</Banner>
        </CardBody>
      ) : banned.length === 0 ? (
        <CardBody>
          <EmptyState
            icon={<IconPlayers width={40} height={40} />}
            title="No banned players"
            description="Players you ban appear here so the ban can be reviewed or lifted."
          />
        </CardBody>
      ) : (
        <CardBody flush style={{ overflowX: "auto" }}>
          <table className="table">
            <thead>
              <tr>
                <th>Player</th>
                <th>Level</th>
                <th>Last seen</th>
                <th className="actions"></th>
              </tr>
            </thead>
            <tbody>
              {banned.map((p) => (
                <tr key={p.uid}>
                  <td>
                    <div className="who-cell">
                      <PlayerAvatar name={p.name} uid={p.uid} />
                      <div>
                        <div className="name">{p.name}</div>
                        <div className="id">steam_{p.steamId}</div>
                      </div>
                    </div>
                  </td>
                  <td className="num">{p.level}</td>
                  <td className="num">{lastSeenLabel(p)}</td>
                  <td className="actions">
                    {isAdmin && (
                      <button type="button" className="btn btn-sm" onClick={() => setAction({ kind: "unban", player: p })}>
                        Unban
                      </button>
                    )}
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
        </CardBody>
      )}
      <PlayerActionDialogs action={action} onClose={() => setAction(null)} />
    </Card>
  );
}
