# Palhelm Server-Mod Integrations Plan

Status: item-grant control plane implemented; native-Linux loader/transport validated, inventory mutation still gated
Last reviewed: 2026-07-18

This document plans optional server-side mutation features for Palhelm. Nothing in
this plan authorizes installing a mod, changing the game-server runtime, editing a
live save, or restarting the Palworld server. Every provider described here is
disabled by default and requires an isolated compatibility test, a current backup,
an empty maintenance window, and explicit operator approval before a production
game-server restart.

### Implementation checkpoint (2026-07-18)

The repository now contains the item-grant portion of this design:

- disabled-by-default config and a fail-closed capability endpoint;
- a strict, versioned operator catalogue plus complete-icon importer;
- an independently generated bridge allowlist with per-item quantity ceilings;
- same-origin item icon serving and an admin player-detail grant dialog;
- SQLite migration 013 with actor, target, reason, idempotency, status, and result;
- a one-at-a-time atomic local spool that never retries an uncertain mutation; and
- `mods/palhelm-item-bridge`, a vanilla-client UE4SS server bridge source package.

The bridge is deliberately shipped with `ProductionValidated=false`. BES Pals runs
the native Linux `PalServer-Linux-Shipping` binary, which cannot load the officially
supported Windows server mod format. On 2026-07-18, after a verified zero-player
backup, the server successfully loaded the checksum-pinned experimental native-Linux
UE4SS build and the path-neutral Palhelm Lua bridge on Palworld `v1.0.1.100619`.
REST, game-data API, world identity, and the bridge heartbeat remained healthy. A
synthetic spool request was rejected with `validation_required`, proving the atomic
request/result transport without reaching player lookup or inventory mutation.

This does **not** validate item grants. The remaining gate requires a consenting
online test player on the exact build, one bounded low-value grant, inventory and
UID verification, save/reconnect persistence, duplicate-request rejection, and a
post-test backup. Until those checks pass, the live capability must remain
`validation_required` and Palhelm must not expose the grant action as ready.

The optional PalMods one-time converter was validated against the 2026-07-18
rendered reference (2,466 records; 2,455 rows with game art). It is not a Palhelm
runtime dependency. Eleven source rows had no game icon and remain non-grantable;
the UI never substitutes another item's art. Operator-imported assets stay in the
data volume and must not be committed or included in a Palhelm release image.

## Constraints

- Keep Palhelm's `/api/integration/v1` API permanently read-only.
- Do not let the AI assistant, viewer sessions, or ordinary integration keys invoke
  mutations.
- Use stable player and Pal instance IDs. Display names are presentation only.
- Never edit a live Palworld save to implement an online action.
- Never expose arbitrary RCON, Lua, shell, item IDs, or Pal templates through a
  typed Palhelm action.
- Keep clients vanilla. Server features may only use Palworld species, items,
  models, and replicated state already present in the game.
- A missing, stale, or incompatible mod must disable the action clearly rather
  than falling back to an unsafe mechanism.

Pocketpair's current server-mod instructions require a game-server restart and say
that officially supported server-side mod loading is limited to the Windows
dedicated-server edition. A package must explicitly contain a server install rule.
Community Linux/Proton/UE4SS claims therefore need separate verification and do not
inherit official support:

- <https://docs.palworldgame.com/settings-and-operation/mod/>
- <https://github.com/pocketpairjp/PalworldModUploader/blob/main/PalworldModUploader/docs/en/01-General.md>

## Shared architecture

```text
Panel typed action ---------+
                            +--> Palhelm authenticated admin API
Discord admin slash command +            |
                                         +--> validation + idempotency
                                         +--> durable audit record
                                         +--> optional mutation provider
                                                   |
                                      structured RCON or local spool
                                                   |
                                         server-side Palworld mod
                                                   |
                                         authoritative game state
```

Palhelm should expose a provider interface rather than depending directly on one
third-party mod:

```go
type MutationProvider interface {
    Capabilities(ctx context.Context) Capabilities
    GrantItem(ctx context.Context, request GrantItemRequest) GrantItemResult
    WonderTrade(ctx context.Context, request WonderTradeRequest) WonderTradeResult
}
```

The default provider reports `disabled`. Diagnostics should distinguish disabled,
ready, incompatible, unreachable, degraded, and version-mismatched states.

