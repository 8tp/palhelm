# Palhelm Item Bridge

Server-only UE4SS bridge for Palhelm's audited item-grant workflow. Players use an
unmodified Palworld client. The bridge consumes one immutable request at a time
from a local shared directory and calls the server-authoritative player inventory
API on the Unreal game thread.

## Compatibility boundary

Pocketpair currently supports its server mod loader on the **Windows dedicated
server only**. This source package targets that runtime and also contains
path-neutral Lua support for a separately validated Windows/Proton or experimental
native-Linux UE4SS setup. Native Linux remains unsupported by Pocketpair and
upstream UE4SS; it must stay fail-closed until an isolated test of the exact loader
and Palworld build passes the validation checklist below.

The bridge starts in `validation_required`, not `ready`. A Palworld update can
change reflected functions or enum behavior. `ProductionValidated = true` must be
set only after an isolated test on the exact game build verifies:

1. the stable save UID resolves to exactly one online `PalPlayerState`;
2. `GetInventoryData()` is callable server-side;
3. `AddItem_ServerInternal` has the configured arity (current 1.0 SDK: five);
4. result `0` means the full quantity was accepted and inventory-full is nonzero;
5. reconnect and normal world save preserve the grant.

## Layout

Copy this directory to the UE4SS Mods directory for the selected runtime, copy
`config.example.lua` to `config.lua`, and point `SpoolDir` and `AllowlistPath` at a
directory mounted locally into both Palhelm and the game container/host. The
experimental native-Linux layout currently used for validation is
`Pal/Binaries/Linux/ue4ss/Mods/PalhelmItemBridge`; the Windows layout is
`Pal/Binaries/Win64/ue4ss/Mods/PalhelmItemBridge`.

The catalogue importer produces the matching allowlist:

```bash
cd backend
go run ./cmd/item-catalog-import \
  --manifest /operator/current-items.json \
  --catalog /data/item-catalog.json \
  --icons /data/item-icons \
  --allowlist /data/item-allowlist.txt
```

For a private/operator installation, the importer can also convert a locally
saved copy of PalMods' current game-ID reference and then cache its icon URLs at a
polite bounded rate. Review the source's current terms before use; Palhelm does not
redistribute the resulting data or art:

```bash
curl -fL -A 'PalhelmItemCatalogImporter/1.0 (+https://github.com/8tp/palhelm)' \
  -o /operator/palmods-items.html \
  https://www.palmods.gg/docs/authors/game-ids/items

cd backend
go run ./cmd/item-catalog-import \
  --palmods-html /operator/palmods-items.html \
  --game-version 1.0.1 \
  --catalog /data/item-catalog.json \
  --icons /data/item-icons \
  --allowlist /data/item-allowlist.txt \
  --request-delay 1s
```

The converter imports every available icon but allowlists only canonical gameplay
items in bounded categories. `Key Item`, `Internal & other`, noncanonical aliases,
and rows without game artwork stay non-grantable. At the 2026-07-18 review the
source exposed 2,466 item records and 2,455 game icons; the 11 rows without source
art therefore cannot receive an honest item-specific icon.

Game icons and extracted game data are not shipped with Palhelm. The manifest is
operator-supplied and must use the exact running Palworld version. Every grantable
item requires an icon and an explicit maximum quantity; unsafe/internal entries
should remain `grantable: false`.

## Spool protocol v1

- Palhelm atomically links a complete request to `<SpoolDir>/inbox.json`.
- The bridge atomically renames it to `processing.json` before execution.
- Results are written atomically to `results/<requestId>.json`.
- A leftover `processing.json` after a crash becomes `failed/outcome_unknown` and
  is never retried, because the mutation may already have happened.
- `capability.json` is refreshed every 10 seconds. Palhelm requires protocol,
  game version, allowlist count, catalogue version, and a heartbeat newer than 45
  seconds before its admin UI can submit.

The bridge has no socket listener, HTTP endpoint, chat command, arbitrary console
surface, name-based player fallback, or save-file editor.

## References

- Official server-mod support: <https://docs.palworldgame.com/settings-and-operation/mod/>
- UE4SS game-thread API: <https://docs.ue4ss.com/dev/lua-api/global-functions/executeingamethread.html>
- Current public Palworld SDK signature (research reference):
  `UPalPlayerInventoryData::AddItem_ServerInternal(FName,int32,bool,float,bool)`
