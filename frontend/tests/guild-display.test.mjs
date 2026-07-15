import assert from "node:assert/strict";
import test from "node:test";
import { guildDisplayName, UNNAMED_GUILD_LABEL } from "../src/app/guildDisplay.ts";

test("a real save name passes straight through", () => {
  assert.equal(
    guildDisplayName({ name: "Sootside Collective", adminUid: "u1", members: [{ uid: "u1", name: "Ada" }] }),
    "Sootside Collective",
  );
});

test("an unnamed guild borrows the admin member's name", () => {
  const label = guildDisplayName({
    name: "",
    adminUid: "u2",
    members: [
      { uid: "u1", name: "Ada" },
      { uid: "u2", name: "Bex" },
    ],
  });
  assert.equal(label, "Bex's guild");
});

test("the literal default name is treated as unnamed and borrows a member", () => {
  const label = guildDisplayName({
    name: "Unnamed Guild",
    adminUid: "u2",
    members: [
      { uid: "u1", name: "Ada" },
      { uid: "u2", name: "Bex" },
    ],
  });
  assert.equal(label, "Bex's guild");
});

test("with no admin match it falls back to the first known member", () => {
  const label = guildDisplayName({
    name: null,
    adminUid: "missing",
    members: [
      { uid: "u1", name: "Ada" },
      { uid: "u2", name: "Bex" },
    ],
  });
  assert.equal(label, "Ada's guild");
});

test("blank member names are skipped when choosing a fallback", () => {
  const label = guildDisplayName({
    name: "",
    members: [
      { uid: "u1", name: "   " },
      { uid: "u2", name: "Cyd" },
    ],
  });
  assert.equal(label, "Cyd's guild");
});

test("an unnamed guild with zero known members stays Unnamed guild", () => {
  assert.equal(guildDisplayName({ name: "", members: [] }), UNNAMED_GUILD_LABEL);
  assert.equal(guildDisplayName({ name: "Unnamed Guild", members: [{ uid: "u1", name: "" }] }), UNNAMED_GUILD_LABEL);
  assert.equal(guildDisplayName({ name: null }), UNNAMED_GUILD_LABEL);
});
