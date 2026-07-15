// Integration API: the bearer-token, read-only surface mounted at /api/integration/v1.
// See docs/specs/integration-api.md for the normative design; comments here call out only
// non-obvious constraints, not the whole rationale.
package server

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"net/http"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/8tp/palhelm/internal/store"
	"github.com/go-chi/chi/v5"
)

// integrationRouter returns the dedicated GET-only sub-router mounted at
// /api/integration/v1 (spec §1). Its middleware runs before route resolution, so an
// unauthenticated probe of any path - real or not - gets the uniform 401 without
// revealing which paths exist.
func (s *Server) integrationRouter() chi.Router {
	ir := chi.NewRouter()
	ir.Use(integrationNoStore, s.integrationMiddleware)
	ir.NotFound(func(w http.ResponseWriter, r *http.Request) {
		writeError(w, http.StatusNotFound, "not_found", "Not found.")
	})
	ir.MethodNotAllowed(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Allow", http.MethodGet)
		writeError(w, http.StatusMethodNotAllowed, "method_not_allowed", "Method not allowed.")
	})
	ir.Get("/players", s.integrationPlayers)
	ir.Get("/players/{uid}", s.integrationPlayer)
	ir.Get("/pals", s.integrationPals)
	ir.Get("/guilds", s.integrationGuilds)
	ir.Get("/map", s.integrationMap)
	ir.Get("/server", s.integrationServer)
	ir.Get("/metrics/current", s.integrationMetricsCurrent)
	ir.Get("/world/summary", s.integrationWorldSummary)
	ir.Get("/world/workers", s.integrationWorldWorkers)
	ir.Get("/events", s.integrationEvents)
	return ir
}

// integrationNoStore sets Cache-Control: no-store on every response in the group,
// unconditionally, before auth or routing decides the status (spec §5): every response
// (200, 304, 400, 401, 404, 405, 429) carries it.
func integrationNoStore(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "no-store")
		next.ServeHTTP(w, r)
	})
}

// integrationMiddleware is the exact chain of spec §8.2: parse -> validate (constant-time)
// -> limiter keyed by key id -> coalesced lastUsedAt touch -> handler. No state is
// allocated for an invalid token: parseBearer and Validate touch no map that outlives the
// request, so unknown/malformed tokens cannot grow any state (the H4 lesson).
func (s *Server) integrationMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		token, ok := parseBearer(r)
		if !ok {
			integrationUnauthorized(w)
			return
		}
		principal, ok := s.integration.Validate(token)
		if !ok {
			integrationUnauthorized(w)
			return
		}
		allowed, retryAfter := s.integration.Allow(principal.ID)
		if !allowed {
			w.Header().Set("Retry-After", strconv.Itoa(retryAfter))
			writeError(w, http.StatusTooManyRequests, "rate_limited", "API key rate limit exceeded; retry later.")
			return
		}
		s.integration.Touch(r.Context(), principal.ID, time.Now().UTC())
		// Attribution lives here, not in the root requestLog middleware, which runs before
		// this downstream r.WithContext and can never observe the key id (spec §2.4).
		s.log.Info("integration request", "keyId", principal.ID, "method", r.Method, "path", r.URL.Path)
		next.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), integrationKey{}, principal)))
	})
}

// integrationInternal writes the generic 500 the integration surface requires (spec §4):
// unlike the session API's internal(), it never echoes err.Error() to the client.
func (s *Server) integrationInternal(w http.ResponseWriter, err error) {
	s.log.Error("integration request failed", "error", err)
	writeError(w, http.StatusInternalServerError, "internal_error", "The server could not complete the request.")
}

// --- envelopes (spec §4) ---
//
// Three shapes, matched to which optional members apply. nextCursor uses a *string (not
// omitempty) so a paginated endpoint on a short page serializes an explicit JSON null,
// never an absent field - the pagination contract requires it to always be present on
// paginated endpoints (spec §7.1).

type integrationPageEnvelope struct {
	Data        any     `json:"data"`
	LastParseAt any     `json:"lastParseAt"`
	FormatDrift bool    `json:"formatDrift"`
	NextCursor  *string `json:"nextCursor"`
}
type integrationSAVEnvelope struct {
	Data        any  `json:"data"`
	LastParseAt any  `json:"lastParseAt"`
	FormatDrift bool `json:"formatDrift"`
}
type integrationEnvelope struct {
	Data any `json:"data"`
}

