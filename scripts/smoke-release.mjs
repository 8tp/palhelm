#!/usr/bin/env node

import { pathToFileURL } from "node:url";

const USER_AGENT = "Palhelm-Release-Smoke/0.6";

/**
 * Run read-only release checks against a deliberately selected Palhelm instance.
 * Secrets are accepted as values by callers but are never included in output or errors.
 */
export async function smokePanel({ baseUrl, adminPassword, integrationKey, fetchImpl = fetch, log = console.log }) {
  const base = normalizedBaseUrl(baseUrl);
  let sessionCookie = "";

  const request = async (label, path, { method = "GET", body, auth = "none" } = {}) => {
    const headers = { Accept: "application/json", "User-Agent": USER_AGENT };
    if (body !== undefined) headers["Content-Type"] = "application/json";
    if (auth === "session") {
      if (!sessionCookie) throw new Error(`${label} failed: no authenticated session`);
      headers.Cookie = sessionCookie;
    }
    if (auth === "integration") headers.Authorization = `Bearer ${integrationKey}`;

    let response;
    try {
      response = await fetchImpl(new URL(path, base), {
        method,
        headers,
        body: body === undefined ? undefined : JSON.stringify(body),
        redirect: "error",
        signal: AbortSignal.timeout(10_000),
      });
    } catch {
      throw new Error(`${label} failed: request error`);
    }
    if (!response.ok) throw new Error(`${label} failed: HTTP ${response.status}`);
    const contentType = response.headers.get("content-type") ?? "";
    if (!contentType.toLowerCase().includes("application/json")) {
      throw new Error(`${label} failed: expected JSON`);
    }
    let value;
    try {
      value = JSON.parse(await response.text());
    } catch {
      throw new Error(`${label} failed: malformed JSON`);
    }
    log(`[ok] ${label}`);
    return { response, value };
  };

  await request("health", "/healthz");
  const openapi = await request("OpenAPI", "/api/openapi.json");
  if (!openapi.value?.paths?.["/api/v1/server"] || !openapi.value?.paths?.["/api/integration/v1/server"]) {
    throw new Error("OpenAPI failed: required release paths are absent");
  }

  const login = await request("admin login", "/api/v1/auth/login", {
    method: "POST",
    body: { password: adminPassword },
  });
  const setCookies = typeof login.response.headers.getSetCookie === "function"
    ? login.response.headers.getSetCookie()
    : [login.response.headers.get("set-cookie")].filter(Boolean);
  sessionCookie = (setCookies[0] ?? "").split(";", 1)[0];
  if (!sessionCookie) throw new Error("admin login failed: session cookie absent");

  for (const [label, path] of [
    ["session", "/api/v1/auth/session"],
    ["server", "/api/v1/server"],
    ["server health", "/api/v1/server/health"],
    ["players", "/api/v1/players"],
    ["Pal explorer", "/api/v1/pals?limit=1"],
    ["backups", "/api/v1/backups"],
    ["configuration", "/api/v1/config"],
    ["save diagnostics", "/api/v1/world"],
    ["Game Data diagnostics", "/api/v1/world/snapshot"],
  ]) await request(label, path, { auth: "session" });

  for (const [label, path] of [
    ["Integration server", "/api/integration/v1/server"],
    ["Integration players", "/api/integration/v1/players?limit=1"],
    ["Integration Pals", "/api/integration/v1/pals?limit=1"],
    ["Integration guilds", "/api/integration/v1/guilds"],
    ["Integration events", "/api/integration/v1/events?limit=1"],
    ["Integration world summary", "/api/integration/v1/world/summary"],
    ["Integration workers", "/api/integration/v1/world/workers"],
  ]) await request(label, path, { auth: "integration" });

  log("Palhelm release smoke checks passed.");
}

function normalizedBaseUrl(raw) {
  let value;
  try { value = new URL(raw); } catch { throw new Error("invalid PALHELM_SMOKE_BASE_URL"); }
  if (!/^https?:$/.test(value.protocol) || value.username || value.password) {
    throw new Error("invalid PALHELM_SMOKE_BASE_URL");
  }
  value.pathname = value.pathname.replace(/\/+$/, "") + "/";
  value.search = "";
  value.hash = "";
  return value;
}

async function main() {
  const required = ["PALHELM_SMOKE_BASE_URL", "PALHELM_SMOKE_ADMIN_PASSWORD", "PALHELM_SMOKE_INTEGRATION_KEY"];
  const missing = required.filter((name) => !process.env[name]);
  if (missing.length > 0) {
    console.error(`Missing required environment variables: ${missing.join(", ")}`);
    process.exitCode = 2;
    return;
  }
  try {
    await smokePanel({
      baseUrl: process.env.PALHELM_SMOKE_BASE_URL,
      adminPassword: process.env.PALHELM_SMOKE_ADMIN_PASSWORD,
      integrationKey: process.env.PALHELM_SMOKE_INTEGRATION_KEY,
    });
  } catch (error) {
    console.error(error instanceof Error ? error.message : "release smoke failed");
    process.exitCode = 1;
  }
}

if (process.argv[1] && pathToFileURL(process.argv[1]).href === import.meta.url) await main();
