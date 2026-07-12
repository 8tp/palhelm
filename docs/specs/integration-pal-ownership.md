# Integration API proposal: resolvable Pal ownership

Status: ownership reconciliation is deployed. A backward-compatible derived
base-worker placement contract is implemented locally pending rollout.

## Problem

`GET /api/integration/v1/pals` currently promises `ownerUid` as the join key to
`/players[].uid`, with `ownerName` joined from the player row. Palworld 1.0 saves
can produce Pal rows whose owner identifier does not join to any public player and
whose `ownerName` is empty. The bot cannot truthfully identify those owners, and
per-player detail lookups cannot recover the same Pal instances.

Live-save tracing confirmed one important case: Palworld clears
`OwnerPlayerUId` when a Pal is deployed into a guild-base worker container. That
container identifies the guild base, not the player who contributed the Pal.
`OldOwnerPlayerUIds` is capture/transfer history and can name a different player
than the current contributor, so it must not be promoted to current ownership.

The prepared store reconciliation therefore:

- treats a unique personal party/Palbox container as authoritative current
  ownership;
- carries the last explicitly or personally observed owner for the same stable
  Pal instance while it remains in a base/cage container;
- lets a later personal-container observation supersede the carried owner; and
- leaves never-observed, ambiguous, zero, or non-player containers unresolved.

This cannot retroactively recover an owner for a Pal that was already deployed
before the fixed panel observed it in a personal container. Moving that Pal into
the correct player's party/Palbox for one successful parse establishes the
observation for subsequent base deployments.

Base membership is a separate, exact relationship. The parser decodes the base
WorkerDirector's worker-container GUID, joins it internally to each Pal's slot
container, and exposes only `placement: "base"` plus the safe `baseId` join key.
The raw container GUID never crosses the Integration API boundary. Base
membership does not imply a contributing player: bases belong to guilds, and
`ownerSource` must still be consulted independently.

The bot must render these as `Owner unavailable`; it must not guess based on guild,
container position, or Discord identity.

## Compatible extension

Add the following fields to each bulk Pal row:

```json
{
  "ownerUid": "...",
  "ownerName": "...",
  "ownerSource": "personal_container",
  "ownerResolved": true
}
```

- `ownerSource` is one of `save`, `personal_container`, `last_observed`, or
  `unresolved`.
- `save` means the current save's `OwnerPlayerUId` joined a public player.
- `personal_container` means the Pal's current container uniquely matched that
  player's party or Palbox; this supersedes a stale raw owner after a transfer.
- `last_observed` means the same stable Pal instance was most recently seen with
  that player, but is currently outside a personal container. Clients should say
  “last observed owner,” not imply current possession.
- `unresolved` means the panel did not make a player attribution; `ownerUid` and
  `ownerName` are empty.
- `ownerResolved` is true when `ownerUid` joins a current public `/players` row.
  It deliberately does not erase the certainty distinction in `ownerSource`.

This keeps identity reconciliation at the public API boundary, where every
standalone bot benefits, rather than adding save-format heuristics to one bot.
