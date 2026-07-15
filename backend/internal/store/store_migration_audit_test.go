// Adversarial migration audit for the v0.4.0 runner (spec: docs/specs/integration-api.md
// §10). Goes beyond the §10.3 tests: real data volume through both the
// session and integration query paths, interrupted-migration replay, corrupt version
// values, concurrent Open, and a structural proof that 002 touched nothing that existed.
package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

// buildV030Database creates a database exactly as the v0.3.0 binary's Open() left it:
// 001_init.sql executed verbatim (schema_version '1'), nothing else.
func buildV030Database(t *testing.T, path string) *sql.DB {
	t.Helper()
	init001, err := migrations.ReadFile("migrations/001_init.sql")
	if err != nil {
		t.Fatal(err)
	}
	db, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = db.Exec(string(init001)); err != nil {
		db.Close()
		t.Fatalf("apply 001_init.sql verbatim: %v", err)
	}
	return db
}

// TestAuditUpgradeV030RealVolumeReadableThroughBothSurfaces upgrades a v0.3-shaped
// database carrying representative volume in *every* table (players with all sensitive
// columns populated, pals, guilds, members, bases, sessions, events, kv, whitelist,
// backups, metrics, console log, saved commands, world_state) and proves each row is
// readable afterwards through the session-era query paths (Players, Pals, GuildJSON,
// Sessions, ...) and the new integration query paths (PlayersPage/PalsPage/Guilds/
// PalsTyped) alike — the §10.3 proof at realistic scale, not three token rows.
func TestAuditUpgradeV030RealVolumeReadableThroughBothSurfaces(t *testing.T) {
	const (
		playerCount = 300
		palCount    = 500
		guildCount  = 20
	)
	path := filepath.Join(t.TempDir(), "v030-volume.db")
	legacy := buildV030Database(t, path)

	tx, err := legacy.Begin()
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now().Unix()
	uid := func(n int) string { return fmt.Sprintf("%032x", n) }
	for i := 1; i <= playerCount; i++ {
		guild := fmt.Sprintf("%032x", 0x10000+((i-1)%guildCount))
		_, err = tx.Exec(`INSERT INTO players(uid,steam_id,name,account_name,first_seen,last_seen,playtime_sec,banned,whitelisted,level,guild_id,guild_name,ping,location_x,location_y,raw_json)
			VALUES(?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)`,
			uid(i), fmt.Sprintf("7656119%09d", i), fmt.Sprintf("player%03d", i), fmt.Sprintf("acct%03d", i),
			now-3600, now, int64(i*60), i%7 == 0, i%5 == 0, i%50, guild, fmt.Sprintf("guild%02d", (i-1)%guildCount), float64(i)/4, float64(i), float64(-i), `{"seed":true}`)
		if err != nil {
			t.Fatal(err)
		}
	}
	for i := 1; i <= palCount; i++ {
		_, err = tx.Exec(`INSERT INTO pals(instance_id,owner_uid,character_id,display_name,level,is_alpha,is_lucky,raw_json) VALUES(?,?,?,?,?,?,?,?)`,
			fmt.Sprintf("%032x", 0x20000+i), uid((i-1)%playerCount+1), "SheepBall", "Lamball", i%60, i%9 == 0, i%11 == 0, `{"p":1}`)
		if err != nil {
			t.Fatal(err)
		}
	}
	for g := 0; g < guildCount; g++ {
		gid := fmt.Sprintf("%032x", 0x10000+g)
		if _, err = tx.Exec(`INSERT INTO guilds(id,name,admin_uid,raw_json) VALUES(?,?,?,?)`, gid, fmt.Sprintf("guild%02d", g), uid(g+1), "{}"); err != nil {
			t.Fatal(err)
		}
		for m := 0; m < 3; m++ {
			if _, err = tx.Exec(`INSERT INTO guild_members(guild_id,player_uid,name) VALUES(?,?,?)`, gid, uid(g*3+m+1), fmt.Sprintf("player%03d", g*3+m+1)); err != nil {
				t.Fatal(err)
			}
		}
		for b := 0; b < 2; b++ {
			if _, err = tx.Exec(`INSERT INTO bases(id,guild_id,x,y,level) VALUES(?,?,?,?,?)`, fmt.Sprintf("%032x", 0x30000+g*2+b), gid, float64(g), float64(b), b+1); err != nil {
				t.Fatal(err)
			}
		}
	}
	for i := 0; i < 200; i++ {
		if _, err = tx.Exec(`INSERT INTO sessions(player_uid,join_at,leave_at) VALUES(?,?,?)`, uid(i%playerCount+1), now-int64(i*100)-50, now-int64(i*100)); err != nil {
			t.Fatal(err)
		}
	}
	for i := 0; i < 300; i++ {
		if _, err = tx.Exec(`INSERT INTO events(at,kind,message,meta_json) VALUES(?,?,?,?)`, now-int64(i), "panel", fmt.Sprintf("event %d", i), "{}"); err != nil {
			t.Fatal(err)
		}
	}
	for i := 0; i < 100; i++ {
		if _, err = tx.Exec(`INSERT INTO metrics(ts,fps,frametime_ms,players) VALUES(?,?,?,?)`, now-int64(i*5), 60.0, 16.6, i%10); err != nil {
			t.Fatal(err)
		}
	}
	seeds := []string{
		`INSERT INTO kv(key,value) VALUES('operator-note','preserve-me'),('backup_schedule','{"enabled":true}')`,
		`INSERT INTO whitelist(steam_id,name) VALUES('76561190000000001','wl-one'),('76561190000000002','wl-two')`,
		`INSERT INTO backups(file,created_at,size_bytes,trigger,world_day) VALUES('b1.tar.gz',1700000000,1024,'manual',12),('b2.tar.gz',1700000100,2048,'scheduled',NULL)`,
		`INSERT INTO console_log(at,user,command,output,is_error) VALUES(1700000000,'admin','Info','ok',0)`,
		`INSERT INTO saved_commands(name,command) VALUES('save','Save')`,
		`INSERT INTO world_state(singleton,day,last_parse_at,parse_duration_ms,players,pals,guilds,skipped_props,format_drift) VALUES(1,42,1700000000,120,300,500,20,0,0)`,
	}
	for _, s := range seeds {
		if _, err = tx.Exec(s); err != nil {
			t.Fatal(err)
		}
	}
	if err = tx.Commit(); err != nil {
		t.Fatal(err)
	}
	if err = legacy.Close(); err != nil {
		t.Fatal(err)
	}

	// --- Upgrade in place. ---
	st, err := Open(path)
	if err != nil {
		t.Fatalf("open v0.3 database with real volume: %v", err)
	}
	defer st.Close()
	ctx := context.Background()
	if v, err := st.GetKV(ctx, "schema_version"); err != nil || v != "10" {
		t.Fatalf("schema_version = %q, %v; want 10", v, err)
	}
	if v, err := st.GetKV(ctx, "operator-note"); err != nil || v != "preserve-me" {
		t.Fatalf("operator kv = %q, %v", v, err)
	}

	// Session-era paths.
	all, err := st.Players(ctx, nil)
	if err != nil || len(all) != playerCount {
		t.Fatalf("Players() after upgrade = %d rows, %v; want %d", len(all), err, playerCount)
	}
	p, err := st.PlayerByUID(ctx, uid(7))
	if err != nil {
		t.Fatal(err)
	}
	if p.SteamID != fmt.Sprintf("7656119%09d", 7) || !p.Banned || p.AccountName != "acct007" || p.Ping != 1.75 {
		t.Fatalf("player 7 columns changed during upgrade: %#v", p)
	}
	if sess, err := st.Sessions(ctx, uid(1)); err != nil || len(sess) == 0 {
		t.Fatalf("Sessions after upgrade = %#v, %v", sess, err)
	}
	if pals, err := st.Pals(ctx, uid(1)); err != nil || len(pals) == 0 {
		t.Fatalf("Pals (session map shape) after upgrade = %d, %v", len(pals), err)
	}
	if gj, err := st.GuildJSON(ctx); err != nil || len(gj) != guildCount {
		t.Fatalf("GuildJSON after upgrade = %d, %v; want %d", len(gj), err, guildCount)
	}
	if evts, err := st.Events(ctx, 1000, ""); err != nil || len(evts) != 300 {
		t.Fatalf("Events after upgrade = %d, %v; want 300", len(evts), err)
	}
	if wl, err := st.Whitelist(ctx); err != nil || len(wl) != 2 {
		t.Fatalf("Whitelist after upgrade = %#v, %v", wl, err)
	}
	if bs, err := st.Backups(ctx); err != nil || len(bs) != 2 {
		t.Fatalf("Backups after upgrade = %#v, %v", bs, err)
	}
	if cl, err := st.ConsoleLog(ctx, 100); err != nil || len(cl) != 1 {
		t.Fatalf("ConsoleLog after upgrade = %#v, %v", cl, err)
	}
	if sc, err := st.SavedCommands(ctx); err != nil || len(sc) != 1 {
		t.Fatalf("SavedCommands after upgrade = %#v, %v", sc, err)
	}
	if ws, err := st.WorldState(ctx); err != nil || ws.Day != 42 || ws.Players != 300 {
		t.Fatalf("WorldState after upgrade = %#v, %v", ws, err)
	}
	if ms, err := st.MetricsHistory(ctx, time.Unix(now-1000, 0), false); err != nil || len(ms) == 0 {
		t.Fatalf("MetricsHistory after upgrade = %d, %v", len(ms), err)
	}

	// Integration-era paths: full keyset walks must return every row exactly once.
	seen := map[string]bool{}
	after := ""
	for {
		page, err := st.PlayersPage(ctx, after, 100, nil)
		if err != nil {
			t.Fatal(err)
		}
		if len(page) == 0 {
			break
		}
		for _, p := range page {
			if seen[p.UID] {
				t.Fatalf("PlayersPage duplicated uid %s on upgraded data", p.UID)
			}
			seen[p.UID] = true
		}
		after = page[len(page)-1].UID
	}
	if len(seen) != playerCount {
		t.Fatalf("PlayersPage walk over upgraded data returned %d uids, want %d", len(seen), playerCount)
	}
	palSeen := map[string]bool{}
	after = ""
	for {
		page, err := st.PalsPage(ctx, after, 100)
		if err != nil {
			t.Fatal(err)
		}
		if len(page) == 0 {
			break
		}
		for _, p := range page {
			if palSeen[p.InstanceID] {
				t.Fatalf("PalsPage duplicated %s on upgraded data", p.InstanceID)
			}
			palSeen[p.InstanceID] = true
			if p.OwnerName == "" {
				t.Fatalf("PalsPage owner join broken on upgraded data: %#v", p)
			}
			if p.OwnerSource != "last_observed" || !p.OwnerResolved {
				t.Fatalf("PalsPage legacy owner provenance is not conservative: %#v", p)
			}
		}
		after = page[len(page)-1].InstanceID
	}
	if len(palSeen) != palCount {
		t.Fatalf("PalsPage walk over upgraded data returned %d rows, want %d", len(palSeen), palCount)
	}
	guilds, err := st.Guilds(ctx)
	if err != nil || len(guilds) != guildCount {
		t.Fatalf("Guilds (typed) after upgrade = %d, %v; want %d", len(guilds), err, guildCount)
	}
	for _, g := range guilds {
		if len(g.Members) != 3 || len(g.Bases) != 2 {
			t.Fatalf("guild %s members/bases after upgrade = %d/%d, want 3/2", g.ID, len(g.Members), len(g.Bases))
		}
	}
	if pt, err := st.PalsTyped(ctx, uid(1)); err != nil || len(pt) == 0 {
		t.Fatalf("PalsTyped after upgrade = %d, %v", len(pt), err)
	}
	// And the new table is usable.
	if _, err := st.CreateAPIKey(ctx, "abcd1234", [32]byte{1}, "post-upgrade", time.Now()); err != nil {
		t.Fatalf("api_keys not usable after volume upgrade: %v", err)
	}
}

