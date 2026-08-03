package server

import (
	"database/sql"
	"errors"
	"fmt"
	"net/http"
	"reflect"
	"strconv"
	"strings"
	"time"

	"github.com/8tp/palhelm/internal/palgrant"
	"github.com/8tp/palhelm/internal/store"
	"github.com/go-chi/chi/v5"
)

func (s *Server) palGrantCapability(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, s.palSpool.Capability(s.cfg.PalGrantsEnabled, s.palCatalog, time.Now().UTC()))
}

func (s *Server) palGrantCatalog(w http.ResponseWriter, r *http.Request) {
	limit := 50
	if raw := r.URL.Query().Get("limit"); raw != "" {
		parsed, err := strconv.Atoi(raw)
		if err != nil || parsed < 1 || parsed > 100 {
			writeError(w, 400, "invalid_limit", "limit must be from 1 to 100.")
			return
		}
		limit = parsed
	}
	items := s.palCatalog.Search(r.URL.Query().Get("q"), limit)
	writeJSON(w, http.StatusOK, map[string]any{"species": items, "catalogVersion": palgrant.CatalogVersion, "catalogItems": s.palCatalog.Count(), "maxLevel": palgrant.MaxSupportedLevel, "passiveCatalogVersion": s.palPassiveCatalog.Version(), "passiveCatalogItems": s.palPassiveCatalog.Count()})
}

func (s *Server) palGrantPassives(w http.ResponseWriter, r *http.Request) {
	limit := 100
	if raw := r.URL.Query().Get("limit"); raw != "" {
		parsed, err := strconv.Atoi(raw)
		if err != nil || parsed < 1 || parsed > 200 {
			writeError(w, 400, "invalid_limit", "limit must be from 1 to 200.")
			return
		}
		limit = parsed
	}
	items := s.palPassiveCatalog.Search(r.URL.Query().Get("q"), limit)
	writeJSON(w, http.StatusOK, map[string]any{
		"passives":       items,
		"catalogVersion": s.palPassiveCatalog.Version(),
		"catalogItems":   s.palPassiveCatalog.Count(),
		"ranks":          s.palPassiveCatalog.SortedRanks(),
		"source":         s.palPassiveCatalog.Source(),
	})
}

type createPalGrantRequest struct {
	CharacterID    string                  `json:"characterId"`
	Spec           palgrant.GenerationSpec `json:"spec"`
	Reason         string                  `json:"reason"`
	IdempotencyKey string                  `json:"idempotencyKey"`
}

