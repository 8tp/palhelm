---
title: Security model
description: The trusted network edge, session-cookie auth, server-side role gating, no CORS, trusted-proxy handling, and an honest threat model.
sidebar:
  order: 5
---

This page covers how Palhelm is meant to be exposed, how it authenticates and authorizes
requests, how it keeps its two API surfaces apart, and what it does and does not protect
against. It is deliberately honest about the threat model.

## Trusted network edge

Palhelm assumes a trusted network edge, the same assumption the official Palworld REST
API makes about itself. It is designed to be published on a private interface: a VPN, a
tailnet, or localhost behind a reverse proxy you control. Do not expose it to the open
internet.

This is not a hedge you can skip. Palhelm holds the game admin password and can stop the
server, manage backups, and edit the Compose file. Its defenses are built for a trusted
edge, not for a hostile public one.

## Session-cookie authentication

Panel logins use two roles: `admin` for full control and `viewer` for read-only. Both
passwords come from environment variables (`PALHELM_ADMIN_PASSWORD`, and the optional
`PALHELM_VIEWER_PASSWORD`). On login, Palhelm issues a session JWT in an HttpOnly cookie.

- The session token is never stored in `localStorage` and is not readable from
  JavaScript.
- The game admin password stays in the Palhelm process at all times. It never reaches
  the browser. Both game passwords are write-only placeholders in the web app: you can
  set them, you cannot read them back.
- Sensitive config and auth responses are marked non-cacheable.
- The `Secure` cookie flag can be forced with `PALHELM_SECURE_COOKIES` when TLS is
  terminated upstream. Direct TLS and trusted forwarded HTTPS are also detected.

## Role gating on the server

Every mutating endpoint is gated by role on the server. The web app also adapts, so a
viewer does not see destructive controls, but the enforcement is server-side. A viewer
who crafts a request by hand still cannot perform an admin action. The UI adapting is a
convenience, not the control.

## Two API surfaces that never cross

Palhelm has two HTTP surfaces, and they are kept apart structurally, not just by policy:

- The panel API at `/api/v1/*` is session-cookie territory. This includes Integration
  API key management at `/api/v1/integration-keys`, which stays admin-only.
- The Integration API at `/api/integration/v1/*` is a separate, GET-only chi sub-router
  with its own middleware chain: parse the bearer token, validate it in constant time,
  apply a per-key rate limit, then reach the handler. It authenticates before routing and
  returns a uniform 401.

These are backed by two distinct principal types stored under different context keys. A
bearer request carries no cookie identity, so a session-gated handler can never see it as
authenticated. A session request carries no bearer token. Because the bearer group is its
own sub-router, scope creep from the Integration API into the panel API is structurally
impossible rather than a rule someone has to remember.

Integration keys have the form `phk_<id>_<secret>`. Only a SHA-256 hash is stored, and
the plaintext is shown exactly once at creation. Revoking a key takes effect on the very
next request.

## No CORS

Palhelm never sets any `Access-Control-Allow-*` header. There is no CORS support
anywhere. A browser-based dashboard hosted on another origin cannot call the Integration
API directly from JavaScript; it must proxy the calls through its own server. This is
intentional. It keeps keys off the public web and out of browser code.

## Trusted-proxy handling

By default, Palhelm ignores forwarded client-IP and forwarded-protocol headers. It only
trusts them when the transport peer's address falls inside the CIDR ranges listed in
`PALHELM_TRUSTED_PROXIES`. This prevents a client from spoofing its source address or
pretending the connection is HTTPS by setting a header. The login rate limiter's state is
bounded and expires, so it cannot be used to exhaust memory.

## Backup and config safety

Two operator features touch the host, and both fail closed:

- Backups resolve the active world from the REST API's normalized world GUID and refuse
  to act on a mismatch or ambiguity. Restore runs a dry-run diff first, then a guided
  swap that requires an explicit confirmation that the server is stopped. Imported
  archives are bounded by entry count, per-entry size, total expanded size, safe path
  checks, and verified copied bytes.
- The config editor does not need or accept a Docker socket. It edits only the targeted
  environment keys in the Compose file, using a content hash as a compare-and-swap token
  so external edits are rejected rather than overwritten, and it verifies that only the
  requested keys changed before an atomic rename. Applying a change still requires an
  operator to run the printed command from the host; Palhelm does not run Docker Compose
  itself.

## Honest threat model

What Palhelm defends against:

- A client on the trusted network trying to act above its role. Server-side role gating
  and the two-principal split handle this.
- A leaked Integration API key. Keys are read-only, redacted so they expose no platform
  ids, no live player position, and no moderation state, hashed at rest, rate-limited,
  and revocable on the next request. A pasted key is safe enough to sit in a public
  channel, though you should still revoke it.
- Spoofed forwarding headers, header-based cache poisoning of sensitive responses, and
  oversized backup imports.

What Palhelm does not defend against, and does not claim to:

- Exposure to the open internet. The trusted-edge assumption is load-bearing.
- A compromised admin session. An attacker with a live admin session can do what an admin
  can do, including using the config editor as a write primitive against the Compose file.
- The security of the game server itself, or of the host it runs on. Those are the
  operator's responsibility.

No secrets live in the repository or the image. All runtime configuration comes from
environment variables, and backups and the SQLite file live in the mounted data volume.