Preferred transport is a short, structured RCON command. If safe custom RCON
registration is not practical, the fallback is an explicitly mounted, local-only
request/result spool using atomic file renames. Do not add a network listener to
the game process merely for these features.

Every mutation carries a unique request ID. The provider must retain enough recent
completed IDs to return the original result instead of performing the action twice.
Palhelm must never blindly retry a timed-out mutation whose outcome is unknown.

## Item grants

### Goal

Allow an administrator to grant a bounded quantity of a known-safe item to one
online player from the Palhelm player view or an admin-only Discord command.

The current Admin Commands mod implements in-game chat commands such as
`!give playername item:amount`, but does not document an RCON or server-console
contract. It targets a name rather than a stable UID and is All Rights Reserved:

- <https://www.curseforge.com/palworld/lua-code-mods/admin-commands>

### Preferred provider order

1. Ask the Admin Commands author for a stable UID-based, machine-readable RCON
   contract and permission boundaries.
2. If that is unavailable, implement a minimal Palhelm-owned server bridge.
3. Do not patch and redistribute the third-party mod without permission.
4. Do not impersonate chat, automate a player client, or edit the save.

Suggested bridge command:

```text
PalhelmGive <request-id> <player-uid> <item-id> <quantity>
```

Suggested result envelope:

```json
{"v":1,"requestId":"...","status":"delivered","accepted":10}
```

### Palhelm contract

```http
GET  /api/v1/items?query=
POST /api/v1/players/{uid}/item-grants
GET  /api/v1/item-grants/{requestId}
GET  /api/v1/item-grants
```

The request contains an allowlisted item ID, bounded quantity, reason,
idempotency key, source, and external actor metadata. The durable record contains
the authenticated panel principal as well as the claimed Discord actor when the
trusted bot is the caller.

Initial delivery is online-only. Later, Palhelm may maintain a cancellable pending
delivery queue keyed by player UID. The bridge must validate inventory capacity and
return the accepted quantity. It must not drop overflow into the world. Palhelm
should rely on the normal world-save interval instead of forcing a save after every
grant.

### Safety policy

- Versioned item catalog with display names and internal asset IDs.
- Reject debug, placeholder, quest, key, paid-content, story, and unsafe internal
  items.
- Per-item quantity caps, per-request caps, cooldowns, and a required reason.
- Admin confirmation and an audit entry for success, failure, and uncertain
  outcomes.
- No free-form command or asset input in the panel or Discord.

### User interfaces

- Panel: **Give item...** on the existing player detail/action surface, with item
  autocomplete, quantity, reason, confirmation, capability state, and recent
  results.
- Discord: `/grant-item player:<player> item:<item> quantity:<n> reason:<text>`,
  centrally gated by the configured admin role and answered ephemerally.

## Wonder Trade

### Feasibility verdict

Feasible in principle as a server-only mod with a Discord slash-command interface,
provided all traded Pals use vanilla game assets and the recipient is online.
Players should not need to install anything: Discord supplies the selection and
confirmation UI, while the server mod performs and replicates the authoritative
Pal-container mutation.

This is **large and high-risk compared with item grants**. The unknown that must be
proven in an isolated server is not random selection or persistence; it is safely
moving one exact, fully populated Pal instance out of a live personal party/Palbox
and replacing it atomically without duplication, loss, stale-container writes, or
save corruption.

Technical precedent exists but is not proof of 1.0 production compatibility:

- UE4SS can hook loaded Unreal `UFunction` calls:
  <https://docs.ue4ss.com/dev/lua-api/global-functions/registerhook.html>
- Current server-oriented Lua mods manipulate authoritative party/summon state:
  <https://www.curseforge.com/palworld/lua-code-mods/multi-pals>
- PalDefender historically exposed RCON `givepal` and Pal-template operations,
  showing that server-authoritative Pal creation and RCON control are possible.
  Its currently indexed release is not a Palworld 1.0 dependency candidate:
  <https://ultimeit.github.io/PalDefender/Commands/>

No current, maintained Palworld 1.0 Wonder Trade implementation was found in the
reviewed CurseForge, Nexus, Workshop, or general web results. This should be treated
as a new mod, not an integration expected to exist already.

### Player experience

The first release should use three Discord subcommands:

```text
/wondertrade trade pal:<linked player's party/box Pal>
/wondertrade pool
/wondertrade history
```

`trade` requires the existing Discord-to-Palworld profile link. Autocomplete shows
only that player's current party and box Pals, with a maximum of 25 Discord choices
per response and search to narrow the list. The visible choice contains species,
nickname when safe, level, sex, Alpha/Lucky markers, and placement; its value is an
opaque short-lived bot token, not a raw Pal instance ID.

The bot replies ephemerally with a confirmation card:

```text
Wonder Trade

Offer: Lv. 31 Anubis · Male · Box 4, slot 12
You will receive one random eligible Pal from the server pool.
This exchange is permanent and has a 12-hour cooldown.

[Confirm trade] [Cancel]
```

After confirmation:

```text
Wonder Trade complete!

You sent: Lv. 31 Anubis
You received: Lv. 28 Petallia ✨ Lucky
Next trade: in 12 hours
Pool: 100 Pals
```

Public announcements should be configurable and should normally announce only
Lucky, Alpha, or high-rarity results. Ordinary trades remain private to avoid
channel spam.

### Eligibility rules

Initial implementation is online-only. The selected Pal must be revalidated by the
mod at execution time:

- The player UID is connected and matches the linked Discord profile.
- The Pal instance still belongs to that player.
- It is still in the party or personal Palbox position selected.
- It is not deployed as a base worker, breeding, viewing-cage, expedition, or other
  facility Pal.
- It is not currently summoned, mounted, incapacitated in a transient state, or
  otherwise held by a live gameplay action.
- Its species and saved fields can be represented by the bridge's current schema.
- It is not a human/NPC, tower/raid-only actor, invalid boss form, placeholder, or
  blocked story/event specimen.

For the first prototype, box-only trades are safer. Party support should be enabled
only after the same atomic swap and disconnect-race tests pass. A party Pal may need
to be recalled before trading, and the active party slot should never be mutated
while summoned.

### Authoritative exchange algorithm

The mod owns the live transaction. Palhelm never constructs or edits a Pal save
record.

1. Accept a request ID, player UID, and exact offered Pal instance ID.
2. Reject a duplicate request by returning its stored result.
3. Revalidate ownership, placement, eligibility, cooldown, and schema version.
4. Select and reserve an eligible pool entry **before** adding the offered Pal to
   the pool, so a player cannot immediately draw the Pal they just submitted.
5. Persist a write-ahead transaction journal containing encrypted or access-
   restricted full snapshots of both specimens and their intended locations.
6. Replace the offered Pal in its exact container slot with the received specimen,
   updating the received specimen's ownership/container references through game
   APIs on the game thread.
7. Add the offered specimen to the virtual pool only after the replacement succeeds.
8. Persist pool state, cooldown, request result, and audit metadata atomically.
9. Clear the journal. On startup, reconcile an incomplete journal without randomly
   repeating the trade.

If the game does not expose a safe slot-replacement operation, the prototype must
stop. A remove-then-create sequence without rollback and idempotency is not
acceptable on the production world.

### Pool persistence

The authoritative pool belongs to the mod, not the Discord bot and not the parsed
world-save projection. Each entry must preserve every transferable per-instance
field needed to recreate the same legitimate Pal, including at least:

- species/character ID and legal variant flags;
- level and experience;
- gender, rank/stars, IV/talent values, and soul upgrades when supported;
- passive and active skills;
- Alpha/Lucky state;
- nickname according to the configured privacy policy;
- schema/game/mod versions and anonymous provenance needed for fairness checks.

Use an atomic, checksummed state file plus rolling last-good copies for an initial
pool of roughly 100 entries. A small SQLite database is preferable if the chosen
native mod runtime can ship it safely; plain Lua should use bounded JSON with temp
file, fsync where available, and rename semantics. Store it on a dedicated durable
volume outside replaceable mod binaries. Palhelm backups should gain an explicit
optional artifact for the pool state rather than assuming the world archive already
contains it.

The pool count remains stable because every successful transaction removes one and
adds one. Admin reseeding, pruning, import, and reset require separate dry-run and
confirmation operations and must never happen automatically after a parse error or
empty read.

