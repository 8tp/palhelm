---
title: Item grants
description: The optional, fail-closed UE4SS item bridge, operator prerequisites, admin flow, and audit trail.
sidebar:
  order: 3
---

Item grants are an optional admin-only feature for giving a bounded quantity of an
allowlisted item to one online player. They use a separate UE4SS server bridge and
do not edit Palworld save files.

:::caution
The source bridge ships with `ProductionValidated=false`. Installing the files is
not enough to make grants available. Validate the exact Palworld and UE4SS build in
a maintenance window before changing that gate.
:::

## Prerequisites

The capability reports ready only when all of these conditions are met:

1. `PALHELM_ITEM_GRANTS_ENABLED=true` explicitly enables the provider.
2. `PALHELM_ITEM_CATALOG_PATH` points to a validated, versioned operator catalogue.
3. The UE4SS bridge has a generated allowlist from the same manifest, including
   per-item quantity ceilings.
4. `PALHELM_ITEM_GRANT_SPOOL_DIR` is a local directory shared only by Palhelm and
   the game-server bridge.
5. A fresh heartbeat matches the expected protocol, catalogue, and game version.
6. The bridge marks that exact game build as production-validated after the
   maintenance checklist succeeds.

The repository includes `item-catalog-import` and an example manifest for creating
the catalogue, icons, and bridge allowlist from operator-provided game data. Palhelm
does not distribute game artwork.

## Admin flow

From an online player's detail view, select **Give item…**, search the installed
catalogue, choose a quantity, and enter a reason. The review step shows the exact
stable player UID, internal item ID, quantity, and current capability before the
request can be confirmed.

Palhelm writes the audit record before moving one atomic request file into the
spool. The bridge validates it again, performs the live inventory mutation, and
writes one result. The player must be online. Display names are never used as the
mutation target.

## Safety and audit behavior

- Only admins can create or inspect grants; viewers cannot see the action.
- The Integration API remains GET-only and has no item-grant surface.
- Arbitrary item IDs, commands, Lua, shell input, and asset paths are rejected.
- Requests are serialized, quantity-capped, reasoned, and idempotent.
- Successes and failures retain actor, target, item, quantity, reason, timestamps,
  and the bounded bridge result.
- A missing or late result is never retried automatically because the original
  mutation may already have succeeded.
- Disabled, stale, mismatched, or unvalidated bridges keep the action unavailable.

The exact-build validation and rollback checklist lives in the repository's
`docs/SERVER-MOD-INTEGRATIONS-PLAN.md`.
