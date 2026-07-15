// Playwright visual/console smoke harness for the Palhelm panel.
//
// Manual Playwright sweeps over mock mode caught every real layout bug this cycle
// (overlapping absolutely-positioned rows, clipped labels, hidden markers). This
// makes that check permanent and cheap: it boots Vite in mock mode, logs in through
// the real UI, then visits every nav route at two viewports and asserts each page
//   - emits no pageerror and no console.error,
//   - renders meaningful content (a real <main>/content region), and
//   - never overflows the document horizontally (the overlap/clipping tripwire).
//
// Route list is derived from the single source of truth — NAV_ITEMS in
// components/Shell.tsx — via Vite's ssrLoadModule, never hardcoded, so a new nav
// entry is smoke-tested automatically and this file can't drift.
//
// Run: npm run test:smoke  (not part of `npm test` — keep unit tests fast).

import { fileURLToPath } from "node:url";
import { dirname, resolve } from "node:path";
import { mkdir, rm } from "node:fs/promises";
import { createServer } from "vite";
import { chromium } from "playwright";

const HERE = dirname(fileURLToPath(import.meta.url));
const FRONTEND_ROOT = resolve(HERE, "..", "..");
const OUTPUT_DIR = resolve(HERE, "output");
const PORT = 51789; // fixed, uncommon port; strictPort so a clash fails loudly
const ORIGIN = `http://localhost:${PORT}`;

const VIEWPORTS = [
  { name: "desktop", width: 1440, height: 940 },
  { name: "narrow", width: 700, height: 940 },
];

// Console messages that are benign in dev/mock mode go here, each with a comment
// justifying why. Keep this list empty unless a message is provably not a bug.
const CONSOLE_ERROR_ALLOWLIST = [
  // (none)
];

function isAllowed(text) {
  return CONSOLE_ERROR_ALLOWLIST.some((rx) => rx.test(text));
}

async function deriveRoutes(server) {
  // Import NAV_ITEMS straight from the app so the route set can never drift from
  // the rail/command-palette source of truth. `/login` is added explicitly because
  // it lives outside the authenticated nav.
  const shell = await server.ssrLoadModule("/src/components/Shell.tsx");
  const navRoutes = shell.NAV_ITEMS.map((item) => item.to);
  return ["/login", ...navRoutes];
}

async function login(page) {
  await page.goto(`${ORIGIN}/login?mock`, { waitUntil: "domcontentloaded" });
  await page.fill("#pw", "admin");
  await Promise.all([
    page.waitForURL((url) => new URL(url).pathname === "/", { timeout: 15000 }),
    page.click('button[type="submit"]'),
  ]);
}

async function settle(page, route) {
  // Mock calls resolve with 150-350ms simulated latency (see api/mock.ts), and lazy
  // route chunks show a `.route-loader` Suspense fallback. Wait for the loader to
  // clear and for skeletons to resolve so layout is final before we measure it.
  await page
    .waitForFunction(() => !document.querySelector(".route-loader"), { timeout: 15000 })
    .catch(() => {});
  const selector = route === "/login" ? ".login-card" : "main.content";
  await page.waitForSelector(selector, { state: "visible", timeout: 15000 });
  await page
    .waitForFunction(() => document.querySelectorAll(".skel").length === 0, { timeout: 5000 })
    .catch(() => {});
}

async function checkPage(page, route) {
  const failures = [];
  const selector = route === "/login" ? ".login-card" : "main.content";

  // meaningful content
  const contentLen = await page.evaluate((sel) => {
    const el = document.querySelector(sel);
    return el ? (el.textContent || "").trim().length : -1;
  }, selector);
  if (contentLen < 1) {
    failures.push(`no meaningful content in "${selector}" (textContent length ${contentLen})`);
  }

  // no horizontal document overflow — the overlap/clipping tripwire
  const overflow = await page.evaluate(() => ({
    scrollWidth: document.documentElement.scrollWidth,
    innerWidth: window.innerWidth,
  }));
  if (overflow.scrollWidth > overflow.innerWidth + 1) {
    failures.push(
      `horizontal overflow: scrollWidth ${overflow.scrollWidth} > innerWidth ${overflow.innerWidth} + 1`,
    );
  }

  return failures;
}

async function run() {
  await rm(OUTPUT_DIR, { recursive: true, force: true });
  await mkdir(OUTPUT_DIR, { recursive: true });

  // VITE_MOCK=1 makes the app route every API call to the in-memory fixture; the
  // per-visit `?mock` query param is a belt-and-suspenders guarantee for each full
  // page load (USE_MOCK is evaluated once per document from either signal).
  process.env.VITE_MOCK = "1";

  const server = await createServer({
    root: FRONTEND_ROOT,
    logLevel: "warn",
    server: { port: PORT, strictPort: true },
  });
  await server.listen();

  const routes = await deriveRoutes(server);
  console.log(`Smoke: ${routes.length} routes x ${VIEWPORTS.length} viewports on ${ORIGIN}`);

  const browser = await chromium.launch();
  const allFailures = [];

  try {
    for (const vp of VIEWPORTS) {
      const context = await browser.newContext({ viewport: { width: vp.width, height: vp.height } });
      const page = await context.newPage();

      // Per-page console/pageerror capture. Re-pointed at each route below.
      let sink = [];
      page.on("pageerror", (err) => sink.push(`pageerror: ${err.message}`));
      page.on("console", (msg) => {
        if (msg.type() === "error" && !isAllowed(msg.text())) {
          sink.push(`console.error: ${msg.text()}`);
        }
      });

      await login(page);

      for (const route of routes) {
        sink = [];
        const label = `${vp.name} ${route}`;
        await page.goto(`${ORIGIN}${route}?mock`, { waitUntil: "domcontentloaded" });
        await settle(page, route);

        const failures = [...sink, ...(await checkPage(page, route))];

        const shot = resolve(OUTPUT_DIR, `${vp.name}--${route.replace(/\W+/g, "_") || "root"}.png`);
        await page.screenshot({ path: shot, fullPage: true });

        if (failures.length) {
          allFailures.push(...failures.map((f) => `[${label}] ${f}`));
          console.error(`FAIL ${label}`);
          for (const f of failures) console.error(`      - ${f}`);
        } else {
          console.log(`ok   ${label}`);
        }
      }

      await context.close();
    }
  } finally {
    await browser.close();
    await server.close();
  }

  if (allFailures.length) {
    console.error(`\nSmoke FAILED with ${allFailures.length} issue(s). Screenshots in ${OUTPUT_DIR}`);
    process.exit(1);
  }
  console.log(`\nSmoke PASSED. Screenshots in ${OUTPUT_DIR}`);
}

run().catch((err) => {
  console.error("Smoke harness crashed:", err);
  process.exit(1);
});
