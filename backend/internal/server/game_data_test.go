package server

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/8tp/palhelm/internal/sav"
	"github.com/8tp/palhelm/internal/store"
)

func gameDataFixtureHandler(t *testing.T) http.Handler {
	t.Helper()
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/api/game-data" {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		_, _ = w.Write([]byte(`{
          "Time":"2026-07-14 13:00:00","FPS":58.5,"AverageFPS":55.25,
          "ActorData":[
            {"Type":"Character","UnitType":"Player","InstanceID":"PRIVATE-PLAYER-ID","NickName":"Player One","userid":"PRIVATE-USER-ID","ip":"192.0.2.55","level":35,"HP":900,"MaxHP":1000,"GuildID":"PRIVATE-GUILD-ID","GuildName":"Example Guild","Action":"PRIVATE-PLAYER-ACTION","LocationX":100,"LocationY":200,"LocationZ":300,"RotationX":9,"IsActive":"true"},
            {"Type":"Character","UnitType":"BaseCampPal","InstanceID":"aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa : bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb","TrainerInstanceID":"PRIVATE-TRAINER-ID","TrainerNickName":"Player One","Class":"/Game/Pal/BOSS_Mammorest.BOSS_Mammorest_C","level":35,"HP":500,"MaxHP":1000,"AI_Action":"TransportItem","LocationX":110,"LocationY":210,"LocationZ":310,"IsActive":"true"},
            {"Type":"Character","UnitType":"WildPal","Class":"PurpleSpider","LocationX":999,"LocationY":999,"LocationZ":0},
            {"Type":"Character","UnitType":"NPC","Class":"Hunter_Rifle","LocationX":998,"LocationY":998,"LocationZ":0},
            {"Type":"PalBox","GuildID":"PRIVATE-GUILD-ID","GuildName":"Example Guild","Class":"PalBox","LocationX":120,"LocationY":220,"LocationZ":320}
          ]
        }`))
	})
}

func TestIntegrationWorldWorkersUsesExactCompoundIDJoinAndRedactsLocation(t *testing.T) {
	s, h, st := newIntegrationTestServer(t, gameDataFixtureHandler(t))
	world := &sav.World{
		Players: []sav.Player{{UID: "11111111111111111111111111111111", Nickname: "Player One", GuildID: "22222222222222222222222222222222"}},
		Pals:    []sav.Pal{{InstanceID: "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb", CharacterID: "Mammorest", Level: 35, IsBoss: true, OwnerUID: "11111111111111111111111111111111", BaseID: "33333333333333333333333333333333", SlotIndex: 0}},
		Guilds:  []sav.Guild{{ID: "22222222222222222222222222222222", Name: "Example Guild", Members: []sav.GuildMember{{UID: "11111111111111111111111111111111", Name: "Player One"}}}},
		Bases:   []sav.BaseCamp{{ID: "33333333333333333333333333333333", GuildID: "22222222222222222222222222222222"}},
	}
	if err := st.ReplaceWorld(context.Background(), world, time.Now().UTC(), time.Second); err != nil {
		t.Fatal(err)
	}
	if err := s.poll.RefreshGameData(context.Background()); err != nil {
		t.Fatal(err)
	}
	token := issueTestAPIKey(t, s, st, "1234abcd", "world-workers")
	rr := integrationRequest(h, http.MethodGet, "/api/integration/v1/world/workers", token)
	if rr.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", rr.Code, rr.Body.String())
	}
	var doc struct {
		Data integrationLiveWorkersView `json:"data"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &doc); err != nil {
		t.Fatal(err)
	}
	if len(doc.Data.Workers) != 1 {
		t.Fatalf("workers = %#v", doc.Data.Workers)
	}
	worker := doc.Data.Workers[0]
	if worker.InstanceID != "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb" || worker.BaseID != "33333333333333333333333333333333" || worker.OwnerName != "Player One" || worker.CharacterID != "Mammorest" || worker.Activity != "transporting" {
		t.Fatalf("worker = %#v", worker)
	}
	body := strings.ToLower(rr.Body.String())
	for _, forbidden := range []string{"location", "trainer", "guildname", "private-", "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", "transportitem"} {
		if strings.Contains(body, forbidden) {
			t.Errorf("worker response leaked %q: %s", forbidden, rr.Body.String())
		}
	}
}

