// Adversarial data-handling audit of the v0.4.0 Integration API (spec:
// docs/specs/integration-api.md). Scope: redaction, pagination, freshness, and JSON
// encoding — auth/limiter/scope enumeration belong to a sibling audit and are not
// duplicated here. Every redaction test proves its fixture non-vacuous against the
// session API first, guarding against fixtures that pass trivially.
package server

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/8tp/palhelm/internal/config"
	"github.com/8tp/palhelm/internal/sav"
	"github.com/8tp/palhelm/internal/store"
)

// auditDataServer builds a wired server with an effectively unlimited integration rate
// limit (these tests issue dozens of paginated requests per key) and an optional config
// mutator for poller intervals / data dir. It deliberately does not touch the shared
// newIntegrationTestServer helper.
func auditDataServer(t *testing.T, rest http.Handler, mutate func(*config.Config)) (*Server, http.Handler, *store.Store) {
	t.Helper()
	st, err := store.Open(filepath.Join(t.TempDir(), "audit.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.Close() })
	restURL := ""
	if rest != nil {
		srv := httptest.NewServer(rest)
		t.Cleanup(srv.Close)
		restURL = srv.URL
	}
	cfg := config.Config{
		AdminPassword: "panelpass", SessionSecret: strings.Repeat("s", 48),
		RESTURL: restURL, RESTUser: "admin", PalworldPassword: "gamepass",
		IntegrationRateLimit: 1_000_000,
	}
	if mutate != nil {
		mutate(&cfg)
	}
	s, h := New(cfg, st, testLogger())
	return s, h, st
}

// auditDecode unmarshals a response body, failing the test on malformed JSON.
func auditDecode(t *testing.T, body []byte) map[string]any {
	t.Helper()
	var m map[string]any
	if err := json.Unmarshal(body, &m); err != nil {
		t.Fatalf("undecodable response body: %v\n%s", err, body)
	}
	return m
}

// auditAssertKeys asserts m has exactly the given key set — a redacted field must not
// exist even as a key with a null value (spec §6: absent, not null).
func auditAssertKeys(t *testing.T, label string, m map[string]any, want ...string) {
	t.Helper()
	got := make([]string, 0, len(m))
	for k := range m {
		got = append(got, k)
	}
	sort.Strings(got)
	sort.Strings(want)
	if strings.Join(got, ",") != strings.Join(want, ",") {
		t.Errorf("%s keys = %v, want exactly %v", label, got, want)
	}
}

// auditWalk pages through a keyset endpoint collecting the given data key from every row,
// asserting the §7.1 normative invariants on every page: page size never exceeds limit, a
// short page always carries a null cursor, a full page's cursor is non-null (unless it is
// the last), empty data never pairs with a non-null cursor, and no key is ever duplicated.
// betweenPages, when non-nil, runs after each page fetch (the churn hook).
func auditWalk(t *testing.T, h http.Handler, token, path, keyField string, limit int, betweenPages func(page int)) []string {
	t.Helper()
	var order []string
	seen := map[string]bool{}
	cursor := ""
	sep := "?"
	if strings.Contains(path, "?") {
		sep = "&"
	}
	for page := 0; ; page++ {
		u := fmt.Sprintf("%s%slimit=%d", path, sep, limit)
		if cursor != "" {
			u += "&cursor=" + cursor
		}
		rr := integrationRequest(h, "GET", u, token)
		if rr.Code != 200 {
			t.Fatalf("page %d of %s: status=%d body=%s", page, path, rr.Code, rr.Body.String())
		}
		var doc struct {
			Data       []map[string]any `json:"data"`
			NextCursor *string          `json:"nextCursor"`
		}
		if err := json.Unmarshal(rr.Body.Bytes(), &doc); err != nil {
			t.Fatal(err)
		}
		if len(doc.Data) > limit {
			t.Fatalf("page %d of %s returned %d rows, over limit %d", page, path, len(doc.Data), limit)
		}
		if len(doc.Data) == 0 && doc.NextCursor != nil {
			t.Fatalf("page %d of %s: empty data paired with a non-null cursor (impossible by construction per spec §7.1)", page, path)
		}
		if len(doc.Data) < limit && doc.NextCursor != nil {
			t.Fatalf("page %d of %s: short page (%d < %d) paired with a non-null cursor", page, path, len(doc.Data), limit)
		}
		for _, row := range doc.Data {
			key, _ := row[keyField].(string)
			if key == "" {
				t.Fatalf("page %d of %s: row missing %s: %#v", page, path, keyField, row)
			}
			if seen[key] {
				t.Fatalf("%s %q returned twice while paginating %s", keyField, key, path)
			}
			seen[key] = true
			order = append(order, key)
		}
		if betweenPages != nil {
			betweenPages(page)
		}
		if doc.NextCursor == nil {
			return order
		}
		cursor = *doc.NextCursor
		if page > 1000 {
			t.Fatalf("pagination of %s did not terminate", path)
		}
	}
}

// hexUID renders n as the canonical 32-char lowercase hex uid used throughout the store.
func hexUID(n int) string { return fmt.Sprintf("%032x", n) }

// TestAuditRedactionSentinelsAcrossPagesErrorsAnd304 is the widened §12.4 proof. The
// existing redaction test checks one player and only the seven happy-path 200s; this one
// plants sentinels on two players (one sorting mid-list), first proves the session API
// really serves them (non-vacuous), then sweeps every page of every paginated walk, both
// player details, /guilds, every 4xx/405 error body, the unauthorized 401, and a 304 —
// asserting no sentinel value and no redacted field name appears anywhere.
//
// Threat: a redacted field leaking through a less-traveled surface (a later page, an error
// body, a revalidation response) that single-response tests never look at.
func TestAuditRedactionSentinelsAcrossPagesErrorsAnd304(t *testing.T) {
	s, h, st := auditDataServer(t, nil, nil)
	ctx := context.Background()

	const total = 6
	guildID := hexUID(0x900)
	var players []sav.Player
	var members []sav.GuildMember
	for i := 1; i <= total; i++ {
		players = append(players, sav.Player{UID: hexUID(i), Nickname: fmt.Sprintf("p%02d", i), Level: int32(i), GuildID: guildID})
		members = append(members, sav.GuildMember{UID: hexUID(i), Name: fmt.Sprintf("p%02d", i)})
	}
	w := &sav.World{
		Players: players,
		Pals:    []sav.Pal{{InstanceID: hexUID(0xA00), CharacterID: "SheepBall", Level: 3, OwnerUID: hexUID(2)}},
		Guilds:  []sav.Guild{{ID: guildID, Name: "Audit Guild", AdminUID: hexUID(1), Members: members}},
		Bases:   []sav.BaseCamp{{ID: hexUID(0xB00), GuildID: guildID, Position: &sav.Vector{X: 100, Y: 200}}},
	}
	if err := st.ReplaceWorld(ctx, w, time.Now().UTC(), 0); err != nil {
		t.Fatal(err)
	}
	// Sentinels on players 2 (sorts early) and 5 (sorts late): steamId, accountName, ping,
	// location, banned, whitelisted, and an open session each.
	sentinelValues := []string{
		"STEAM-AUDIT-76561198999000002", "ACCOUNT-AUDIT-two", "STEAM-AUDIT-76561198999000005", "ACCOUNT-AUDIT-five",
		"131.25", "1234.5", "6789.25", // ping / location floats, chosen exactly representable and unique
	}
	for i, uid := range []string{hexUID(2), hexUID(5)} {
		x, y := 1234.5, 6789.25
		steam, account := sentinelValues[i*2], sentinelValues[i*2+1]
		if err := st.UpsertLivePlayer(ctx, store.Player{
			UID: uid, SteamID: steam, Name: fmt.Sprintf("p%02d", []int{2, 5}[i]),
			AccountName: account, Level: 10, Ping: 131.25, X: &x, Y: &y, Raw: []byte(`{"rawLeak":"RAWJSON-AUDIT-sentinel"}`),
		}, time.Now().UTC()); err != nil {
			t.Fatal(err)
		}
		flag := true
		if err := st.SetPlayerFlags(ctx, uid, &flag, &flag); err != nil {
			t.Fatal(err)
		}
		if err := st.StartSession(ctx, uid, time.Now().UTC()); err != nil {
			t.Fatal(err)
		}
	}
	sentinelValues = append(sentinelValues, "RAWJSON-AUDIT-sentinel")
	token := issueTestAPIKey(t, s, st, "aaaaaaaa", "audit-bot")

	// --- Non-vacuous: the session API demonstrably serves every sentinel. ---
	session := sessionLogin(t, h)
	listBody := session("GET", "/api/v1/players").Body.String()
	for _, v := range sentinelValues[:4] {
		if !strings.Contains(listBody, v) {
			t.Fatalf("session /players lacks sentinel %q; fixture is vacuous:\n%s", v, listBody)
		}
	}
	detailBody := session("GET", "/api/v1/players/"+hexUID(5)).Body.String()
	for _, marker := range []string{"STEAM-AUDIT-76561198999000005", "ACCOUNT-AUDIT-five", "131.25", "1234.5", "6789.25", `"ping"`, `"location"`, `"banned"`, `"whitelisted"`, `"sessions"`} {
		if !strings.Contains(detailBody, marker) {
			t.Fatalf("session /players/{uid} lacks %q; fixture is vacuous:\n%s", marker, detailBody)
		}
	}
	// raw_json is stored (proven by reading the store row) even though the session view
	// does not serialize it — the sentinel guards against any future path exposing it.
	if p, err := st.PlayerByUID(ctx, hexUID(5)); err != nil || !strings.Contains(string(p.Raw), "RAWJSON-AUDIT-sentinel") {
		t.Fatalf("raw_json sentinel not stored: %v %s", err, p.Raw)
	}

	// --- Sweep every integration surface. ---
	var all strings.Builder
	record := func(name string, rr *httptest.ResponseRecorder, wantStatus int) string {
		t.Helper()
		if rr.Code != wantStatus {
			t.Fatalf("%s status=%d want=%d body=%s", name, rr.Code, wantStatus, rr.Body.String())
		}
		all.WriteString(rr.Body.String())
		return rr.Body.String()
	}
	// Every page of both paginated walks at limit=1 (max page count; exercises the final
	// empty exact-multiple page too).
	base := "/api/integration/v1"
	cursor := ""
	for page := 0; ; page++ {
		u := base + "/players?limit=1"
		if cursor != "" {
			u += "&cursor=" + cursor
		}
		rr := integrationRequest(h, "GET", u, token)
		body := record(fmt.Sprintf("players page %d", page), rr, 200)
		var doc struct {
			NextCursor *string `json:"nextCursor"`
		}
		if err := json.Unmarshal([]byte(body), &doc); err != nil {
			t.Fatal(err)
		}
		if doc.NextCursor == nil {
			break
		}
		cursor = *doc.NextCursor
	}
	record("player detail 2", integrationRequest(h, "GET", base+"/players/"+hexUID(2), token), 200)
	record("player detail 5", integrationRequest(h, "GET", base+"/players/"+hexUID(5), token), 200)
	record("pals", integrationRequest(h, "GET", base+"/pals?limit=1", token), 200)
	record("guilds", integrationRequest(h, "GET", base+"/guilds", token), 200)
	record("map", integrationRequest(h, "GET", base+"/map", token), 200)
	record("server", integrationRequest(h, "GET", base+"/server", token), 200)
	record("metrics", integrationRequest(h, "GET", base+"/metrics/current", token), 200)
	record("world summary", integrationRequest(h, "GET", base+"/world/summary", token), 200)
	// Error bodies: 400 (limit), 400 (cursor), 404 (unknown uid), 404 (unknown path),
	// 405 (POST), 401 (no token) — none may echo data or key names.
	record("400 limit", integrationRequest(h, "GET", base+"/players?limit=0", token), 400)
	record("400 cursor", integrationRequest(h, "GET", base+"/players?cursor=!!!", token), 400)
	record("404 uid", integrationRequest(h, "GET", base+"/players/"+hexUID(0xDEAD), token), 404)
	record("404 path", integrationRequest(h, "GET", base+"/steam-profiles", token), 404)
	{
		req := httptest.NewRequest("POST", base+"/players", nil)
		req.Header.Set("Authorization", "Bearer "+token)
		rr := httptest.NewRecorder()
		h.ServeHTTP(rr, req)
		record("405", rr, 405)
	}
	record("401", integrationRequest(h, "GET", base+"/players", ""), 401)
	// 304 revalidation of the guilds body must stay empty.
	first := integrationRequest(h, "GET", base+"/guilds", token)
	req := httptest.NewRequest("GET", base+"/guilds", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("If-None-Match", first.Header().Get("ETag"))
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)
	if rr.Code != 304 || rr.Body.Len() != 0 {
		t.Fatalf("304 revalidation: status=%d bodyLen=%d", rr.Code, rr.Body.Len())
	}

	swept := all.String()
	for _, v := range sentinelValues {
		if strings.Contains(swept, v) {
			t.Errorf("integration surface leaked sentinel %q", v)
		}
	}
	for _, key := range []string{`"steamId"`, `"accountName"`, `"ping"`, `"banned"`, `"whitelisted"`, `"sessions"`, `"worldGuid"`, `"panelVersion"`, `"raw"`, `"rawJson"`, `"raw_json"`, `"containerId"`, `"otomoContainerId"`, `"palStorageContainerId"`, `"slotIndex"`} {
		if strings.Contains(swept, key) {
			t.Errorf("integration surface contained redacted key %s", key)
		}
	}
	// "location" is legal only for guild bases; player-scoped bodies must never carry it.
	playerScoped := integrationRequest(h, "GET", base+"/players?limit=500", token).Body.String() +
		integrationRequest(h, "GET", base+"/players/"+hexUID(5), token).Body.String()
	for _, key := range []string{`"location"`, `"x"`, `"y"`} {
		if strings.Contains(playerScoped, key) {
			t.Errorf("player-scoped integration bodies leaked %s:\n%s", key, playerScoped)
		}
	}
}

// TestAuditRedactionKeySetsExact asserts the exact JSON key set of every view object —
// stronger than string scanning: a redacted field cannot exist even as a key with a null
// value (spec §6 "absent, not null"), and a *new* store field can never walk onto the
// token surface without this test noticing the extra key.
func TestAuditRedactionKeySetsExact(t *testing.T) {
	s, h, st := auditDataServer(t, nil, nil)
	uidA, _, _, _ := seedBasicWorld(t, st)
	token := issueTestAPIKey(t, s, st, "aaaaaaaa", "audit-bot")
	base := "/api/integration/v1"

	get := func(path string) map[string]any {
		t.Helper()
		rr := integrationRequest(h, "GET", path, token)
		if rr.Code != 200 {
			t.Fatalf("%s status=%d body=%s", path, rr.Code, rr.Body.String())
		}
		return auditDecode(t, rr.Body.Bytes())
	}

	// Envelope shapes (spec §4): paginated, SAV-only, bare.
	playersEnv := get(base + "/players")
	auditAssertKeys(t, "players envelope", playersEnv, "data", "lastParseAt", "formatDrift", "nextCursor")
	detailEnv := get(base + "/players/" + uidA)
	auditAssertKeys(t, "player detail envelope", detailEnv, "data", "lastParseAt", "formatDrift")
	palsEnv := get(base + "/pals")
	auditAssertKeys(t, "pals envelope", palsEnv, "data", "lastParseAt", "formatDrift", "nextCursor")
	guildsEnv := get(base + "/guilds")
	auditAssertKeys(t, "guilds envelope", guildsEnv, "data", "lastParseAt", "formatDrift")
	serverEnv := get(base + "/server")
	auditAssertKeys(t, "server envelope", serverEnv, "data")
	metricsEnv := get(base + "/metrics/current")
	auditAssertKeys(t, "metrics envelope", metricsEnv, "data")
	worldSummaryEnv := get(base + "/world/summary")
	auditAssertKeys(t, "world summary envelope", worldSummaryEnv, "data")
	mapEnv := get(base + "/map")
	auditAssertKeys(t, "map envelope", mapEnv, "data")

	playerKeys := []string{"uid", "name", "online", "level", "guildId", "guildName", "firstSeenAt", "lastSeenAt", "playtimeSec"}
	for _, row := range playersEnv["data"].([]any) {
		player := row.(map[string]any)
		keys := playerKeys
		if player["uid"] == uidA {
			keys = append(append([]string{}, playerKeys...), "captureTotal", "uniquePalsCaptured", "paldeckUnlocked")
		}
		auditAssertKeys(t, "player row", player, keys...)
	}
	detail := detailEnv["data"].(map[string]any)
	detailKeys := append(append([]string{}, playerKeys...), "captureTotal", "uniquePalsCaptured", "paldeckUnlocked", "pals")
	auditAssertKeys(t, "player detail", detail, detailKeys...)
	for _, p := range detail["pals"].([]any) {
		pal := p.(map[string]any)
		auditAssertKeys(t, "detail pal", pal, "instanceId", "characterId", "displayName", "level", "isAlpha", "isLucky", "inParty", "partySlot", "boxPage", "boxSlot", "placement", "baseId", "hp", "gender", "talents", "passiveSkillIds", "equippedSkillIds")
		if pal["inParty"] != true || pal["partySlot"] != float64(2) || pal["boxPage"] != nil || pal["boxSlot"] != nil {
			t.Errorf("detail pal placement = %#v", pal)
		}
	}
	for _, p := range palsEnv["data"].([]any) {
		pal := p.(map[string]any)
		auditAssertKeys(t, "pals row", pal, "instanceId", "characterId", "displayName", "level", "isAlpha", "isLucky", "inParty", "partySlot", "boxPage", "boxSlot", "placement", "baseId", "ownerUid", "ownerName", "ownerSource", "ownerResolved", "hp", "gender", "talents", "passiveSkillIds", "equippedSkillIds")
		if pal["inParty"] != true || pal["partySlot"] != float64(2) || pal["boxPage"] != nil || pal["boxSlot"] != nil {
			t.Errorf("bulk pal placement = %#v", pal)
		}
	}
	palBodies, err := json.Marshal([]any{detail, palsEnv})
	if err != nil {
		t.Fatal(err)
	}
	for _, containerID := range []string{"66666666666666666666666666666666", "77777777777777777777777777777777"} {
		if strings.Contains(string(palBodies), containerID) {
			t.Errorf("integration pal surface leaked raw container GUID %q: %s", containerID, palBodies)
		}
	}
	for _, g := range guildsEnv["data"].([]any) {
		guild := g.(map[string]any)
		auditAssertKeys(t, "guild", guild, "id", "name", "adminUid", "memberCount", "members", "bases")
		for _, m := range guild["members"].([]any) {
			auditAssertKeys(t, "guild member", m.(map[string]any), "uid", "name")
		}
		for _, b := range guild["bases"].([]any) {
			bm := b.(map[string]any)
			auditAssertKeys(t, "guild base", bm, "id", "location", "level")
			auditAssertKeys(t, "base location", bm["location"].(map[string]any), "x", "y")
		}
	}
	serverData := serverEnv["data"].(map[string]any)
	auditAssertKeys(t, "server data", serverData, "name", "description", "version", "state", "uptimeSec", "save")
	auditAssertKeys(t, "server save", serverData["save"].(map[string]any), "state", "formatDrift", "lastParseAt", "players", "pals", "guilds")
	auditAssertKeys(t, "metrics data", metricsEnv["data"].(map[string]any),
		"fps", "fpsAvg", "frameTimeMs", "players", "maxPlayers", "day", "uptimeSec", "baseCamps")
	worldSummaryData := worldSummaryEnv["data"].(map[string]any)
	auditAssertKeys(t, "world summary data", worldSummaryData, "state", "capturedAt", "lastAttemptAt", "fps", "fpsAvg", "counts", "activity", "linkedBasePals")
	auditAssertKeys(t, "world summary counts", worldSummaryData["counts"].(map[string]any), "players", "partyPals", "basePals", "wildPals", "npcs", "palBoxes", "unknown")
	auditAssertKeys(t, "world summary activity", worldSummaryData["activity"].(map[string]any), "working", "transporting", "eating", "sleeping", "idle", "inactive", "combat", "incapacitated", "moving", "unknown")

	// Integer-typed fields serialize as JSON integers, times as RFC3339 UTC ("Z" suffix,
	// second granularity — the store persists unix seconds).
	dec := json.NewDecoder(strings.NewReader(integrationRequest(h, "GET", base+"/players", token).Body.String()))
	dec.UseNumber()
	var typed struct {
		Data []map[string]any `json:"data"`
	}
	if err := dec.Decode(&typed); err != nil {
		t.Fatal(err)
	}
	rfc3339utc := regexp.MustCompile(`^\d{4}-\d{2}-\d{2}T\d{2}:\d{2}:\d{2}Z$`)
	for _, row := range typed.Data {
		for _, f := range []string{"level", "playtimeSec"} {
			n, ok := row[f].(json.Number)
			if !ok || strings.ContainsAny(n.String(), ".eE") {
				t.Errorf("player %v field %s = %v, want a plain JSON integer", row["uid"], f, row[f])
			}
		}
		for _, f := range []string{"firstSeenAt", "lastSeenAt"} {
			if v, ok := row[f].(string); ok && !rfc3339utc.MatchString(v) {
				t.Errorf("player %v field %s = %q, want RFC3339 UTC", row["uid"], f, v)
			}
		}
	}
}

// TestAuditMapRedactsSidecarPathNonVacuously proves the /map re-shape drops the
// session-only "path" field (spec §4) with a fixture that demonstrably flows: the sidecar
// carries a sentinel path, the session /api/v1/map/dataset serves it, the integration /map
// serves the same layer without it.
func TestAuditMapRedactsSidecarPathNonVacuously(t *testing.T) {
	dataDir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dataDir, "map-tiles"), 0o755); err != nil {
		t.Fatal(err)
	}
	sidecar := `{"fetched_at":"2026-07-01T00:00:00Z","game_version":"v1.0.0.100427","source":"thgl","notes":"audit","layers":[{"id":"default","label":"Palpagos","path":"PATH-SENTINEL-internal-dir","format":"webp","tile_size":512,"min_zoom":0,"max_zoom":6,"transform":{"a":1,"b":2,"c":3,"d":4},"bounds":[[0,0],[1,1]]}]}`
	if err := os.WriteFile(filepath.Join(dataDir, "map-tiles", "dataset.json"), []byte(sidecar), 0o644); err != nil {
		t.Fatal(err)
	}
	s, h, st := auditDataServer(t, nil, func(c *config.Config) { c.DataDir = dataDir })
	token := issueTestAPIKey(t, s, st, "aaaaaaaa", "audit-bot")

	// Non-vacuous: the session dataset endpoint serves the sentinel path.
	session := sessionLogin(t, h)
	sessionBody := session("GET", "/api/v1/map/dataset").Body.String()
	if !strings.Contains(sessionBody, "PATH-SENTINEL-internal-dir") || !strings.Contains(sessionBody, `"path"`) {
		t.Fatalf("session /map/dataset lacks the path sentinel; fixture is vacuous:\n%s", sessionBody)
	}

	rr := integrationRequest(h, "GET", "/api/integration/v1/map", token)
	if rr.Code != 200 {
		t.Fatalf("status=%d body=%s", rr.Code, rr.Body.String())
	}
	body := rr.Body.String()
	if !strings.Contains(body, `"default"`) || !strings.Contains(body, `"Palpagos"`) {
		t.Fatalf("integration /map did not carry the sidecar layer at all (omission test would be vacuous):\n%s", body)
	}
	if strings.Contains(body, "PATH-SENTINEL-internal-dir") || strings.Contains(body, `"path"`) {
		t.Fatalf("integration /map leaked the sidecar path:\n%s", body)
	}
	doc := auditDecode(t, rr.Body.Bytes())
	layers := doc["data"].(map[string]any)["layers"].([]any)
	auditAssertKeys(t, "map layer", layers[0].(map[string]any),
		"id", "label", "format", "tileSize", "minZoom", "maxZoom", "transform", "bounds")
}