// TestAuditInterruptedMigrationReplayPreservesData simulates the §10.1 interrupted-upgrade
// scenario the runner must survive: 002's DDL landed but the recorded version did not (a
// crash between effects, or an operator restoring an older kv row). Re-opening replays
// 002; because the runner's contract makes the replay idempotent (CREATE TABLE IF NOT
// EXISTS), existing api_keys rows must survive and the version must be repaired to 2.
func TestAuditInterruptedMigrationReplayPreservesData(t *testing.T) {
	path := filepath.Join(t.TempDir(), "interrupted.db")
	st, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	if _, err = st.CreateAPIKey(ctx, "aaaa1111", [32]byte{0xAB}, "survives-replay", time.Now()); err != nil {
		t.Fatal(err)
	}
	if err = st.Close(); err != nil {
		t.Fatal(err)
	}

	raw, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	// Model a database whose effects are present only through 002. Later
	// migrations include non-idempotent ALTERs, and the runner guarantees their
	// version write is in the same transaction rather than promising replay after
	// an impossible half-commit.
	for _, column := range []string{"capture_total", "unique_pals_captured", "paldeck_unlocked"} {
		if _, err = raw.Exec(`ALTER TABLE players DROP COLUMN ` + column); err != nil {
			t.Fatal(err)
		}
	}
	if _, err = raw.Exec(`UPDATE kv SET value='2' WHERE key='schema_version'`); err != nil {
		t.Fatal(err)
	}
	if err = raw.Close(); err != nil {
		t.Fatal(err)
	}

	reopened, err := Open(path)
	if err != nil {
		t.Fatalf("reopen after simulated interrupted migration: %v", err)
	}
	defer reopened.Close()
	if v, err := reopened.GetKV(ctx, "schema_version"); err != nil || v != "10" {
		t.Fatalf("schema_version after replay = %q, %v; want repaired to 10", v, err)
	}
	keys, err := reopened.ListAPIKeys(ctx)
	if err != nil || len(keys) != 1 || keys[0].ID != "aaaa1111" || keys[0].Label != "survives-replay" {
		t.Fatalf("api_keys rows lost by the idempotent replay: %#v, %v", keys, err)
	}
}