// integrationEventView is deliberately smaller than store.Event: meta is never
// reachable from the public API, backup details are replaced with a generic
// state, and only explicitly allowlisted system transitions survive projection.
type integrationEventView struct {
	At      time.Time `json:"at"`
	Kind    string    `json:"kind"`
	Message string    `json:"message"`
}

var integrationPublicSystemMessages = map[string]struct{}{
	"Palworld REST API is reachable":   {},
	"Palworld REST API is unreachable": {},
	"world save format drift detected": {},
	"world save format drift resolved": {},
}

const integrationEventScanLimit = 500

// integrationEvents returns a bounded recent activity window. It intentionally
// does not expose a kind filter or cursor: the server must examine and project a
// bounded superset so disallowed panel/config/audit rows cannot influence the
// response shape or escape through query behavior.
func (s *Server) integrationEvents(w http.ResponseWriter, r *http.Request) {
	limit := 50
	if raw := r.URL.Query().Get("limit"); raw != "" {
		n, err := strconv.Atoi(raw)
		if err != nil || n < 1 || n > 100 {
			writeError(w, http.StatusBadRequest, "invalid_limit", "limit must be an integer from 1 to 100.")
			return
		}
		limit = n
	}
	events, err := s.store.Events(r.Context(), integrationEventScanLimit, "")
	if err != nil {
		s.integrationInternal(w, err)
		return
	}
	out := make([]integrationEventView, 0, limit)
	for _, event := range events {
		if projected, ok := projectIntegrationEvent(event); ok {
			out = append(out, projected)
			if len(out) == limit {
				break
			}
		}
	}
	s.writeIntegration(w, r, integrationEnvelope{Data: out})
}

func projectIntegrationEvent(event store.Event) (integrationEventView, bool) {
	view := integrationEventView{At: event.At, Kind: event.Kind}
	switch event.Kind {
	case "join", "leave":
		suffix := " joined"
		if event.Kind == "leave" {
			suffix = " left"
		}
		if !strings.HasSuffix(event.Message, suffix) {
			return integrationEventView{}, false
		}
		name := strings.TrimSpace(strings.NewReplacer("\r", " ", "\n", " ", "\t", " ").Replace(strings.TrimSuffix(event.Message, suffix)))
		if name == "" {
			return integrationEventView{}, false
		}
		runes := []rune(name)
		if len(runes) > 100 {
			name = string(runes[:100])
		}
		view.Message = name + suffix
	case "backup":
		view.Message = "Backup completed"
	case "system":
		if _, ok := integrationPublicSystemMessages[event.Message]; !ok {
			return integrationEventView{}, false
		}
		view.Message = event.Message
	default:
		return integrationEventView{}, false
	}
	return view, true
}