// TestAuditInternalErrorGenericOnStoreFailure forces the 500 path (spec §4/§11): with the
// store closed, an integration 500 must carry the generic envelope, no ETag, and no trace
// of the underlying error, SQL, or file paths — unlike the session internal() helper,
// which echoes err.Error().
func TestAuditInternalErrorGenericOnStoreFailure(t *testing.T) {
	s, h, st := auditDataServer(t, nil, nil)
	seedBasicWorld(t, st)
	token := issueTestAPIKey(t, s, st, "aaaaaaaa", "audit-bot")
	if err := st.Close(); err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{"/api/integration/v1/players", "/api/integration/v1/pals", "/api/integration/v1/guilds", "/api/integration/v1/players/" + hexUID(1)} {
		rr := integrationRequest(h, "GET", path, token)
		if rr.Code != 500 {
			t.Fatalf("%s with a failed store: status=%d body=%s", path, rr.Code, rr.Body.String())
		}
		want := `{"error":{"code":"internal_error","message":"The server could not complete the request."}}` + "\n"
		if rr.Body.String() != want {
			t.Fatalf("%s 500 body=%q, want the generic envelope %q", path, rr.Body.String(), want)
		}
		if etag := rr.Header().Get("ETag"); etag != "" {
			t.Fatalf("%s 500 carried an ETag %q (spec §7.2: never on 4xx/5xx)", path, etag)
		}
		for _, leak := range []string{"sql", "SQL", "database", ".db", "/tmp", os.TempDir()} {
			if strings.Contains(rr.Body.String(), leak) {
				t.Fatalf("%s 500 leaked internals (%q): %s", path, leak, rr.Body.String())
			}
		}
	}
}