An optional persisted maintenance timer may keep the pool fresh without changing
its size. At each interval it can retire a configured number of ordinary pool
entries and replace them with newly generated, legal server-seed specimens from the
same quality bands. Rotation defaults off, records the exact next deadline, and
must not run a burst of missed rotations after a restart. If the pool is below its
target because recovery quarantined entries, a separate `replenish-below-target`
policy may fill only the deficit. Neither timer may consume player-owned Pals.

### Initial seed and pool quality

The first seed should contain approximately 100 legal, vanilla Pals generated once
from version-pinned templates. Proposed distribution:

| Band | Count | Intended contents |
|---|---:|---|
| Everyday | 55 | Useful common/uncommon species, mostly levels 15-30 |
| Useful/rare | 30 | Better workers, mounts, or less common species, mostly levels 20-38 |
| Special | 12 | Rare species or notably strong legal specimens, mostly levels 25-42 |
| Jackpot | 3 | A small mix of Lucky/Alpha or rare specimens; no endgame-exclusive giveaway |

Exact species and stats must come from the installed 1.0 game-data revision, remain
inside vanilla legal ranges, and pass a PalDefender-style legality audit. Do not use
raw `BOSS_` identifiers merely to create an Alpha appearance; variant construction
must match the game's current legitimate representation.

Unrestricted one-for-one exchange will eventually turn a good seed into a pool of
low-value common Pals. Recommended policy is a soft quality band:

- score the offered specimen from species availability, level band, legal IVs,
  passives, rank, Alpha/Lucky state, and configured exclusions;
- normally draw from the same band;
- allow a small configurable chance to draw one neighboring band;
- never permit a low-band offer to consume the small jackpot reserve directly;
- periodically report pool health to operators, but do not silently manufacture
  replacements on a timer.

For a more chaotic server, operators may configure uniform random selection and a
scheduled refill target, accepting the economy inflation explicitly. All selection
uses a cryptographically strong or OS-provided random source when available; never
derive results from predictable request IDs or timestamps.

### Cooldowns and abuse controls

- Default cooldown proposal: 12 hours per stable Palworld player UID.
- Configurable minimum/maximum level, placement policy, excluded species/forms,
  pool size target, quality policy, and rare-result announcement channel.
- Optional daily and weekly trade caps in addition to the cooldown.
- Draw before deposit and avoid the same anonymous donor when enough eligible pool
  entries exist.
- Rate-limit failed requests so repeatedly selecting a moving/summoned Pal cannot
  stress the server.
- Do not reveal the full pool or odds of individual specimens to ordinary users.
- Admin reset/bypass actions require a reason and permanent audit entry.

Cooldown and completed request IDs must persist with the authoritative pool state.
Palhelm may mirror them for UI, history, and diagnostics, but the mod must enforce
them so another bridge caller cannot bypass policy.

### Bridge contract

Suggested command:

```text
PalhelmWonderTrade <request-id> <player-uid> <pal-instance-id>
```

Suggested bounded response:

```json
{
  "v": 1,
  "requestId": "...",
  "status": "delivered",
  "sent": {"speciesId": "Anubis", "level": 31},
  "received": {"speciesId": "FlowerDinosaur", "level": 28, "lucky": true},
  "poolSize": 100,
  "cooldownUntil": "2026-07-17T03:00:00Z"
}
```

Expected failure codes include `provider_unavailable`, `player_offline`,
`pal_not_found`, `not_owner`, `placement_changed`, `pal_busy`, `pal_ineligible`,
`cooldown`, `pool_unavailable`, `pool_schema_mismatch`, `inventory_changed`,
`delivery_uncertain`, and `rollback_failed`.

### Palhelm APIs and storage

Admin/session mutation routes:

```http
GET  /api/v1/wondertrade/capabilities
GET  /api/v1/wondertrade/pool-summary
POST /api/v1/wondertrade/trades
GET  /api/v1/wondertrade/trades/{requestId}
GET  /api/v1/wondertrade/history
```

The public Integration API may eventually expose only an aggregate pool count,
whether trading is available, and cooldown-policy text. It must never expose the
pool roster, full specimen stats, donor identity, internal paths, transaction
journals, or mutation endpoints.

Palhelm stores a durable audit mirror containing request ID, linked Discord actor,
player UID alias, offered/received public summary, timestamps, outcome, provider
version, and failure category. Full Pal records remain in the mod's restricted
state, not in general panel events or Discord logs.

