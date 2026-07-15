---
title: Updating Palhelm
description: Bump the image tag, understand what the database migrations do, back up the data volume first, and roll back safely.
sidebar:
  order: 5
---

This page covers updating Palhelm to a newer image. It explains the backup you should take first, what the database migrations do on boot, and how rollback works.

## Back up first

Before any update, back up the `/data` volume. It holds the SQLite database, your backups, the downloaded map tiles and Pal icons, and the Oodle library. This is your panel's entire state.

Stop the panel and copy the host directory that backs `/data`. In the install example that host directory is `./palhelm-data`:

```sh
docker compose -f ./compose/docker-compose.yml stop palhelm
cp -a ./palhelm-data ./palhelm-data.backup-$(date +%Y%m%d)
```

A file copy of a stopped SQLite database is a clean snapshot. Keep the copy until you have confirmed the new version works.

## Bump the image tag

Update the `image:` tag for the `palhelm` service in your Compose file, then pull and recreate just that service:

```sh
docker compose -f ./compose/docker-compose.yml pull palhelm
docker compose -f ./compose/docker-compose.yml up -d palhelm
```

Only the `palhelm` service is recreated. The game server is untouched. Watch the logs to confirm a clean start:

```sh
docker compose -f ./compose/docker-compose.yml logs -f palhelm
```

## What migrations do

Palhelm keeps its schema current with a small embedded migration runner. On boot it opens the SQLite database in `/data`. A brand-new database is created at the newest schema in one step. An existing database has any missing migrations applied in order, each in its own transaction, with the schema version tracked in the database itself.

You do not run migrations by hand. They apply automatically when the new image boots. The migrations to date are:

| Migration | What it does |
|---|---|
| `001_init` | Creates the full base schema: metrics and rollups, players, sessions, events, guilds, bases, pals, world state, console log, saved commands, allow list, backups, and the key-value store. |
| `002_api_keys` | Adds the `api_keys` table that backs the Integration API. |
| `003_remove_pending_identity_players` | Cleans up placeholder player and session rows with no real identity. |
| `004_pal_party_box` | Adds the `pals` table for party and box Pals. |
| `005_pal_owner_provenance` | Records how each Pal's owner was resolved. |
| `006_player_progress` | Adds capture totals, unique Pals captured, and Paldeck unlock counts to players. |
| `007_pal_instance_details` | Adds per-Pal details such as HP, gender, and talents. |
| `008_pal_base_workers` | Adds a base id to Pals so base workers can be attributed. |
| `009_game_data_activity` | Adds aggregate-only Game Data FPS, worker activity, and link-coverage samples with 30-day retention. It stores no actor identities, names, health, guilds, or locations. |

The runner fails closed. If the database was written by a newer Palhelm than the running binary knows about, it refuses to open rather than risk corrupting data. That refusal is what makes rollback predictable.

## Rolling back

If a new version misbehaves, put the old image tag back and recreate the service:

```sh
docker compose -f ./compose/docker-compose.yml up -d palhelm
```

Whether the old binary starts depends on the schema. A database written at a schema the old binary already knows opens cleanly. If a newer migration ran and the old binary does not recognize the schema version, the runner fails closed and the panel will not start on the old tag. In that case, restore the `/data` copy you made before the update and start the old tag against it:

Version 0.5.0 applies migration 009. Rolling back from it to a 0.4.x image therefore requires restoring the pre-upgrade `/data` backup; changing only the image tag is not sufficient.

```sh
docker compose -f ./compose/docker-compose.yml stop palhelm
rm -rf ./palhelm-data
mv ./palhelm-data.backup-YYYYMMDD ./palhelm-data
docker compose -f ./compose/docker-compose.yml up -d palhelm
```

This is exactly why the pre-update backup matters. Some release notes call out when a database written by the new version stays compatible with the previous version, in which case rollback needs no restore. Check the notes for the version you are moving between.

## After updating

Map tiles, Pal icons, and the Oodle library persist in `/data` across updates. You do not refetch them on a normal version bump. Refetch only when you want fresher assets or a new game version's tiles. See [Map tiles and icons](/getting-started/map-tiles-and-icons/).
