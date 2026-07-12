---
title: Development
description: Backend and frontend dev loops, mock mode, the bot, and the make targets that build and test Palhelm.
sidebar:
  order: 1
---

This page covers how to run Palhelm while working on it: the backend and frontend dev
loops, the web app's mock mode, the Discord bot, and the make targets that build and
test everything. For where files live, see [project layout](/contributing/project-layout/).
For what the test suites do, see [testing](/contributing/testing/).

## Prerequisites

- Go, matching the version in `backend/go.mod`.
- Node, matching the version used by the Docker build.
- Docker, if you want to build or run the shipping image.

## Make targets

The `Makefile` at the repository root drives the common tasks:

```sh
make build      # build the frontend, then the Go binary with the app embedded
make test       # go vet, go test, and a frontend type check
make docker      # build the shipping Docker image
```

`make build` builds the frontend with Vite, copies the result into the backend's embed
directory, then compiles a single static binary at `./palhelm` with `CGO_ENABLED=0`. The
web app is embedded, so the one binary serves both the API and the app.

## Backend dev loop

To run the backend directly against real Palworld endpoints, use the run target:

```sh
make dev-backend       # runs: go run ./cmd/palhelm serve
```

The server needs the Palworld REST URL, admin password, RCON address, and save directory
from environment variables to do anything useful. See the configuration reference in the
getting-started guide for the full list. The server listens on `:8080` by default.

## Frontend dev loop and mock mode

The web app can run on its own against mock data, with no backend and no game server. This
is the fastest loop for UI work:

```sh
cd frontend && npm run dev -- --port 5199
```

Then open the app with the mock flag:

```text
http://localhost:5199/?mock
```

In mock mode the app serves a fixed roster and fixture data, so you can work on every
screen offline. Log in with the documented mock passwords: `admin` for the admin role and
`viewer` for the read-only role. The mock roster is the set of fictional players used
throughout these docs: Kestrel, VossR, mika_o, HaruQ, and tessellate.

There is also a plain dev target that starts Vite without pinning a port:

```sh
make dev-frontend      # runs: npm run dev
```

## Building and running the image

```sh
make docker
```

The Docker build is multi-stage: it builds the frontend, builds the Go binary with the
frontend embedded, and produces a small Alpine runtime image. The runtime image adds
`gcompat` and `libstdc++` so the process can load the glibc Oodle library it downloads at
run time. The container runs as a non-root user, exposes port 8080, mounts `/data` as a
volume, and has a healthcheck against `/healthz`. To run Palhelm as an operator would,
see the install guide in getting-started.

## The Discord bot

The Discord bot lives in `bot/` and is a separate Node project with its own dependencies
and scripts:

```sh
cd bot
npm install
npm test         # run the vitest suite
npm run typecheck
npm run register # register slash commands with Discord
npm start        # run the bot
```

The bot talks to Palhelm only through the Integration API bearer token and, for a few
administrative features, the session API. Configure it by copying `.env.example` to
`.env` and filling in the values; each variable is documented in the file. It needs no
privileged Discord intents. The bot setup and configuration pages cover this in full.