func TestIntegrationWorldSummaryIsAggregateOnly(t *testing.T) {
	var upstreamCalls atomic.Int32
	fixture := gameDataFixtureHandler(t)
	s, h, st := newIntegrationTestServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		upstreamCalls.Add(1)
		fixture.ServeHTTP(w, r)
	}))
	if err := s.poll.RefreshGameData(context.Background()); err != nil {
		t.Fatal(err)
	}
	token := issueTestAPIKey(t, s, st, "1234abcd", "world-summary")
	rr := integrationRequest(h, http.MethodGet, "/api/integration/v1/world/summary", token)
	if rr.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", rr.Code, rr.Body.String())
	}
	if got := upstreamCalls.Load(); got != 1 {
		t.Fatalf("summary request called Palworld upstream: calls=%d, want only the explicit refresh", got)
	}
	if rr.Header().Get("Cache-Control") != "no-store" || rr.Header().Get("ETag") == "" {
		t.Fatalf("headers = %#v", rr.Header())
	}
	var doc struct {
		Data integrationWorldSummaryView `json:"data"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &doc); err != nil {
		t.Fatal(err)
	}
	if doc.Data.State != "ready" || doc.Data.Counts.Players != 1 || doc.Data.Counts.BasePals != 1 || doc.Data.Counts.WildPals != 1 || doc.Data.Counts.NPCs != 1 || doc.Data.Counts.PalBoxes != 1 {
		t.Fatalf("summary = %#v", doc.Data)
	}
	body := strings.ToLower(rr.Body.String())
	for _, forbidden := range []string{"192.0.2.55", "private-", "player one", "example guild", "mammorest", "purplespider", "location", "action", "userid", "trainer", "diagnostics", "requestduration", "acceptedactor", "nextattempt", "errorcategory"} {
		if strings.Contains(body, forbidden) {
			t.Errorf("Integration summary leaked %q: %s", forbidden, rr.Body.String())
		}
	}
}

func TestSessionWorldSnapshotProjectsUsefulActorsWithoutCredentialFields(t *testing.T) {
	s, h, _ := newIntegrationTestServer(t, gameDataFixtureHandler(t))
	if err := s.poll.RefreshGameData(context.Background()); err != nil {
		t.Fatal(err)
	}
	cookie := loginAs(t, h, "panelpass")
	req := httptest.NewRequest(http.MethodGet, "/api/v1/world/snapshot", nil)
	req.AddCookie(cookie)
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", rr.Code, rr.Body.String())
	}
	if rr.Header().Get("Cache-Control") != "no-store" {
		t.Fatalf("Cache-Control = %q", rr.Header().Get("Cache-Control"))
	}
	var doc sessionWorldSnapshotView
	if err := json.Unmarshal(rr.Body.Bytes(), &doc); err != nil {
		t.Fatal(err)
	}
	if len(doc.Actors) != 3 || doc.Counts.WildPals != 1 || doc.Counts.NPCs != 1 {
		t.Fatalf("session snapshot = %#v", doc)
	}
	if doc.State != "ready" || doc.Truncated || doc.Diagnostics.LastAcceptedActorCount != 5 || doc.Diagnostics.LastErrorCategory != "none" || doc.Diagnostics.LastRequestDurationMS < 0 || doc.Diagnostics.ScheduledDelayMS != 0 || doc.Diagnostics.NextAttemptAt != nil {
		t.Fatalf("session diagnostics = %#v", doc.Diagnostics)
	}
	pal := doc.Actors[1]
	if pal.CharacterID != "Mammorest" || !pal.IsBoss || pal.Activity != "transporting" || pal.HPPercent == nil || *pal.HPPercent != 50 {
		t.Fatalf("projected base Pal = %#v", pal)
	}
	body := strings.ToLower(rr.Body.String())
	for _, forbidden := range []string{"192.0.2.55", "private-", "userid", "instanceid", "trainerinstanceid", "rotation", "private-player-action", "transportitem"} {
		if strings.Contains(body, forbidden) {
			t.Errorf("session snapshot leaked %q: %s", forbidden, rr.Body.String())
		}
	}
}

func TestSessionWorldSnapshotClearsExactActorsAfterTerminalFailure(t *testing.T) {
	var calls atomic.Int32
	fixture := gameDataFixtureHandler(t)
	s, h, _ := newIntegrationTestServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if calls.Add(1) == 1 {
			fixture.ServeHTTP(w, r)
			return
		}
		w.WriteHeader(http.StatusNotFound)
	}))
	if err := s.poll.RefreshGameData(context.Background()); err != nil {
		t.Fatal(err)
	}
	if err := s.poll.RefreshGameData(context.Background()); err == nil {
		t.Fatal("expected unsupported endpoint response")
	}
	cookie := loginAs(t, h, "panelpass")
	req := httptest.NewRequest(http.MethodGet, "/api/v1/world/snapshot", nil)
	req.AddCookie(cookie)
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)
	var doc sessionWorldSnapshotView
	if err := json.Unmarshal(rr.Body.Bytes(), &doc); err != nil {
		t.Fatal(err)
	}
	if doc.State != "unsupported" || len(doc.Actors) != 0 || doc.Counts.Players != 0 || strings.Contains(strings.ToLower(rr.Body.String()), "player one") {
		t.Fatalf("terminal response retained exact data: %s", rr.Body.String())
	}
	if doc.Diagnostics.LastAcceptedActorCount != 5 || doc.Diagnostics.LastErrorCategory != "unsupported" || doc.Diagnostics.LastRequestDurationMS < 0 {
		t.Fatalf("terminal diagnostics = %#v", doc.Diagnostics)
	}
}

func TestWorldActivityHistoryIsBoundedAndDescribesItsBucket(t *testing.T) {
	_, h, st := newIntegrationTestServer(t, gameDataFixtureHandler(t))
	now := time.Now().UTC().Truncate(time.Minute)
	for i := 0; i < 720; i++ {
		if err := st.AddGameDataActivity(context.Background(), store.GameDataActivity{
			At: now.Add(time.Duration(i-720) * time.Minute), FPS: float64(i), Players: i % 4,
		}); err != nil {
			t.Fatal(err)
		}
	}
	cookie := loginAs(t, h, "panelpass")
	req := httptest.NewRequest(http.MethodGet, "/api/v1/world/activity?window=7d", nil)
	req.AddCookie(cookie)
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", rr.Code, rr.Body.String())
	}
	var doc struct {
		Window    string                   `json:"window"`
		BucketSec int64                    `json:"bucketSec"`
		Samples   []store.GameDataActivity `json:"samples"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &doc); err != nil {
		t.Fatal(err)
	}
	if doc.Window != "7d" || doc.BucketSec != 1800 || len(doc.Samples) > 336 {
		t.Fatalf("activity response = window %q bucket %d samples %d", doc.Window, doc.BucketSec, len(doc.Samples))
	}
}
