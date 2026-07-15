package server

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/8tp/palhelm/internal/config"
	"github.com/8tp/palhelm/internal/sav"
	"github.com/8tp/palhelm/internal/store"
)

// seedBasicWorld populates a small, internally consistent world (two players in one guild,
// one pal, one base) and layers REST-only sentinel fields onto player A - the fixture every
// redaction and general-shape test below shares.
func seedBasicWorld(t *testing.T, st *store.Store) (uidA, uidB, guildID, palID string) {
	t.Helper()
	ctx := context.Background()
	uidA, uidB = "11111111111111111111111111111111", "22222222222222222222222222222222"
	guildID, palID = "33333333333333333333333333333333", "44444444444444444444444444444444"
	baseID := "55555555555555555555555555555555"
	partyID := "66666666666666666666666666666666"
	boxID := "77777777777777777777777777777777"
	w := &sav.World{
		Players: []sav.Player{{UID: uidA, Nickname: "Player One", Level: 10, GuildID: guildID, OtomoContainerID: partyID, PalStorageContainerID: boxID, CaptureTotal: int64TestPtr(42), UniquePalsCaptured: intTestPtr(12), PaldeckUnlocked: intTestPtr(15)}, {UID: uidB, Nickname: "Player Two", Level: 5, GuildID: guildID}},
		Pals: []sav.Pal{{InstanceID: palID, CharacterID: "SheepBall", Level: 3, HP: 432.5, Gender: "female", OwnerUID: uidA, IsLucky: true, ContainerID: partyID, SlotIndex: 2,
			Talents: map[string]int{"Talent_HP": 71, "Talent_Melee": 62, "Talent_Shot": 83, "Talent_Defense": 54}, PassiveSkillIDs: []string{"CraftSpeed_up2"}, EquippedSkillIDs: []string{"AirCanon"}}},
		Guilds: []sav.Guild{{
			ID: guildID, Name: "Test Guild", AdminUID: uidA,
			Members:    []sav.GuildMember{{UID: uidA, Name: "Player One"}, {UID: uidB, Name: "Player Two"}},
			MemberUIDs: []string{uidA, uidB},
		}},
		Bases: []sav.BaseCamp{{ID: baseID, GuildID: guildID, Position: &sav.Vector{X: 100, Y: 200}}},
	}
	if err := st.ReplaceWorld(ctx, w, time.Now().UTC(), time.Millisecond); err != nil {
		t.Fatal(err)
	}
	x, y := 1234.5, 6789.5
	if err := st.UpsertLivePlayer(ctx, store.Player{
		UID: uidA, SteamID: "STEAM-SENTINEL-76561198000000001", Name: "Player One",
		AccountName: "ACCOUNT-SENTINEL-live", Level: 10, Ping: 123.456, X: &x, Y: &y, Raw: []byte("{}"),
	}, time.Now().UTC()); err != nil {
		t.Fatal(err)
	}
	banned, whitelisted := true, true
	if err := st.SetPlayerFlags(ctx, uidA, &banned, &whitelisted); err != nil {
		t.Fatal(err)
	}
	if err := st.StartSession(ctx, uidA, time.Now().UTC()); err != nil {
		t.Fatal(err)
	}
	return uidA, uidB, guildID, palID
}

func intTestPtr(v int) *int       { return &v }
func int64TestPtr(v int64) *int64 { return &v }