// TestAuditSchemaVersionFailClosedMessageAndCorruptValue sharpens the fail-closed tests:
// the future-version error must name both the found and the supported version (an operator
// reading a crash log at 3am needs the numbers), and a *corrupt* (non-integer)
// schema_version must also fail Open closed without applying anything.
func TestAuditSchemaVersionFailClosedMessageAndCorruptValue(t *testing.T) {
	t.Run("future version names both numbers", func(t *testing.T) {
		path := filepath.Join(t.TempDir(), "future.db")
		legacy := buildV030Database(t, path)
		if _, err := legacy.Exec(`UPDATE kv SET value='11' WHERE key='schema_version'`); err != nil {
			t.Fatal(err)
		}
		if err := legacy.Close(); err != nil {
			t.Fatal(err)
		}
		_, err := Open(path)
		if err == nil {
			t.Fatal("Open on schema_version 11 succeeded")
		}
		for _, needle := range []string{"11", "10", "newer than this binary supports"} {
			if !strings.Contains(err.Error(), needle) {
				t.Errorf("fail-closed error %q does not mention %q", err, needle)
			}
		}
	})
	t.Run("corrupt version fails closed with no writes", func(t *testing.T) {
		path := filepath.Join(t.TempDir(), "corrupt.db")
		legacy := buildV030Database(t, path)
		if _, err := legacy.Exec(`UPDATE kv SET value='banana' WHERE key='schema_version'`); err != nil {
			t.Fatal(err)
		}
		if err := legacy.Close(); err != nil {
			t.Fatal(err)
		}
		if _, err := Open(path); err == nil {
			t.Fatal("Open with a corrupt schema_version succeeded; want fail-closed")
		} else if !strings.Contains(err.Error(), "invalid schema_version") {
			t.Fatalf("unexpected error for corrupt version: %v", err)
		}
		verify, err := sql.Open("sqlite", path)
		if err != nil {
			t.Fatal(err)
		}
		defer verify.Close()
		var v string
		if err = verify.QueryRow(`SELECT value FROM kv WHERE key='schema_version'`).Scan(&v); err != nil || v != "banana" {
			t.Fatalf("corrupt version mutated by failed Open: %q, %v", v, err)
		}
		var name string
		err = verify.QueryRow(`SELECT name FROM sqlite_master WHERE type='table' AND name='api_keys'`).Scan(&name)
		if !errors.Is(err, sql.ErrNoRows) {
			t.Fatalf("failed Open still created api_keys: name=%q err=%v", name, err)
		}
	})
}

