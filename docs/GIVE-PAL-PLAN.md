# Give Pal — implementation scope

Status: panel/backend contract and fail-closed runtime bridge are installed;
the runtime bridge remains `validation_required` after rejected reflected-RPC
and Lua-native creation paths and is unavailable for ordinary panel requests.

## User experience

From an online player's detail panel, an administrator selects **Give Pal…**,
searches the safe Pal catalogue, chooses a level or level range, supplies a reason,
reviews the generated specimen, and confirms once.

The default **Natural roll** delegates gender, IVs, passives, active skills, and
other specimen details to the validated game-side generator. **Custom** exposes
only fields Palhelm can validate honestly:

- gender: random, female, or male;
- normal, Alpha, or Lucky (Alpha and Lucky cannot both be forced);
- natural, minimum, or exact HP/Melee Attack/Ranged Attack/Defense IVs from 0–100;
- condensation from 0–4 displayed stars, kept distinct from the save's 1–5 rank;
- zero through four unique standard passive skills from a pinned PalCalc v23
  catalogue. The editor includes negative, bronze, silver, gold, rainbow, and
  World Tree traits, shows their effect and normal inheritance status, and treats
  rare/fixed selections as an explicit audited admin override;
- level range from 1 through the pinned 1.0 cap of 80.

Unknown or duplicate raw passive IDs are rejected by the backend even when a
request bypasses the panel. Active skills remain out of v1 until their exact write
and read-back fields are validated. The capture-native candidate can deliver to a
free party slot before the Palbox, so v1 must report and verify the actual owned
destination rather than claiming Palbox-only delivery.

## Implemented offline

- Independent `PALHELM_PAL_GRANTS_ENABLED` flag, default false.
- Independent Pal grant spool and capability heartbeat.
- Safe catalogue built from Palhelm's pinned 1.0 Paldeck; humans and BOSS_ save
  variants are excluded.
- Complete embedded standard-passive catalogue generated from pinned PalCalc
  commit `b5e13e90fedc2e95d54fa223da77be464c313001`, plus searchable viewer API
  and a four-slot expert selector.
- Bounded generation-spec validation.
- Admin-only API contract, online-player check, idempotency, audit event, durable
  request/result history, and no automatic retry.
- Panel search, Pal icons, natural/custom controls, review screen, status polling,
  and recent history.
- Fail-closed UE4SS bridge with exact one-request validation gates, authoritative
  owner/container read-back, and no retry after an uncertain mutation.

## Runtime reverse-engineering checkpoint

The current `v1.0.1.100619` Linux UHT dump and installed symbol bundle expose
these relevant names:

```text
APalPlayerState::RequestSpawnMonsterForPlayer(FName, int, int)
APalPlayerState::Debug_CaptureNewMonster_ToServer(FName)
APalPlayerState::Debug_CaptureNewMonsterByDebugOtomoInfo_ToServer(
    FPalDebugOtomoPalInfo, bool bRandomPassiveSkill)
UPalUtility::PalCaptureSuccessFromHandle(
    APalPlayerCharacter*, UPalIndividualCharacterHandle*)
```

The first live path constructed a synthetic `UPalCheatManager`; the reflected call
returned but created nothing. The second used the real player's
`RequestSpawnMonsterForPlayer` server RPC with Palworld's Palbox-transport debug
flag enabled. A player-bound preflight passed, but the call again produced no new
party or Palbox specimen. Both outcomes were verified against owned instance IDs
and were never retried.

The next staged path uses the real player's reflected
`Debug_CaptureNewMonsterByDebugOtomoInfo_ToServer` RPC. Its
`FPalDebugOtomoPalInfo` parameter contains species, level, rank, talent preset,
active-skill, passive-skill, status-rank, friendship, and awakening fields. The
installed UE4SS build contains Lua-table-to-`UScriptStruct` conversion support,
so the bridge can populate the parameter without raw memory writes. The bridge
supplies only the allowlisted species, requested level, uncondensed rank, and
natural-generation defaults, then asks Palworld to randomize passives. It
snapshots and verifies both party and Palbox because normal capture semantics can
use either.

The same loaded bridge includes bounded custom level/range, gender, generic
Alpha (`BOSS_` form), Lucky, exact/minimum IV, passive, and condensation
handling. `AdvancedEnabled` loads those paths without bypassing the independent
production-validation gate. Runtime validation policy is re-read from
`config.lua`, allowing promotion after the natural specimen survives
save/reconnect testing without another game restart.

### Lua-native capture route rejected (2026-07-19)

After the player-bound debug capture RPC returned without creating a specimen,
the exact-build UHT dump exposed a theoretically cleaner normal-capture route:

```text
UPalUtility::GetInitializedCharacterSaveParemter(..., out SaveParameter)
UPalCharacterManager::SpawnNewCharacter(SaveParameter, SpawnParameter, Callback)
UPalUtility::PalCaptureSuccess(PlayerCharacter, SpawnedPalActor)
```