func TestIntegrationEventsStrictPublicProjection(t *testing.T) {
	s, h, st := newIntegrationTestServer(t, nil)
	token := issueTestAPIKey(t, s, st, "eeeeeeee", "history-bot")
	ctx := context.Background()
	base := time.Date(2026, 7, 11, 12, 0, 0, 0, time.UTC)
	events := []store.Event{
		{At: base.Add(time.Second), Kind: "join", Message: "Player\nAdmin joined", Meta: map[string]any{"uid": "PRIVATE-UID"}},
		{At: base.Add(2 * time.Second), Kind: "leave", Message: "Player Two left", Meta: map[string]any{"steamId": "PRIVATE-STEAM"}},
		{At: base.Add(3 * time.Second), Kind: "backup", Message: "backup /private/world.zip completed", Meta: map[string]any{"path": "/private/world.zip"}},
		{At: base.Add(4 * time.Second), Kind: "system", Message: "Palworld REST API is unreachable", Meta: map[string]any{"detail": "PRIVATE-SYSTEM"}},
		{At: base.Add(5 * time.Second), Kind: "system", Message: "ran private repair command", Meta: map[string]any{"secret": "PRIVATE-SECRET"}},
		{At: base.Add(6 * time.Second), Kind: "panel", Message: "created integration key PRIVATE-TOKEN", Meta: map[string]any{"actor": "admin"}},
		{At: base.Add(7 * time.Second), Kind: "config", Message: "changed admin password", Meta: map[string]any{"password": "PRIVATE-PASSWORD"}},
		{At: base.Add(8 * time.Second), Kind: "join", Message: "malformed join event", Meta: nil},
	}
	for _, event := range events {
		if err := st.AddEvent(ctx, event); err != nil {
			t.Fatal(err)
		}
	}

	rr := integrationRequest(h, "GET", "/api/integration/v1/events?limit=10", token)
	if rr.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", rr.Code, rr.Body.String())
	}
	var doc struct {
		Data []integrationEventView `json:"data"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &doc); err != nil {
		t.Fatal(err)
	}
	if len(doc.Data) != 4 {
		t.Fatalf("public events = %#v, want exactly four allowlisted rows", doc.Data)
	}
	want := []struct{ kind, message string }{
		{"system", "Palworld REST API is unreachable"},
		{"backup", "Backup completed"},
		{"leave", "Player Two left"},
		{"join", "Player Admin joined"},
	}
	for i, expected := range want {
		if doc.Data[i].Kind != expected.kind || doc.Data[i].Message != expected.message {
			t.Errorf("event[%d] = %#v, want kind=%q message=%q", i, doc.Data[i], expected.kind, expected.message)
		}
	}
	body := rr.Body.String()
	for _, forbidden := range []string{"meta", "PRIVATE-", "admin password", "integration key", "/private/"} {
		if strings.Contains(body, forbidden) {
			t.Errorf("public events leaked %q: %s", forbidden, body)
		}
	}

	limited := integrationRequest(h, "GET", "/api/integration/v1/events?limit=2", token)
	var limitedDoc struct {
		Data []integrationEventView `json:"data"`
	}
	if err := json.Unmarshal(limited.Body.Bytes(), &limitedDoc); err != nil {
		t.Fatal(err)
	}
	if limited.Code != http.StatusOK || len(limitedDoc.Data) != 2 {
		t.Fatalf("limited events status=%d data=%#v", limited.Code, limitedDoc.Data)
	}
	for _, query := range []string{"0", "101", "nope"} {
		bad := integrationRequest(h, "GET", "/api/integration/v1/events?limit="+query, token)
		if bad.Code != http.StatusBadRequest || !strings.Contains(bad.Body.String(), "invalid_limit") {
			t.Errorf("limit=%q status=%d body=%s", query, bad.Code, bad.Body.String())
		}
	}
}

func TestIntegrationPlayerProgressViewOmitsUnknownAndExposesDecodedCounts(t *testing.T) {
	present := newIntegrationPlayerView(store.Player{
		UID: "u", CaptureTotal: int64TestPtr(12), UniquePalsCaptured: intTestPtr(3), PaldeckUnlocked: intTestPtr(4),
	}, false)
	b, err := json.Marshal(present)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{`"captureTotal":12`, `"uniquePalsCaptured":3`, `"paldeckUnlocked":4`} {
		if !strings.Contains(string(b), want) {
			t.Fatalf("progress view %s missing %s", b, want)
		}
	}
	unknown, err := json.Marshal(newIntegrationPlayerView(store.Player{UID: "u"}, false))
	if err != nil {
		t.Fatal(err)
	}
	for _, absent := range []string{"captureTotal", "uniquePalsCaptured", "paldeckUnlocked"} {
		if strings.Contains(string(unknown), absent) {
			t.Fatalf("unknown progress must be omitted: %s", unknown)
		}
	}
}

// sessionLogin logs into the session API (viewer cookie carries the fixture through the
// existing session handlers, whose responses are asserted non-vacuous below) and returns a
// request helper bound to that cookie.
func sessionLogin(t *testing.T, h http.Handler) func(method, path string) *httptest.ResponseRecorder {
	t.Helper()
	login := httptest.NewRequest("POST", "/api/v1/auth/login", strings.NewReader(`{"password":"panelpass"}`))
	login.Header.Set("Content-Type", "application/json")
	lr := httptest.NewRecorder()
	h.ServeHTTP(lr, login)
	if lr.Code != 200 {
		t.Fatalf("login status=%d body=%s", lr.Code, lr.Body.String())
	}
	cookie := lr.Result().Cookies()[0]
	return func(method, path string) *httptest.ResponseRecorder {
		req := httptest.NewRequest(method, path, nil)
		req.AddCookie(cookie)
		rr := httptest.NewRecorder()
		h.ServeHTTP(rr, req)
		return rr
	}
}

// TestIntegrationRedactionIsNonVacuousAndComplete is the spec §12.4 proof: sentinel values
// first must appear in the real session-API pipeline (proving the fixture is not vacuous -
// the v0.3.0 lesson), then must not appear anywhere in any integration response.
func TestIntegrationRedactionIsNonVacuousAndComplete(t *testing.T) {
	oldVersion := PanelVersion
	PanelVersion = "PANELVER-SENTINEL-42"
	t.Cleanup(func() { PanelVersion = oldVersion })

	restCalls := &atomic.Int64{}
	rest := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		restCalls.Add(1)
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/v1/api/info":
			_, _ = io.WriteString(w, `{"servername":"Test Server","version":"9.9.9","worldguid":"WORLDGUID-SENTINEL-9999","uptime":100}`)
		case "/v1/api/metrics":
			_, _ = io.WriteString(w, `{"serverfps":60,"serverframetime":16.6,"currentplayernum":0,"maxplayernum":32,"uptime":100,"days":1,"basecampnum":0}`)
		case "/v1/api/players":
			_, _ = io.WriteString(w, `{"players":[]}`)
		default:
			http.NotFound(w, r)
		}
	})
	st, err := storeOpenTemp(t)
	if err != nil {
		t.Fatal(err)
	}
	srv := httptest.NewServer(rest)
	t.Cleanup(srv.Close)
	cfg := config.Config{
		AdminPassword: "panelpass", SessionSecret: strings.Repeat("s", 48),
		RESTURL: srv.URL, RESTUser: "admin", PalworldPassword: "gamepass",
		IntegrationRateLimit: 60, MetricsInterval: 10 * time.Millisecond, PlayersInterval: time.Hour, SaveSyncInterval: time.Hour,
	}
	s, h := New(cfg, st, testLogger())
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() { s.RunPollers(ctx); close(done) }()
	t.Cleanup(func() { cancel(); <-done })

	uidA, _, _, _ := seedBasicWorld(t, st)
	token := issueTestAPIKey(t, s, st, "aaaaaaaa", "bot")

	waitForCachedInfoReachable(t, s)

	sentinels := []string{
		"STEAM-SENTINEL-76561198000000001", "ACCOUNT-SENTINEL-live",
		"WORLDGUID-SENTINEL-9999", "PANELVER-SENTINEL-42",
	}

	// Non-vacuous: the session API actually surfaces these sentinels and the redacted keys.
	session := sessionLogin(t, h)
	playerBody := session("GET", "/api/v1/players/"+uidA).Body.String()
	for _, marker := range []string{"STEAM-SENTINEL-76561198000000001", "ACCOUNT-SENTINEL-live", `"ping"`, `"location"`, `"banned"`, `"whitelisted"`, `"sessions"`} {
		if !strings.Contains(playerBody, marker) {
			t.Fatalf("session /players/%s does not contain %q; fixture is vacuous:\n%s", uidA, marker, playerBody)
		}
	}
	serverBody := session("GET", "/api/v1/server").Body.String()
	for _, marker := range []string{"WORLDGUID-SENTINEL-9999", "PANELVER-SENTINEL-42", `"worldGuid"`, `"panelVersion"`} {
		if !strings.Contains(serverBody, marker) {
			t.Fatalf("session /server does not contain %q; fixture is vacuous:\n%s", marker, serverBody)
		}
	}

	// Complete: no integration response, across every endpoint, leaks any sentinel or
	// redacted key name.
	endpoints := []string{
		"/api/integration/v1/players",
		"/api/integration/v1/players/" + uidA,
		"/api/integration/v1/pals",
		"/api/integration/v1/guilds",
		"/api/integration/v1/map",
		"/api/integration/v1/server",
		"/api/integration/v1/metrics/current",
		"/api/integration/v1/world/summary",
	}
	var all strings.Builder
	for _, ep := range endpoints {
		rr := integrationRequest(h, "GET", ep, token)
		if rr.Code != 200 {
			t.Fatalf("%s status=%d body=%s", ep, rr.Code, rr.Body.String())
		}
		all.WriteString(rr.Body.String())
	}
	body := all.String()
	for _, marker := range sentinels {
		if strings.Contains(body, marker) {
			t.Fatalf("integration responses leaked sentinel %q:\n%s", marker, body)
		}
	}
	for _, key := range []string{`"steamId"`, `"accountName"`, `"ping"`, `"banned"`, `"whitelisted"`, `"sessions"`, `"worldGuid"`, `"panelVersion"`} {
		if strings.Contains(body, key) {
			t.Fatalf("integration responses contained redacted key %s:\n%s", key, body)
		}
	}
	// "location" is legitimately present for guild bases (persistent, communal) but must
	// never appear on a player row; scope the check to the /players response bodies alone.
	playersOnly := integrationRequest(h, "GET", "/api/integration/v1/players", token).Body.String() +
		integrationRequest(h, "GET", "/api/integration/v1/players/"+uidA, token).Body.String()
	if strings.Contains(playersOnly, `"location"`) {
		t.Fatalf("player rows leaked location:\n%s", playersOnly)
	}
	_ = restCalls // background poller call volume is not asserted here; see the dedicated upstream-isolation test
}

// TestIntegrationServerUnreachableShapeWithNoSnapshot proves spec §4/§12.13's unreachable
// contract when no successful Info snapshot has ever been cached: 200, state "unreachable",
// zero-value fields, never a 5xx.
func TestIntegrationServerUnreachableShapeWithNoSnapshot(t *testing.T) {
	s, h, st := newIntegrationTestServer(t, nil)
	token := issueTestAPIKey(t, s, st, "aaaaaaaa", "bot")
	rr := integrationRequest(h, "GET", "/api/integration/v1/server", token)
	if rr.Code != 200 {
		t.Fatalf("status=%d body=%s", rr.Code, rr.Body.String())
	}
	var doc struct {
		Data struct {
			Name, Description, Version, State string
			UptimeSec                         int64
		} `json:"data"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &doc); err != nil {
		t.Fatal(err)
	}
	if doc.Data.State != "unreachable" || doc.Data.Name != "" || doc.Data.Version != "" || doc.Data.UptimeSec != 0 {
		t.Fatalf("unreachable /server shape = %#v", doc.Data)
	}
}

