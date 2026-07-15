import assert from "node:assert/strict";
import { readFile } from "node:fs/promises";
import test from "node:test";

const workflowUrl = new URL("../../.github/workflows/ci.yml", import.meta.url);
const imageWorkflowUrl = new URL("../../.github/workflows/ghcr.yml", import.meta.url);

test("frontend CI prefetches Go fixture dependencies before its bounded test timeout", async () => {
  const workflow = await readFile(workflowUrl, "utf8");
  const frontendJob = workflow.split(/\n  frontend:\n/, 2)[1] ?? "";
  assert.match(frontendJob, /cache-dependency-path:\s*backend\/go\.sum/);
  assert.match(frontendJob, /working-directory:\s*backend\s*\n\s*run:\s*go mod download/);
  assert.doesNotMatch(frontendJob, /cache:\s*false/);
  assert.ok(frontendJob.indexOf("run: go mod download") < frontendJob.indexOf("run: npm test"));
});

test("workflows use the current Node 24 action majors", async () => {
  const [ci, image] = await Promise.all([
    readFile(workflowUrl, "utf8"),
    readFile(imageWorkflowUrl, "utf8"),
  ]);
  assert.match(ci, /actions\/checkout@v6/);
  assert.match(ci, /actions\/setup-node@v6/);
  assert.match(ci, /actions\/setup-go@v6/);
  assert.doesNotMatch(ci, /actions\/(?:checkout@v4|setup-node@v4|setup-go@v5)/);

  for (const action of [
    "actions/checkout@v6",
    "docker/setup-buildx-action@v4",
    "docker/login-action@v4",
    "docker/metadata-action@v6",
    "docker/build-push-action@v7",
    "actions/attest@v4",
  ]) {
    assert.match(image, new RegExp(action.replace("/", "\\/")));
  }
});
