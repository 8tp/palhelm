import assert from "node:assert/strict";
import test from "node:test";
import { readFile } from "node:fs/promises";

test("authenticated routes expose dedicated lazy Paldeck and guild pages", async () => {
  const app = await readFile(new URL("../src/app/App.tsx", import.meta.url), "utf8");
  const shell = await readFile(new URL("../src/components/Shell.tsx", import.meta.url), "utf8");
  assert.match(app, /path="paldeck"/);
  assert.match(app, /path="guilds\/:guildId"/);
  assert.match(shell, /to: "\/paldeck"/);
  assert.match(shell, /to: "\/guilds"/);
});

test("guild detail links members, bases, Pals, activity, and progression", async () => {
  const source = await readFile(new URL("../src/routes/guilds/Guilds.tsx", import.meta.url), "utf8");
  assert.match(source, /api\.guilds\.detail/);
  assert.match(source, /panel-observed/);
  assert.match(source, /current membership/);
  assert.match(source, /\/players\?player=/);
  assert.match(source, /\/paldeck\?player=/);
  assert.match(source, /\/map\?x=/);
  assert.match(source, /palExplorerHref/);
});

test("Paldeck screen distinguishes partial save observations from pinned progression", async () => {
  const source = await readFile(new URL("../src/routes/paldeck/Paldeck.tsx", import.meta.url), "utf8");
  assert.match(source, /playersWithCaptureCounts === data\.coverage\.playersTotal/);
  assert.match(source, /Species captured/);
  assert.match(source, /Unique species counter/);
  assert.match(source, /Missing data is never counted as zero/);
  assert.match(source, /api\.paldeck\.iconDataset/);
  assert.match(source, /Pal icons are not installed/);
  assert.match(source, /Unseen \(needs full data\)/);
});

test("Paldeck icon notice uses the container downloader command", async () => {
  const source = await readFile(new URL("../src/routes/paldeck/Paldeck.tsx", import.meta.url), "utf8");
  assert.match(source, /docker compose exec palhelm palhelm fetch-pal-icons/);
});

test("Paldeck icon notice uses the server-resolved Compose container command", async () => {
  const source = await readFile(new URL("../src/routes/paldeck/Paldeck.tsx", import.meta.url), "utf8");
  assert.match(source, /serverQuery\.data\?\.palIconsCommand/);
});
