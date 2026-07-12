import test from "node:test";
import assert from "node:assert/strict";
import { spawn } from "node:child_process";
import { createInterface } from "node:readline";
import { fileURLToPath } from "node:url";
import { QueryClient } from "@tanstack/react-query";
import { parseConfigDoc, setConfigCache } from "../src/api/configContract.ts";

async function startBackend(t) {
  const binary = process.env.PALHELM_CONTRACT_SERVER_BIN;
  // detached: the `go run` path re-execs the fixture as a grandchild; killing the whole
  // process group is the only way to release its stdout pipe so node:test can exit.
  const child = binary
    ? spawn(binary, [], { detached: true, stdio: ["ignore", "pipe", "pipe"] })
    : spawn("go", ["run", "./internal/server/testfixture"], {
        cwd: fileURLToPath(new URL("../../backend", import.meta.url)),
        detached: true,
        stdio: ["ignore", "pipe", "pipe"],
      });
  let stderr = "";
  child.stderr.on("data", (chunk) => {
    stderr += chunk;
  });
  const lines = createInterface({ input: child.stdout });
  t.after(() => {
    lines.close();
    try {
      process.kill(-child.pid, "SIGKILL");
    } catch {
      child.kill("SIGKILL");
    }
  });
  let timeoutID;
  const timeout = new Promise((_, reject) => {
    timeoutID = setTimeout(() => reject(new Error(`backend fixture timeout: ${stderr}`)), 30_000);
  });
  const ready = (async () => {
    for await (const line of lines) {
      if (line.startsWith("LISTEN ")) return line.slice("LISTEN ".length);
    }
    throw new Error(`backend fixture exited before listening: ${stderr}`);
  })();
  try {
    return await Promise.race([ready, timeout]);
  } finally {
    clearTimeout(timeoutID);
  }
}

test("frontend Config contract and cache use the real backend document", async (t) => {
  const base = await startBackend(t);
  const login = await fetch(`${base}/api/v1/auth/login`, {
    method: "POST",
    headers: { "content-type": "application/json" },
    body: JSON.stringify({ password: "panelpass" }),
  });
  assert.equal(login.status, 200);
  const cookie = login.headers.get("set-cookie")?.split(";", 1)[0];
  assert.ok(cookie);

  const get = await fetch(`${base}/api/v1/config`, { headers: { cookie } });
  assert.equal(get.status, 200);
  const initial = parseConfigDoc(await get.json());
  assert.equal(initial.source, "compose");
  assert.ok(initial.version);
  assert.equal(initial.settings.find((setting) => setting.key === "ADMIN_PASSWORD")?.value, "•••");

  const put = await fetch(`${base}/api/v1/config`, {
    method: "PUT",
    headers: { cookie, "content-type": "application/json" },
    body: JSON.stringify({ version: initial.version, changes: { PLAYERS: 24 } }),
  });
  assert.equal(put.status, 200);
  const updated = parseConfigDoc(await put.json());

  const queryClient = new QueryClient();
  setConfigCache(queryClient, updated);
  const cached = queryClient.getQueryData(["config"]);
  assert.ok(!Array.isArray(cached), "successful PUT must not install a bare settings array");
  assert.ok(Array.isArray(cached.settings));
  assert.equal(cached.settings.find((setting) => setting.key === "PLAYERS")?.value, 24);
  assert.throws(() => parseConfigDoc(updated.settings), /not an object|does not match/);
});
