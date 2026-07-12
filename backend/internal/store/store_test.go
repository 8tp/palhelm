package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/palhelm/palhelm/internal/sav"
)

func TestUpgradeFromV020DatabasePreservesData(t *testing.T) {
	path := filepath.Join(t.TempDir(), "palhelm-v0.2.0.db")
	legacy, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	// These are the v0.2.0 tables and columns whose operator data must survive an in-place open.
	for _, statement := range []string{
		`CREATE TABLE kv (key TEXT PRIMARY KEY, value TEXT NOT NULL)`,
		`CREATE TABLE players (uid TEXT PRIMARY KEY, steam_id TEXT NOT NULL DEFAULT '', name TEXT NOT NULL DEFAULT '', account_name TEXT NOT NULL DEFAULT '', first_seen INTEGER, last_seen INTEGER, playtime_sec INTEGER NOT NULL DEFAULT 0, banned INTEGER NOT NULL DEFAULT 0, whitelisted INTEGER NOT NULL DEFAULT 0, level INTEGER NOT NULL DEFAULT 0, guild_id TEXT NOT NULL DEFAULT '', guild_name TEXT NOT NULL DEFAULT '', ping REAL NOT NULL DEFAULT 0, location_x REAL, location_y REAL, raw_json TEXT NOT NULL DEFAULT '{}')`,
		`INSERT INTO kv(key,value) VALUES('schema_version','1'),('operator-note','preserve-me')`,
		`INSERT INTO players(uid,steam_id,name,level,raw_json) VALUES('legacyplayer','steam_123','Legacy',42,'{}')`,
	} {
		if _, err = legacy.Exec(statement); err != nil {
			legacy.Close()
			t.Fatal(err)
		}
	}
	if err = legacy.Close(); err != nil {
		t.Fatal(err)
	}

	upgraded, err := Open(path)
	if err != nil {
		t.Fatalf("open v0.2.0 database with v0.3.0: %v", err)
	}
	defer upgraded.Close()
	ctx := context.Background()
	if value, getErr := upgraded.GetKV(ctx, "operator-note"); getErr != nil || value != "preserve-me" {
		t.Fatalf("operator KV after upgrade = %q, %v", value, getErr)
	}
	player, err := upgraded.PlayerByUID(ctx, "legacyplayer")
	if err != nil {
		t.Fatal(err)
	}
	if player.Name != "Legacy" || player.Level != 42 || player.SteamID != "steam_123" {
		t.Fatalf("legacy player changed during upgrade: %#v", player)
	}
}

func TestPlayerQueriesIncludeCurrentOpenSessionPlaytimeWithoutMutatingIt(t *testing.T) {
	s, err := Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	ctx := context.Background()
	now := time.Now().UTC()
	if err = s.UpsertLivePlayer(ctx, Player{UID: "active-player", Name: "Active"}, now); err != nil {
		t.Fatal(err)
	}
	joined := now.Add(-2 * time.Hour)
	if err = s.StartSession(ctx, "active-player", joined); err != nil {
		t.Fatal(err)
	}

	player, err := s.PlayerByUID(ctx, "active-player")
	if err != nil {
		t.Fatal(err)
	}
	if player.PlaytimeSec < 2*60*60-2 || player.PlaytimeSec > 2*60*60+2 {
		t.Fatalf("active playtime = %d, want about 7200 seconds", player.PlaytimeSec)
	}
	page, err := s.PlayersPage(ctx, "", 10, nil)
	if err != nil || len(page) != 1 || page[0].PlaytimeSec != player.PlaytimeSec {
		t.Fatalf("paged active playtime = %#v, %v; want %d", page, err, player.PlaytimeSec)
	}
	sessions, err := s.Sessions(ctx, "active-player")
	if err != nil || len(sessions) != 1 || sessions[0].LeaveAt != nil || !sessions[0].JoinAt.Equal(joined.Truncate(time.Second)) {
		t.Fatalf("open session was mutated by read: %#v, %v", sessions, err)
	}

	left := now.Add(time.Minute)
	if err = s.EndSession(ctx, "active-player", left); err != nil {
		t.Fatal(err)
	}
	player, err = s.PlayerByUID(ctx, "active-player")
	if err != nil || player.PlaytimeSec != int64(left.Sub(joined)/time.Second) {
		t.Fatalf("closed playtime = %d, %v; want %d", player.PlaytimeSec, err, int64(left.Sub(joined)/time.Second))
	}
}

func TestReplaceWorldPopulatesPaldeckDisplayName(t *testing.T) {
	s, err := Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	ctx := context.Background()
	w := &sav.World{
		Players: []sav.Player{{UID: "owner-1"}},
		Pals: []sav.Pal{
			{InstanceID: "pal-1", OwnerUID: "owner-1", CharacterID: "SheepBall"},     // known legacy mapping -> Lamball
			{InstanceID: "pal-2", OwnerUID: "owner-1", CharacterID: "SomeFuturePal"}, // unmapped -> falls back to raw id
			{InstanceID: "pal-3", OwnerUID: "owner-1", CharacterID: "BOSS_GrassMammoth"},
		},
	}
	if err = s.ReplaceWorld(ctx, w, time.Now(), 0); err != nil {
		t.Fatal(err)
	}
	pals, err := s.Pals(ctx, "owner-1")
	if err != nil {
		t.Fatal(err)
	}
	got := map[string]any{}
	alpha := map[string]bool{}
	for _, p := range pals {
		got[p["characterId"].(string)] = p["displayName"]
		alpha[p["characterId"].(string)] = p["isAlpha"].(bool)
	}
	if got["SheepBall"] != "Lamball" {
		t.Errorf("SheepBall displayName = %v, want Lamball", got["SheepBall"])
	}
	if got["SomeFuturePal"] != "SomeFuturePal" {
		t.Errorf("SomeFuturePal displayName = %v, want raw-id fallback", got["SomeFuturePal"])
	}
	if got["BOSS_GrassMammoth"] != "Mammorest" {
		t.Errorf("BOSS_GrassMammoth displayName = %v, want Mammorest", got["BOSS_GrassMammoth"])
	}
	if !alpha["BOSS_GrassMammoth"] {
		t.Error("BOSS_GrassMammoth must be exposed as Alpha/boss even when IsBoss is false")
	}
}