// writeIntegration serializes envelope, computes its weak content-hash ETag, and handles
// If-None-Match revalidation (spec §7.2). ETag is set on 200 and echoed on 304 only - the
// caller must route error responses through writeError instead, which never touches ETag.
func (s *Server) writeIntegration(w http.ResponseWriter, r *http.Request, envelope any) {
	body, err := json.Marshal(envelope)
	if err != nil {
		s.integrationInternal(w, err)
		return
	}
	sum := sha256.Sum256(body)
	etag := `W/"` + hex.EncodeToString(sum[:16]) + `"`
	w.Header().Set("ETag", etag)
	if ifNoneMatchHit(r.Header.Get("If-None-Match"), etag) {
		w.WriteHeader(http.StatusNotModified)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(body)
}

// ifNoneMatchHit implements RFC 7232 weak comparison member-wise over a comma-separated
// If-None-Match list: a bare "*" matches any representation, and each member is compared
// byte-equal after stripping any "W/" prefix - never as a whole-header string (spec §7.2).
func ifNoneMatchHit(header, etag string) bool {
	if header == "" {
		return false
	}
	target := strings.TrimPrefix(etag, "W/")
	for _, part := range strings.Split(header, ",") {
		part = strings.TrimSpace(part)
		if part == "*" || strings.TrimPrefix(part, "W/") == target {
			return true
		}
	}
	return false
}

// --- pagination (spec §7.1) ---

const cursorTag = "v1|"

func encodeCursor(key string) string {
	return base64.RawURLEncoding.EncodeToString([]byte(cursorTag + key))
}

// decodeCursor rejects anything undecodable, wrong-version, or wrong-charset in one pass:
// base64.RawURLEncoding.DecodeString itself rejects any byte outside the base64url
// alphabet, which is the "wrong-charset" case; a decodable value lacking the "v1|" tag is
// "wrong-version".
func decodeCursor(v string) (string, bool) {
	b, err := base64.RawURLEncoding.DecodeString(v)
	if err != nil {
		return "", false
	}
	s := string(b)
	if !strings.HasPrefix(s, cursorTag) {
		return "", false
	}
	return strings.TrimPrefix(s, cursorTag), true
}

// parseIntegrationPage reads and validates the shared limit/cursor query parameters. ok is
// false when either is invalid; code names which one (spec §7.1: invalid_limit vs
// invalid_cursor).
func parseIntegrationPage(r *http.Request) (limit int, after string, code string, ok bool) {
	limit = 100
	if raw := r.URL.Query().Get("limit"); raw != "" {
		n, err := strconv.Atoi(raw)
		if err != nil || n < 1 || n > 500 {
			return 0, "", "invalid_limit", false
		}
		limit = n
	}
	if raw := r.URL.Query().Get("cursor"); raw != "" {
		key, decOK := decodeCursor(raw)
		if !decOK {
			return 0, "", "invalid_cursor", false
		}
		after = key
	}
	return limit, after, "", true
}

// --- view structs (spec §4/§6) ---
//
// Every field below is an explicit "expose" row of the spec §6 redaction table. There is no
// path from store.Player, store.Guild, store.Pal, poller.CurrentMetrics, or
// mapDatasetInfo into a response that does not go through one of these.

type integrationPlayerView struct {
	UID                string `json:"uid"`
	Name               string `json:"name"`
	Online             bool   `json:"online"`
	Level              int    `json:"level"`
	GuildID            string `json:"guildId"`
	GuildName          string `json:"guildName"`
	FirstSeenAt        any    `json:"firstSeenAt"`
	LastSeenAt         any    `json:"lastSeenAt"`
	PlaytimeSec        int64  `json:"playtimeSec"`
	CaptureTotal       *int64 `json:"captureTotal,omitempty"`
	UniquePalsCaptured *int   `json:"uniquePalsCaptured,omitempty"`
	PaldeckUnlocked    *int   `json:"paldeckUnlocked,omitempty"`
}

func newIntegrationPlayerView(p store.Player, online bool) integrationPlayerView {
	return integrationPlayerView{
		UID: p.UID, Name: p.Name, Online: online, Level: p.Level,
		GuildID: p.GuildID, GuildName: p.GuildName,
		FirstSeenAt: nullableTime(p.FirstSeenAt), LastSeenAt: nullableTime(p.LastSeenAt),
		PlaytimeSec:  p.PlaytimeSec,
		CaptureTotal: p.CaptureTotal, UniquePalsCaptured: p.UniquePalsCaptured,
		PaldeckUnlocked: p.PaldeckUnlocked,
	}
}

type integrationPalView struct {
	InstanceID       string                `json:"instanceId"`
	CharacterID      string                `json:"characterId"`
	DisplayName      string                `json:"displayName"`
	Level            int                   `json:"level"`
	IsAlpha          bool                  `json:"isAlpha"`
	IsLucky          bool                  `json:"isLucky"`
	InParty          bool                  `json:"inParty"`
	PartySlot        *int                  `json:"partySlot"`
	BoxPage          *int                  `json:"boxPage"`
	BoxSlot          *int                  `json:"boxSlot"`
	Placement        string                `json:"placement"`
	BaseID           *string               `json:"baseId"`
	HP               *float64              `json:"hp"`
	Gender           string                `json:"gender"`
	Rank             *int                  `json:"rank"`
	Talents          integrationPalTalents `json:"talents"`
	PassiveSkillIDs  []string              `json:"passiveSkillIds"`
	EquippedSkillIDs []string              `json:"equippedSkillIds"`
}

type integrationPalTalents struct {
	HP      *int `json:"hp"`
	Melee   *int `json:"melee"`
	Shot    *int `json:"shot"`
	Defense *int `json:"defense"`
}

func newIntegrationPalView(p store.Pal) integrationPalView {
	return integrationPalView{
		InstanceID: p.InstanceID, CharacterID: p.CharacterID, DisplayName: p.DisplayName,
		Level: p.Level, IsAlpha: p.IsAlpha, IsLucky: p.IsLucky,
		InParty: p.InParty, PartySlot: p.PartySlot, BoxPage: p.BoxPage, BoxSlot: p.BoxSlot,
		Placement: integrationPalPlacement(p), BaseID: integrationString(p.BaseID),
		HP: p.HP, Gender: p.Gender, Rank: p.Rank,
		Talents:         integrationPalTalents{HP: p.TalentHP, Melee: p.TalentMelee, Shot: p.TalentShot, Defense: p.TalentDefense},
		PassiveSkillIDs: nonnilStrings(p.PassiveSkillIDs), EquippedSkillIDs: nonnilStrings(p.EquippedSkillIDs),
	}
}

// integrationPlayerDetailView embeds integrationPlayerView so its fields are promoted flat
// into the JSON object (Go's encoding/json rules), keeping GET /players/{uid} field order
// identical to the spec §4 table with pals appended last.
type integrationPlayerDetailView struct {
	integrationPlayerView
	Pals []integrationPalView `json:"pals"`
}

type integrationPalListView struct {
	InstanceID       string                `json:"instanceId"`
	CharacterID      string                `json:"characterId"`
	DisplayName      string                `json:"displayName"`
	Level            int                   `json:"level"`
	IsAlpha          bool                  `json:"isAlpha"`
	IsLucky          bool                  `json:"isLucky"`
	InParty          bool                  `json:"inParty"`
	PartySlot        *int                  `json:"partySlot"`
	BoxPage          *int                  `json:"boxPage"`
	BoxSlot          *int                  `json:"boxSlot"`
	Placement        string                `json:"placement"`
	BaseID           *string               `json:"baseId"`
	OwnerUID         string                `json:"ownerUid"`
	OwnerName        string                `json:"ownerName"`
	OwnerSource      string                `json:"ownerSource"`
	OwnerResolved    bool                  `json:"ownerResolved"`
	HP               *float64              `json:"hp"`
	Gender           string                `json:"gender"`
	Rank             *int                  `json:"rank"`
	Talents          integrationPalTalents `json:"talents"`
	PassiveSkillIDs  []string              `json:"passiveSkillIds"`
	EquippedSkillIDs []string              `json:"equippedSkillIds"`
}

func integrationPalPlacement(p store.Pal) string {
	switch {
	case p.InParty:
		return "party"
	case p.BoxPage != nil:
		return "box"
	case p.BaseID != "":
		return "base"
	default:
		return "unknown"
	}
}

func integrationString(value string) *string {
	if value == "" {
		return nil
	}
	return &value
}

type integrationGuildMemberView struct {
	UID  string `json:"uid"`
	Name string `json:"name"`
}
type integrationLocationView struct {
	X float64 `json:"x"`
	Y float64 `json:"y"`
}
type integrationBaseView struct {
	ID       string                  `json:"id"`
	Location integrationLocationView `json:"location"`
	Level    int                     `json:"level"`
}
type integrationGuildView struct {
	ID          string                       `json:"id"`
	Name        string                       `json:"name"`
	AdminUID    string                       `json:"adminUid"`
	MemberCount int                          `json:"memberCount"`
	Members     []integrationGuildMemberView `json:"members"`
	Bases       []integrationBaseView        `json:"bases"`
}

type integrationMapTransform struct {
	A float64 `json:"a"`
	B float64 `json:"b"`
	C float64 `json:"c"`
	D float64 `json:"d"`
}
type integrationMapLayer struct {
	ID        string                   `json:"id"`
	Label     string                   `json:"label,omitempty"`
	Format    string                   `json:"format,omitempty"`
	TileSize  int                      `json:"tileSize,omitempty"`
	MinZoom   int                      `json:"minZoom"`
	MaxZoom   int                      `json:"maxZoom"`
	Transform *integrationMapTransform `json:"transform,omitempty"`
	Bounds    *[2][2]float64           `json:"bounds,omitempty"`
}
type integrationMapView struct {
	Source      string                `json:"source"`
	GameVersion string                `json:"gameVersion"`
	FetchedAt   *string               `json:"fetchedAt"`
	Notes       string                `json:"notes,omitempty"`
	Layers      []integrationMapLayer `json:"layers"`
}

type integrationServerView struct {
	Name        string              `json:"name"`
	Description string              `json:"description"`
	Version     string              `json:"version"`
	State       string              `json:"state"`
	UptimeSec   int64               `json:"uptimeSec"`
	Save        integrationSaveView `json:"save"`
}

type integrationSaveView struct {
	State       string `json:"state"`
	FormatDrift bool   `json:"formatDrift"`
	LastParseAt any    `json:"lastParseAt"`
	Players     int    `json:"players"`
	Pals        int    `json:"pals"`
	Guilds      int    `json:"guilds"`
}

type integrationMetricsView struct {
	FPS         float64 `json:"fps"`
	FPSAvg      float64 `json:"fpsAvg"`
	FrameTimeMS float64 `json:"frameTimeMs"`
	Players     int     `json:"players"`
	MaxPlayers  int     `json:"maxPlayers"`
	Day         int64   `json:"day"`
	UptimeSec   int64   `json:"uptimeSec"`
	BaseCamps   int     `json:"baseCamps"`
}

// --- handlers ---

// integrationPlayers serves GET /players: keyset-paginated by uid, with an optional
// ?online=true predicate against the poller's in-memory online-uid snapshot (spec §7.1).
//
// Two ordering rules, both audit-driven:
//   - The online snapshot is taken exactly once (`online` below) and reused for both the
//     keyset predicate (onlyUIDs) and each row's `online` flag. Taking two separate
//     snapshots - one for the WHERE-uid-IN predicate, one for the per-row flag - let a row
//     appear in an online-filtered page with `online:false` under a mid-request poller tick.
//   - WorldState is queried before the row query (PlayersPage), not after. If ReplaceWorld
//     lands between the two queries, querying world state first yields the old, conservative
//     lastParseAt paired with possibly-new rows (honest-stale); querying it last would yield a
//     new lastParseAt paired with the rows that existed before the reparse (a false freshness
//     signal). This ordering applies to every SAV-derived handler in this file.
func (s *Server) integrationPlayers(w http.ResponseWriter, r *http.Request) {
	limit, after, code, ok := parseIntegrationPage(r)
	if !ok {
		writeError(w, http.StatusBadRequest, code, "The limit or cursor parameter is invalid.")
		return
	}
	online := s.poll.Online()
	var onlyUIDs []string
	filterOnline := false
	if raw := r.URL.Query().Get("online"); raw != "" {
		if raw != "true" {
			writeError(w, http.StatusBadRequest, "invalid_request", "online must be true.")
			return
		}
		filterOnline = true
		onlyUIDs = make([]string, 0, len(online))
		for uid := range online {
			onlyUIDs = append(onlyUIDs, uid)
		}
		sort.Strings(onlyUIDs)
	}
	world, err := s.store.WorldState(r.Context())
	if err != nil {
		s.integrationInternal(w, err)
		return
	}
	// Zero-online short-circuit (spec §7.1, normative): return the empty page without
	// issuing any query - an empty IN() is a SQLite syntax error and must be unreachable.
	if filterOnline && len(onlyUIDs) == 0 {
		s.writeIntegration(w, r, integrationPageEnvelope{Data: []integrationPlayerView{}, LastParseAt: nullableTime(world.LastParseAt), FormatDrift: world.FormatDrift, NextCursor: nil})
		return
	}
	players, err := s.store.PlayersPage(r.Context(), after, limit, onlyUIDs)
	if err != nil {
		s.integrationInternal(w, err)
		return
	}
	views := make([]integrationPlayerView, 0, len(players))
	for _, p := range players {
		views = append(views, newIntegrationPlayerView(p, online[p.UID]))
	}
	var next *string
	if len(players) == limit {
		c := encodeCursor(players[len(players)-1].UID)
		next = &c
	}
	s.writeIntegration(w, r, integrationPageEnvelope{Data: views, LastParseAt: nullableTime(world.LastParseAt), FormatDrift: world.FormatDrift, NextCursor: next})
}

// integrationUIDPattern gates GET /players/{uid} before any store call (spec §4): unvalidated
// input would let NormalizeUID's LIKE-based ResolveUID resolve a "%"/"_" wildcard to an
// arbitrary player, a pre-existing session-API behavior this surface must not inherit.
var integrationUIDPattern = regexp.MustCompile(`^[0-9a-fA-F-]{1,36}$`)

func (s *Server) integrationPlayer(w http.ResponseWriter, r *http.Request) {
	uid := chi.URLParam(r, "uid")
	if !integrationUIDPattern.MatchString(uid) {
		writeError(w, http.StatusNotFound, "not_found", "Player not found.")
		return
	}
	// WorldState before the row queries (spec: lastParseAt ordering, see integrationPlayers).
	world, err := s.store.WorldState(r.Context())
	if err != nil {
		s.integrationInternal(w, err)
		return
	}
	p, err := s.store.PlayerByUID(r.Context(), uid)
	if errors.Is(err, sql.ErrNoRows) {
		writeError(w, http.StatusNotFound, "not_found", "Player not found.")
		return
	}
	if err != nil {
		s.integrationInternal(w, err)
		return
	}
	pals, err := s.store.PalsTyped(r.Context(), p.UID)
	if err != nil {
		s.integrationInternal(w, err)
		return
	}
	palViews := make([]integrationPalView, 0, len(pals))
	for _, pal := range pals {
		palViews = append(palViews, newIntegrationPalView(pal))
	}
	online := s.poll.Online()[p.UID]
	detail := integrationPlayerDetailView{integrationPlayerView: newIntegrationPlayerView(p, online), Pals: palViews}
	s.writeIntegration(w, r, integrationSAVEnvelope{Data: detail, LastParseAt: nullableTime(world.LastParseAt), FormatDrift: world.FormatDrift})
}

// integrationPals serves GET /pals: bulk, keyset-paginated by instance_id, with owner
// uid/name joined in so bots avoid an N+1 per-player call (spec §4).
func (s *Server) integrationPals(w http.ResponseWriter, r *http.Request) {
	limit, after, code, ok := parseIntegrationPage(r)
	if !ok {
		writeError(w, http.StatusBadRequest, code, "The limit or cursor parameter is invalid.")
		return
	}
	// WorldState before the row query (spec: lastParseAt ordering, see integrationPlayers).
	world, err := s.store.WorldState(r.Context())
	if err != nil {
		s.integrationInternal(w, err)
		return
	}
	rows, err := s.store.PalsPage(r.Context(), after, limit)
	if err != nil {
		s.integrationInternal(w, err)
		return
	}
	views := make([]integrationPalListView, 0, len(rows))
	for _, row := range rows {
		views = append(views, integrationPalListView{
			InstanceID: row.InstanceID, CharacterID: row.CharacterID, DisplayName: row.DisplayName,
			Level: row.Level, IsAlpha: row.IsAlpha, IsLucky: row.IsLucky,
			InParty: row.InParty, PartySlot: row.PartySlot, BoxPage: row.BoxPage, BoxSlot: row.BoxSlot,
			Placement: integrationPalPlacement(row.Pal), BaseID: integrationString(row.BaseID),
			OwnerUID: row.OwnerUID, OwnerName: row.OwnerName,
			OwnerSource: row.OwnerSource, OwnerResolved: row.OwnerResolved,
			HP: row.HP, Gender: row.Gender, Rank: row.Rank,
			Talents:         integrationPalTalents{HP: row.TalentHP, Melee: row.TalentMelee, Shot: row.TalentShot, Defense: row.TalentDefense},
			PassiveSkillIDs: nonnilStrings(row.PassiveSkillIDs), EquippedSkillIDs: nonnilStrings(row.EquippedSkillIDs),
		})
	}
	var next *string
	if len(rows) == limit {
		c := encodeCursor(rows[len(rows)-1].InstanceID)
		next = &c
	}
	s.writeIntegration(w, r, integrationPageEnvelope{Data: views, LastParseAt: nullableTime(world.LastParseAt), FormatDrift: world.FormatDrift, NextCursor: next})
}

func nonnilStrings(values []string) []string {
	if values == nil {
		return []string{}
	}
	return values
}

// integrationGuilds serves GET /guilds: not paginated (spec §4 - guild counts are tens).
func (s *Server) integrationGuilds(w http.ResponseWriter, r *http.Request) {
	// WorldState before the row query (spec: lastParseAt ordering, see integrationPlayers).
	world, err := s.store.WorldState(r.Context())
	if err != nil {
		s.integrationInternal(w, err)
		return
	}
	guilds, err := s.store.Guilds(r.Context())
	if err != nil {
		s.integrationInternal(w, err)
		return
	}
	views := make([]integrationGuildView, 0, len(guilds))
	for _, g := range guilds {
		members := make([]integrationGuildMemberView, 0, len(g.Members))
		for _, m := range g.Members {
			members = append(members, integrationGuildMemberView{UID: m.UID, Name: m.Name})
		}
		bases := make([]integrationBaseView, 0, len(g.Bases))
		for _, b := range g.Bases {
			bases = append(bases, integrationBaseView{ID: b.ID, Location: integrationLocationView{X: b.X, Y: b.Y}, Level: b.Level})
		}
		views = append(views, integrationGuildView{ID: g.ID, Name: g.Name, AdminUID: g.AdminUID, MemberCount: len(members), Members: members, Bases: bases})
	}
	s.writeIntegration(w, r, integrationSAVEnvelope{Data: views, LastParseAt: nullableTime(world.LastParseAt), FormatDrift: world.FormatDrift})
}

// integrationMap serves GET /map: a camelCase re-shape of the session mapDataset sidecar
// data, omitting the session-only "path" field (spec §4).
func (s *Server) integrationMap(w http.ResponseWriter, r *http.Request) {
	info := s.loadMapDataset()
	layers := make([]integrationMapLayer, 0, len(info.Layers))
	for _, l := range info.Layers {
		var t *integrationMapTransform
		if l.Transform != nil {
			t = &integrationMapTransform{A: l.Transform.A, B: l.Transform.B, C: l.Transform.C, D: l.Transform.D}
		}
		layers = append(layers, integrationMapLayer{ID: l.ID, Label: l.Label, Format: l.Format, TileSize: l.TileSize, MinZoom: l.MinZoom, MaxZoom: l.MaxZoom, Transform: t, Bounds: l.Bounds})
	}
	v := integrationMapView{Source: info.Source, GameVersion: info.GameVersion, FetchedAt: info.FetchedAt, Notes: info.Notes, Layers: layers}
	s.writeIntegration(w, r, integrationEnvelope{Data: v})
}

// integrationServer serves GET /server from the poller's cached last-successful Info
// snapshot only - never a per-request upstream call (spec §4, §S1, §12.13). When the
// poller currently reports the snapshot unreachable, every field is zeroed rather than
// serving stale data under a misleading state.
func (s *Server) integrationServer(w http.ResponseWriter, r *http.Request) {
	world, err := s.store.WorldState(r.Context())
	if err != nil {
		s.integrationInternal(w, err)
		return
	}
	info, reachable := s.poll.CachedInfo()
	state := "unreachable"
	if reachable {
		state = s.shutdown.State()
	} else {
		info.ServerName, info.Description, info.Version, info.Uptime = "", "", "", 0
	}
	saveState := "ok"
	if world.FormatDrift {
		saveState = "drift"
	} else if world.LastParseAt.IsZero() {
		saveState = "unknown"
	}
	v := integrationServerView{
		Name: info.ServerName, Description: info.Description, Version: info.Version, State: state, UptimeSec: info.Uptime,
		Save: integrationSaveView{State: saveState, FormatDrift: world.FormatDrift, LastParseAt: nullableTime(world.LastParseAt), Players: world.Players, Pals: world.Pals, Guilds: world.Guilds},
	}
	s.writeIntegration(w, r, integrationEnvelope{Data: v})
}

// integrationMetricsCurrent serves GET /metrics/current: a field-for-field copy of
// poller.CurrentMetrics into a dedicated view struct (spec §4), so a future panel-only
// field added to CurrentMetrics does not walk onto the token surface automatically.
func (s *Server) integrationMetricsCurrent(w http.ResponseWriter, r *http.Request) {
	m := s.poll.Current()
	v := integrationMetricsView{FPS: m.FPS, FPSAvg: m.FPSAvg, FrameTimeMS: m.FrameTimeMS, Players: m.Players, MaxPlayers: m.MaxPlayers, Day: m.Day, UptimeSec: m.UptimeSec, BaseCamps: m.BaseCamps}
	s.writeIntegration(w, r, integrationEnvelope{Data: v})
}