// TestAuditETagDerivesFromBodyOnly pins the §7.2 contract: identical content yields an
// identical tag, different content yields different tags, a stale tag stops matching after
// a data change, and no 4xx ever carries a tag. Weak comparison accepts the strong form and
// list members; garbage never matches.
func TestAuditETagDerivesFromBodyOnly(t *testing.T) {
	s, h, st := auditDataServer(t, nil, nil)
	seedBasicWorld(t, st)
	token := issueTestAPIKey(t, s, st, "aaaaaaaa", "audit-bot")
	base := "/api/integration/v1"

	fetch := func(path string) (*httptest.ResponseRecorder, string) {
		t.Helper()
		rr := integrationRequest(h, "GET", path, token)
		if rr.Code != 200 {
			t.Fatalf("%s status=%d body=%s", path, rr.Code, rr.Body.String())
		}
		return rr, rr.Header().Get("ETag")
	}
	revalidate := func(path, inm string) int {
		t.Helper()
		req := httptest.NewRequest("GET", path, nil)
		req.Header.Set("Authorization", "Bearer "+token)
		req.Header.Set("If-None-Match", inm)
		rr := httptest.NewRecorder()
		h.ServeHTTP(rr, req)
		return rr.Code
	}

	// Same content -> same tag; the tag is the hash of the body, nothing request-specific.
	rr1, tagA1 := fetch(base + "/guilds")
	rr2, tagA2 := fetch(base + "/guilds")
	if tagA1 == "" || tagA1 != tagA2 || rr1.Body.String() != rr2.Body.String() {
		t.Fatalf("identical content produced tags %q vs %q", tagA1, tagA2)
	}
	// Different content -> different tag (two different endpoints).
	_, tagPals := fetch(base + "/pals")
	if tagPals == tagA1 {
		t.Fatalf("different bodies shared ETag %q", tagA1)
	}
	// Weak comparison: exact tag, strong-form tag, member of a list, and * all match; a
	// wrong-but-well-formed tag and garbage do not.
	if code := revalidate(base+"/guilds", tagA1); code != 304 {
		t.Fatalf("exact tag revalidation = %d, want 304", code)
	}
	if code := revalidate(base+"/guilds", strings.TrimPrefix(tagA1, "W/")); code != 304 {
		t.Fatalf("strong-form tag revalidation = %d, want 304 (weak comparison)", code)
	}
	if code := revalidate(base+"/guilds", `W/"0000000000000000000000000000dead", `+tagA1); code != 304 {
		t.Fatalf("list-member revalidation = %d, want 304", code)
	}
	if code := revalidate(base+"/guilds", `W/"0000000000000000000000000000dead"`); code != 200 {
		t.Fatalf("wrong tag revalidation = %d, want 200", code)
	}
	if code := revalidate(base+"/guilds", "not-even-a-tag"); code != 200 {
		t.Fatalf("garbage If-None-Match = %d, want 200", code)
	}
	// A data change invalidates the old tag: the 304 channel can never serve stale data as
	// current.
	w := &sav.World{Guilds: []sav.Guild{{ID: hexUID(0xC00), Name: "Changed Guild", AdminUID: hexUID(1)}}}
	if err := st.ReplaceWorld(context.Background(), w, time.Now().UTC(), 0); err != nil {
		t.Fatal(err)
	}
	if code := revalidate(base+"/guilds", tagA1); code != 200 {
		t.Fatalf("stale tag after data change = %d, want 200 with the fresh body", code)
	}
	_, tagB := fetch(base + "/guilds")
	if tagB == tagA1 {
		t.Fatalf("changed content kept ETag %q", tagA1)
	}
	// No tag on 4xx (writeError path) — 400, 404, 401.
	for _, tc := range []struct {
		path, token string
		want        int
	}{
		{base + "/players?limit=0", token, 400},
		{base + "/players/" + hexUID(0xDEAD), token, 404},
		{base + "/players", "", 401},
	} {
		rr := integrationRequest(h, "GET", tc.path, tc.token)
		if rr.Code != tc.want {
			t.Fatalf("%s status=%d want=%d", tc.path, rr.Code, tc.want)
		}
		if etag := rr.Header().Get("ETag"); etag != "" {
			t.Errorf("%d response carried ETag %q (spec §7.2: 200/304 only)", tc.want, etag)
		}
	}
}