The first read-only live probe stopped before `SpawnNewCharacter`, but the
installed experimental Linux UE4SS beta corrupted the allocator while marshalling
the large `FPalIndividualCharacterSaveParameter&` out struct. Palworld raised an
`FMallocBinned2 Attempt to realloc an unrecognized block` fatal error and SIGSEGV;
Docker then restarted it under the existing `unless-stopped` policy. No Pal was
created and this Lua route must not be retried on the live server.

The next implementation candidate is therefore a version-pinned UE4SS C++ bridge.
It calls reflected Palworld functions with native parameter storage, retaining the
existing Palhelm spool, idempotency, capacity, read-back, cleanup, and no-retry-on-
uncertainty gates. The installed `.sym` file cannot provide the native handle
helpers: it predates the current executable, and offline decoding proved its
records do not land on the named functions in the current ELF. Those addresses
are prohibited. The first non-mutating C++ capability artifact now lives in
`mods/palhelm-pal-native`; it must not be installed or loaded without a fresh
backup and explicit restart approval.

## Existing-mod audit (2026-07-18)

### Admin Commands (Server Side) 3.0.2

This current UE4SS mod advertises server-only Linux, Proton/Wine, and Windows
support, with `/spawn` and `/capture` commands. That is useful feasibility
evidence for native Linux UE4SS. It is not a safe implementation dependency for
Palhelm:

- `/capture`/the newer spawn workflow creates or hijacks a world actor and then
  auto-captures it rather than documenting a direct Pal-storage call;
- the maintainer currently documents a 1.0-era bug where a captured Pal may not be
  summonable or movable until the player rejoins or the server restarts;
- command targeting and success text are not equivalent to verifying the final
  owner, container, slot, and persistent individual;
- the distributed implementation is not licensed for reuse or modification.

Palhelm should therefore not copy the spawn-then-capture route. It may be useful
only as an isolated behavioral comparison during the same future maintenance
window.

### PalDefender 1.8.x

PalDefender exposes direct `/givepal` and bearer-authenticated
`POST /v1/pdapi/give/pals/{player_identifier}` operations. Its documented design
provides several patterns worth matching:

- stable platform ID or PlayerUID targeting, never a display name;
- Pal-storage capacity validation before mutation;
- execution on the game thread with a bounded five-second request deadline;
- a simple natural path where only Pal ID and level are supplied;
- filename-selected templates for advanced specimens instead of arbitrary JSON
  embedded in a command;
- server-wide and per-species import policy that can block or clamp levels, ranks,
  IVs, souls, passives, and forbidden species;
- separate validation, timeout, storage-full, and request-failure outcomes.

PalDefender itself cannot be our native-Linux implementation: it is currently
Windows-only and closed source. A future provider adapter could use its documented
REST API on Windows/Proton deployments, while the native Linux provider retains
the same Palhelm-side contract.

### PalSchema

PalSchema is valuable for versioned data-table and asset changes, but it is not a
player-targeted runtime Pal-delivery API. It should not sit in the critical grant
path.

## Contract changes from the audit

Before the bridge can report `ready`, it must additionally prove:

1. Party and Palbox free-space are checked before individual creation.
2. Delivery uses Palworld's authoritative capture/grant logic; Palhelm itself
   never writes a live save or raw container memory.
3. Success is returned only after owner, container, slot, instance ID, and generated
   fields can be read back from the authoritative handle/container.
4. Game-thread work has a bounded deadline, but a timeout after dispatch is
   `outcome_unknown` and is never retried.
5. The exact-build UE4SS `MemberVariableLayout.ini` is present and validated.
6. Advanced templates eventually pass through a separate operator policy layer;
   the current hard validator remains the upper safety boundary.

The first production tranche remains natural generation plus bounded
gender/Alpha/Lucky/IV/rank/passive overrides. Active skills, learned skills,
souls, work-suitability overrides, skins, and nicknames remain disabled until
their exact 1.0 fields, versioned ID catalogues, and policy rules are validated.

## Maintenance-window acceptance test

1. Back up the world and verify the archive.
2. Ensure zero players online; use a disposable player/Palbox if possible.
3. Deploy only the non-mutating `palhelm-pal-native` capability probe and restart
   once.
4. Confirm the exact-build `MemberVariableLayout.ini`, build-ID gate, native mod
   lifecycle, and reflected function metadata without submitting a mutation.
5. Prove the target party and Palbox lookup plus at least one free owned slot
   before creation.
6. Only after a current-build authoritative rollback route is proven, test one
   ordinary natural Lamball at level 1 through the native reflected capture path.
7. Read the resulting handle/container back, then save normally, reconnect, parse
   the save, and confirm owner/container/instance
   fields and all generated stats.
8. Exercise full-box, disconnect, duplicate request, timeout-after-dispatch, and
   process-crash cases.
9. Keep `ProductionValidated=false` unless every check passes for the exact build.

No runtime deployment or Palworld restart is authorized by this document.
