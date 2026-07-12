---
title: Pagination and limits
description: Keyset pagination with cursors, conditional requests with ETags, per-key rate limits, and the uniform 401.
sidebar:
  order: 4
---

This page covers the mechanics a polling client needs to get right: how to walk a paginated endpoint with cursors, how to skip re-downloading unchanged data with ETags, how the rate limit behaves, and why every authentication failure looks identical.

## Keyset pagination

Two endpoints are paginated: `/players` (ordered by `uid`) and `/pals` (ordered by `instanceId`). Both use keyset cursors rather than page numbers or offsets. A cursor points at a stable, immutable key, so a save re-parse that deletes and reinserts rows in the middle of your walk cannot make you skip or repeat a row that existed the whole time.

Two query parameters control a page:

- `limit`: page size. Default 100, minimum 1, maximum 500. Out of range or non-integer returns `400 invalid_limit`.
- `cursor`: an opaque value from the previous page's `nextCursor`. Treat it as a black box; never build one by hand. An undecodable, wrong-version, or wrong-charset cursor returns `400 invalid_cursor`. An empty `?cursor=` is treated as absent, meaning the first page.

### Walking the pages

Request the first page with no cursor:

```bash
curl -H "Authorization: Bearer phk_a1b2c3d4_..." \
  "https://panel.example.com/api/integration/v1/players?limit=100"
```

The response ends with a `nextCursor`:

```json
{
  "data": [ "... 100 players ..." ],
  "lastParseAt": "2026-07-10T02:00:00Z",
  "formatDrift": false,
  "nextCursor": "djF8YjgwZDQ3ZjFhOWMyNGU2ZmIzNWM4MTkwMmFkN2U2ZjQ"
}
```

Pass that value back as `cursor` to get the next page:

```bash
curl -H "Authorization: Bearer phk_a1b2c3d4_..." \
  "https://panel.example.com/api/integration/v1/players?limit=100&cursor=djF8YjgwZDQ3ZjFhOWMyNGU2ZmIzNWM4MTkwMmFkN2U2ZjQ"
```

Keep going until `nextCursor` is `null`. That is the only stop signal.

### The nextCursor rule

`nextCursor` is always the key of the last row **returned**, and it is `null` whenever the page returned fewer than `limit` rows. This has one consequence worth knowing: when the total number of rows is an exact multiple of `limit`, the last full page still comes back with a non-null cursor, and the page after it is empty:

```json
{"data": [], "lastParseAt": "2026-07-10T02:00:00Z", "formatDrift": false, "nextCursor": null}
```

An empty `data` array is always paired with a `null` cursor. A non-null cursor with an empty `data` array never happens. So the rule "stop when `nextCursor` is `null`" is always correct, and an empty final page is a normal, valid response.

### The online filter

`/players` accepts `?online=true` to return only players the poller currently sees online. The keyset contract is unchanged: same ordering, same cursor, same stop rule. Any value other than `true` returns `400 invalid_request`. When nobody is online, the endpoint returns `{"data": [], "nextCursor": null, ...}` immediately.

### Consistency across a walk

Each individual page is internally consistent. A save re-parse swaps the whole table in a single transaction, so no page ever shows you a half-replaced table. Across pages, any row that exists for the entire walk is returned exactly once, and no such row is ever duplicated. Rows created or deleted while you are paginating may or may not appear. That is the standard keyset contract.

If you need a single consistent snapshot, watch `lastParseAt` on each page. If it changes mid-walk, a re-parse landed between your requests; restart the walk to get a clean snapshot.

## Conditional requests with ETags

Every `200` carries a weak `ETag`, which is a content hash of the response body:

```http
HTTP/1.1 200 OK
ETag: W/"3f1a9c8b2d4e6f0a1b2c3d4e5f60718"
Cache-Control: no-store
Content-Type: application/json
```

Hold onto that value. On your next poll of the same resource, send it back in `If-None-Match`:

```bash
curl -H "Authorization: Bearer phk_a1b2c3d4_..." \
  -H 'If-None-Match: W/"3f1a9c8b2d4e6f0a1b2c3d4e5f60718"' \
  https://panel.example.com/api/integration/v1/metrics/current
```

If the body has not changed, you get a bodyless `304` with the same `ETag`:

```http
HTTP/1.1 304 Not Modified
ETag: W/"3f1a9c8b2d4e6f0a1b2c3d4e5f60718"
Cache-Control: no-store
```

`If-None-Match` accepts a comma-separated list of tags, and a bare `*` matches any current representation. Comparison is weak (byte-equal opaque tags, `W/` prefix ignored). The `ETag` appears on `200` and `304` only, never on `4xx` or `5xx`.

:::note
A `304` saves you bandwidth, not server work. The server still runs the query behind the resource, so a conditional request still counts against your rate limit. For a bot polling on a timer this is still a clear win, because it skips re-downloading a large body when nothing changed.
:::

Note that `Cache-Control: no-store` and ETag revalidation are not in conflict. `no-store` tells caches not to store the body; the ETag is an application-level revalidation channel your client drives explicitly by holding the last tag in memory.

## Rate limits

Each key is limited to 60 requests per minute by default, as a sliding window. An operator can change this with the `PALHELM_INTEGRATION_RATE_LIMIT` environment variable.

The limit is keyed on the key id, not the client IP. Spoofing `X-Forwarded-For` or `X-Real-IP` has no effect, because the limit follows the token, not the network address. Two different keys have independent budgets; exhausting one does not touch the other.

Exceeding the limit returns `429` with a `Retry-After` header:

```http
HTTP/1.1 429 Too Many Requests
Retry-After: 37
Cache-Control: no-store
Content-Type: application/json

{"error": {"code": "rate_limited", "message": "API key rate limit exceeded; retry later."}}
```

`Retry-After` is integer seconds. Honor it: it is the time until the oldest request in your window ages out. Backing off on the header turns a hammering loop into polite polling. A well-behaved client stays well under the limit and uses ETags to make each poll cheap.

## The uniform 401

Every authentication failure returns the exact same response. Missing header, malformed header, unknown key id, wrong secret, and revoked key all produce:

```http
HTTP/1.1 401 Unauthorized
WWW-Authenticate: Bearer
Cache-Control: no-store
Content-Type: application/json

{"error": {"code": "unauthorized", "message": "A valid API key is required."}}
```

This uniformity is deliberate. If a revoked key produced a different response from an unknown key, an attacker could use that difference to learn which key ids exist. Because authentication runs before routing, the 401 is also identical whether or not the path you requested exists, so probing cannot map the surface either. There is no response variant to distinguish, so there is no oracle to probe.

When your key is valid but the path is not, you get the normal `404 not_found` instead, since you have passed authentication.