// TestAuditPalsPaginationUnderReplaceWorldChurn walks /pals while ReplaceWorld runs between
// every page fetch — the exact scenario keyset pagination was chosen for (spec §7.1):
// /pals is the delete-and-reinsert table, so offset pagination would duplicate or skip
// rows. Core rows present throughout the walk must each appear exactly once; churn rows
// may come and go but must never duplicate.
func TestAuditPalsPaginationUnderReplaceWorldChurn(t *testing.T) {
	s, h, st := auditDataServer(t, nil, nil)
	ctx := context.Background()
	token := issueTestAPIKey(t, s, st, "aaaaaaaa", "audit-bot")

	const coreCount = 30
	owner := hexUID(1)
	makeWorld := func(gen int) *sav.World {
		w := &sav.World{Players: []sav.Player{{UID: owner, Nickname: "Owner"}}}
		for i := 1; i <= coreCount; i++ {
			// Core ids are even so odd churn ids interleave at arbitrary sort positions.
			w.Pals = append(w.Pals, sav.Pal{InstanceID: hexUID(2 * i), CharacterID: "SheepBall", OwnerUID: owner, Level: int32(i)})
		}
		if gen >= 0 {
			w.Pals = append(w.Pals, sav.Pal{InstanceID: hexUID(2*gen + 1), CharacterID: "Foxparks", OwnerUID: owner, Level: 1})
		}
		return w
	}
	if err := st.ReplaceWorld(ctx, makeWorld(0), time.Now().UTC(), 0); err != nil {
		t.Fatal(err)
	}

	got := auditWalk(t, h, token, "/api/integration/v1/pals", "instanceId", 4, func(page int) {
		// Simulate a save re-parse between every page: same core rows, a different churn
		// row each time (the previous churn row is deleted, a new one inserted).
		if err := st.ReplaceWorld(ctx, makeWorld(page+1), time.Now().UTC(), 0); err != nil {
			t.Fatal(err)
		}
	})
	seen := map[string]bool{}
	for _, id := range got {
		seen[id] = true
	}
	for i := 1; i <= coreCount; i++ {
		if !seen[hexUID(2*i)] {
			t.Errorf("core pal %s present for the whole walk was never returned (gap)", hexUID(2*i))
		}
	}
	// auditWalk already fails on any duplicate, over-limit page, or empty-with-cursor page.
}