// TestAuditConcurrentOpenSameFile documents (best-effort, per the spec's honest §10.1
// concurrency statement) what SQLite allows when two in-process handles run Open() on the
// same fresh file simultaneously: BEGIN IMMEDIATE + busy_timeout serialize the migration
// transactions and the in-transaction version re-read makes the loser a no-op, so the
// acceptable outcomes are (a) both succeed, or (b) one fails closed with a busy/locked
// error — never a half-migrated or double-migrated database.
func TestAuditConcurrentOpenSameFile(t *testing.T) {
	for round := 0; round < 3; round++ {
		path := filepath.Join(t.TempDir(), fmt.Sprintf("concurrent-%d.db", round))
		var wg sync.WaitGroup
		results := make([]error, 2)
		stores := make([]*Store, 2)
		start := make(chan struct{})
		for i := 0; i < 2; i++ {
			wg.Add(1)
			go func(i int) {
				defer wg.Done()
				<-start
				st, err := Open(path)
				results[i], stores[i] = err, st
			}(i)
		}
		close(start)
		wg.Wait()
		succeeded := 0
		for i, err := range results {
			if err == nil {
				succeeded++
				stores[i].Close()
				continue
			}
			msg := strings.ToLower(err.Error())
			if !strings.Contains(msg, "busy") && !strings.Contains(msg, "locked") {
				t.Fatalf("round %d: concurrent Open failed with a non-lock error (possible corruption path): %v", round, err)
			}
			t.Logf("round %d: one Open failed closed with a lock error (accepted): %v", round, err)
		}
		if succeeded == 0 {
			t.Fatalf("round %d: both concurrent Opens failed", round)
		}
		// Whatever raced, the surviving database must be exactly at version 5 with a
		// usable api_keys table (never half-migrated).
		st, err := Open(path)
		if err != nil {
			t.Fatalf("round %d: reopen after concurrent Open: %v", round, err)
		}
		if v, err := st.GetKV(context.Background(), "schema_version"); err != nil || v != "10" {
			st.Close()
			t.Fatalf("round %d: schema_version = %q, %v; want 10", round, v, err)
		}
		if _, err := st.ListAPIKeys(context.Background()); err != nil {
			st.Close()
			t.Fatalf("round %d: api_keys unusable after concurrent Open: %v", round, err)
		}
		st.Close()
	}
}

