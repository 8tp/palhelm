// Mock-level proofs for the Settings "Integration API" card (docs/specs/integration-api.md
// §9): create-once plaintext exposure, revoke idempotency, the 100-active-key cap, label
// validation, and viewer role gating. This exercises src/api/mock.ts only — a later agent
// owns the real-backend contract test (see tests/config-real-backend.test.mjs for that pattern).
import test from "node:test";
import assert from "node:assert/strict";

// mock.ts's session helpers read `sessionStorage`, which only exists in a browser. Install a
// minimal in-memory polyfill before importing the module (a static `import` would run before
// any top-of-file setup, so this must be a dynamic import after the polyfill is installed).
class MemoryStorage {
  values = new Map();
  getItem(key) {
    return this.values.has(key) ? this.values.get(key) : null;
  }
  setItem(key, value) {
    this.values.set(key, String(value));
  }
  removeItem(key) {
    this.values.delete(key);
  }
}
globalThis.sessionStorage = new MemoryStorage();

const mock = await import("../src/api/mock.ts");
const { ApiRequestError } = await import("../src/api/types.ts");

const KEY_FORMAT = /^phk_[0-9a-f]{8}_[A-Za-z0-9_-]{43}$/;

async function asAdmin() {
  await mock.login("admin");
}

async function asViewer() {
  await mock.login("viewer");
}

async function signedOut() {
  await mock.logout();
}

test.beforeEach(() => {
  mock.__resetIntegrationKeysForTests();
});

test("create returns the plaintext key exactly once, in spec shape, and list never carries it", async () => {
  await asAdmin();
  const created = await mock.createIntegrationKey("discord-bot");

  assert.match(created.key, KEY_FORMAT, "mock key must match phk_<8 hex>_<43 base64url>");
  assert.equal(created.key.length, 56);
  assert.equal(created.label, "discord-bot");
  assert.equal(created.revokedAt, null);
  assert.equal(created.lastUsedAt, null);

  const list = await mock.listIntegrationKeys();
  assert.equal(list.length, 1);
  const [entry] = list;
  assert.equal(entry.id, created.id);
  assert.equal("key" in entry, false, "list entries must never carry the plaintext key");
  assert.deepEqual(Object.keys(entry).sort(), ["createdAt", "id", "label", "lastUsedAt", "revokedAt"].sort());
});

test("label is trimmed, and empty/too-long/control-character labels are rejected with 400 invalid_request", async () => {
  await asAdmin();

  const padded = await mock.createIntegrationKey("  spaced-label  ");
  assert.equal(padded.label, "spaced-label");

  await assert.rejects(
    () => mock.createIntegrationKey("   "),
    (err) => err instanceof ApiRequestError && err.status === 400 && err.code === "invalid_request",
    "whitespace-only label must be rejected",
  );

  await assert.rejects(
    () => mock.createIntegrationKey("x".repeat(65)),
    (err) => err instanceof ApiRequestError && err.status === 400 && err.code === "invalid_request",
    "labels over 64 chars after trim must be rejected",
  );

  // The backend validates per-rune with Go's unicode.IsControl, which covers C0, DEL, and C1
  // controls — the mock must refuse the same inputs so mock-mode behavior matches production.
  for (const [label, kind] of [
    ["bad\x01label", "C0 control (U+0001)"],
    ["bad\x7flabel", "DEL (U+007F)"],
    ["bad\u0085label", "C1 control (U+0085 NEL)"],
    ["bad\u009flabel", "C1 control (U+009F)"],
  ]) {
    await assert.rejects(
      () => mock.createIntegrationKey(label),
      (err) => err instanceof ApiRequestError && err.status === 400 && err.code === "invalid_request",
      `${kind} must be rejected`,
    );
  }
});

test("revoke sets revokedAt and is idempotent on repeat calls; unknown id is 404 not_found", async () => {
  await asAdmin();
  const created = await mock.createIntegrationKey("saved-once");

  const revoked = await mock.revokeIntegrationKey(created.id);
  assert.ok(revoked.revokedAt, "revokedAt must be set");
  assert.equal(revoked.id, created.id);

  const revokedAgain = await mock.revokeIntegrationKey(created.id);
  assert.equal(revokedAgain.revokedAt, revoked.revokedAt, "revoking twice must return the original revokedAt (idempotent, 200)");

  const list = await mock.listIntegrationKeys();
  assert.equal(list.find((k) => k.id === created.id)?.revokedAt, revoked.revokedAt);

  await assert.rejects(
    () => mock.revokeIntegrationKey("00000000"),
    (err) => err instanceof ApiRequestError && err.status === 404 && err.code === "not_found",
  );
});

test("creation past the 100 active-key cap returns 409 too_many_keys, and a revoke frees a slot", async () => {
  await asAdmin();
  // Seed 99 directly (bypassing the mock's per-call latency) so the boundary itself — the
  // 100th create succeeding, the 101st failing — is still exercised through the real
  // createIntegrationKey path, without paying for 100 real round-trips.
  const seeded = mock.__seedActiveIntegrationKeysForTests(99);
  assert.equal((await mock.listIntegrationKeys()).length, 99);

  const hundredth = await mock.createIntegrationKey("key-100");
  assert.match(hundredth.key, KEY_FORMAT);
  assert.equal((await mock.listIntegrationKeys()).length, 100);

  await assert.rejects(
    () => mock.createIntegrationKey("one-too-many"),
    (err) => err instanceof ApiRequestError && err.status === 409 && err.code === "too_many_keys",
  );

  // Revoking one active key drops the active count below the cap, so creation succeeds again —
  // proves the cap counts active (non-revoked) keys only, matching §2.6.
  await mock.revokeIntegrationKey(seeded[0].id);
  const afterRevoke = await mock.createIntegrationKey("fits-again");
  assert.match(afterRevoke.key, KEY_FORMAT);
});

test("list is newest first", async () => {
  await asAdmin();
  const first = await mock.createIntegrationKey("first");
  const second = await mock.createIntegrationKey("second");
  const list = await mock.listIntegrationKeys();
  assert.deepEqual(
    list.map((k) => k.id),
    [second.id, first.id],
  );
});

test("viewer role is refused 403 forbidden on every integration-key operation", async () => {
  await asViewer();
  for (const call of [
    () => mock.listIntegrationKeys(),
    () => mock.createIntegrationKey("viewer-attempt"),
    () => mock.revokeIntegrationKey("00000000"),
  ]) {
    await assert.rejects(call, (err) => err instanceof ApiRequestError && err.status === 403 && err.code === "forbidden");
  }
});

test("unauthenticated calls are refused 401", async () => {
  await signedOut();
  await assert.rejects(
    () => mock.listIntegrationKeys(),
    (err) => err instanceof ApiRequestError && err.status === 401,
  );
});
