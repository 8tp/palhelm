---
title: Backups and restore
description: Scheduled and manual snapshots, the schedule and retention, and the safe restore flow.
sidebar:
  order: 5
---

This page covers backups: scheduled and manual snapshots, retention, browsing a snapshot, and the restore flow that refuses to run while the server is up.

Each backup is a snapshot of the world save. The list shows every snapshot with its file name, the world day it captured, the creation time, the size, and how it was triggered. Triggers are scheduled, manual, pre-restore, and imported.

## Taking and browsing backups

An admin can select "Back up now" to take a snapshot immediately. Search snapshots by name, or filter by trigger.

Select "Browse contents" on a snapshot to list the files inside the archive with their sizes. Select "Download" to download the snapshot as a `.tar.gz` archive. An admin can delete a snapshot. Deleting removes it from the backup volume permanently and cannot be undone.

## Schedule and retention

The schedule card sets how often a scheduled backup runs, from every hour up to every 12 hours, and how many days of snapshots to keep, from 7 up to 60 days. When the schedule is enabled, the card shows the next run time. Snapshots older than the retention window are pruned.

The storage card shows the total size of kept snapshots against a reference capacity.

:::note
The backup volume's real capacity is not reported by the API yet, so the storage meter uses a fixed reference figure for its denominator. Read the "used" total as the accurate number.
:::

## Restoring a snapshot

Restore is deliberate and multi-step so you do not overwrite a live world by accident.

1. Select "Restore" on a snapshot. The panel computes a dry-run diff of the snapshot against the current live save and lists every file that would be added, modified, or deleted.
2. Type `RESTORE` in the confirm field. The restore button stays disabled until you do.
3. Confirm. The panel always takes a pre-restore backup first, then restores the snapshot.

:::caution
Restore refuses to run while the game server is running. The request returns a conflict whenever the game REST API is still reachable. Stop the server first, then restore. When the panel cannot perform the restore itself, it shows the exact host command to run instead. A pre-restore backup is always taken before any restore, so you can roll back the restore itself.
:::

## Data sources

Backups read `GET /api/v1/backups`, `GET /api/v1/backups/{id}/contents`, and `GET/PUT /api/v1/backups/schedule`. Creating uses `POST /api/v1/backups`. Restore uses `POST /api/v1/backups/{id}/restore/dry-run` then `POST /api/v1/backups/{id}/restore` with the typed confirmation. Download uses `GET /api/v1/backups/{id}/download`, and delete uses `DELETE /api/v1/backups/{id}`.