// TestIntegrationServerReachableUsesCachedSnapshotNoUpstreamCalls proves spec §4/§S1/§12.13:
// once a snapshot is cached, the integration /server handler serves it directly and makes
// zero additional upstream REST calls of its own, regardless of request volume.
func TestIntegrationServerReachableUsesCachedSnapshotNoUpstreamCalls(t *testing.T) {
	var calls atomic.Int64
	rest := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/v1/api/info":
			_, _ = io.WriteString(w, `{"servername":"Cached Server","version":"1.2.3","worldguid":"guid","uptime":42}`)
		case "/v1/api/metrics":
			_, _ = io.WriteString(w, `{"serverfps":60,"serverframetime":16.6,"currentplayernum":0,"maxplayernum":32,"uptime":42,"days":1,"basecampnum":0}`)
		case "/v1/api/players":
			_, _ = io.WriteString(w, `{"players":[]}`)
		default:
			http.NotFound(w, r)
		}
	})
	st, err := storeOpenTemp(t)
	if err != nil {
		t.Fatal(err)
	}
	srv := httptest.NewServer(rest)
	t.Cleanup(srv.Close)
	cfg := config.Config{
		AdminPassword: "panelpass", SessionSecret: strings.Repeat("s", 48),
		RESTURL: srv.URL, RESTUser: "admin", PalworldPassword: "gamepass",
		IntegrationRateLimit: 60, MetricsInterval: 10 * time.Millisecond, PlayersInterval: time.Hour, SaveSyncInterval: time.Hour,
	}
	s, h := New(cfg, st, testLogger())
	token := issueTestAPIKey(t, s, st, "aaaaaaaa", "bot")
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() { s.RunPollers(ctx); close(done) }()
	waitForCachedInfoReachable(t, s)
	// Stop all background polling deterministically before measuring: RunPollers only
	// returns after every sub-loop has exited, so no poller-driven call can race the
	// baseline read below.
	cancel()
	<-done

	baseline := calls.Load()
	for i := 0; i < 5; i++ {
		rr := integrationRequest(h, "GET", "/api/integration/v1/server", token)
		if rr.Code != 200 {
			t.Fatalf("request %d status=%d body=%s", i, rr.Code, rr.Body.String())
		}
	}
	if got := calls.Load(); got != baseline {
		t.Fatalf("integration /server made %d upstream calls, want 0 (baseline %d)", got-baseline, baseline)
	}
	var doc struct {
		Data struct{ Name, Version, State string } `json:"data"`
	}
	rr := integrationRequest(h, "GET", "/api/integration/v1/server", token)
	if err := json.Unmarshal(rr.Body.Bytes(), &doc); err != nil {
		t.Fatal(err)
	}
	if doc.Data.Name != "Cached Server" || doc.Data.Version != "1.2.3" || doc.Data.State != "running" {
		t.Fatalf("reachable /server shape = %#v", doc.Data)
	}
}