// TestAuditCursorAndLimitBoundaryMatrix covers the tampering and boundary cases the
// existing matrix misses: standard-base64 charset, raw pipe, huge cursors, the bare "v1|"
// cursor, a cursor naming a deleted row, and the remaining limit boundaries (-1, 1, 100
// implicit default, 500 accepted).
func TestAuditCursorAndLimitBoundaryMatrix(t *testing.T) {
	s, h, st := auditDataServer(t, nil, nil)
	ctx := context.Background()
	var players []sav.Player
	for i := 1; i <= 3; i++ {
		players = append(players, sav.Player{UID: hexUID(i), Nickname: fmt.Sprintf("p%d", i)})
	}
	if err := st.ReplaceWorld(ctx, &sav.World{Players: players}, time.Now().UTC(), 0); err != nil {
		t.Fatal(err)
	}
	token := issueTestAPIKey(t, s, st, "aaaaaaaa", "audit-bot")
	base := "/api/integration/v1/players"

	type pageDoc struct {
		Data []struct {
			UID string `json:"uid"`
		} `json:"data"`
		NextCursor *string `json:"nextCursor"`
	}
	get := func(query string) (*httptest.ResponseRecorder, pageDoc) {
		t.Helper()
		rr := integrationRequest(h, "GET", base+"?"+query, token)
		var doc pageDoc
		if rr.Code == 200 {
			if err := json.Unmarshal(rr.Body.Bytes(), &doc); err != nil {
				t.Fatal(err)
			}
		}
		return rr, doc
	}
	wantCode := func(query string, status int, code string) {
		t.Helper()
		rr, _ := get(query)
		if rr.Code != status {
			t.Fatalf("?%s status=%d want=%d body=%s", query, rr.Code, status, rr.Body.String())
		}
		if code != "" {
			var envelope struct {
				Error struct{ Code string } `json:"error"`
			}
			if err := json.Unmarshal(rr.Body.Bytes(), &envelope); err != nil || envelope.Error.Code != code {
				t.Fatalf("?%s code=%q want=%q (err=%v)", query, envelope.Error.Code, code, err)
			}
		}
	}

	// Wrong charset: '+', '/', '=' are standard-base64, not base64url -> invalid_cursor.
	wantCode("cursor=abc+def", 400, "invalid_cursor")
	wantCode("cursor=abc/def", 400, "invalid_cursor")
	wantCode("cursor=YWJjZA==", 400, "invalid_cursor")
	// A raw, un-encoded pipe (someone pasting the decoded form) is outside the alphabet.
	wantCode("cursor=v1%7C"+hexUID(1), 400, "invalid_cursor")
	// Decodable but version-tag-less content is wrong-version.
	wantCode("cursor="+base64.RawURLEncoding.EncodeToString([]byte(hexUID(1))), 400, "invalid_cursor")
	// Remaining limit boundaries.
	wantCode("limit=-1", 400, "invalid_limit")
	wantCode("limit=1.5", 400, "invalid_limit")
	if rr, doc := get("limit=1"); rr.Code != 200 || len(doc.Data) != 1 || doc.NextCursor == nil {
		t.Fatalf("limit=1: status=%d rows=%d cursor=%v", rr.Code, len(doc.Data), doc.NextCursor)
	}
	if rr, doc := get("limit=500"); rr.Code != 200 || len(doc.Data) != 3 || doc.NextCursor != nil {
		t.Fatalf("limit=500: status=%d rows=%d cursor=%v", rr.Code, len(doc.Data), doc.NextCursor)
	}

	// Bare "v1|" (empty key) decodes cleanly; WHERE uid > '' is the first page. Graceful
	// continue, consistent with the keyset contract.
	if rr, doc := get("cursor=" + base64.RawURLEncoding.EncodeToString([]byte("v1|"))); rr.Code != 200 || len(doc.Data) != 3 {
		t.Fatalf("bare v1| cursor: status=%d rows=%d", rr.Code, len(doc.Data))
	}
	// A huge but well-formed cursor stays bounded: it keys past every row and returns the
	// empty terminal page rather than erroring or allocating per-row work.
	huge := base64.RawURLEncoding.EncodeToString([]byte("v1|" + strings.Repeat("z", 8192)))
	if rr, doc := get("cursor=" + huge); rr.Code != 200 || len(doc.Data) != 0 || doc.NextCursor != nil {
		t.Fatalf("huge cursor: status=%d rows=%d cursor=%v", rr.Code, len(doc.Data), doc.NextCursor)
	}
	// Empty cursor parameter: the implementation treats `cursor=` as absent (200, first
	// page) rather than 400. Spec §7.1 now blesses this explicitly ("an empty cursor
	// parameter is treated as absent"), closing the ambiguity this test used to just record.
	if rr, doc := get("cursor="); rr.Code != 200 || len(doc.Data) != 3 {
		t.Fatalf("empty cursor param: status=%d rows=%d (spec-blessed treat-as-absent behavior changed)", rr.Code, len(doc.Data))
	}

	// A cursor naming a row deleted mid-walk continues gracefully after that key.
	_, page1 := get("limit=1")
	cursorTo1 := base64.RawURLEncoding.EncodeToString([]byte("v1|" + page1.Data[0].UID))
	// Delete player 2 (the row after the cursor) — ReplaceWorld never deletes players, so
	// exercise deletion on /pals, whose table is genuinely replaced; here instead verify
	// the /players cursor for a *missing* key: a cursor naming uid 0x02½ that never existed.
	ghost := base64.RawURLEncoding.EncodeToString([]byte("v1|" + hexUID(1) + "0"))
	if rr, doc := get("cursor=" + ghost); rr.Code != 200 || len(doc.Data) != 2 {
		t.Fatalf("ghost-key cursor: status=%d rows=%d (keyset must continue after a nonexistent key)", rr.Code, len(doc.Data))
	}
	if rr, doc := get("cursor=" + cursorTo1); rr.Code != 200 || len(doc.Data) != 2 || doc.Data[0].UID != hexUID(2) {
		t.Fatalf("resume-after cursor: status=%d rows=%d", rr.Code, len(doc.Data))
	}

	// Same deleted-row case on /pals, where deletion is actually possible.
	if err := st.ReplaceWorld(ctx, &sav.World{
		Players: []sav.Player{{UID: hexUID(1)}},
		Pals: []sav.Pal{
			{InstanceID: hexUID(0x10), CharacterID: "A", OwnerUID: hexUID(1)},
			{InstanceID: hexUID(0x20), CharacterID: "B", OwnerUID: hexUID(1)},
			{InstanceID: hexUID(0x30), CharacterID: "C", OwnerUID: hexUID(1)},
		},
	}, time.Now().UTC(), 0); err != nil {
		t.Fatal(err)
	}
	palCursor := base64.RawURLEncoding.EncodeToString([]byte("v1|" + hexUID(0x10)))
	if err := st.ReplaceWorld(ctx, &sav.World{
		Players: []sav.Player{{UID: hexUID(1)}},
		Pals: []sav.Pal{
			{InstanceID: hexUID(0x20), CharacterID: "B", OwnerUID: hexUID(1)},
			{InstanceID: hexUID(0x30), CharacterID: "C", OwnerUID: hexUID(1)},
		},
	}, time.Now().UTC(), 0); err != nil {
		t.Fatal(err)
	}
	rr := integrationRequest(h, "GET", "/api/integration/v1/pals?cursor="+palCursor, token)
	if rr.Code != 200 {
		t.Fatalf("cursor to a deleted pal row: status=%d body=%s", rr.Code, rr.Body.String())
	}
	var palDoc struct {
		Data []struct {
			InstanceID string `json:"instanceId"`
		} `json:"data"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &palDoc); err != nil {
		t.Fatal(err)
	}
	if len(palDoc.Data) != 2 || palDoc.Data[0].InstanceID != hexUID(0x20) {
		t.Fatalf("cursor to a deleted row must continue from the next surviving key: %#v", palDoc.Data)
	}
}

// TestAuditOnlineFilterB2Probe250 is the spec §12.6 B2 probe the existing tests skip: 250
// players with exactly 3 online whose uids sort into the *last* keyset page. A pathological
// implementation that filters after LIMIT (or loads a page then intersects) returns zero
// rows on the first ?online=true&limit=100 page; the correct predicate-in-SQL returns all 3
// with a null cursor. Then walks the online set with limit=2 (online set larger than a
// page) and limit=3 (exact multiple), and closes the invalid-value matrix.
func TestAuditOnlineFilterB2Probe250(t *testing.T) {
	const total = 250
	onlineIdx := []int{248, 249, 250} // sort into the last keyset page
	var restPlayers []string
	for _, i := range onlineIdx {
		restPlayers = append(restPlayers, fmt.Sprintf(`{"name":"p%03d","accountName":"a","playerId":"%032x","userId":"steam_%d","level":1}`, i, i, i))
	}
	playersJSON := "[" + strings.Join(restPlayers, ",") + "]"
	rest := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/v1/api/info":
			_, _ = io.WriteString(w, `{"servername":"s","version":"1","worldguid":"g","uptime":1}`)
		case "/v1/api/metrics":
			_, _ = io.WriteString(w, `{"serverfps":60,"serverframetime":16.6,"currentplayernum":3,"maxplayernum":32,"uptime":1,"days":1,"basecampnum":0}`)
		case "/v1/api/players":
			_, _ = io.WriteString(w, `{"players":`+playersJSON+`}`)
		default:
			http.NotFound(w, r)
		}
	})
	s, h, st := auditDataServer(t, rest, func(c *config.Config) {
		c.MetricsInterval = time.Hour
		c.PlayersInterval = 10 * time.Millisecond
		c.SaveSyncInterval = time.Hour
	})
	ctx := context.Background()
	var players []sav.Player
	for i := 1; i <= total; i++ {
		players = append(players, sav.Player{UID: hexUID(i), Nickname: fmt.Sprintf("p%03d", i)})
	}
	if err := st.ReplaceWorld(ctx, &sav.World{Players: players}, time.Now().UTC(), 0); err != nil {
		t.Fatal(err)
	}
	token := issueTestAPIKey(t, s, st, "aaaaaaaa", "audit-bot")

	runCtx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() { s.RunPollers(runCtx); close(done) }()
	waitForOnlineCount(t, s, len(onlineIdx))
	// Freeze the world: no poller may mutate the online set mid-assertion.
	cancel()
	<-done

	// The B2 probe proper.
	rr := integrationRequest(h, "GET", "/api/integration/v1/players?online=true&limit=100", token)
	if rr.Code != 200 {
		t.Fatalf("status=%d body=%s", rr.Code, rr.Body.String())
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
	if len(doc.Data) != len(onlineIdx) || doc.NextCursor != nil {
		t.Fatalf("B2 probe: got %d rows (cursor=%v), want all %d online players on the first page with a null cursor — filtering after LIMIT?",
			len(doc.Data), doc.NextCursor, len(onlineIdx))
	}
	for i, row := range doc.Data {
		if row.UID != hexUID(onlineIdx[i]) || !row.Online {
			t.Fatalf("B2 probe row %d = %+v, want uid %s online", i, row, hexUID(onlineIdx[i]))
		}
	}

	// Online set (3) larger than the page (2): cursor pagination inside the filter.
	got := auditWalk(t, h, token, "/api/integration/v1/players?online=true", "uid", 2, nil)
	if len(got) != 3 || got[0] != hexUID(248) || got[2] != hexUID(250) {
		t.Fatalf("online walk limit=2 = %v", got)
	}
	// Exact multiple: 3 online, limit 3 -> full page with cursor, then the empty terminal page.
	rr = integrationRequest(h, "GET", "/api/integration/v1/players?online=true&limit=3", token)
	if err := json.Unmarshal(rr.Body.Bytes(), &doc); err != nil {
		t.Fatal(err)
	}
	if len(doc.Data) != 3 || doc.NextCursor == nil {
		t.Fatalf("exact-multiple online page: rows=%d cursor=%v", len(doc.Data), doc.NextCursor)
	}
	rr = integrationRequest(h, "GET", "/api/integration/v1/players?online=true&limit=3&cursor="+*doc.NextCursor, token)
	if err := json.Unmarshal(rr.Body.Bytes(), &doc); err != nil {
		t.Fatal(err)
	}
	if len(doc.Data) != 0 || doc.NextCursor != nil {
		t.Fatalf("terminal online page: rows=%d cursor=%v, want empty with null", len(doc.Data), doc.NextCursor)
	}
	// Invalid values (spec §7.1: anything but "true", absence aside, is invalid_request).
	for _, v := range []string{"false", "1", "TRUE", "yes"} {
		rr := integrationRequest(h, "GET", "/api/integration/v1/players?online="+v, token)
		if rr.Code != 400 {
			t.Errorf("online=%s status=%d, want 400", v, rr.Code)
		}
		var envelope struct {
			Error struct{ Code string } `json:"error"`
		}
		if err := json.Unmarshal(rr.Body.Bytes(), &envelope); err != nil || envelope.Error.Code != "invalid_request" {
			t.Errorf("online=%s code=%q, want invalid_request", v, envelope.Error.Code)
		}
	}
}

// TestAuditOnlineUIDUnknownToStoreOmittedGracefully documents what happens when the poller
// knows an online player the save-derived table does not have yet (the store row is
// missing): the uid inside the IN() predicate simply matches nothing — no error, no
// placeholder row. This matches the spec's keyset contract ("rows created mid-pagination
// may be missed"); in practice the window is tiny because pollPlayers upserts every REST
// player before publishing the online set.
func TestAuditOnlineUIDUnknownToStoreOmittedGracefully(t *testing.T) {
	st, err := storeOpenTemp(t)
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	if err := st.ReplaceWorld(ctx, &sav.World{Players: []sav.Player{{UID: hexUID(1), Nickname: "known"}}}, time.Now().UTC(), 0); err != nil {
		t.Fatal(err)
	}
	rows, err := st.PlayersPage(ctx, "", 10, []string{hexUID(1), strings.Repeat("f", 32)})
	if err != nil {
		t.Fatalf("PlayersPage with a store-unknown online uid errored: %v", err)
	}
	if len(rows) != 1 || rows[0].UID != hexUID(1) {
		t.Fatalf("rows = %#v, want only the known player (ghost silently omitted)", rows)
	}
}

// TestAuditFreshnessLastParseAtNullThenExactValue pins the lastParseAt contract (spec §4):
// present-and-null before any parse has completed, then the exact RFC3339 UTC second of
// the parse on every SAV-derived envelope — not merely "present".
func TestAuditFreshnessLastParseAtNullThenExactValue(t *testing.T) {
	s, h, st := auditDataServer(t, nil, nil)
	token := issueTestAPIKey(t, s, st, "aaaaaaaa", "audit-bot")
	base := "/api/integration/v1"

	for _, ep := range []string{"/players", "/pals", "/guilds"} {
		body := integrationRequest(h, "GET", base+ep, token).Body.String()
		if !strings.Contains(body, `"lastParseAt":null`) {
			t.Errorf("%s before any parse: lastParseAt not an explicit null:\n%s", ep, body)
		}
		if !strings.Contains(body, `"formatDrift":false`) {
			t.Errorf("%s before any parse: formatDrift is not false:\n%s", ep, body)
		}
	}

	parseAt := time.Date(2026, 7, 10, 3, 4, 5, 987654321, time.UTC) // sub-second precision must truncate
	w := &sav.World{Players: []sav.Player{{UID: hexUID(1), Nickname: "p"}}, Pals: []sav.Pal{{InstanceID: hexUID(2), CharacterID: "SheepBall", OwnerUID: hexUID(1)}}}
	if err := st.ReplaceWorld(context.Background(), w, parseAt, time.Millisecond); err != nil {
		t.Fatal(err)
	}
	want := `"lastParseAt":"2026-07-10T03:04:05Z"`
	for _, ep := range []string{"/players", "/pals", "/guilds", "/players/" + hexUID(1)} {
		body := integrationRequest(h, "GET", base+ep, token).Body.String()
		if !strings.Contains(body, want) {
			t.Errorf("%s lastParseAt is not the exact parse instant %s:\n%s", ep, want, body)
		}
		if !strings.Contains(body, `"formatDrift":false`) {
			t.Errorf("%s clean parse: formatDrift is not false:\n%s", ep, body)
		}
	}

	w.Stats.SkippedProperties = 1
	if err := st.ReplaceWorld(context.Background(), w, parseAt.Add(time.Second), time.Millisecond); err != nil {
		t.Fatal(err)
	}
	for _, ep := range []string{"/players", "/pals", "/guilds", "/players/" + hexUID(1)} {
		body := integrationRequest(h, "GET", base+ep, token).Body.String()
		if !strings.Contains(body, `"formatDrift":true`) {
			t.Errorf("%s drift parse: formatDrift is not true:\n%s", ep, body)
		}
	}
}

// TestAuditMetricsViewMatchesPollerFieldForField proves /metrics/current is a faithful
// field-for-field copy of poller.CurrentMetrics (spec §4): every value the poller holds
// appears under the documented key with the same number, nothing more, nothing less.
func TestAuditMetricsViewMatchesPollerFieldForField(t *testing.T) {
	rest := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/v1/api/info":
			_, _ = io.WriteString(w, `{"servername":"s","version":"1","worldguid":"g","uptime":12345}`)
		case "/v1/api/metrics":
			// Values chosen exactly representable in binary floating point so the JSON
			// round trip is equality-comparable.
			_, _ = io.WriteString(w, `{"serverfps":57.25,"serverframetime":17.5,"currentplayernum":3,"maxplayernum":32,"uptime":12345,"days":42,"basecampnum":7}`)
		case "/v1/api/players":
			_, _ = io.WriteString(w, `{"players":[]}`)
		default:
			http.NotFound(w, r)
		}
	})
	s, h, st := auditDataServer(t, rest, func(c *config.Config) {
		c.MetricsInterval = 10 * time.Millisecond
		c.PlayersInterval = time.Hour
		c.SaveSyncInterval = time.Hour
	})
	token := issueTestAPIKey(t, s, st, "aaaaaaaa", "audit-bot")
	runCtx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() { s.RunPollers(runCtx); close(done) }()
	deadline := time.Now().Add(2 * time.Second)
	for s.poll.Current().FPS != 57.25 {
		if time.Now().After(deadline) {
			t.Fatal("poller never captured the metrics snapshot")
		}
		time.Sleep(5 * time.Millisecond)
	}
	cancel()
	<-done

	m := s.poll.Current()
	rr := integrationRequest(h, "GET", "/api/integration/v1/metrics/current", token)
	if rr.Code != 200 {
		t.Fatalf("status=%d body=%s", rr.Code, rr.Body.String())
	}
	var doc struct {
		Data map[string]float64 `json:"data"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &doc); err != nil {
		t.Fatal(err)
	}
	want := map[string]float64{
		"fps": m.FPS, "fpsAvg": m.FPSAvg, "frameTimeMs": m.FrameTimeMS,
		"players": float64(m.Players), "maxPlayers": float64(m.MaxPlayers),
		"day": float64(m.Day), "uptimeSec": float64(m.UptimeSec), "baseCamps": float64(m.BaseCamps),
	}
	if len(doc.Data) != len(want) {
		t.Fatalf("metrics view has %d fields, poller copy needs exactly %d: %#v", len(doc.Data), len(want), doc.Data)
	}
	for k, v := range want {
		if doc.Data[k] != v {
			t.Errorf("metrics field %s = %v, poller holds %v", k, doc.Data[k], v)
		}
	}
}

