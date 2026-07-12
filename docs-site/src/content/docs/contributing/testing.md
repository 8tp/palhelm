---
title: Testing
description: The Go backend tests, the bot's vitest suite, the frontend checks, and what CI should run.
sidebar:
  order: 3
---

This page covers the test suites in Palhelm: the Go backend tests, the frontend checks,
the bot's vitest suite, and a suggestion for what continuous integration should run.

## Backend tests

The backend has the bulk of the test coverage. Run it with the make target or directly:

```sh
make test
# which runs, for the backend:
cd backend && go vet ./... && go test ./...
```

Notable areas:

- The save parser has hermetic tests. Small real save fixtures are copied into
  `backend/internal/sav/testdata/`, so the tests run without any network access or game
  server. They assert on concrete facts: that a known save decompresses to an exact byte
  length beginning with `GVAS`, that parsing yields the expected guild and player counts,
  and that no input causes a panic. A benchmark on a real save guards the bounded-memory
  goal, so a regression that starts decoding the large opaque blobs would show up as a
  memory jump.
- The container header logic has unit tests over small synthetic fixtures covering the
  zlib, Oodle, and chunk-prefixed variants.
- The store has migration tests that exercise the ordered migrations and the version
  tracking, so an additive migration cannot silently break an upgrade or a rollback.
- The server has integration tests over the HTTP surface, including auth, role gating,
  the Integration API redaction and scope boundary, and the OpenAPI spec.

`go vet` runs as part of `make test`, and the code is expected to be gofmt clean.

## Frontend checks

`make test` also runs a TypeScript type check on the frontend:

```sh
cd frontend && npx tsc --noEmit
```

The frontend also has its own scripts: `npm run lint` runs oxlint, and `npm test` runs the
Node test files under `frontend/tests/`.

## Bot tests

The Discord bot is a separate Node project with a vitest suite:

```sh
cd bot
npm install
npm test         # vitest run
npm run typecheck
```

The suite covers the command registry, the assistant tool loop, the Pal knowledge data,
the Integration API client, notifications, and history handling. The tests use fixtures
and do not call Discord or a live panel.

## What CI should run

A full check across the three projects is:

- Backend: `go vet ./...` and `go test ./...`.
- Frontend: `tsc --noEmit`, and the lint and Node tests if you want them enforced.
- Bot: `npm test` and `npm run typecheck`.

`make test` covers the backend and the frontend type check in one command. The bot runs
on its own because it is a separate project with its own dependencies.