func waitForCachedInfoReachable(t *testing.T, s *Server) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for {
		if _, ok := s.poll.CachedInfo(); ok {
			return
		}
		if time.Now().After(deadline) {
			t.Fatal("poller never cached a reachable Info snapshot")
		}
		time.Sleep(5 * time.Millisecond)
	}
}

// TestIntegrationRouteGroupUnknownPathUniformlyGated proves spec §1/§4: an unknown path
// under the mount returns the uniform 401 without a token (auth precedes routing) and 404
// not_found with a valid token (path genuinely does not exist).
func TestIntegrationRouteGroupUnknownPathUniformlyGated(t *testing.T) {
	s, h, st := newIntegrationTestServer(t, nil)
	token := issueTestAPIKey(t, s, st, "aaaaaaaa", "bot")
	if rr := integrationRequest(h, "GET", "/api/integration/v1/does-not-exist", ""); rr.Code != 401 {
		t.Fatalf("unauthenticated unknown path status=%d body=%s", rr.Code, rr.Body.String())
	}
	rr := integrationRequest(h, "GET", "/api/integration/v1/does-not-exist", token)
	if rr.Code != 404 {
		t.Fatalf("authenticated unknown path status=%d body=%s", rr.Code, rr.Body.String())
	}
	var envelope struct {
		Error struct{ Code, Message string } `json:"error"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &envelope); err != nil {
		t.Fatal(err)
	}
	if envelope.Error.Code != "not_found" {
		t.Fatalf("code=%q", envelope.Error.Code)
	}
}

// TestIntegrationMethodNotAllowedUsesStandardEnvelope proves spec §1: a non-GET method on a
// real path is rejected after auth, with Allow: GET and the standard error envelope.
func TestIntegrationMethodNotAllowedUsesStandardEnvelope(t *testing.T) {
	s, h, st := newIntegrationTestServer(t, nil)
	token := issueTestAPIKey(t, s, st, "aaaaaaaa", "bot")
	req := httptest.NewRequest("POST", "/api/integration/v1/players", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)
	if rr.Code != 405 {
		t.Fatalf("status=%d body=%s", rr.Code, rr.Body.String())
	}
	if got := rr.Header().Get("Allow"); got != "GET" {
		t.Fatalf("Allow=%q", got)
	}
}

// TestIntegrationHeadersOnEveryStatus proves spec §5/§12.10: Cache-Control: no-store and the
// v0.3.0 security header set accompany every integration response regardless of status.
func TestIntegrationHeadersOnEveryStatus(t *testing.T) {
	s, h, st := newIntegrationTestServer(t, nil)
	token := issueTestAPIKey(t, s, st, "aaaaaaaa", "bot")
	cases := []struct {
		name, method, path, token string
		want                      int
	}{
		{"200", "GET", "/api/integration/v1/guilds", token, 200},
		{"401", "GET", "/api/integration/v1/guilds", "", 401},
		{"404", "GET", "/api/integration/v1/nope", token, 404},
		{"400", "GET", "/api/integration/v1/players?limit=0", token, 400},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			rr := integrationRequest(h, tc.method, tc.path, tc.token)
			if rr.Code != tc.want {
				t.Fatalf("status=%d, want %d, body=%s", rr.Code, tc.want, rr.Body.String())
			}
			if got := rr.Header().Get("Cache-Control"); got != "no-store" {
				t.Errorf("Cache-Control=%q", got)
			}
			for _, header := range []string{"Content-Security-Policy", "X-Content-Type-Options", "X-Frame-Options", "Referrer-Policy", "Permissions-Policy"} {
				if rr.Header().Get(header) == "" {
					t.Errorf("missing %s", header)
				}
			}
		})
	}
}

// TestIntegrationRateLimit429Envelope proves spec §8.1's exact envelope and Retry-After via
// a real HTTP round trip, using a one-request-per-minute key so the second request in the
// same test is guaranteed to be denied.
func TestIntegrationRateLimit429Envelope(t *testing.T) {
	st, err := storeOpenTemp(t)
	if err != nil {
		t.Fatal(err)
	}
	cfg := config.Config{AdminPassword: "panelpass", SessionSecret: strings.Repeat("s", 48), IntegrationRateLimit: 1}
	s, h := New(cfg, st, testLogger())
	token := issueTestAPIKey(t, s, st, "aaaaaaaa", "bot")
	if rr := integrationRequest(h, "GET", "/api/integration/v1/guilds", token); rr.Code != 200 {
		t.Fatalf("first request status=%d body=%s", rr.Code, rr.Body.String())
	}
	rr := integrationRequest(h, "GET", "/api/integration/v1/guilds", token)
	if rr.Code != 429 {
		t.Fatalf("second request status=%d body=%s", rr.Code, rr.Body.String())
	}
	if got := rr.Header().Get("Retry-After"); got == "" || got == "0" {
		t.Fatalf("Retry-After=%q", got)
	}
	want := `{"error":{"code":"rate_limited","message":"API key rate limit exceeded; retry later."}}` + "\n"
	if rr.Body.String() != want {
		t.Fatalf("body=%s want=%s", rr.Body.String(), want)
	}
}

// TestIntegrationRateLimitIgnoresForwardedHeaders proves spec §8.1: the limiter is keyed by
// key id, so X-Forwarded-For cannot be used to evade or fragment it.
func TestIntegrationRateLimitIgnoresForwardedHeaders(t *testing.T) {
	st, err := storeOpenTemp(t)
	if err != nil {
		t.Fatal(err)
	}
	cfg := config.Config{AdminPassword: "panelpass", SessionSecret: strings.Repeat("s", 48), IntegrationRateLimit: 1}
	s, h := New(cfg, st, testLogger())
	token := issueTestAPIKey(t, s, st, "aaaaaaaa", "bot")
	req1 := httptest.NewRequest("GET", "/api/integration/v1/guilds", nil)
	req1.Header.Set("Authorization", "Bearer "+token)
	req1.Header.Set("X-Forwarded-For", "203.0.113.1")
	rr1 := httptest.NewRecorder()
	h.ServeHTTP(rr1, req1)
	if rr1.Code != 200 {
		t.Fatalf("first status=%d body=%s", rr1.Code, rr1.Body.String())
	}
	req2 := httptest.NewRequest("GET", "/api/integration/v1/guilds", nil)
	req2.Header.Set("Authorization", "Bearer "+token)
	req2.Header.Set("X-Forwarded-For", "198.51.100.99") // different spoofed source, same key
	rr2 := httptest.NewRecorder()
	h.ServeHTTP(rr2, req2)
	if rr2.Code != 429 {
		t.Fatalf("spoofed-source second request status=%d, want 429 (limiter must key on the token, not IP)", rr2.Code)
	}
}

// TestIntegrationETagRoundTrip proves spec §7.2: a 200 carries a weak ETag, and replaying it
// via If-None-Match yields a 304 with no body and the same headers.
func TestIntegrationETagRoundTrip(t *testing.T) {
	s, h, st := newIntegrationTestServer(t, nil)
	seedBasicWorld(t, st)
	token := issueTestAPIKey(t, s, st, "aaaaaaaa", "bot")
	rr := integrationRequest(h, "GET", "/api/integration/v1/guilds", token)
	if rr.Code != 200 {
		t.Fatalf("status=%d body=%s", rr.Code, rr.Body.String())
	}
	etag := rr.Header().Get("ETag")
	if etag == "" || !strings.HasPrefix(etag, `W/"`) {
		t.Fatalf("ETag=%q", etag)
	}
	req := httptest.NewRequest("GET", "/api/integration/v1/guilds", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("If-None-Match", etag)
	rr2 := httptest.NewRecorder()
	h.ServeHTTP(rr2, req)
	if rr2.Code != 304 {
		t.Fatalf("revalidation status=%d body=%s", rr2.Code, rr2.Body.String())
	}
	if rr2.Body.Len() != 0 {
		t.Fatalf("304 carried a body: %s", rr2.Body.String())
	}
	if got := rr2.Header().Get("ETag"); got != etag {
		t.Fatalf("304 ETag=%q, want %q", got, etag)
	}
	if got := rr2.Header().Get("Cache-Control"); got != "no-store" {
		t.Fatalf("304 Cache-Control=%q", got)
	}
	// A bare "*" matches any current representation.
	req3 := httptest.NewRequest("GET", "/api/integration/v1/guilds", nil)
	req3.Header.Set("Authorization", "Bearer "+token)
	req3.Header.Set("If-None-Match", "*")
	rr3 := httptest.NewRecorder()
	h.ServeHTTP(rr3, req3)
	if rr3.Code != 304 {
		t.Fatalf("wildcard revalidation status=%d", rr3.Code)
	}
}

// TestIntegrationUIDPathValidationRejectsBeforeStoreCall proves spec §4: a uid that would
// resolve as a SQL LIKE wildcard through the session API's ResolveUID must 404 on the
// integration surface without ever reaching the store - proven structurally by seeding one
// real player and confirming a "%"/"_" wildcard request does not resolve to it.
func TestIntegrationUIDPathValidationRejectsBeforeStoreCall(t *testing.T) {
	s, h, st := newIntegrationTestServer(t, nil)
	uidA, _, _, _ := seedBasicWorld(t, st)
	token := issueTestAPIKey(t, s, st, "aaaaaaaa", "bot")

	// A genuinely valid uid still resolves.
	if rr := integrationRequest(h, "GET", "/api/integration/v1/players/"+uidA, token); rr.Code != 200 {
		t.Fatalf("valid uid status=%d body=%s", rr.Code, rr.Body.String())
	}
	for _, path := range []string{
		"/api/integration/v1/players/%25",                        // percent-encoded '%' - a SQL LIKE wildcard
		"/api/integration/v1/players/_",                          // SQL LIKE single-char wildcard
		"/api/integration/v1/players/" + strings.Repeat("a", 37), // over-length
		"/api/integration/v1/players/not-hex-zz",
	} {
		rr := integrationRequest(h, "GET", path, token)
		if rr.Code != 404 {
			t.Fatalf("%s status=%d body=%s, want 404 (must not wildcard-resolve to the seeded player)", path, rr.Code, rr.Body.String())
		}
	}
}

// TestIntegrationPaginationWalkAndExactMultipleFinalPage proves spec §7.1: deterministic
// keyset order, a non-null cursor on a full page, a null cursor on a short page, and an
// exact multiple of limit yielding an empty-but-200 final page with a null cursor.
func TestIntegrationPaginationWalkAndExactMultipleFinalPage(t *testing.T) {
	s, h, st := newIntegrationTestServer(t, nil)
	ctx := context.Background()
	const total = 6
	var players []sav.Player
	for i := 1; i <= total; i++ {
		players = append(players, sav.Player{UID: fmt.Sprintf("%032x", i), Nickname: fmt.Sprintf("p%02d", i)})
	}
	if err := st.ReplaceWorld(ctx, &sav.World{Players: players}, time.Now().UTC(), 0); err != nil {
		t.Fatal(err)
	}
	token := issueTestAPIKey(t, s, st, "aaaaaaaa", "bot")

	seen := map[string]bool{}
	cursor := ""
	pages := 0
	for {
		path := "/api/integration/v1/players?limit=3"
		if cursor != "" {
			path += "&cursor=" + cursor
		}
		rr := integrationRequest(h, "GET", path, token)
		if rr.Code != 200 {
			t.Fatalf("page %d status=%d body=%s", pages, rr.Code, rr.Body.String())
		}
		var doc struct {
			Data []struct {
				UID string `json:"uid"`
			} `json:"data"`
			NextCursor *string `json:"nextCursor"`
		}
		if err := json.Unmarshal(rr.Body.Bytes(), &doc); err != nil {
			t.Fatal(err)
		}
		pages++
		for _, p := range doc.Data {
			if seen[p.UID] {
				t.Fatalf("uid %s returned twice across pages", p.UID)
			}
			seen[p.UID] = true
		}
		if doc.NextCursor == nil {
			if len(doc.Data) == 3 {
				t.Fatalf("full page paired with a null cursor")
			}
			break
		}
		if len(doc.Data) != 3 {
			t.Fatalf("short page paired with a non-null cursor")
		}
		cursor = *doc.NextCursor
		if pages > total {
			t.Fatal("pagination did not terminate")
		}
	}
	if len(seen) != total {
		t.Fatalf("saw %d distinct uids across %d pages, want %d", len(seen), pages, total)
	}
	// total is an exact multiple of the page size, so the walk must have taken exactly
	// total/limit+1 pages: the last one empty (spec §7.1's normative exact-multiple case).
	if pages != total/3+1 {
		t.Fatalf("pages=%d, want %d (exact-multiple must yield one extra empty final page)", pages, total/3+1)
	}
}

// TestIntegrationCursorTamperAndLimitBounds proves spec §7.1: an undecodable or
// wrong-version cursor and an out-of-range limit both 400 with the documented codes.
func TestIntegrationCursorTamperAndLimitBounds(t *testing.T) {
	s, h, st := newIntegrationTestServer(t, nil)
	token := issueTestAPIKey(t, s, st, "aaaaaaaa", "bot")
	cases := []struct {
		name, query, code string
	}{
		{"undecodable cursor", "cursor=not-valid-base64!!!", "invalid_cursor"},
		{"wrong version tag", "cursor=" + encodeWrongVersionCursor("11111111111111111111111111111111"), "invalid_cursor"},
		{"limit zero", "limit=0", "invalid_limit"},
		{"limit too large", "limit=501", "invalid_limit"},
		{"limit not an integer", "limit=abc", "invalid_limit"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			rr := integrationRequest(h, "GET", "/api/integration/v1/players?"+tc.query, token)
			if rr.Code != 400 {
				t.Fatalf("status=%d body=%s", rr.Code, rr.Body.String())
			}
			var envelope struct {
				Error struct{ Code string } `json:"error"`
			}
			if err := json.Unmarshal(rr.Body.Bytes(), &envelope); err != nil {
				t.Fatal(err)
			}
			if envelope.Error.Code != tc.code {
				t.Fatalf("code=%q, want %q", envelope.Error.Code, tc.code)
			}
		})
	}
}

