---
title: Configuration
description: Editing server settings through the Compose environment, pending versus effective values, and the manual apply flow.
sidebar:
  order: 6
---

This page covers the configuration editor: why it edits your Compose file instead of the ini, how pending and effective values differ, and the manual apply step that is intentional.

## Why it edits the Compose file

The popular `thijsvanloef/palworld-server-docker` image regenerates `PalWorldSettings.ini` from Compose environment variables on every boot. If the panel edited the ini directly, the next restart would overwrite the change. Editing the ini would be a lie.

So the panel edits the `environment:` block of your Compose file instead. It writes surgically, preserving comments and ordering. On the next server restart the image regenerates the ini from those variables, and the change takes effect.

A banner at the top of the screen states this plainly. The editor is available only when the Compose file is mounted so the panel can write it safely. If the mount does not support safe atomic writes, the editor is read-only and the banner explains why.

## Editing settings

The **Settings editor** tab groups settings into cards. Each setting shows a control suited to its type: a text field, a number field, a dropdown for options like difficulty or death penalty, or an enabled/disabled toggle for booleans. Password fields are write-only. The current value is never returned to the browser, so you enter a new value to change it and leave it blank to keep the current one.

Each setting carries an honest hint about its state:

- **default** when the value matches the game default.
- **modified** when you have changed it but not yet written it.
- **written, not yet applied** when it is in the Compose file but the running server still uses the old value.
- **read-only in this deployment** when the setting cannot be changed here.

When you have unsaved edits, a bar shows the count of pending changes. Select "Write to compose file" to save them, or "Discard" to drop them. The **Raw ini** tab shows the live `PalWorldSettings.ini` text, read-only, for reference.

## Applying a change

After you write to the Compose file, the setting is pending until the server restarts. The panel shows the exact host command to run:

```sh
docker compose up -d palworld
```

Run it from the host directory that contains your Compose file, because relative bind paths resolve from there.

:::caution
One-click apply is disabled on purpose. A Compose command run from inside the panel container cannot safely preserve arbitrary host project paths and identity. The panel prints the command and you run it on the host. The panel does not need or accept a Docker socket.
:::

:::note
The panel guards against concurrent edits. Each read carries a version derived from the Compose file, checked with a SHA-256 compare-and-swap on write. If the file changed since you loaded it, the write is rejected with a conflict, the configuration reloads, and you review your edit and try again. Nothing is silently overwritten.
:::

## Data sources

The editor reads `GET /api/v1/config` and `GET /api/v1/config/raw`. Writes go through `PUT /api/v1/config` with the version and the changed keys. The disabled one-click apply endpoint, `POST /api/v1/config/apply`, returns a not-implemented status with the manual command in its error body.
