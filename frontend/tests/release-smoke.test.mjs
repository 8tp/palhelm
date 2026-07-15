import assert from "node:assert/strict";
import test from "node:test";
import { smokePanel } from "../../scripts/smoke-release.mjs";

const adminSecret = "admin-secret-sentinel";
const integrationSecret = "phk_secret-sentinel";

function response(value, init = {}) {
  return new Response(JSON.stringify(value), {
    status: 200,
    ...init,
    headers: { "Content-Type": "application/json", ...init.headers },
  });
}

test("release smoke covers public, session, and Integration read-only contracts without logging secrets", async () => {
  const requests = [];
  const logs = [];
  const fetchImpl = async (url, init) => {
    const parsed = new URL(url);
    requests.push({ path: `${parsed.pathname}${parsed.search}`, init });
    if (parsed.pathname === "/api/v1/auth/login") {
      assert.deepEqual(JSON.parse(init.body), { password: adminSecret });
      return response({ role: "admin" }, { headers: { "Set-Cookie": "palhelm_session=test-token; Path=/; HttpOnly" } });
    }
    if (parsed.pathname === "/api/openapi.json") {
      return response({ paths: { "/api/v1/server": {}, "/api/integration/v1/server": {} } });
    }
    return response({ data: [] });
  };

  await smokePanel({
    baseUrl: "https://panel.example.test/root-is-ignored",
    adminPassword: adminSecret,
    integrationKey: integrationSecret,
    fetchImpl,
    log: (line) => logs.push(line),
  });

  assert.equal(requests.length, 19);
  assert.ok(requests.some((item) => item.path === "/healthz"));
  assert.ok(requests.some((item) => item.path === "/api/v1/world/snapshot"));
  assert.ok(requests.some((item) => item.path === "/api/v1/pals?limit=1"));
  assert.ok(requests.some((item) => item.path === "/api/integration/v1/pals?limit=1"));
  const session = requests.find((item) => item.path === "/api/v1/auth/session");
  assert.equal(session.init.headers.Cookie, "palhelm_session=test-token");
  const integration = requests.find((item) => item.path === "/api/integration/v1/server");
  assert.equal(integration.init.headers.Authorization, `Bearer ${integrationSecret}`);
  const renderedLogs = logs.join("\n");
  assert.doesNotMatch(renderedLogs, new RegExp(adminSecret));
  assert.doesNotMatch(renderedLogs, new RegExp(integrationSecret));
  assert.match(renderedLogs, /release smoke checks passed/i);
});

test("release smoke failures expose only the check and status, never response bodies", async () => {
  const secretBody = "private-upstream-body";
  await assert.rejects(
    smokePanel({
      baseUrl: "https://panel.example.test",
      adminPassword: adminSecret,
      integrationKey: integrationSecret,
      fetchImpl: async () => new Response(secretBody, { status: 503 }),
      log: () => {},
    }),
    (error) => {
      assert.match(error.message, /health failed: HTTP 503/);
      assert.doesNotMatch(error.message, new RegExp(secretBody));
      return true;
    },
  );
});