func encodeWrongVersionCursor(key string) string {
	return base64.RawURLEncoding.EncodeToString([]byte("v2|" + key))
}

// TestIntegrationOnlineFilterZeroAndNonZero proves spec §7.1's ?online=true mechanics: the
// zero-online short-circuit returns an empty 200 page without a query, and a nonzero online
// set returns exactly those players regardless of where their uids sort in the keyset.
func TestIntegrationOnlineFilterZeroAndNonZero(t *testing.T) {
	const total = 5
	onlineIdx := map[int]bool{2: true, 5: true} // 1-indexed; player 5 sorts last
	var restPlayers []string
	for i, on := range onlineIdx {
		if on {
			restPlayers = append(restPlayers, fmt.Sprintf(`{"name":"p%02d","accountName":"a","playerId":"%032x","userId":"steam_%d","level":1}`, i, i, i))
		}
	}
	playersJSON := "[" + strings.Join(restPlayers, ",") + "]"

	rest := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/v1/api/info":
			_, _ = io.WriteString(w, `{"servername":"s","version":"1","worldguid":"g","uptime":1}`)
		case "/v1/api/metrics":
			_, _ = io.WriteString(w, `{"serverfps":60,"serverframetime":16.6,"currentplayernum":2,"maxplayernum":32,"uptime":1,"days":1,"basecampnum":0}`)
		case "/v1/api/players":
			_, _ = io.WriteString(w, `{"players":`+playersJSON+`}`)
		default:
			http.NotFound(w, r)
		}
	})
	st, err := storeOpenTemp(t)
	if err != nil {
		t.Fatal(err)
	}
	srv := httptest.NewServer(rest)
	t.Cleanup(srv.Close)
	cfg := config.Config{
		AdminPassword: "panelpass", SessionSecret: strings.Repeat("s", 48),
		RESTURL: srv.URL, RESTUser: "admin", PalworldPassword: "gamepass",
		IntegrationRateLimit: 60, MetricsInterval: time.Hour, PlayersInterval: 10 * time.Millisecond, SaveSyncInterval: time.Hour,
	}
	s, h := New(cfg, st, testLogger())
	token := issueTestAPIKey(t, s, st, "aaaaaaaa", "bot")

	ctx := context.Background()
	var players []sav.Player
	for i := 1; i <= total; i++ {
		players = append(players, sav.Player{UID: fmt.Sprintf("%032x", i), Nickname: fmt.Sprintf("p%02d", i)})
	}
	if err := st.ReplaceWorld(ctx, &sav.World{Players: players}, time.Now().UTC(), 0); err != nil {
		t.Fatal(err)
	}

	// Zero-online short-circuit, proven before any REST poll has populated the online map.
	rr := integrationRequest(h, "GET", "/api/integration/v1/players?online=true", token)
	if rr.Code != 200 {
		t.Fatalf("zero-online status=%d body=%s", rr.Code, rr.Body.String())
	}
	var zeroDoc struct {
		Data       []any   `json:"data"`
		NextCursor *string `json:"nextCursor"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &zeroDoc); err != nil {
		t.Fatal(err)
	}
	if len(zeroDoc.Data) != 0 || zeroDoc.NextCursor != nil {
		t.Fatalf("zero-online page = %#v, want empty data and null cursor", zeroDoc)
	}

	runCtx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() { s.RunPollers(runCtx); close(done) }()
	t.Cleanup(func() { cancel(); <-done })
	waitForOnlineCount(t, s, len(onlineIdx))

	rr = integrationRequest(h, "GET", "/api/integration/v1/players?online=true&limit=100", token)
	if rr.Code != 200 {
		t.Fatalf("online=true status=%d body=%s", rr.Code, rr.Body.String())
	}
	var doc struct {
		Data []struct {
			UID    string `json:"uid"`
			Online bool   `json:"online"`
		} `json:"data"`
		NextCursor *string `json:"nextCursor"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &doc); err != nil {
		t.Fatal(err)
	}
	if doc.NextCursor != nil {
		t.Fatalf("nextCursor=%v, want null on a short page", doc.NextCursor)
	}
	if len(doc.Data) != len(onlineIdx) {
		t.Fatalf("online page returned %d players, want %d: %#v", len(doc.Data), len(onlineIdx), doc.Data)
	}
	for _, p := range doc.Data {
		if !p.Online {
			t.Fatalf("player %s in the online-filtered page was not marked online", p.UID)
		}
	}
}

