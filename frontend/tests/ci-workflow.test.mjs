import assert from "node:assert/strict";
import { readFile } from "node:fs/promises";
import test from "node:test";

const workflowUrl = new URL("../../.github/workflows/ci.yml", import.meta.url);

test("frontend CI prefetches Go fixture dependencies before its bounded test timeout", async () => {
  const workflow = await readFile(workflowUrl, "utf8");
  const frontendJob = workflow.split(/\n  frontend:\n/, 2)[1] ?? "";
  assert.match(frontendJob, /cache-dependency-path:\s*backend\/go\.sum/);
  assert.match(frontendJob, /working-directory:\s*backend\s*\n\s*run:\s*go mod download/);
  assert.doesNotMatch(frontendJob, /cache:\s*false/);
  assert.ok(frontendJob.indexOf("run: go mod download") < frontendJob.indexOf("run: npm test"));
});