// TestAuditServerFlipsToUnreachableZeroesStaleSnapshot closes the gap between the two
// existing /server tests (never-reachable, and reachable): when REST *becomes* unreachable
// after a successful snapshot was cached, the handler must zero every field rather than
// serve the stale snapshot under state "unreachable" — or worse, under "running" (spec §4
// unreachable shape; poller CachedInfo contract).
func TestAuditServerFlipsToUnreachableZeroesStaleSnapshot(t *testing.T) {
	var fail atomic.Bool
	rest := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if fail.Load() {
			http.Error(w, `{"message":"boom"}`, http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/v1/api/info":
			_, _ = io.WriteString(w, `{"servername":"Flip Server","description":"d","version":"9.9.9","worldguid":"g","uptime":777}`)
		case "/v1/api/metrics":
			_, _ = io.WriteString(w, `{"serverfps":60,"serverframetime":16.6,"currentplayernum":0,"maxplayernum":32,"uptime":777,"days":1,"basecampnum":0}`)
		case "/v1/api/players":
			_, _ = io.WriteString(w, `{"players":[]}`)
		default:
			http.NotFound(w, r)
		}
	})
	s, h, st := auditDataServer(t, rest, func(c *config.Config) {
		c.MetricsInterval = 10 * time.Millisecond
		c.PlayersInterval = time.Hour
		c.SaveSyncInterval = time.Hour
	})
	token := issueTestAPIKey(t, s, st, "aaaaaaaa", "audit-bot")
	runCtx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() { s.RunPollers(runCtx); close(done) }()
	t.Cleanup(func() { cancel(); <-done })
	waitForCachedInfoReachable(t, s)

	rr := integrationRequest(h, "GET", "/api/integration/v1/server", token)
	if !strings.Contains(rr.Body.String(), "Flip Server") {
		t.Fatalf("reachable snapshot not served (test setup broken): %s", rr.Body.String())
	}

	fail.Store(true)
	deadline := time.Now().Add(2 * time.Second)
	for {
		if _, ok := s.poll.CachedInfo(); !ok {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("poller never observed the REST outage")
		}
		time.Sleep(5 * time.Millisecond)
	}

	rr = integrationRequest(h, "GET", "/api/integration/v1/server", token)
	if rr.Code != 200 {
		t.Fatalf("unreachable /server status=%d, want 200 (never a 5xx)", rr.Code)
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
	if doc.Data.State != "unreachable" || doc.Data.Name != "" || doc.Data.Description != "" || doc.Data.Version != "" || doc.Data.UptimeSec != 0 {
		t.Fatalf("stale snapshot served after REST outage: %#v (want zeroed fields, state unreachable)", doc.Data)
	}
}