func TestReplaceWorldPersistsPlayerProgressWithoutZeroFillingMissingData(t *testing.T) {
	s, err := Open(filepath.Join(t.TempDir(), "progress.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	ctx := context.Background()
	total, caught, unlocked := int64(87), 23, 31
	w := &sav.World{Players: []sav.Player{{
		UID: "progress-player", Nickname: "Progress", CaptureTotal: &total,
		UniquePalsCaptured: &caught, PaldeckUnlocked: &unlocked,
	}}}
	if err = s.ReplaceWorld(ctx, w, time.Now(), 0); err != nil {
		t.Fatal(err)
	}
	got, err := s.PlayerByUID(ctx, "progress-player")
	if err != nil || got.CaptureTotal == nil || *got.CaptureTotal != total || got.UniquePalsCaptured == nil || *got.UniquePalsCaptured != caught || got.PaldeckUnlocked == nil || *got.PaldeckUnlocked != unlocked {
		t.Fatalf("stored progress = %#v, %v", got, err)
	}

	// A temporarily missing/unreadable player RecordData block must retain the
	// last authoritative counters rather than replacing them with zero.
	if err = s.ReplaceWorld(ctx, &sav.World{Players: []sav.Player{{UID: "progress-player", Nickname: "Progress"}}}, time.Now().Add(time.Minute), 0); err != nil {
		t.Fatal(err)
	}
	got, err = s.PlayerByUID(ctx, "progress-player")
	if err != nil || got.CaptureTotal == nil || *got.CaptureTotal != total {
		t.Fatalf("progress after unavailable parse = %#v, %v", got, err)
	}
}

func TestReplaceWorldReconcilesPlayerUIDFormatsBeforePalDerivation(t *testing.T) {
	s, err := Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	ctx := context.Background()
	const (
		dashedUID = "5de71995-0000-0000-0000-000000000000"
		rawUID    = "5DE71995000000000000000000000000"
		partyID   = "AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA"
		boxID     = "BBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBB"
	)
	w := &sav.World{
		Players: []sav.Player{
			{UID: dashedUID, Nickname: "Merged Player", Level: 47, GuildID: "CCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCC"},
			{UID: rawUID, OtomoContainerID: partyID, PalStorageContainerID: boxID},
		},
		Pals: []sav.Pal{{InstanceID: "party-pal", OwnerUID: rawUID, ContainerID: partyID, SlotIndex: 3}},
	}
	if err = s.ReplaceWorld(ctx, w, time.Now(), 0); err != nil {
		t.Fatal(err)
	}
	players, err := s.Players(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(players) != 1 {
		t.Fatalf("players = %#v, want the two UID forms reconciled to one row", players)
	}
	p := players[0]
	if p.UID != NormalizeUID(rawUID) || p.Name != "Merged Player" || p.Level != 47 || p.GuildID != strings.ToLower("CCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCC") {
		t.Fatalf("merged player = %#v", p)
	}
	pals, err := s.PalsTyped(ctx, rawUID)
	if err != nil || len(pals) != 1 || !pals[0].InParty || pals[0].PartySlot == nil || *pals[0].PartySlot != 3 {
		t.Fatalf("pals derived from reconciled container row = %#v, %v", pals, err)
	}
	state, err := s.WorldState(ctx)
	if err != nil || state.Players != 1 {
		t.Fatalf("world player count = %d, %v; want reconciled count 1", state.Players, err)
	}
}

func TestReplaceWorldDerivesPartyAndBoxPlacement(t *testing.T) {
	s, err := Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	ctx := context.Background()
	w := &sav.World{
		Players: []sav.Player{{UID: "owner", OtomoContainerID: "party", PalStorageContainerID: "storage"}},
		Pals: []sav.Pal{
			{InstanceID: "a-party", OwnerUID: "owner", ContainerID: "party", SlotIndex: 4, HP: 321.5, Gender: "female", Talents: map[string]int{"Talent_HP": 77, "Talent_Shot": 88}, PassiveSkillIDs: []string{"CraftSpeed_up2"}, EquippedSkillIDs: []string{"AirCanon"}},
			{InstanceID: "b-box", OwnerUID: "owner", ContainerID: "storage", SlotIndex: 61},
			{InstanceID: "c-other", OwnerUID: "owner", ContainerID: "base", SlotIndex: 2, BaseID: "base-one"},
			{InstanceID: "d-negative", OwnerUID: "owner", ContainerID: "party", SlotIndex: -1},
			{InstanceID: "e-unknown", OwnerUID: "missing", ContainerID: "party", SlotIndex: 1},
		},
	}
	if err = s.ReplaceWorld(ctx, w, time.Now(), 0); err != nil {
		t.Fatal(err)
	}
	rows, err := s.PalsPage(ctx, "", 10)
	if err != nil || len(rows) != 5 {
		t.Fatalf("PalsPage = %#v, %v", rows, err)
	}
	party, boxed, other, negative, recovered := rows[0], rows[1], rows[2], rows[3], rows[4]
	if !party.InParty || party.PartySlot == nil || *party.PartySlot != 4 || party.BoxPage != nil || party.BoxSlot != nil {
		t.Errorf("party placement = %#v", party)
	}
	if party.HP == nil || *party.HP != 321.5 || party.Gender != "female" || party.TalentHP == nil || *party.TalentHP != 77 || party.TalentShot == nil || *party.TalentShot != 88 || len(party.PassiveSkillIDs) != 1 || party.PassiveSkillIDs[0] != "CraftSpeed_up2" || len(party.EquippedSkillIDs) != 1 || party.EquippedSkillIDs[0] != "AirCanon" {
		t.Errorf("party rich instance detail = %#v", party)
	}
	if boxed.InParty || boxed.PartySlot != nil || boxed.BoxPage == nil || *boxed.BoxPage != 2 || boxed.BoxSlot == nil || *boxed.BoxSlot != 1 {
		t.Errorf("box placement = %#v", boxed)
	}
	if other.BaseID != "baseone" {
		t.Errorf("base placement = %#v, want normalized base id", other)
	}
	for _, p := range []PalWithOwner{other, negative} {
		if p.InParty || p.PartySlot != nil || p.BoxPage != nil || p.BoxSlot != nil {
			t.Errorf("unresolved placement = %#v, want false/null/null/null", p)
		}
	}
	if recovered.OwnerUID != "owner" || !recovered.InParty || recovered.PartySlot == nil || *recovered.PartySlot != 1 {
		t.Errorf("personal-container owner recovery = %#v", recovered)
	}
	sessionPals, err := s.Pals(ctx, "owner")
	if err != nil || len(sessionPals) != 5 {
		t.Fatalf("session pals = %#v, %v", sessionPals, err)
	}
	var sessionParty map[string]any
	for _, pal := range sessionPals {
		passives, passivesOK := pal["passiveSkillIds"].([]string)
		equipped, equippedOK := pal["equippedSkillIds"].([]string)
		if !passivesOK || !equippedOK || passives == nil || equipped == nil {
			t.Fatalf("session Pal skill arrays must be non-null: %#v", pal)
		}
		if pal["instanceId"] == "aparty" {
			sessionParty = pal
		}
	}
	if sessionParty == nil {
		t.Fatal("session party Pal missing")
	}
	hp, hpOK := sessionParty["hp"].(*float64)
	if !hpOK || hp == nil || *hp != 321.5 || sessionParty["gender"] != "female" {
		t.Fatalf("session rich Pal detail = %#v", sessionParty)
	}
	talents, talentsOK := sessionParty["talents"].(map[string]any)
	if !talentsOK {
		t.Fatalf("session Pal talents = %#v", sessionParty["talents"])
	}
	talentHP, hpTalentOK := talents["hp"].(*int)
	talentShot, shotTalentOK := talents["shot"].(*int)
	if !talentsOK || !hpTalentOK || !shotTalentOK || talentHP == nil || talentShot == nil || *talentHP != 77 || *talentShot != 88 {
		t.Fatalf("session Pal talents = %#v", sessionParty["talents"])
	}
}

func TestReplaceWorldCarriesLastOwnerAcrossGuildBaseDeployment(t *testing.T) {
	s, err := Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	ctx := context.Background()
	at := time.Now()
	players := []sav.Player{
		{UID: "owner-a", Nickname: "Owner A", PalStorageContainerID: "box-a"},
		{UID: "owner-b", Nickname: "Owner B", PalStorageContainerID: "box-b"},
	}

	// First observe the Pal in Owner A's personal box.
	first := &sav.World{Players: players, Pals: []sav.Pal{{
		InstanceID: "stable-pal", OwnerUID: "owner-a", ContainerID: "box-a", SlotIndex: 0,
	}}}
	if err = s.ReplaceWorld(ctx, first, at, 0); err != nil {
		t.Fatal(err)
	}
	initialRows, err := s.PalsPage(ctx, "", 10)
	if err != nil || len(initialRows) != 1 || initialRows[0].OwnerSource != "personal_container" || !initialRows[0].OwnerResolved {
		t.Fatalf("initial personal owner provenance = %#v, %v", initialRows, err)
	}

	// Palworld 1.0 clears OwnerPlayerUId when the Pal enters a guild-base
	// worker container. The stable instance retains the last observed owner.
	deployed := &sav.World{Players: players, Pals: []sav.Pal{{
		InstanceID: "stable-pal", ContainerID: "guild-base-workers", SlotIndex: 2,
	}, {
		InstanceID: "never-observed", ContainerID: "guild-base-workers", SlotIndex: 3,
	}}}
	if err = s.ReplaceWorld(ctx, deployed, at.Add(time.Minute), 0); err != nil {
		t.Fatal(err)
	}
	rows, err := s.PalsPage(ctx, "", 10)
	if err != nil || len(rows) != 2 {
		t.Fatalf("deployed rows = %#v, %v", rows, err)
	}
	if rows[0].InstanceID != "neverobserved" || rows[0].OwnerUID != "" || rows[0].OwnerName != "" || rows[0].OwnerSource != "unresolved" || rows[0].OwnerResolved {
		t.Fatalf("unobserved base Pal was guessed = %#v", rows[0])
	}
	if rows[1].InstanceID != "stablepal" || rows[1].OwnerUID != "ownera" || rows[1].OwnerName != "Owner A" || rows[1].OwnerSource != "last_observed" || !rows[1].OwnerResolved {
		t.Fatalf("last observed owner was not retained = %#v", rows[1])
	}
	if rows[1].InParty || rows[1].BoxPage != nil {
		t.Fatalf("base Pal was mislabeled as personally placed = %#v", rows[1])
	}

	// A unique personal container is newer and authoritative, so a direct
	// transfer to Owner B supersedes both the stale raw owner and carried value.
	transferred := &sav.World{Players: players, Pals: []sav.Pal{{
		InstanceID: "stable-pal", OwnerUID: "owner-a", ContainerID: "box-b", SlotIndex: 31,
	}}}
	if err = s.ReplaceWorld(ctx, transferred, at.Add(2*time.Minute), 0); err != nil {
		t.Fatal(err)
	}
	rows, err = s.PalsPage(ctx, "", 10)
	if err != nil || len(rows) != 1 || rows[0].OwnerUID != "ownerb" || rows[0].OwnerName != "Owner B" || rows[0].OwnerSource != "personal_container" || !rows[0].OwnerResolved || rows[0].BoxPage == nil || *rows[0].BoxPage != 1 {
		t.Fatalf("personal-container transfer did not win = %#v, %v", rows, err)
	}

	// The new last owner then survives a later base deployment.
	if err = s.ReplaceWorld(ctx, deployed, at.Add(3*time.Minute), 0); err != nil {
		t.Fatal(err)
	}
	rows, err = s.PalsPage(ctx, "", 10)
	if err != nil || len(rows) != 2 || rows[1].OwnerUID != "ownerb" || rows[1].OwnerName != "Owner B" || rows[1].OwnerSource != "last_observed" || !rows[1].OwnerResolved {
		t.Fatalf("transferred owner was not retained at base = %#v, %v", rows, err)
	}
}

func TestReplaceWorldMarksJoinedRawOwnerAsSaveProvenance(t *testing.T) {
	s, err := Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	w := &sav.World{
		Players: []sav.Player{{UID: "owner-a", Nickname: "Owner A"}},
		Pals:    []sav.Pal{{InstanceID: "pal-a", OwnerUID: "owner-a", SlotIndex: -1}},
	}
	if err = s.ReplaceWorld(context.Background(), w, time.Now(), 0); err != nil {
		t.Fatal(err)
	}
	rows, err := s.PalsPage(context.Background(), "", 10)
	if err != nil || len(rows) != 1 || rows[0].OwnerSource != "save" || !rows[0].OwnerResolved {
		t.Fatalf("raw save owner provenance = %#v, %v", rows, err)
	}
}

func TestReplaceWorldDoesNotInferFromAmbiguousOrZeroContainers(t *testing.T) {
	s, err := Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	w := &sav.World{
		Players: []sav.Player{
			{UID: "a", OtomoContainerID: "shared", PalStorageContainerID: "00000000-0000-0000-0000-000000000000"},
			{UID: "b", PalStorageContainerID: "shared"},
		},
		Pals: []sav.Pal{
			{InstanceID: "ambiguous", ContainerID: "shared", SlotIndex: 0},
			{InstanceID: "zero", ContainerID: "00000000-0000-0000-0000-000000000000", SlotIndex: 0},
		},
	}
	if err = s.ReplaceWorld(context.Background(), w, time.Now(), 0); err != nil {
		t.Fatal(err)
	}
	rows, err := s.PalsPage(context.Background(), "", 10)
	if err != nil || len(rows) != 2 {
		t.Fatalf("rows = %#v, %v", rows, err)
	}
	for _, row := range rows {
		if row.OwnerUID != "" || row.OwnerSource != "unresolved" || row.OwnerResolved || row.InParty || row.BoxPage != nil {
			t.Errorf("unsafe container inference = %#v", row)
		}
	}
}

func TestReplaceWorldMarksUnexpectedSkipsAsFormatDrift(t *testing.T) {
	s, err := Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	w := &sav.World{Stats: sav.ParseStats{SkippedProperties: 1}}
	if err = s.ReplaceWorld(context.Background(), w, time.Now(), 0); err != nil {
		t.Fatal(err)
	}
	state, err := s.WorldState(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if !state.FormatDrift {
		t.Fatal("unexpected skipped property did not set formatDrift")
	}
}

func TestReplaceWorldDoesNotMarkKnownOpaqueDataAsFormatDrift(t *testing.T) {
	s, err := Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	// Known opaque byte blobs are retained without incrementing any public skip
	// or decode-failure counter; those clean stats must remain non-drift.
	w := &sav.World{Stats: sav.ParseStats{DecodeFailures: map[string]int{}}}
	if err = s.ReplaceWorld(context.Background(), w, time.Now(), 0); err != nil {
		t.Fatal(err)
	}
	state, err := s.WorldState(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if state.FormatDrift {
		t.Fatal("deliberately opaque data produced a false formatDrift warning")
	}
}

func TestResolveUIDEmptyReturnsNotFound(t *testing.T) {
	s, err := Open(filepath.Join(t.TempDir(), "resolve.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	ctx := context.Background()
	if err = s.UpsertLivePlayer(ctx, Player{UID: "abcdef", Name: "Existing"}, time.Now()); err != nil {
		t.Fatal(err)
	}
	for _, value := range []string{"", "  ", "---"} {
		if got := s.ResolveUID(ctx, value); got != "" {
			t.Errorf("ResolveUID(%q) = %q, want not-found empty result", value, got)
		}
	}
}

func TestMigration003RemovesPendingIdentityRowsAndSessionsOnly(t *testing.T) {
	path := filepath.Join(t.TempDir(), "v2.db")
	db, err := sql.Open("sqlite", path)
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
	for _, statement := range []string{
		string(init001),
		string(apiKeys002),
		`UPDATE kv SET value='2' WHERE key='schema_version'`,
		`INSERT INTO players(uid,name) VALUES('none','Pending'),('realuid','Real')`,
		`INSERT INTO sessions(player_uid,join_at) VALUES('none',1),('realuid',2)`,
		`INSERT INTO events(at,kind,message,meta_json) VALUES(1,'join','Pending joined','{"uid":"none"}')`,
	} {
		if _, err = db.Exec(statement); err != nil {
			db.Close()
			t.Fatal(err)
		}
	}
	if err = db.Close(); err != nil {
		t.Fatal(err)
	}

	st, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	ctx := context.Background()
	players, err := st.Players(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(players) != 1 || players[0].UID != "realuid" {
		t.Fatalf("players after migration = %#v, want only realuid", players)
	}
	sessions, err := st.Sessions(ctx, "realuid")
	if err != nil || len(sessions) != 1 {
		t.Fatalf("real sessions after migration = %#v, %v", sessions, err)
	}
	ghostSessions, err := st.Sessions(ctx, "none")
	if err != nil || len(ghostSessions) != 0 {
		t.Fatalf("pending sessions after migration = %#v, %v", ghostSessions, err)
	}
	events, err := st.Events(ctx, 10, "")
	if err != nil || len(events) != 1 || events[0].Message != "Pending joined" {
		t.Fatalf("audit events after migration = %#v, %v", events, err)
	}
}

func TestMigration004AddsNullablePalPlacementColumns(t *testing.T) {
	path := filepath.Join(t.TempDir(), "v3.db")
	db, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"001_init.sql", "002_api_keys.sql", "003_remove_pending_identity_players.sql"} {
		sqlBytes, readErr := migrations.ReadFile("migrations/" + name)
		if readErr != nil {
			t.Fatal(readErr)
		}
		if _, err = db.Exec(string(sqlBytes)); err != nil {
			db.Close()
			t.Fatal(err)
		}
	}
	for _, statement := range []string{
		`UPDATE kv SET value='3' WHERE key='schema_version'`,
		`INSERT INTO pals(instance_id,owner_uid,character_id,level,is_alpha,is_lucky,raw_json) VALUES('legacy-pal','owner','SheepBall',0,0,0,'{}')`,
	} {
		if _, err = db.Exec(statement); err != nil {
			db.Close()
			t.Fatal(err)
		}
	}
	if err = db.Close(); err != nil {
		t.Fatal(err)
	}

	st, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	if v, getErr := st.GetKV(context.Background(), "schema_version"); getErr != nil || v != "8" {
		t.Fatalf("schema_version = %q, %v; want 8", v, getErr)
	}
	pals, err := st.PalsTyped(context.Background(), "owner")
	if err != nil || len(pals) != 1 {
		t.Fatalf("legacy pals = %#v, %v", pals, err)
	}
	if pals[0].InParty || pals[0].PartySlot != nil || pals[0].BoxPage != nil || pals[0].BoxSlot != nil {
		t.Fatalf("legacy pal placement = %#v, want false/null/null/null", pals[0])
	}
}

func TestMetricsRollupAndPrune(t *testing.T) {
	s, err := Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	ctx := context.Background()
	now := time.Now().UTC().Truncate(time.Minute)
	old := now.Add(-25 * time.Hour)
	for i, v := range []float64{30, 60} {
		if err = s.AddMetric(ctx, Metric{At: old.Add(time.Duration(i) * time.Second), FPS: v, FrameTimeMS: v / 10, Players: i + 1}); err != nil {
			t.Fatal(err)
		}
	}
	expired := now.Add(-31 * 24 * time.Hour)
	if err = s.AddMetric(ctx, Metric{At: expired, FPS: 1}); err != nil {
		t.Fatal(err)
	}
	if err = s.MaintainMetrics(ctx, now); err != nil {
		t.Fatal(err)
	}
	var avg float64
	if err = s.db.QueryRow("SELECT fps_avg FROM metrics_rollup WHERE ts=?", old.Unix()/60*60).Scan(&avg); err != nil {
		t.Fatal(err)
	}
	if avg != 45 {
		t.Fatalf("avg=%v", avg)
	}
	var n int
	if err = s.db.QueryRow("SELECT COUNT(*) FROM metrics WHERE ts<?", now.Add(-24*time.Hour).Unix()).Scan(&n); err != nil || n != 0 {
		t.Fatalf("raw old count=%d err=%v", n, err)
	}
	if err = s.db.QueryRow("SELECT COUNT(*) FROM metrics_rollup WHERE ts<?", now.Add(-30*24*time.Hour).Unix()).Scan(&n); err != nil || n != 0 {
		t.Fatalf("rollup old count=%d err=%v", n, err)
	}
}

// TestFreshDatabaseReachesLatestSchemaAndAPIKeysUsable proves a brand-new database
// (kv table does not exist yet, schemaVersion reads 0) runs every embedded migration
// in order and lands with a usable api_keys table.
func TestFreshDatabaseReachesLatestSchemaAndAPIKeysUsable(t *testing.T) {
	s, err := Open(filepath.Join(t.TempDir(), "fresh.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	ctx := context.Background()
	v, err := s.GetKV(ctx, "schema_version")
	if err != nil || v != "8" {
		t.Fatalf("schema_version = %q, %v; want 8", v, err)
	}
	hash := [32]byte{1, 2, 3}
	created, err := s.CreateAPIKey(ctx, "abcd1234", hash, "fresh-db-key", time.Now())
	if err != nil {
		t.Fatal(err)
	}
	if created.ID != "abcd1234" || created.Label != "fresh-db-key" || created.Hash != hash {
		t.Fatalf("created key mismatch: %#v", created)
	}
	keys, err := s.ListAPIKeys(ctx)
	if err != nil || len(keys) != 1 {
		t.Fatalf("ListAPIKeys = %#v, %v", keys, err)
	}
}

// TestUpgradeFromV030SchemaAppliesAPIKeysMigration builds a database exactly as
// v0.3.0's Open() would (001_init.sql only, schema_version=1), seeds representative
// player/kv/event rows, then reopens it through the new migration runner. This is
// the §10.3 upgrade proof: the runner must apply exactly the pending migration(s),
// preserve every pre-existing row, and land on the latest schema_version.
func TestUpgradeFromV030SchemaAppliesAPIKeysMigration(t *testing.T) {
	path := filepath.Join(t.TempDir(), "palhelm-v0.3.0.db")
	init001, err := migrations.ReadFile("migrations/001_init.sql")
	if err != nil {
		t.Fatal(err)
	}
	legacy, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = legacy.Exec(string(init001)); err != nil {
		legacy.Close()
		t.Fatalf("apply 001_init.sql verbatim: %v", err)
	}
	for _, statement := range []string{
		`INSERT INTO kv(key,value) VALUES('operator-note','preserve-me')`,
		`INSERT INTO players(uid,steam_id,name,level,raw_json) VALUES('v030player','steam_030','V030Player',7,'{}')`,
		`INSERT INTO events(at,kind,message,meta_json) VALUES(1700000000,'panel','pre-upgrade event','{}')`,
	} {
		if _, err = legacy.Exec(statement); err != nil {
			legacy.Close()
			t.Fatal(err)
		}
	}
	var seededVersion string
	if err = legacy.QueryRow("SELECT value FROM kv WHERE key='schema_version'").Scan(&seededVersion); err != nil {
		legacy.Close()
		t.Fatal(err)
	}
	if seededVersion != "1" {
		legacy.Close()
		t.Fatalf("seeded schema_version = %q, want 1 (001_init.sql's own seed)", seededVersion)
	}
	if err = legacy.Close(); err != nil {
		t.Fatal(err)
	}

	upgraded, err := Open(path)
	if err != nil {
		t.Fatalf("open v0.3.0 database with the migration runner: %v", err)
	}
	defer upgraded.Close()
	ctx := context.Background()

	v, err := upgraded.GetKV(ctx, "schema_version")
	if err != nil || v != "8" {
		t.Fatalf("schema_version after upgrade = %q, %v; want 8", v, err)
	}
	if _, err = upgraded.ListAPIKeys(ctx); err != nil {
		t.Fatalf("api_keys table not usable after upgrade: %v", err)
	}
	if value, getErr := upgraded.GetKV(ctx, "operator-note"); getErr != nil || value != "preserve-me" {
		t.Fatalf("operator KV after upgrade = %q, %v", value, getErr)
	}
	player, err := upgraded.PlayerByUID(ctx, "v030player")
	if err != nil {
		t.Fatal(err)
	}
	if player.Name != "V030Player" || player.Level != 7 || player.SteamID != "steam_030" {
		t.Fatalf("pre-existing player changed during upgrade: %#v", player)
	}
	events, err := upgraded.Events(ctx, 10, "")
	if err != nil || len(events) != 1 || events[0].Message != "pre-upgrade event" {
		t.Fatalf("pre-existing event not readable after upgrade: %#v, %v", events, err)
	}

	// Re-opening is idempotent: no pending migrations, no error, version unchanged.
	if err = upgraded.Close(); err != nil {
		t.Fatal(err)
	}
	reopened, err := Open(path)
	if err != nil {
		t.Fatalf("second Open (already at latest version) returned an error: %v", err)
	}
	defer reopened.Close()
	if v, err = reopened.GetKV(ctx, "schema_version"); err != nil || v != "8" {
		t.Fatalf("schema_version after no-op reopen = %q, %v; want 8", v, err)
	}
}

// TestOpenFailsClosedOnFutureSchemaVersion proves a database whose schema_version
// exceeds the newest migration this binary knows about is never opened, silently or
// otherwise — the fail-closed contract in §10.1 for a downgrade scenario (e.g. a
// v0.5 data dir under this v0.4 binary).
func TestOpenFailsClosedOnFutureSchemaVersion(t *testing.T) {
	path := filepath.Join(t.TempDir(), "future.db")
	legacy, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	for _, statement := range []string{
		`CREATE TABLE kv (key TEXT PRIMARY KEY, value TEXT NOT NULL)`,
		`INSERT INTO kv(key,value) VALUES('schema_version','99')`,
	} {
		if _, err = legacy.Exec(statement); err != nil {
			legacy.Close()
			t.Fatal(err)
		}
	}
	if err = legacy.Close(); err != nil {
		t.Fatal(err)
	}

	if _, err = Open(path); err == nil {
		t.Fatal("Open on a future-versioned database succeeded; want a fail-closed error")
	} else if !strings.Contains(err.Error(), "newer than this binary supports") {
		t.Fatalf("unexpected error: %v", err)
	}

	// No migration writes must have happened: schema_version is untouched and no
	// api_keys table exists.
	verify, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	defer verify.Close()
	var v string
	if err = verify.QueryRow("SELECT value FROM kv WHERE key='schema_version'").Scan(&v); err != nil || v != "99" {
		t.Fatalf("schema_version mutated by a failed Open: %q, %v", v, err)
	}
	var name string
	err = verify.QueryRow("SELECT name FROM sqlite_master WHERE type='table' AND name='api_keys'").Scan(&name)
	if !errors.Is(err, sql.ErrNoRows) {
		t.Fatalf("api_keys table created by a failed Open: name=%q err=%v", name, err)
	}
}

// TestApplyMigrationSkipsAlreadyAppliedVersion exercises the in-transaction re-read
// path directly: a migration whose version the database already carries (as if a
// concurrent process had just applied it) must be skipped without re-executing its
// SQL or erroring.
func TestApplyMigrationSkipsAlreadyAppliedVersion(t *testing.T) {
	path := filepath.Join(t.TempDir(), "skip.db")
	db, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	for _, statement := range []string{
		`CREATE TABLE kv (key TEXT PRIMARY KEY, value TEXT NOT NULL)`,
		`INSERT INTO kv(key,value) VALUES('schema_version','2')`,
	} {
		if _, err = db.Exec(statement); err != nil {
			t.Fatal(err)
		}
	}
	ctx := context.Background()
	// A migration claiming to be version 2, with SQL that would error if executed
	// twice (api_keys already doesn't exist here, so a bogus non-idempotent
	// statement proves the skip: if applyMigration ran it, this table would exist).
	m := migration{version: 2, name: "002_would_error.sql", sql: "CREATE TABLE sentinel_should_not_exist(x)"}
	if err = applyMigration(ctx, db, m); err != nil {
		t.Fatalf("applyMigration on an already-applied version returned an error: %v", err)
	}
	var name string
	err = db.QueryRow("SELECT name FROM sqlite_master WHERE type='table' AND name='sentinel_should_not_exist'").Scan(&name)
	if !errors.Is(err, sql.ErrNoRows) {
		t.Fatalf("already-applied migration executed its SQL anyway: name=%q err=%v", name, err)
	}
}

// TestAPIKeyLifecycle covers create, list (including revoked), the active-keys
// subset used for the startup validation cache, revoke idempotency, unknown-id
// revoke, and lastUsedAt persistence.
func TestAPIKeyLifecycle(t *testing.T) {
	s, err := Open(filepath.Join(t.TempDir(), "keys.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	ctx := context.Background()
	now := time.Now().UTC().Truncate(time.Second)

	hashA := [32]byte{0xAA}
	hashB := [32]byte{0xBB}
	a, err := s.CreateAPIKey(ctx, "aaaaaaaa", hashA, "bot-a", now)
	if err != nil {
		t.Fatal(err)
	}
	if a.RevokedAt != nil || a.LastUsedAt != nil {
		t.Fatalf("new key already has revoked/lastUsed set: %#v", a)
	}
	_, err = s.CreateAPIKey(ctx, "bbbbbbbb", hashB, "bot-b", now.Add(time.Second))
	if err != nil {
		t.Fatal(err)
	}

	// Duplicate id is a constraint violation the caller retries with a fresh id.
	if _, err = s.CreateAPIKey(ctx, "aaaaaaaa", [32]byte{0xCC}, "dup", now); err == nil {
		t.Fatal("CreateAPIKey with a duplicate id succeeded; want a constraint error")
	}

	all, err := s.ListAPIKeys(ctx)
	if err != nil || len(all) != 2 {
		t.Fatalf("ListAPIKeys = %#v, %v; want 2 keys", all, err)
	}
	if all[0].ID != "bbbbbbbb" || all[1].ID != "aaaaaaaa" {
		t.Fatalf("ListAPIKeys order = %s,%s; want newest first", all[0].ID, all[1].ID)
	}

	active, err := s.ActiveAPIKeys(ctx)
	if err != nil || len(active) != 2 {
		t.Fatalf("ActiveAPIKeys = %#v, %v; want 2 active keys", active, err)
	}

	revokeAt := now.Add(time.Minute)
	revoked, err := s.RevokeAPIKey(ctx, "aaaaaaaa", revokeAt)
	if err != nil {
		t.Fatal(err)
	}
	if revoked.RevokedAt == nil || !revoked.RevokedAt.Equal(revokeAt) {
		t.Fatalf("revoked.RevokedAt = %v, want %v", revoked.RevokedAt, revokeAt)
	}

	// Idempotent: revoking again returns the original revokedAt, unchanged.
	revokedAgain, err := s.RevokeAPIKey(ctx, "aaaaaaaa", revokeAt.Add(time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	if revokedAgain.RevokedAt == nil || !revokedAgain.RevokedAt.Equal(revokeAt) {
		t.Fatalf("re-revoke changed revokedAt: got %v, want %v", revokedAgain.RevokedAt, revokeAt)
	}

	// Unknown id -> sql.ErrNoRows.
	if _, err = s.RevokeAPIKey(ctx, "unknownid", now); !errors.Is(err, sql.ErrNoRows) {
		t.Fatalf("RevokeAPIKey(unknown id) err = %v, want sql.ErrNoRows", err)
	}

	// Revocation removes the key from the active set (row is retained: it still
	// appears in ListAPIKeys with its revokedAt, per §2.6).
	active, err = s.ActiveAPIKeys(ctx)
	if err != nil || len(active) != 1 || active[0].ID != "bbbbbbbb" {
		t.Fatalf("ActiveAPIKeys after revoke = %#v, %v; want only bbbbbbbb", active, err)
	}
	all, err = s.ListAPIKeys(ctx)
	if err != nil || len(all) != 2 {
		t.Fatalf("ListAPIKeys after revoke = %#v, %v; want the revoked row retained", all, err)
	}

	// lastUsedAt persistence, as used by the coalescing writer.
	touchAt := now.Add(2 * time.Minute)
	if err = s.TouchAPIKeyLastUsed(ctx, "bbbbbbbb", touchAt); err != nil {
		t.Fatal(err)
	}
	active, err = s.ActiveAPIKeys(ctx)
	if err != nil || len(active) != 1 {
		t.Fatalf("ActiveAPIKeys after touch = %#v, %v", active, err)
	}
	if active[0].LastUsedAt == nil || !active[0].LastUsedAt.Equal(touchAt) {
		t.Fatalf("lastUsedAt = %v, want %v", active[0].LastUsedAt, touchAt)
	}
}

// TestPlayersPageKeysetPagination proves the integration API's keyset pagination query
// (spec §7.1): ascending uid order, WHERE uid > after, and the online-uid IN() predicate,
// including its "no rows for an empty, non-nil set" guard.
func TestPlayersPageKeysetPagination(t *testing.T) {
	s, err := Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	ctx := context.Background()
	var players []sav.Player
	for i := 1; i <= 5; i++ {
		players = append(players, sav.Player{UID: fmt.Sprintf("%032x", i), Nickname: fmt.Sprintf("p%d", i)})
	}
	if err = s.ReplaceWorld(ctx, &sav.World{Players: players}, time.Now(), 0); err != nil {
		t.Fatal(err)
	}

	page1, err := s.PlayersPage(ctx, "", 2, nil)
	if err != nil || len(page1) != 2 || page1[0].UID != fmt.Sprintf("%032x", 1) || page1[1].UID != fmt.Sprintf("%032x", 2) {
		t.Fatalf("page1 = %#v, %v", page1, err)
	}
	page2, err := s.PlayersPage(ctx, page1[len(page1)-1].UID, 2, nil)
	if err != nil || len(page2) != 2 || page2[0].UID != fmt.Sprintf("%032x", 3) {
		t.Fatalf("page2 = %#v, %v", page2, err)
	}

	// The online-uid predicate restricts results to the given set without disturbing order.
	only := []string{fmt.Sprintf("%032x", 2), fmt.Sprintf("%032x", 4)}
	filtered, err := s.PlayersPage(ctx, "", 10, only)
	if err != nil || len(filtered) != 2 || filtered[0].UID != only[0] || filtered[1].UID != only[1] {
		t.Fatalf("filtered = %#v, %v", filtered, err)
	}

	// An empty, non-nil uid set matches nothing and must not attempt "IN ()", which SQLite
	// rejects as a syntax error.
	empty, err := s.PlayersPage(ctx, "", 10, []string{})
	if err != nil || len(empty) != 0 {
		t.Fatalf("empty-set page = %#v, %v", empty, err)
	}
}

// TestPalsPageJoinsOwner proves the integration /pals bulk endpoint's owner join (spec §4):
// ordering by instance_id and an empty owner name when the owner has none.
func TestPalsPageJoinsOwner(t *testing.T) {
	s, err := Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	ctx := context.Background()
	w := &sav.World{
		Players: []sav.Player{{UID: "owner-a", Nickname: "Owner A"}, {UID: "owner-b"}},
		Pals: []sav.Pal{
			{InstanceID: "pal-1", OwnerUID: "owner-a", CharacterID: "SheepBall"},
			{InstanceID: "pal-2", OwnerUID: "owner-b", CharacterID: "Foxparks"},
		},
	}
	if err = s.ReplaceWorld(ctx, w, time.Now(), 0); err != nil {
		t.Fatal(err)
	}
	rows, err := s.PalsPage(ctx, "", 10)
	if err != nil || len(rows) != 2 {
		t.Fatalf("PalsPage = %#v, %v", rows, err)
	}
	// NormalizeUID strips dashes, so "pal-1"/"owner-a" become "pal1"/"ownera" in storage.
	if rows[0].InstanceID != "pal1" || rows[0].OwnerUID != "ownera" || rows[0].OwnerName != "Owner A" {
		t.Fatalf("row0 = %#v", rows[0])
	}
	if rows[1].InstanceID != "pal2" || rows[1].OwnerUID != "ownerb" || rows[1].OwnerName != "" {
		t.Fatalf("row1 = %#v (owner with no name must join to an empty string, not fail)", rows[1])
	}

	solo, err := s.PalsTyped(ctx, "owner-a")
	if err != nil || len(solo) != 1 || solo[0].InstanceID != "pal1" {
		t.Fatalf("PalsTyped(owner-a) = %#v, %v", solo, err)
	}
}

// TestGuildsTypedIncludesMembersAndBases proves the integration /guilds endpoint's typed
// query (spec §4/§6), which must not reuse GuildJSON's map[string]any shape.
func TestGuildsTypedIncludesMembersAndBases(t *testing.T) {
	s, err := Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	ctx := context.Background()
	w := &sav.World{
		Players: []sav.Player{{UID: "p1"}},
		Guilds: []sav.Guild{{
			ID: "g1", Name: "Guild One", AdminUID: "p1",
			Members: []sav.GuildMember{{UID: "p1", Name: "Player One"}},
		}},
		Bases: []sav.BaseCamp{{ID: "b1", GuildID: "g1", Position: &sav.Vector{X: 10, Y: 20}}},
	}
	if err = s.ReplaceWorld(ctx, w, time.Now(), 0); err != nil {
		t.Fatal(err)
	}
	guilds, err := s.Guilds(ctx)
	if err != nil || len(guilds) != 1 {
		t.Fatalf("Guilds = %#v, %v", guilds, err)
	}
	g := guilds[0]
	if g.ID != "g1" || g.Name != "Guild One" || g.AdminUID != "p1" {
		t.Fatalf("guild = %#v", g)
	}
	if len(g.Members) != 1 || g.Members[0].UID != "p1" || g.Members[0].Name != "Player One" {
		t.Fatalf("members = %#v", g.Members)
	}
	if len(g.Bases) != 1 || g.Bases[0].ID != "b1" || g.Bases[0].X != 10 || g.Bases[0].Y != 20 {
		t.Fatalf("bases = %#v", g.Bases)
	}
}
