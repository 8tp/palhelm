# Spec: frontend

React SPA in `frontend/`, embedded into the Go binary at build time. The app renders against a
fixed field-guide design system. API contract: `docs/API.md`.

## Stack (fixed, no substitutions)
- Vite + React 19 + TypeScript (strict). Router: `react-router` v7 (library mode, `BrowserRouter`).
- Data: TanStack Query v5. Charts: `uplot` (+ thin React wrapper written in-repo). No UI kit,
  no Tailwind, no CSS-in-JS.
- Styling: the design tokens and component styles live in `frontend/src/styles/`
  (`tokens.css`, `ui.css`); fonts are the woff2 files under `frontend/src/assets/fonts/`.
  Page-specific styles go in per-route CSS files reusing tokens. NEVER hardcode a color.
  Shared component patterns: toggle chip, code well, diff list, meter bar, pressed-button state.
- Icons: inline SVGs in `src/components/icons.tsx` (16px, stroke currentColor). Do not add an
  icon package.

## Structure
```
frontend/src/
  api/        client.ts (typed fetch, error envelope), types.ts (from docs/API.md), mock.ts
  app/        App.tsx, routes.tsx, AuthProvider (role context), useSSE hook
  components/ Shell (Rail, HelmStrip), Card, StatTile, Pill, Table, Tabs, Button, Field,
              EmptyState, Banner, Toast, ConfirmDialog, Chart (uplot wrapper), Sparkline,
              ToggleChip, CodeWell, DiffList, Meter, icons.tsx
  routes/     login, dashboard, players, console, map, backups, config, settings
  styles/     tokens.css, ui.css, per-route css
```

## Behavior
- `VITE_MOCK=1` (or `?mock` query param): `api/client.ts` routes every call to `mock.ts`,
  which returns realistic fixtures matching the mockup content (a demo server, 2/16 players,
  Kestrel/VossR/mika_o/HaruQ/tessellate, 7 guilds, backups list, config groups...) with
  believable latencies (50–200ms) and live-ish drift (fps wobbles each poll). This makes
  `npm run dev` fully browsable with no backend, and is how you verify your work.
- Auth: on load `GET /auth/session`; 401 → `/login`. Login posts password, stores role in
  context. Viewer role: destructive/mutating controls hidden.
- HelmStrip: polls `/metrics/current` every 5s (Query refetchInterval) + `/server` every 15s;
  SSE upgrade later (hook exists, falls back to polling). Broadcast/Save/Shut down buttons open
  dialogs (graceful-shutdown dialog: waittime + message + countdown toggle, admin only; Palhelm
  cannot start the server again — restart is external, see v0.3.0 release notes).
- Dashboard: stat row (fps, players, base camps, last backup), performance chart
  (`/metrics/history?window=1h|24h`, uPlot: fps line; separate small frame-time chart —
  never dual-axis), players-online chart (24h), server card (info + health pills), recent
  events. Loading states: skeleton shims on cards; error states: banner in the card, never
  a blank page.
- Every route exists from day one; phase A ships login + dashboard fully and the other routes
  as designed empty-state placeholders ("wired in phase B") using the real page chrome.

## Phase B (after phase-A review)
players (tabs, table, detail panel, kick/ban/unban dialogs, whitelist editor), console
(exec + history + saved commands), map (Leaflet-free: custom pan/zoom canvas or CSS-transform
tile layer reading `/data/map-tiles/{z}/{x}/{y}.png`, PST-compatible world→map transform from
docs/ARCHITECTURE.md, layer chips, empty state when tiles missing), backups (table, create,
browse contents drawer, restore dry-run flow with typed confirmation, schedule + storage cards),
config (grouped editor, pending-change state, footer bar, raw ini tab, apply dialog honoring
501-manual-command answer), settings (connections, save sync, auth readouts, theme switcher —
set `data-theme` on `<html>`, persist localStorage, "System" clears it).

## Quality bar
- `npm run build` clean; `tsc --noEmit` clean; no console errors in the browser.
- Both themes verified against the design reference; fix drift in spacing, colors, and type sizes.
- Keyboard: dialogs trap focus and close on Esc; tables' hover actions also reachable via a
  row kebab button (visible on focus).
- `prefers-reduced-motion` honored (pulse/settle animations off).