func waitForOnlineCount(t *testing.T, s *Server, want int) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for {
		if len(s.poll.Online()) == want {
			return
		}
		if time.Now().After(deadline) {
			t.Fatalf("online set never reached %d entries, last saw %d", want, len(s.poll.Online()))
		}
		time.Sleep(5 * time.Millisecond)
	}
}

// TestIntegrationPalsIncludesOwner proves the bulk /pals endpoint joins owner uid/name
// (spec §4), avoiding an N+1 per-player call.
func TestIntegrationPalsIncludesOwner(t *testing.T) {
	s, h, st := newIntegrationTestServer(t, nil)
	uidA, _, _, palID := seedBasicWorld(t, st)
	token := issueTestAPIKey(t, s, st, "aaaaaaaa", "bot")
	rr := integrationRequest(h, "GET", "/api/integration/v1/pals", token)
	if rr.Code != 200 {
		t.Fatalf("status=%d body=%s", rr.Code, rr.Body.String())
	}
	var doc struct {
		Data []struct {
			InstanceID, OwnerUID, OwnerName, OwnerSource string
			Placement                                    string
			BaseID                                       *string
			OwnerResolved                                bool
			InParty                                      bool
			PartySlot, BoxPage, BoxSlot                  *int
			HP                                           *float64
			Gender                                       string
			Talents                                      integrationPalTalents
			PassiveSkillIDs, EquippedSkillIDs            []string
		} `json:"data"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &doc); err != nil {
		t.Fatal(err)
	}
	if len(doc.Data) != 1 || doc.Data[0].InstanceID != palID || doc.Data[0].OwnerUID != uidA || doc.Data[0].OwnerName != "Player One" || doc.Data[0].OwnerSource != "personal_container" || !doc.Data[0].OwnerResolved || !doc.Data[0].InParty || doc.Data[0].PartySlot == nil || *doc.Data[0].PartySlot != 2 || doc.Data[0].BoxPage != nil || doc.Data[0].BoxSlot != nil {
		t.Fatalf("pals = %#v", doc.Data)
	}
	pal := doc.Data[0]
	if pal.Placement != "party" || pal.BaseID != nil {
		t.Fatalf("safe placement = %#v", pal)
	}
	if pal.HP == nil || *pal.HP != 432.5 || pal.Gender != "female" || pal.Talents.HP == nil || *pal.Talents.HP != 71 || len(pal.PassiveSkillIDs) != 1 || pal.PassiveSkillIDs[0] != "CraftSpeed_up2" || len(pal.EquippedSkillIDs) != 1 || pal.EquippedSkillIDs[0] != "AirCanon" {
		t.Fatalf("rich pal fields = %#v", pal)
	}
}

func TestIntegrationPalsExposesDerivedBaseWorkerWithoutContainerGUID(t *testing.T) {
	s, h, st := newIntegrationTestServer(t, nil)
	const uid = "11111111111111111111111111111111"
	const guildID = "22222222222222222222222222222222"
	const baseID = "33333333333333333333333333333333"
	const containerID = "44444444444444444444444444444444"
	const palID = "55555555555555555555555555555555"
	w := &sav.World{
		Players: []sav.Player{{UID: uid, Nickname: "Builder", GuildID: guildID}},
		Pals:    []sav.Pal{{InstanceID: palID, CharacterID: "Anubis", OwnerUID: uid, ContainerID: containerID, BaseID: baseID, SlotIndex: 3}},
		Guilds:  []sav.Guild{{ID: guildID, Name: "Builders", MemberUIDs: []string{uid}}},
		Bases:   []sav.BaseCamp{{ID: baseID, GuildID: guildID}},
	}
	if err := st.ReplaceWorld(context.Background(), w, time.Now().UTC(), time.Millisecond); err != nil {
		t.Fatal(err)
	}
	token := issueTestAPIKey(t, s, st, "bbbbbbbb", "base-bot")
	rr := integrationRequest(h, "GET", "/api/integration/v1/pals", token)
	if rr.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", rr.Code, rr.Body.String())
	}
	var doc struct {
		Data []struct {
			Placement string  `json:"placement"`
			BaseID    *string `json:"baseId"`
		} `json:"data"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &doc); err != nil {
		t.Fatal(err)
	}
	if len(doc.Data) != 1 || doc.Data[0].Placement != "base" || doc.Data[0].BaseID == nil || *doc.Data[0].BaseID != baseID {
		t.Fatalf("derived base placement = %#v", doc.Data)
	}
	if strings.Contains(rr.Body.String(), containerID) || strings.Contains(rr.Body.String(), `"containerId"`) {
		t.Fatalf("raw worker container leaked: %s", rr.Body.String())
	}
}

