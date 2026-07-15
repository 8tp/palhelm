package server

import (
	"net/http"
	"strconv"
	"strings"
	"unicode"

	"github.com/8tp/palhelm/internal/paldeck"
	"github.com/8tp/palhelm/internal/store"
)

const (
	defaultPalExplorerLimit = 48
	maxPalExplorerLimit     = 100
)

type sessionPalTalents struct {
	HP      *int `json:"hp"`
	Melee   *int `json:"melee"`
	Shot    *int `json:"shot"`
	Defense *int `json:"defense"`
}

// sessionPalExplorerView is an explicit viewer-safe allowlist. In particular, it never exposes
// pals.raw_json or any player Steam/account fields through the server-wide roster.
type sessionPalExplorerView struct {
	InstanceID    string   `json:"instanceId"`
	CharacterID   string   `json:"characterId"`
	DisplayName   string   `json:"displayName"`
	Level         int      `json:"level"`
	IsAlpha       bool     `json:"isAlpha"`
	IsLucky       bool     `json:"isLucky"`
	IsBoss        bool     `json:"isBoss"`
	InParty       bool     `json:"inParty"`
	PartySlot     *int     `json:"partySlot"`
	BoxPage       *int     `json:"boxPage"`
	BoxSlot       *int     `json:"boxSlot"`
	Placement     string   `json:"placement"`
	BaseID        *string  `json:"baseId"`
	OwnerUID      string   `json:"ownerUid"`
	OwnerName     string   `json:"ownerName"`
	OwnerSource   string   `json:"ownerSource"`
	OwnerResolved bool     `json:"ownerResolved"`
	HP            *float64 `json:"hp"`
	Gender        string   `json:"gender"`
	// Rank is the Pal Condenser rank (1..5) or null when the save carried no Rank
	// property. Displayed stars are rank-1; null stays "unavailable", never 0.
	Rank             *int              `json:"rank"`
	Talents          sessionPalTalents `json:"talents"`
	PassiveSkillIDs  []string          `json:"passiveSkillIds"`
	EquippedSkillIDs []string          `json:"equippedSkillIds"`
}

type sessionPalExplorerPage struct {
	Data       []sessionPalExplorerView `json:"data"`
	NextCursor *string                  `json:"nextCursor"`
}

func (s *Server) pals(w http.ResponseWriter, r *http.Request) {
	filter, ok := parsePalExplorerQuery(w, r)
	if !ok {
		return
	}
	requested := filter.Limit
	filter.Limit++ // one look-ahead row determines whether another page exists
	rows, err := s.store.PalsExplorerPage(r.Context(), filter)
	if err != nil {
		internal(w, err)
		return
	}
	var next *string
	if len(rows) > requested {
		cursor := rows[requested-1].InstanceID
		next = &cursor
		rows = rows[:requested]
	}
	data := make([]sessionPalExplorerView, 0, len(rows))
	for _, row := range rows {
		data = append(data, newSessionPalExplorerView(row))
	}
	writeJSON(w, http.StatusOK, sessionPalExplorerPage{Data: data, NextCursor: next})
}

func parsePalExplorerQuery(w http.ResponseWriter, r *http.Request) (store.PalExplorerQuery, bool) {
	values := r.URL.Query()
	filter := store.PalExplorerQuery{
		After:       values.Get("cursor"),
		Limit:       defaultPalExplorerLimit,
		Search:      strings.TrimSpace(values.Get("q")),
		OwnerSource: values.Get("ownerSource"),
		Placement:   values.Get("placement"),
		Specimen:    values.Get("specimen"),
	}
	if raw := values.Get("limit"); raw != "" {
		limit, err := strconv.Atoi(raw)
		if err != nil || limit < 1 || limit > maxPalExplorerLimit {
			writeError(w, http.StatusBadRequest, "invalid_limit", "limit must be an integer from 1 to 100.")
			return filter, false
		}
		filter.Limit = limit
	}
	if len(filter.After) > 128 || containsControl(filter.After) {
		writeError(w, http.StatusBadRequest, "invalid_cursor", "cursor is invalid.")
		return filter, false
	}
	if len(filter.Search) > 100 || containsControl(filter.Search) {
		writeError(w, http.StatusBadRequest, "invalid_request", "q must be at most 100 characters and contain no control characters.")
		return filter, false
	}
	if !oneOf(filter.OwnerSource, "", "save", "personal_container", "last_observed", "unresolved") {
		writeError(w, http.StatusBadRequest, "invalid_request", "ownerSource is invalid.")
		return filter, false
	}
	if !oneOf(filter.Placement, "", "party", "box", "base", "unknown") {
		writeError(w, http.StatusBadRequest, "invalid_request", "placement is invalid.")
		return filter, false
	}
	if !oneOf(filter.Specimen, "", "standard", "alpha", "lucky", "boss") {
		writeError(w, http.StatusBadRequest, "invalid_request", "specimen is invalid.")
		return filter, false
	}
	var ok bool
	if filter.MinLevel, ok = optionalPalLevel(values.Get("minLevel")); !ok {
		writeError(w, http.StatusBadRequest, "invalid_request", "minLevel must be an integer from 0 to 999.")
		return filter, false
	}
	if filter.MaxLevel, ok = optionalPalLevel(values.Get("maxLevel")); !ok {
		writeError(w, http.StatusBadRequest, "invalid_request", "maxLevel must be an integer from 0 to 999.")
		return filter, false
	}
	if filter.MinLevel != nil && filter.MaxLevel != nil && *filter.MinLevel > *filter.MaxLevel {
		writeError(w, http.StatusBadRequest, "invalid_request", "minLevel cannot exceed maxLevel.")
		return filter, false
	}
	return filter, true
}

func optionalPalLevel(raw string) (*int, bool) {
	if raw == "" {
		return nil, true
	}
	value, err := strconv.Atoi(raw)
	if err != nil || value < 0 || value > 999 {
		return nil, false
	}
	return &value, true
}

func oneOf(value string, allowed ...string) bool {
	for _, candidate := range allowed {
		if value == candidate {
			return true
		}
	}
	return false
}

func containsControl(value string) bool {
	return strings.IndexFunc(value, unicode.IsControl) >= 0
}

func newSessionPalExplorerView(p store.PalWithOwner) sessionPalExplorerView {
	var baseID *string
	if p.BaseID != "" {
		baseID = &p.BaseID
	}
	passives := p.PassiveSkillIDs
	if passives == nil {
		passives = []string{}
	}
	equipped := p.EquippedSkillIDs
	if equipped == nil {
		equipped = []string{}
	}
	return sessionPalExplorerView{
		InstanceID: p.InstanceID, CharacterID: p.CharacterID, DisplayName: p.DisplayName,
		Level: p.Level, IsAlpha: p.IsAlpha, IsLucky: p.IsLucky, IsBoss: paldeck.IsBossID(p.CharacterID),
		InParty: p.InParty, PartySlot: p.PartySlot, BoxPage: p.BoxPage, BoxSlot: p.BoxSlot,
		Placement: integrationPalPlacement(p.Pal), BaseID: baseID,
		OwnerUID: p.OwnerUID, OwnerName: p.OwnerName, OwnerSource: p.OwnerSource, OwnerResolved: p.OwnerResolved,
		HP: p.HP, Gender: p.Gender, Rank: p.Rank,
		Talents:         sessionPalTalents{HP: p.TalentHP, Melee: p.TalentMelee, Shot: p.TalentShot, Defense: p.TalentDefense},
		PassiveSkillIDs: passives, EquippedSkillIDs: equipped,
	}
}