// TestAuditUnicodeNamesRoundTrip proves multi-byte, escaped, and HTML-significant player,
// guild, and member names survive the store round trip and JSON encoding byte-for-byte on
// the integration surface (spec §4: JSON, UTF-8; encoding/json escapes <, >, & by default,
// which decode back to the identical string).
func TestAuditUnicodeNamesRoundTrip(t *testing.T) {
	s, h, st := auditDataServer(t, nil, nil)
	names := []string{
		"Pál🐑Δ名前",                   // combining Latin, emoji, Greek, CJK
		`He said "hi" \ back`,       // JSON metacharacters
		"<script>alert(1)</script>", // HTML-significant (must round-trip, encoded or not)
		"tab\tand\nnewline",         // control whitespace
		"z​width joiner️…",          // zero-width and non-breaking chars
	}
	guildID := hexUID(0x700)
	var players []sav.Player
	var members []sav.GuildMember
	for i, n := range names {
		players = append(players, sav.Player{UID: hexUID(i + 1), Nickname: n, GuildID: guildID})
		members = append(members, sav.GuildMember{UID: hexUID(i + 1), Name: n})
	}
	w := &sav.World{
		Players: players,
		Pals:    []sav.Pal{{InstanceID: hexUID(0x800), CharacterID: "SheepBall", OwnerUID: hexUID(1)}},
		Guilds:  []sav.Guild{{ID: guildID, Name: names[0] + " guild", AdminUID: hexUID(1), Members: members}},
	}
	if err := st.ReplaceWorld(context.Background(), w, time.Now().UTC(), 0); err != nil {
		t.Fatal(err)
	}
	token := issueTestAPIKey(t, s, st, "aaaaaaaa", "audit-bot")

	rr := integrationRequest(h, "GET", "/api/integration/v1/players", token)
	var doc struct {
		Data []struct{ UID, Name string } `json:"data"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &doc); err != nil {
		t.Fatal(err)
	}
	got := map[string]string{}
	for _, p := range doc.Data {
		got[p.UID] = p.Name
	}
	for i, n := range names {
		if got[hexUID(i+1)] != n {
			t.Errorf("player %d name round trip: got %q want %q", i+1, got[hexUID(i+1)], n)
		}
	}
	// Owner name on /pals and member names on /guilds take different query paths.
	rr = integrationRequest(h, "GET", "/api/integration/v1/pals", token)
	var pals struct {
		Data []struct{ OwnerName string } `json:"data"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &pals); err != nil {
		t.Fatal(err)
	}
	if len(pals.Data) != 1 || pals.Data[0].OwnerName != names[0] {
		t.Errorf("pals ownerName round trip: %#v", pals.Data)
	}
	rr = integrationRequest(h, "GET", "/api/integration/v1/guilds", token)
	var guilds struct {
		Data []struct {
			Name    string
			Members []struct{ UID, Name string }
		} `json:"data"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &guilds); err != nil {
		t.Fatal(err)
	}
	if len(guilds.Data) != 1 || guilds.Data[0].Name != names[0]+" guild" {
		t.Fatalf("guild name round trip: %#v", guilds.Data)
	}
	memberNames := map[string]string{}
	for _, m := range guilds.Data[0].Members {
		memberNames[m.UID] = m.Name
	}
	for i, n := range names {
		if memberNames[hexUID(i+1)] != n {
			t.Errorf("guild member %d name round trip: got %q want %q", i+1, memberNames[hexUID(i+1)], n)
		}
	}
}

// TestAuditCursorOpacityMatchesDocumentedFormat confirms nextCursor is exactly the
// spec-documented base64url("v1|" + key) — trivially decodable *by design* (spec §7.1
// documents the format; the key it reveals is the uid already present in the same response
// row, so the cursor discloses nothing the body does not).
func TestAuditCursorOpacityMatchesDocumentedFormat(t *testing.T) {
	s, h, st := auditDataServer(t, nil, nil)
	if err := st.ReplaceWorld(context.Background(), &sav.World{Players: []sav.Player{
		{UID: hexUID(1)}, {UID: hexUID(2)}, {UID: hexUID(3)},
	}}, time.Now().UTC(), 0); err != nil {
		t.Fatal(err)
	}
	token := issueTestAPIKey(t, s, st, "aaaaaaaa", "audit-bot")
	rr := integrationRequest(h, "GET", "/api/integration/v1/players?limit=2", token)
	var doc struct {
		Data []struct {
			UID string `json:"uid"`
		} `json:"data"`
		NextCursor *string `json:"nextCursor"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &doc); err != nil {
		t.Fatal(err)
	}
	if doc.NextCursor == nil {
		t.Fatal("full page missing cursor")
	}
	decoded, err := base64.RawURLEncoding.DecodeString(*doc.NextCursor)
	if err != nil {
		t.Fatalf("nextCursor is not base64url: %v", err)
	}
	wantKey := doc.Data[len(doc.Data)-1].UID
	if string(decoded) != "v1|"+wantKey {
		t.Fatalf("cursor decodes to %q, spec documents v1|%s (last *returned* row)", decoded, wantKey)
	}
}
