# Palhelm Pal Grant Bridge

Server-only UE4SS bridge for Palhelm's audited **Give Pal** workflow.

## Current status

`validation_required` by design. Validation attempts proved that Palworld's
reflected `APalPlayerState::RequestSpawnMonsterForPlayer` RPC returns without an
error in a server-side UE4SS context but does not create an owned specimen. The
bridge therefore uses the target player's reflected, reliable server RPC
`APalPlayerState::Debug_CaptureNewMonsterByDebugOtomoInfo_ToServer` for the next
isolated validation. Its reflected `FPalDebugOtomoPalInfo` parameter carries the
allowlisted species and exact level while Palworld generates the remaining
natural specimen data. This is Palworld's capture path rather than its
grant/spawn path.
The bridge snapshots both the player's party and Palbox and reports success
only after exactly one matching new specimen can be read back from either owned
container. This avoids constructing or attaching a synthetic cheat manager and
does not mutate global debug settings. Any uncertain call remains
non-retryable. Player-controller identity resolution and all reflected
Palworld calls execute on the game thread; the background loop only performs
spool-file coordination.

Before production validation, only the exact request configured by
`ValidationPreflightRequestID`, `ValidationRequestID`, `ValidationCharacterID`,
and `ValidationLevel` can run, and only when `ValidationMode=true`. A matching
`preflightOnly=true` request must first prove, on the game thread, that the
target controller, Palbox, empty slot, and transport setting are readable. The
actual validation request remains blocked until that read-only preflight result
exists. The capability heartbeat remains
`validation_required`, so Palhelm cannot submit ordinary requests.
`AdvancedEnabled=true` loads bounded custom level/range, gender, generic Alpha,
Lucky, IV, passive, and condensation handling during the same startup, but does
not bypass validation. The validation IDs and `ProductionValidated` are re-read
from `config.lua`, so the exact build can be promoted after persistence testing
without another Palworld restart.

Read-only inspection of the installed `v1.0.1.100619` Linux symbol bundle found:

```text
UPalCharacterManager::CreateIndividual(
  FPalIndividualCharacterSaveParameter,
  UPalCharacterManager::FIndividualIDCallback)

UPalNetworkIndividualComponent::CreateIndividualID_ServerInternal(
  FPalIndividualCharacterSaveParameter,
  FGuid,
  int)

UPalCharacterManager::GrantWorldCharacterToPlayerServerInternal(
  UPalIndividualCharacterHandle*,
  FGuid const&)

UPalCharacterManager::GrantWorldCharacterToPlayerFixedSlotIdServerInternal(
  UPalIndividualCharacterHandle*,
  FGuid const&,
  FPalCharacterSlotId const&)
```

`CreateIndividualID_ServerInternal` is reflected as a `UFunction`, but it only
proves character creation, not Palbox delivery. The bridge therefore does not
use it. The two direct Grant helpers appear native-only.

The reflected Lua alternative that initializes a natural save parameter, spawns
a temporary Pal actor, and feeds that actor to `PalCaptureSuccess` is also disabled.
On the exact 1.0 Linux build, a read-only call to
`GetInitializedCharacterSaveParemter` crashed the experimental UE4SS beta while
marshalling its large struct out parameter, before any actor or individual was
created. Do not retry that route in Lua. A future version-pinned C++ bridge is the
only remaining planned creation provider; it is intentionally not loaded by this
mod.

The probe also checks the reflected read-side container functions `GetContainer`,
`FindEmptySlot`, and `FindByHandle`, plus the presence of
`MemberVariableLayout.ini`. These are the minimum pieces needed to preflight
Palbox capacity and verify a completed delivery; they do not make mutation safe
on their own.

## Required validation gates

1. Take a fresh world backup and use a maintenance window with no players.
2. Confirm exactly one authoritative `PalNetworkIndividualComponent` context.
3. Confirm the version-pinned UHT dump still exposes
   `Debug_CaptureNewMonsterByDebugOtomoInfo_ToServer`,
   `FPalDebugOtomoPalInfo`, player party and Pal storage, the character-container
   manager, and owned-container slot enumeration.
4. Grant a natural, ordinary Lamball through the one-request validation gate to
   a test player with an empty Palbox slot.
5. Verify identity, owner UID, container/slot, level, gender, IVs, passives,
   active skills, rank, Alpha/Lucky flags, Paldeck behavior, replication,
   reconnect, normal world save, and subsequent save parse.
6. Test full Palbox, disconnect during creation, duplicate request ID, process
   death after the game call, and outcome-unknown recovery.
7. Only then implement request consumption and set `ProductionValidated=true`
   for that exact game version.

The bridge must never retry a request whose mutation outcome is uncertain.
Humans, tower/raid actors, BOSS_ identifiers, placeholders, and unallowlisted
forms remain rejected by the panel contract.
