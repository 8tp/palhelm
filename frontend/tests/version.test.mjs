import assert from "node:assert/strict";
import { readFile } from "node:fs/promises";
import test from "node:test";

const rootUrl = new URL("../../", import.meta.url);

async function read(path) {
  return readFile(new URL(path, rootUrl), "utf8");
}

test("panel version stays consistent across release artifacts", async () => {
  const version = (await read("VERSION")).trim();
  const frontendPackage = JSON.parse(await read("frontend/package.json"));
  const frontendLock = JSON.parse(await read("frontend/package-lock.json"));
  const dockerfile = await read("Dockerfile");
  const mockApi = await read("frontend/src/api/mock.ts");

  assert.match(version, /^\d+\.\d+\.\d+$/);
  assert.equal(frontendPackage.version, version);
  assert.equal(frontendLock.version, version);
  assert.equal(frontendLock.packages[""].version, version);
  assert.match(dockerfile, new RegExp(`ARG VERSION=${version.replaceAll(".", "\\.")}`));
  assert.match(mockApi, new RegExp(`panelVersion: "${version.replaceAll(".", "\\.")}"`));
});
