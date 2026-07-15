---
title: Overview
description: What the Integration API is for, its read-only GET-only design, base URL, versioning, and response envelopes.
sidebar:
  order: 1
---

This page covers what the Integration API is, how it differs from the panel's own API, and the ground rules that apply to every request: the base URL, the GET-only design, bearer authentication, and the response envelope.

## What it is for

The Integration API is a read-only surface for programs. It exists so a Discord bot, a status page, or a script can read live and save-derived data from your server without a browser session. It is the API the Palhelm Discord bot uses, and the one you would point your own tooling at.

It is deliberately small and safe to expose to automation:

- Read-only. Every route is a `GET`. There is no way to kick a player, run a console command, restore a backup, or change a setting through this API.
- Redacted. Responses are a strict subset of what a signed-in viewer sees. Platform account IDs, live player positions, and moderation state never appear. See [Keys and redaction](/integration-api/keys-and-redaction/).
- Token-authenticated. Access uses a bearer key, not a login. Keys are created and revoked from the panel.

## How it differs from the panel API

Palhelm has two separate HTTP surfaces. Keeping them apart is a deliberate design choice, not an accident of routing.

| | Panel API | Integration API |
|---|---|---|
| Base path | `/api/v1` | `/api/integration/v1` |
| Auth | Session cookie from a password login | Bearer key (`Authorization` header) |
| Methods | GET and mutating (POST, PUT, DELETE) | GET only |
| Audience | The panel's own web UI | Bots, dashboards, scripts |
| Data shape | Full viewer data | Redacted, viewer-minus |

The two never mix. A session cookie grants nothing on the Integration API, and a bearer key grants nothing on the panel API. A bearer request is a distinct kind of caller inside the server and can never be mistaken for a signed-in session.

## Base URL and versioning

Every endpoint lives under a single versioned base path:

```
https://panel.example.com/api/integration/v1
```

The `v1` in the path is the contract version. A future release that needs to change response shapes in an incompatible way would add `v2` beside it rather than change `v1` under existing clients. Within `v1`, changes are additive: new fields may appear, but existing fields keep their meaning.

## GET-only by construction

The router registers only `GET` handlers. This is not a convention that a handler could forget. Any other method on any path in this group returns `405 method_not_allowed` with an `Allow: GET` header, decided by the router itself before any handler runs.

```http
POST /api/integration/v1/players HTTP/1.1
Host: panel.example.com
Authorization: Bearer phk_a1b2c3d4_...
```

```http
HTTP/1.1 405 Method Not Allowed
Allow: GET
Cache-Control: no-store
Content-Type: application/json

{"error": {"code": "method_not_allowed", "message": "Method not allowed."}}
```

## The response envelope

Every successful response is a JSON object with a `data` member. Save-derived endpoints add freshness fields, and paginated endpoints add a cursor. There are three shapes:

Paginated, save-derived (`/players`, `/pals`):

```json
{
  "data": [],
  "lastParseAt": "2026-07-10T02:00:00Z",
  "formatDrift": false,
  "nextCursor": null
}
```

Save-derived, not paginated (`/players/{uid}`, `/guilds`):

```json
{
  "data": {},
  "lastParseAt": "2026-07-10T02:00:00Z",
  "formatDrift": false
}
```

Live-only (`/map`, `/server`, `/metrics/current`, `/world/summary`, `/world/workers`, `/events`):

```json
{
  "data": {}
}
```

- `data` is always present.
- `lastParseAt` is the time of the last completed save parse, or `null` if none has finished. It is your staleness signal for anything read from the save file. It appears only on save-derived endpoints.
- `formatDrift` is `true` when the last parse hit an unrecognized save layout and had to skip some data. It also appears only on save-derived endpoints.
- `nextCursor` appears only on paginated endpoints. See [Pagination and limits](/integration-api/pagination-and-limits/).

All timestamps are RFC 3339 in UTC. The ten endpoints and their full field lists are in [Endpoints](/integration-api/endpoints/).

## Transport and browser access

:::caution
Serve this API over TLS at the network edge and treat keys as passwords. A bearer key sent over plain HTTP is readable by anyone on the network path. Palhelm assumes it runs behind a trusted network edge that terminates TLS. That assumption is the operating posture, not a substitute for HTTPS on the key holder's side.
:::

There are no CORS headers on this surface, by design. A browser page on another origin cannot call the API directly. If you are building a browser dashboard, proxy the request through your own server so the bearer key never lives in browser-held JavaScript.

Every response also carries `Cache-Control: no-store`. Do not cache or persist a response body.

## Where to go next

- [Keys and redaction](/integration-api/keys-and-redaction/): creating and revoking keys, the key format, and exactly what is hidden from token responses.
- [Endpoints](/integration-api/endpoints/): all ten routes with example requests and responses.
- [Pagination and limits](/integration-api/pagination-and-limits/): cursors, conditional requests, rate limits, and the uniform 401.
- [OpenAPI spec](/integration-api/openapi/): the machine-readable contract and how to generate a client.