func (s *Server) createPalGrant(w http.ResponseWriter, r *http.Request) {
	capability := s.palSpool.Capability(s.cfg.PalGrantsEnabled, s.palCatalog, time.Now().UTC())
	if !capability.Ready {
		writeError(w, http.StatusServiceUnavailable, "pal_grants_unavailable", capability.Reason)
		return
	}
	var req createPalGrantRequest
	if !decode(w, r, &req) {
		return
	}
	req.CharacterID, req.Reason, req.IdempotencyKey = strings.TrimSpace(req.CharacterID), strings.TrimSpace(req.Reason), strings.TrimSpace(req.IdempotencyKey)
	if !idempotencyKeyPattern.MatchString(req.IdempotencyKey) {
		writeError(w, 400, "invalid_idempotency_key", "idempotencyKey must be 8 to 128 safe characters.")
		return
	}
	if len(req.Reason) < 3 || len(req.Reason) > 200 {
		writeError(w, 400, "invalid_reason", "reason must be from 3 to 200 characters.")
		return
	}
	species, ok := s.palCatalog.Species(req.CharacterID)
	if !ok {
		writeError(w, 400, "pal_not_grantable", "That species or form is not in the safe Pal creation catalogue.")
		return
	}
	if err := req.Spec.Validate(); err != nil {
		writeError(w, 400, "invalid_pal_spec", err.Error()+".")
		return
	}
	if err := s.palPassiveCatalog.ValidateIDs(req.Spec.PassiveSkillIDs); err != nil {
		writeError(w, 400, "invalid_pal_spec", err.Error()+".")
		return
	}
	uid := s.store.ResolveUID(r.Context(), chi.URLParam(r, "uid"))
	player, err := s.store.PlayerByUID(r.Context(), uid)
	if errors.Is(err, sql.ErrNoRows) {
		writeError(w, 404, "player_not_found", "Player not found.")
		return
	}
	if err != nil {
		internal(w, err)
		return
	}
	if !s.poll.Online()[player.UID] {
		writeError(w, 409, "player_offline", "The player must be online to receive a Pal safely.")
		return
	}
	now := time.Now().UTC()
	requestID, err := palgrant.NewRequestID()
	if err != nil {
		internal(w, err)
		return
	}
	grant := store.PalGrant{RequestID: requestID, IdempotencyKey: req.IdempotencyKey, CreatedAt: now, UpdatedAt: now, Actor: principalFrom(r).Username, PlayerUID: player.UID, PlayerName: player.Name, CharacterID: species.CharacterID, DisplayName: species.DisplayName, Spec: req.Spec, Reason: req.Reason, Status: "queued"}
	grant, created, err := s.store.CreatePalGrant(r.Context(), grant)
	if err != nil {
		internal(w, err)
		return
	}
	if !created {
		if grant.PlayerUID != player.UID || grant.CharacterID != species.CharacterID || grant.Reason != req.Reason || !reflect.DeepEqual(grant.Spec, req.Spec) {
			writeError(w, 409, "idempotency_conflict", "That idempotency key was already used for a different Pal grant.")
			return
		}
		writeJSON(w, 200, s.reconcilePalGrant(r, grant))
		return
	}
	bridgeReq := palgrant.Request{ProtocolVersion: palgrant.BridgeProtocolVersion, RequestID: requestID, CreatedAt: now.Format(time.RFC3339), PlayerUID: player.UID, PlayerName: player.Name, CharacterID: species.CharacterID, Spec: req.Spec}
	if err := s.palSpool.Submit(bridgeReq); err != nil {
		_ = s.store.CompletePalGrant(r.Context(), requestID, "failed", nil, "bridge_submit_failed", "The Pal bridge could not accept the request.", now)
		if errors.Is(err, palgrant.ErrBridgeBusy) {
			writeError(w, http.StatusConflict, "pal_grant_busy", "Another Pal grant is already being processed; wait for it to finish.")
			return
		}
		internal(w, err)
		return
	}
	s.audit(r, "pal_grant", fmt.Sprintf("Queued Lv. %d-%d %s for %s", req.Spec.LevelMin, req.Spec.LevelMax, species.DisplayName, player.Name), map[string]any{"requestId": requestID, "playerUid": player.UID, "characterId": species.CharacterID, "mode": req.Spec.Mode})
	writeJSON(w, http.StatusAccepted, grant)
}

func (s *Server) getPalGrant(w http.ResponseWriter, r *http.Request) {
	grant, err := s.store.PalGrant(r.Context(), chi.URLParam(r, "requestId"))
	if errors.Is(err, sql.ErrNoRows) {
		writeError(w, 404, "grant_not_found", "Pal grant not found.")
		return
	}
	if err != nil {
		internal(w, err)
		return
	}
	writeJSON(w, 200, s.reconcilePalGrant(r, grant))
}

func (s *Server) listPalGrants(w http.ResponseWriter, r *http.Request) {
	grants, err := s.store.PalGrants(r.Context(), 100)
	if err != nil {
		internal(w, err)
		return
	}
	for index := range grants {
		grants[index] = s.reconcilePalGrant(r, grants[index])
	}
	writeJSON(w, 200, map[string]any{"grants": grants})
}

func (s *Server) reconcilePalGrant(r *http.Request, grant store.PalGrant) store.PalGrant {
	if grant.Status != "queued" {
		return grant
	}
	result, err := s.palSpool.Result(grant.RequestID)
	if err != nil {
		return grant
	}
	completedAt := time.Now().UTC()
	if parsed, parseErr := time.Parse(time.RFC3339, result.CompletedAt); parseErr == nil && parsed.After(completedAt.Add(-5*time.Minute)) && parsed.Before(completedAt.Add(5*time.Minute)) {
		completedAt = parsed
	}
	var storedResult *palgrant.Result
	if result.Status == "succeeded" {
		storedResult = &result
	}
	_ = s.store.CompletePalGrant(r.Context(), grant.RequestID, result.Status, storedResult, result.ErrorCode, result.ErrorMessage, completedAt)
	updated, err := s.store.PalGrant(r.Context(), grant.RequestID)
	if err == nil {
		return updated
	}
	return grant
}