// TestIntegrationSaveStatusFieldsStayOnContractedSurfaces proves that save-derived list
// envelopes carry freshness/drift while /server carries the dedicated nested save object.
func TestIntegrationSaveStatusFieldsStayOnContractedSurfaces(t *testing.T) {
	s, h, st := newIntegrationTestServer(t, nil)
	seedBasicWorld(t, st)
	token := issueTestAPIKey(t, s, st, "aaaaaaaa", "bot")
	for _, ep := range []string{"/api/integration/v1/players", "/api/integration/v1/pals", "/api/integration/v1/guilds"} {
		body := integrationRequest(h, "GET", ep, token).Body.String()
		if !strings.Contains(body, `"lastParseAt"`) {
			t.Errorf("%s missing lastParseAt", ep)
		}
		if !strings.Contains(body, `"formatDrift"`) {
			t.Errorf("%s missing formatDrift", ep)
		}
	}
	for _, ep := range []string{"/api/integration/v1/map", "/api/integration/v1/metrics/current", "/api/integration/v1/world/summary"} {
		body := integrationRequest(h, "GET", ep, token).Body.String()
		if strings.Contains(body, `"lastParseAt"`) {
			t.Errorf("%s unexpectedly carries lastParseAt", ep)
		}
		if strings.Contains(body, `"nextCursor"`) {
			t.Errorf("%s unexpectedly carries nextCursor", ep)
		}
	}
	serverBody := integrationRequest(h, "GET", "/api/integration/v1/server", token).Body.String()
	if !strings.Contains(serverBody, `"save"`) || !strings.Contains(serverBody, `"lastParseAt"`) || !strings.Contains(serverBody, `"formatDrift"`) {
		t.Errorf("/server missing nested save status: %s", serverBody)
	}
}

