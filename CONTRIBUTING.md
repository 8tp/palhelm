# Contributing to Palhelm

Thanks for helping. This page covers the dev setup and how to run the checks CI runs. The contributing section at [docs.palhelm.com](https://docs.palhelm.com) has more detail on the dev loops, mock mode, and project layout.

## Prerequisites

- Go, matching the version in `backend/go.mod` (currently 1.26).
- Node 24 (the version the Docker build uses).
- Docker, only if you want to build or run the shipping image.

## Build and test

The `Makefile` at the repo root drives the common tasks:

```sh
make build      # build the frontend, then the Go binary with the app embedded
make test       # go vet, go test, and a frontend type check
make docker     # build the shipping Docker image
```

Run the suites directly:

```sh
# backend
cd backend && go vet ./... && go test ./...

# frontend
cd frontend && npm ci
npm run build        # tsc -b && vite build
npm run lint         # oxlint
npm test             # node --test; one test spawns a Go fixture, so Go must be installed
npm run test:smoke   # Playwright smoke: boots Vite in mock mode, visits every nav route
                     # at two viewports, asserts no console errors and no horizontal overflow.
                     # Kept out of `npm test`; needs a Chromium browser: npx playwright install chromium
```

Go code is expected to be gofmt clean.

## Dev loops

Backend against a real server (needs the Palworld REST URL, admin password, RCON address, and save dir in env vars):

```sh
make dev-backend    # go run ./cmd/palhelm serve
```

Frontend on its own, against mock data, no backend or game server needed. This is the fastest loop for UI work:

```sh
cd frontend && npm run dev -- --port 5199
# then open http://localhost:5199/?mock
# mock logins: "admin" (admin) and "viewer" (read-only)
```

## Pull requests

- Keep PRs focused. One change per PR is easier to review.
- Run `make test` before pushing. CI runs the backend tests and the frontend build.
- New behavior should come with a test, especially in the save parser, the store, and the HTTP surface.
- Plain English in docs and UI copy. No hype. Honest about limits.

## Security issues

Do not open a public issue for a vulnerability. See [SECURITY.md](SECURITY.md).