### Configuration proposal

```yaml
wonderTrade:
  enabled: false
  cooldown: 12h
  onlineOnly: true
  allowedPlacements: [box]
  poolTarget: 100
  selectionPolicy: soft-bands
  rotation:
    enabled: false
    every: 24h
    count: 2
  replenishBelowTarget: true
  announceRareResults: true
  announceAlpha: true
  announceLucky: true
  nicknamePolicy: reset-to-species
  minLevel: 10
  maxLevel: 60
```

Configuration edits do not regenerate or normalize an existing pool. Any operation
that changes stored specimens must be a separately previewed admin action.

## Delivery phases

### Phase 0 — zero-impact feasibility

- Confirm whether the production game container is native Linux or a Windows
  server under Proton/Wine, without changing it.
- Static-review current 1.0-compatible mod loaders, signatures, licenses, and
  server-only behavior.
- Build a disposable isolated server using separate ports and a copied or synthetic
  test world; never point a prototype at the live save volume.
- Prove read-only discovery of exact player UID, party/box containers, Pal instance
  IDs, and live placement.
- Prove creation of one legal vanilla test Pal on the disposable server.
- Prove an atomic same-slot swap, save, restart, and rollback before implementing a
  random pool.

### Phase 1 — provider contracts and simulators

- Add disabled-by-default provider interfaces and capability diagnostics.
- Implement item-grant and Wonder Trade request/result schemas, idempotency, audit
  migrations, fake providers, OpenAPI, and contract tests.
- Build a deterministic pool simulator and property tests for constant size,
  cooldowns, band invariants, duplicate requests, and crash recovery.

### Phase 2 — isolated server bridge

- Implement the smallest possible server-only bridge.
- Add restricted persistent state, checksums, last-good copies, and transaction
  journal recovery.
- Exercise disconnect races, full containers, moved Pals, summoned party Pals,
  invalid forms, corrupted state, timeouts, and duplicate delivery.
- Verify an unmodified client can join, trade through Discord, see the received Pal,
  save, leave, and rejoin without installing anything.

### Phase 3 — Palhelm and Discord UX

- Add panel capability/pool/audit views and guarded admin actions.
- Add `/grant-item` for administrators.
- Add `/wondertrade trade|pool|history`, profile-link enforcement, ephemeral
  confirmation, short-lived selection tokens, and rare-result announcements.
- Keep raw IDs, donor identity, paths, and full provider responses out of Discord.

### Phase 4 — production decision

- Review compatibility against the exact live Palworld revision.
- Take and verify a full world backup plus a separate pool-state backup.
- Require zero online players and explicit approval immediately before the first
  game-server restart.
- Install one component at a time, observe startup and save behavior, run a bounded
  admin-only smoke trade, and retain a tested `-NoMods`/known-good rollback path.

## Required test matrix

- Viewer, non-admin Discord member, admin, and forged actor authorization.
- Linked/unlinked/mislinked Discord profile.
- Online/offline player and disconnect during each transaction stage.
- Party, box, moved, summoned, base-assigned, breeding, invalid, human, Alpha,
  Lucky, boss, and future unknown Pal forms.
- Full/changed destination containers and stale Palhelm snapshots.
- Duplicate request before, during, and after completion.
- Provider timeout before mutation, after mutation, and during response writing.
- Process crash after journal, after removal, after replacement, and before pool
  persistence.
- Pool checksum failure, old schema, partial file, missing state, and recovery from
  last-good copy.
- Save, scheduled save overlap, graceful server shutdown, restart, mod disable, and
  mod upgrade.
- 100-entry pool selection distribution, cooldown persistence, band conservation,
  and absence of immediate self-draw.
- Bot and panel restart while the game server remains running.

## Stop conditions

Do not proceed to production if any of the following remains true:

- Only offline save editing can perform the exchange.
- The server cannot identify and mutate an exact Pal instance atomically.
- A failed add can permanently consume the offered Pal.
- Request replay can duplicate either specimen.
- The mod requires client installation for vanilla Pal replication.
- The supported mod loader requires replacing the live runtime without an isolated
  compatibility and rollback proof.
- Pool state cannot be backed up and recovered independently.
- A normal Palworld update silently loads an incompatible bridge.
