---
title: Install with Docker Compose
description: Add the Palhelm service next to your Palworld container, set the volumes and env vars, and verify the first boot.
sidebar:
  order: 2
---

This page covers installing Palhelm with Docker Compose next to a Palworld server you already run. It shows the service block, the volumes, the required env vars, the healthcheck, and what happens on first boot.

## Before you start

You need:

- A running Palworld 1.0 dedicated server managed by Docker Compose. The examples assume the popular `thijsvanloef/palworld-server-docker` image, but any 1.0 server works.
- RCON enabled on the game server (`RCON_ENABLED=true`) and an admin password set (`ADMIN_PASSWORD`). The admin password also enables the official REST API.
- The user and group id that own the save files. The examples use `1000:1000`. Match your server's `PUID`/`PGID`.

:::caution
Never expose the panel to the open internet. Bind its port to a private interface such as localhost, a VPN, or a tailnet. This is the same advice Pocketpair gives for the game's own REST API. Do not put Palhelm behind a public reverse proxy without a network-level access control in front of it.
:::

## Directory layout

Palhelm can edit your Compose file for the config editor. To let it do that safely, keep the Compose file in its own directory owned by the panel's user id, and mount that directory read-write. Do not mount the Compose file by itself: an atomic rename over a single-file bind mount is not supported.

Put the project file at `./compose/docker-compose.yml` and run Compose from the host that way:

```sh
mkdir -p compose
chown 1000:1000 compose
docker compose -f ./compose/docker-compose.yml up -d
```

Paths inside that file resolve from `./compose`, so the relative volume sources below use `../data` and `../palhelm-data`.

## The Compose file

Here is a full example with Palhelm sitting next to the game server. Replace the passwords with your own strong values.

```yaml
services:
  palworld:
    image: thijsvanloef/palworld-server-docker:latest
    container_name: palworld
    restart: unless-stopped
    stop_grace_period: 30s
    ports:
      - "8211:8211/udp"
      - "27015:27015/udp"
    environment:
      PUID: 1000
      PGID: 1000
      PLAYERS: 16
      RCON_ENABLED: "true"
      RCON_PORT: 25575
      SERVER_NAME: "my palworld server"
      ADMIN_PASSWORD: "choose-a-strong-password"   # also enables the REST API
    volumes:
      - ../data:/palworld

  palhelm:
    image: ghcr.io/8tp/palhelm:latest   # or build locally with: docker build -t palhelm .
    container_name: palhelm
    restart: unless-stopped
    depends_on:
      - palworld
    user: "1000:1000"                       # match the uid/gid that owns the saves
    ports:
      - "127.0.0.1:8080:8080"               # private interface only, never the internet
    environment:
      PALHELM_ADMIN_PASSWORD: "choose-a-strong-password"   # panel admin login
      # PALHELM_VIEWER_PASSWORD: "choose-a-strong-password" # optional read-only login
      PALWORLD_REST_URL: "http://palworld:8212"
      PALWORLD_ADMIN_PASSWORD: "choose-a-strong-password"  # same as the game server's ADMIN_PASSWORD
      PALWORLD_RCON_ADDR: "palworld:25575"
      PALWORLD_SAVE_DIR: "/game/Saved"
      PALHELM_COMPOSE_FILE: "/compose/docker-compose.yml"  # enables the config editor
      PALHELM_GAME_SERVICE: "palworld"
      # PALHELM_TRUSTED_PROXIES: "10.0.0.0/8"  # proxy CIDRs allowed to supply forwarded IP/HTTPS
      # PALHELM_SECURE_COOKIES: "true"          # force Secure cookies behind TLS termination
    volumes:
      - ../data/Pal/Saved:/game/Saved         # the save directory, read-write so restore can write
      - ../palhelm-data:/data                 # panel database, backups, map tiles, Oodle lib
      - ./:/compose                           # the dedicated compose directory, read-write
```

## Volumes

Palhelm uses two host volumes plus the Compose directory.

- **`/game/Saved`** is the game's save directory. Palhelm reads `Level.sav`, `LevelMeta.sav`, and `Players/*.sav`. It is mounted read-write only so backup restore can write into it. Normal operation is read-only.
- **`/data`** is the Palhelm data volume. It holds the SQLite database, backups, downloaded map tiles and Pal icons, and the downloaded Oodle library. Keep this volume. It is your panel's state. See [Updating](/getting-started/updating/) for backup advice.
- **`/compose`** is the directory that contains your Compose file. It is mounted read-write so the config editor can rewrite the `environment:` block with an atomic rename. Palhelm never needs a Docker socket.

## Environment variables

These are the ones you set most often. The full table is in the project README.

| Env var | Required | Purpose |
|---|---|---|
| `PALHELM_ADMIN_PASSWORD` | yes | Panel admin login. |
| `PALHELM_VIEWER_PASSWORD` | no | Optional read-only login. Unset means no viewer account. |
| `PALWORLD_REST_URL` | yes | The game REST API, for example `http://palworld:8212`. |
| `PALWORLD_ADMIN_PASSWORD` | yes | The game admin password, used for REST basic auth and RCON. |
| `PALWORLD_RCON_ADDR` | yes | The game RCON address, for example `palworld:25575`. |
| `PALWORLD_SAVE_DIR` | yes | The mounted save directory, `/game/Saved` above. |
| `PALHELM_COMPOSE_FILE` | no | Path to the Compose file inside the container. Enables the config editor. |
| `PALHELM_GAME_SERVICE` | no | The service name of the game server in that Compose file. Defaults to `palworld`. |
| `PALHELM_DATA_DIR` | no | The data directory. Defaults to `/data`. |
| `PALHELM_GAME_DATA_ENABLED` | no | Turns on the optional live game-data poller for live map positions, base workers, and activity diagnostics. Defaults to off. |
| `PALHELM_ADDR` | no | Listen address. Defaults to `:8080`. |
| `PALHELM_TRUSTED_PROXIES` | no | Comma-separated proxy CIDRs allowed to supply a forwarded client IP and HTTPS. Headers from other peers are ignored. |
| `PALHELM_SECURE_COOKIES` | no | Force the session cookie's `Secure` flag behind a TLS-terminating proxy. Defaults to `false`. |

The game admin password reaches Palhelm through `PALWORLD_ADMIN_PASSWORD`, but it never reaches the browser. Palhelm proxies every game call server-side.

## First boot

When the container starts for the first time:

1. Palhelm opens the SQLite database in `/data`. If the database is new, it creates it at the newest schema. If it already exists, the embedded migration runner applies any missing migrations in order. See [Updating](/getting-started/updating/) for what the migrations do.
2. The panel starts listening on `:8080` inside the container, published to `127.0.0.1:8080` on the host in the example.
3. The Oodle decompressor is not downloaded yet. It is fetched into `/data` the first time Palhelm parses a save, and a pinned SHA-256 is verified before it loads. If you are air-gapped, drop the file into the data directory yourself or point `PALHELM_OODLE_LIB` at it.
4. Map tiles and Pal icons are not present yet. The live map and the Pal art stay empty until you run the fetch scripts. See [Map tiles and icons](/getting-started/map-tiles-and-icons/).

## Verify it is up

The container has a built-in healthcheck that polls `/healthz`, a public liveness endpoint. Check container health and the endpoint directly:

```sh
docker compose -f ./compose/docker-compose.yml ps
curl -fsS http://127.0.0.1:8080/healthz
```

When the container reports healthy, open `http://127.0.0.1:8080` in a browser on the same private network and continue to [First login](/getting-started/first-login/).
