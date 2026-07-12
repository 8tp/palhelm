---
title: OpenAPI spec
description: The live OpenAPI 3.1 document, its bearer security scheme, and how to generate a typed client from it.
sidebar:
  order: 5
---

This page covers the machine-readable contract: where to fetch the OpenAPI document, what it describes, and how to turn it into a client library so you are not hand-writing request code.

## The live document

Palhelm serves an OpenAPI 3.1 document from a single public path:

```
https://panel.example.com/api/openapi.json
```

This endpoint needs no authentication, so you can fetch it with any tool:

```bash
curl https://panel.example.com/api/openapi.json -o palhelm-openapi.json
```

The document describes the whole Palhelm API, both the session-cookie panel routes and the bearer-token integration routes. The integration paths are the ones under `/api/integration/v1` plus the three key-management routes under `/api/v1/integration-keys`.

Because the document is public, it contains no secrets. Any example key value in it is deliberately fake and does not match the real key grammar, so it can never validate:

```
phk_00000000_EXAMPLE-NOT-A-REAL-KEY
```

## The bearer security scheme

The integration paths declare an HTTP bearer security scheme:

```json
"bearerAuth": {
  "type": "http",
  "scheme": "bearer",
  "description": "Integration API key, format phk_<8 lowercase hex>_<43 base64url chars> (56 chars total)."
}
```

Each integration operation references it:

```json
"/api/integration/v1/players": {
  "get": {
    "summary": "Keyset-paginated player roster",
    "security": [{"bearerAuth": []}]
  }
}
```

A generated client picks this up and gives you a place to set the bearer token once, rather than attaching the `Authorization` header on every call yourself. The panel routes use a separate cookie scheme, so a generator will present the two surfaces as distinct auth methods.

## What the document covers

For the integration surface the document is complete enough to drive a client end to end:

- Every path and its single `GET` operation, plus the `405` on non-GET methods.
- All query parameters (`limit`, `cursor`, `online`) and the `If-None-Match` header, with their bounds and validation rules.
- Full response schemas for every endpoint, including the envelope wrappers and the nested player, pal, guild, map, server, metrics, and event objects.
- The shared error envelope, referenced by every `4xx` response.
- The response headers that matter to a client: `ETag`, `Retry-After`, `WWW-Authenticate`, `Allow`, and `Cache-Control`.

## Generating a client

Any OpenAPI 3.1 generator works. The example below uses [OpenAPI Generator](https://openapi-generator.tech/), but tools like `oapi-codegen` (Go) or `openapi-typescript` (TypeScript) consume the same document.

Fetch the spec, then generate:

```bash
# 1. Save the live document
curl https://panel.example.com/api/openapi.json -o palhelm-openapi.json

# 2. Generate a TypeScript client
openapi-generator-cli generate \
  -i palhelm-openapi.json \
  -g typescript-fetch \
  -o ./palhelm-client
```

Swap `-g typescript-fetch` for another generator name to target a different language, for example `-g go`, `-g python`, or `-g rust`.

Then set the bearer token once and call the typed methods. The exact class and method names depend on the generator; the shape is:

```ts
import { Configuration, PlayersApi } from "./palhelm-client";

const config = new Configuration({
  basePath: "https://panel.example.com",
  accessToken: process.env.PALHELM_KEY, // phk_a1b2c3d4_...
});

const players = new PlayersApi(config);
const firstPage = await players.getPlayers({ limit: 100 });
```

:::caution
Keep the key in an environment variable or secret store, never hardcoded and never in browser-shipped code. If you are calling from a browser app, run the generated client on your own server and proxy the request, so the key never reaches the browser. See the CORS note in the [Overview](/integration-api/overview/).
:::

## Keeping a client current

Within `v1`, changes are additive: new fields may appear on responses, but existing fields keep their meaning, so a regenerated client stays compatible. Re-fetch `/api/openapi.json` and regenerate after a Palhelm upgrade to pick up new fields and endpoints. A breaking change would arrive under a new base path (`v2`), not as a silent change to `v1`.