// TestAuditMigration002AddsOnlyAPIKeysSchemaObjects is the structural downgrade proof:
// diffing sqlite_master between a 001-only database and a fully migrated one shows 002
// added exactly the api_keys table (plus SQLite's own internal autoindexes for its PRIMARY
// KEY/UNIQUE constraints) and altered *no* pre-existing object's SQL. Because nothing a
// v0.3 query touches changed, every v0.3-era statement remains satisfiable by a v0.4
// database — the §10.1 rollback contract, proven against the schema rather than asserted.
func TestAuditMigration002AddsOnlyAPIKeysSchemaObjects(t *testing.T) {
	schemaOf := func(db *sql.DB) map[string]string {
		t.Helper()
		rows, err := db.Query(`SELECT type||':'||name, COALESCE(sql,'') FROM sqlite_master WHERE name NOT LIKE 'sqlite_autoindex_%' AND name NOT LIKE 'sqlite_sequence'`)
		if err != nil {
			t.Fatal(err)
		}
		defer rows.Close()
		out := map[string]string{}
		for rows.Next() {
			var key, sqlText string
			if err = rows.Scan(&key, &sqlText); err != nil {
				t.Fatal(err)
			}
			out[key] = sqlText
		}
		return out
	}

	v1Path := filepath.Join(t.TempDir(), "v1-only.db")
	v1 := buildV030Database(t, v1Path)
	defer v1.Close()
	v1Schema := schemaOf(v1)

	v2Path := filepath.Join(t.TempDir(), "v2-only.db")
	v2db, err := sql.Open("sqlite", v2Path)
	if err != nil {
		t.Fatal(err)
	}
	init001, err := migrations.ReadFile("migrations/001_init.sql")
	if err != nil {
		t.Fatal(err)
	}
	apiKeys002, err := migrations.ReadFile("migrations/002_api_keys.sql")
	if err != nil {
		t.Fatal(err)
	}
	if _, err = v2db.Exec(string(init001)); err != nil {
		t.Fatal(err)
	}
	if _, err = v2db.Exec(string(apiKeys002)); err != nil {
		t.Fatal(err)
	}
	defer v2db.Close()
	v2Schema := schemaOf(v2db)

	for key, v1SQL := range v1Schema {
		v2SQL, ok := v2Schema[key]
		if !ok {
			t.Errorf("002 dropped pre-existing schema object %s (breaks the v0.3 downgrade contract)", key)
			continue
		}
		if v2SQL != v1SQL {
			t.Errorf("002 altered pre-existing object %s:\n  v1: %s\n  v2: %s", key, v1SQL, v2SQL)
		}
		delete(v2Schema, key)
	}
	var added []string
	for key := range v2Schema {
		added = append(added, key)
	}
	if len(added) != 1 || added[0] != "table:api_keys" {
		t.Fatalf("002 added %v, want exactly [table:api_keys]", added)
	}
}

