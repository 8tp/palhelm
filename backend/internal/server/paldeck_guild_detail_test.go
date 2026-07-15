package server

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/8tp/palhelm/internal/sav"
	"github.com/8tp/palhelm/internal/store"
)

func TestPaldeckAndGuildDetailAreViewerSafeTruthfulAndAuthenticated(t *testing.T) {
	_, h, st := newKeyManagementTestServer(t)
	ctx := context.Background()
	now := time.Now().UTC().Truncate(time.Second)
	uid := "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
	guildID := "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"
	baseID := "cccccccccccccccccccccccccccccccc"
	total, unique, unlocked := int64(9), 2, 3
	world := &sav.World{
		Players: []sav.Player{{UID: uid, Nickname: "Safe Member", GuildID: guildID, CaptureTotal: &total, UniquePalsCaptured: &unique, PaldeckUnlocked: &unlocked,
			PalCaptureCounts: map[string]int64{"BOSS_Anubis": 2, "Unknown_Test_Pal": 1}, PaldeckUnlockFlags: map[string]bool{"BOSS_Anubis": true}}},
		Guilds: []sav.Guild{{ID: guildID, Name: "Safe Guild", AdminUID: uid, Members: []sav.GuildMember{{UID: uid, Name: "Safe Member"}}}},
		Bases:  []sav.BaseCamp{{ID: baseID, GuildID: guildID, Position: &sav.Vector{X: 120, Y: 340}}},
		Pals:   []sav.Pal{{InstanceID: "dddddddddddddddddddddddddddddddd", CharacterID: "BOSS_Anubis", OwnerUID: uid, BaseID: baseID, SlotIndex: -1}},
	}
	if err := st.ReplaceWorld(ctx, world, now, time.Millisecond); err != nil {
		t.Fatal(err)
	}
	if err := st.UpsertLivePlayer(ctx, store.Player{UID: uid, SteamID: "PRIVATE-STEAM", AccountName: "PRIVATE-ACCOUNT", Name: "Safe Member", Ping: 99}, now); err != nil {
		t.Fatal(err)
	}
	if err := st.StartSession(ctx, uid, now.Add(-time.Hour)); err != nil {
		t.Fatal(err)
	}

	viewer := loginAs(t, h, "viewerpass")
	serverRR := sessionRequest(h, http.MethodGet, "/api/v1/paldeck", "", viewer)
	if serverRR.Code != http.StatusOK {
		t.Fatalf("server paldeck status=%d body=%s", serverRR.Code, serverRR.Body.String())
	}
	var serverDoc store.ServerPaldeck
	if err := json.Unmarshal(serverRR.Body.Bytes(), &serverDoc); err != nil {
		t.Fatal(err)
	}
	if serverDoc.Coverage.Source != "player_save_record_data" || serverDoc.Catalog.KnownSpecies == 0 || serverDoc.Catalog.ObservedUnknownSpecies != 1 || len(serverDoc.Species) != serverDoc.Catalog.KnownSpecies+1 {
		t.Fatalf("server paldeck = %#v", serverDoc)
	}

	playerRR := sessionRequest(h, http.MethodGet, "/api/v1/players/"+uid+"/paldeck", "", viewer)
	if playerRR.Code != http.StatusOK {
		t.Fatalf("player paldeck status=%d body=%s", playerRR.Code, playerRR.Body.String())
	}
	var playerDoc store.PlayerPaldeck
	if err := json.Unmarshal(playerRR.Body.Bytes(), &playerDoc); err != nil {
		t.Fatal(err)
	}
	if playerDoc.Player.UID != uid || !playerDoc.Coverage.CaptureCountsAvailable || len(playerDoc.Species) != playerDoc.Catalog.KnownSpecies+1 {
		t.Fatalf("player paldeck = %#v", playerDoc)
	}

	guildRR := sessionRequest(h, http.MethodGet, "/api/v1/guilds/"+guildID, "", viewer)
	if guildRR.Code != http.StatusOK {
		t.Fatalf("guild status=%d body=%s", guildRR.Code, guildRR.Body.String())
	}
	var guildDoc store.GuildDetail
	if err := json.Unmarshal(guildRR.Body.Bytes(), &guildDoc); err != nil {
		t.Fatal(err)
	}
	if guildDoc.ID != guildID || guildDoc.MemberCount != 1 || len(guildDoc.Bases) != 1 || guildDoc.PalCount != 1 || guildDoc.Activity.Coverage != "panel_observed_sessions" || guildDoc.Activity.DurationSec < 3600 || guildDoc.Activity.DurationSec > 3602 {
		t.Fatalf("guild detail = %#v", guildDoc)
	}

	for path, body := range map[string]string{"server paldeck": serverRR.Body.String(), "player paldeck": playerRR.Body.String(), "guild detail": guildRR.Body.String()} {
		lower := strings.ToLower(body)
		for _, forbidden := range []string{"private-steam", "private-account", "steamid", "accountname", "raw_json", "rawjson", "ping", "platformid", "runtimeactor"} {
			if strings.Contains(lower, forbidden) {
				t.Errorf("%s leaked %q: %s", path, forbidden, body)
			}
		}
	}

	for _, path := range []string{"/api/v1/players/not-a-guid/paldeck", "/api/v1/guilds/not-a-guid", "/api/v1/guilds/dddddddddddddddddddddddddddddddd"} {
		rr := sessionRequest(h, http.MethodGet, path, "", viewer)
		if rr.Code != http.StatusNotFound {
			t.Errorf("%s status=%d body=%s", path, rr.Code, rr.Body.String())
		}
	}
	for _, path := range []string{"/api/v1/paldeck", "/api/v1/players/" + uid + "/paldeck", "/api/v1/guilds/" + guildID} {
		rr := sessionRequest(h, http.MethodGet, path, "", nil)
		if rr.Code != http.StatusUnauthorized {
			t.Errorf("unauthenticated %s status=%d", path, rr.Code)
		}
	}
}