func TestIntegrationServerSaveStates(t *testing.T) {
	cases := []struct {
		name                  string
		world                 *sav.World
		wantState             string
		wantDrift             bool
		wantLastParse         bool
		players, pals, guilds int
	}{
		{name: "unknown", wantState: "unknown"},
		{name: "ok", world: &sav.World{Players: []sav.Player{{UID: hexUID(1)}}, Pals: []sav.Pal{{InstanceID: hexUID(2)}}, Guilds: []sav.Guild{{ID: hexUID(3)}}}, wantState: "ok", wantLastParse: true, players: 1, pals: 1, guilds: 1},
		{name: "drift", world: &sav.World{Players: []sav.Player{{UID: hexUID(1)}, {UID: hexUID(2)}}, Stats: sav.ParseStats{SkippedProperties: 2}}, wantState: "drift", wantDrift: true, wantLastParse: true, players: 2},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			s, h, st := newIntegrationTestServer(t, nil)
			if tc.world != nil {
				if err := st.ReplaceWorld(context.Background(), tc.world, time.Date(2026, 7, 10, 3, 4, 5, 0, time.UTC), time.Millisecond); err != nil {
					t.Fatal(err)
				}
			}
			token := issueTestAPIKey(t, s, st, "aaaaaaaa", "bot")
			rr := integrationRequest(h, "GET", "/api/integration/v1/server", token)
			if rr.Code != http.StatusOK {
				t.Fatalf("status=%d body=%s", rr.Code, rr.Body.String())
			}
			var doc struct {
				Data struct {
					Save struct {
						State       string     `json:"state"`
						FormatDrift bool       `json:"formatDrift"`
						LastParseAt *time.Time `json:"lastParseAt"`
						Players     int        `json:"players"`
						Pals        int        `json:"pals"`
						Guilds      int        `json:"guilds"`
					} `json:"save"`
				} `json:"data"`
			}
			if err := json.Unmarshal(rr.Body.Bytes(), &doc); err != nil {
				t.Fatal(err)
			}
			got := doc.Data.Save
			if got.State != tc.wantState || got.FormatDrift != tc.wantDrift || (got.LastParseAt != nil) != tc.wantLastParse || got.Players != tc.players || got.Pals != tc.pals || got.Guilds != tc.guilds {
				t.Fatalf("save = %#v, want state=%q drift=%v parsed=%v counts=%d/%d/%d", got, tc.wantState, tc.wantDrift, tc.wantLastParse, tc.players, tc.pals, tc.guilds)
			}
		})
	}
}

func storeOpenTemp(t *testing.T) (*store.Store, error) {
	t.Helper()
	st, err := store.Open(t.TempDir() + "/test.db")
	if err != nil {
		return nil, err
	}
	t.Cleanup(func() { _ = st.Close() })
	return st, nil
}