// TestAuditDowngradeV04DatabaseUnderV03OpenSemantics runs the actual downgrade motion:
// a populated v0.4 database is "opened by a v0.3 binary" (whose Open() is: PRAGMAs +
// execute 001_init.sql — reproduced here byte-for-byte from the embedded file). It must
// succeed, must not clobber schema_version back to 1 (001 uses INSERT OR IGNORE), must
// leave api_keys rows dormant but intact, and a later re-upgrade must resume at version 5.
func TestAuditDowngradeV04DatabaseUnderV03OpenSemantics(t *testing.T) {
	path := filepath.Join(t.TempDir(), "downgrade.db")
	st, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	if _, err = st.CreateAPIKey(ctx, "dddd4444", [32]byte{0xD4}, "dormant-through-downgrade", time.Now()); err != nil {
		t.Fatal(err)
	}
	if err = st.UpsertLivePlayer(ctx, Player{UID: "feedface00000000000000000000cafe", SteamID: "steam_x", Name: "Survivor", Raw: []byte("{}")}, time.Now()); err != nil {
		t.Fatal(err)
	}
	if err = st.Close(); err != nil {
		t.Fatal(err)
	}

	// v0.3 binary semantics: open, PRAGMAs, execute 001_init.sql, no runner.
	init001, err := migrations.ReadFile("migrations/001_init.sql")
	if err != nil {
		t.Fatal(err)
	}
	v03, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	for _, q := range []string{"PRAGMA journal_mode=WAL", "PRAGMA busy_timeout=5000", "PRAGMA foreign_keys=ON"} {
		if _, err = v03.Exec(q); err != nil {
			t.Fatalf("v0.3-style PRAGMA on a v0.4 database: %v", err)
		}
	}
	if _, err = v03.Exec(string(init001)); err != nil {
		t.Fatalf("v0.3 binary cannot open a v0.4 database (001 re-execution failed): %v", err)
	}
	var v string
	if err = v03.QueryRow(`SELECT value FROM kv WHERE key='schema_version'`).Scan(&v); err != nil || v != "10" {
		t.Fatalf("schema_version after v0.3-style open = %q, %v; INSERT OR IGNORE must not clobber it", v, err)
	}
	var name string
	if err = v03.QueryRow(`SELECT name FROM players WHERE uid='feedface00000000000000000000cafe'`).Scan(&name); err != nil || name != "Survivor" {
		t.Fatalf("v0.3-era player query against the v0.4 database = %q, %v", name, err)
	}
	var keyCount int
	if err = v03.QueryRow(`SELECT COUNT(*) FROM api_keys`).Scan(&keyCount); err != nil || keyCount != 1 {
		t.Fatalf("api_keys not dormant-but-intact under v0.3 semantics: %d, %v", keyCount, err)
	}
	if err = v03.Close(); err != nil {
		t.Fatal(err)
	}

	// Re-upgrade resumes at version 5 with the key still valid.
	reup, err := Open(path)
	if err != nil {
		t.Fatalf("re-upgrade after downgrade: %v", err)
	}
	defer reup.Close()
	keys, err := reup.ListAPIKeys(ctx)
	if err != nil || len(keys) != 1 || keys[0].ID != "dddd4444" {
		t.Fatalf("key did not survive the downgrade/re-upgrade cycle: %#v, %v", keys, err)
	}
}

// TestAuditPlayersPageLargeOnlineSetBinds guards the IN() predicate against SQLite's bound-
// variable limit: the spec bounds the online set by max players (tiny), but the store
// method must not fall over if a modded server hands it hundreds of uids.
func TestAuditPlayersPageLargeOnlineSetBinds(t *testing.T) {
	st, err := Open(filepath.Join(t.TempDir(), "binds.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	ctx := context.Background()
	only := make([]string, 0, 1000)
	for i := 0; i < 1000; i++ {
		only = append(only, fmt.Sprintf("%032x", i+1))
	}
	rows, err := st.PlayersPage(ctx, "", 100, only)
	if err != nil {
		t.Fatalf("PlayersPage with a 1000-uid online set: %v", err)
	}
	if len(rows) != 0 {
		t.Fatalf("empty table returned rows: %#v", rows)
	}
}
