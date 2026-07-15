package server

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
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
            {"Type":"Character","UnitType":"Player","InstanceID":"PRIVATE-PLAYER-ID","NickName":"Hunter","userid":"PRIVATE-USER-ID","ip":"192.0.2.55","level":35,"HP":900,"MaxHP":1000,"GuildID":"PRIVATE-GUILD-ID","GuildName":"BES Pals","Action":"PRIVATE-PLAYER-ACTION","LocationX":100,"LocationY":200,"LocationZ":300,"RotationX":9,"IsActive":"true"},
            {"Type":"Character","UnitType":"BaseCampPal","InstanceID":"PRIVATE-PAL-ID","TrainerInstanceID":"PRIVATE-TRAINER-ID","TrainerNickName":"Hunter","Class":"/Game/Pal/BOSS_Mammorest.BOSS_Mammorest_C","level":35,"HP":500,"MaxHP":1000,"AI_Action":"TransportItem","LocationX":110,"LocationY":210,"LocationZ":310,"IsActive":"true"},
            {"Type":"Character","UnitType":"WildPal","Class":"PurpleSpider","LocationX":999,"LocationY":999,"LocationZ":0},
            {"Type":"Character","UnitType":"NPC","Class":"Hunter_Rifle","LocationX":998,"LocationY":998,"LocationZ":0},
            {"Type":"PalBox","GuildID":"PRIVATE-GUILD-ID","GuildName":"BES Pals","Class":"PalBox","LocationX":120,"LocationY":220,"LocationZ":320}
          ]
        }`))
	})
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
	for _, forbidden := range []string{"192.0.2.55", "private-", "hunter", "bes pals", "mammorest", "purplespider", "location", "action", "userid", "trainer", "diagnostics", "requestduration", "acceptedactor", "nextattempt", "errorcategory"} {
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
	if doc.State != "unsupported" || len(doc.Actors) != 0 || doc.Counts.Players != 0 || strings.Contains(strings.ToLower(rr.Body.String()), "hunter") {
		t.Fatalf("terminal response retained exact data: %s", rr.Body.String())
	}
	if doc.Diagnostics.LastAcceptedActorCount != 5 || doc.Diagnostics.LastErrorCategory != "unsupported" || doc.Diagnostics.LastRequestDurationMS < 0 {
		t.Fatalf("terminal diagnostics = %#v", doc.Diagnostics)
	}
}
