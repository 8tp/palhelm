package server

import (
	"database/sql"
	"errors"
	"fmt"
	"mime"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/8tp/palhelm/internal/itemgrant"
	"github.com/8tp/palhelm/internal/store"
	"github.com/go-chi/chi/v5"
)

var idempotencyKeyPattern = regexp.MustCompile(`^[A-Za-z0-9._:-]{8,128}$`)

func (s *Server) itemGrantCapability(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, s.itemSpool.Capability(s.cfg.ItemGrantsEnabled, s.itemCatalog, time.Now().UTC()))
}

func (s *Server) items(w http.ResponseWriter, r *http.Request) {
	if s.itemCatalog == nil {
		writeJSON(w, http.StatusOK, map[string]any{"items": []any{}, "total": 0, "gameVersion": "", "source": nil})
		return
	}
	limit := 50
	offset := 0
	var err error
	if raw := r.URL.Query().Get("limit"); raw != "" {
		limit, err = strconv.Atoi(raw)
		if err != nil || limit < 1 || limit > 100 {
			writeError(w, 400, "invalid_limit", "limit must be from 1 to 100.")
			return
		}
	}
	if raw := r.URL.Query().Get("offset"); raw != "" {
		offset, err = strconv.Atoi(raw)
		if err != nil || offset < 0 || offset > 10000 {
			writeError(w, 400, "invalid_offset", "offset must be from 0 to 10000.")
			return
		}
	}
	items, total := s.itemCatalog.Search(r.URL.Query().Get("q"), limit, offset)
	writeJSON(w, http.StatusOK, map[string]any{"items": items, "total": total, "gameVersion": s.itemCatalog.GameVersion(), "source": s.itemCatalog.Source()})
}

func (s *Server) itemIcon(w http.ResponseWriter, r *http.Request) {
	if s.itemCatalog == nil {
		http.NotFound(w, r)
		return
	}
	item, ok := s.itemCatalog.Item(chi.URLParam(r, "itemId"))
	if !ok || item.Icon == "" {
		http.NotFound(w, r)
		return
	}
	path := filepath.Join(s.cfg.ItemIconDir, item.Icon)
	if filepath.Dir(path) != filepath.Clean(s.cfg.ItemIconDir) {
		http.NotFound(w, r)
		return
	}
	f, err := os.Open(path)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	defer f.Close()
	stat, err := f.Stat()
	if err != nil || !stat.Mode().IsRegular() {
		http.NotFound(w, r)
		return
	}
	contentType := mime.TypeByExtension(strings.ToLower(filepath.Ext(item.Icon)))
	if contentType == "" {
		contentType = "application/octet-stream"
	}
	w.Header().Set("Content-Type", contentType)
	w.Header().Set("Cache-Control", "private, max-age=86400")
	http.ServeContent(w, r, item.Icon, stat.ModTime(), f)
}

type createGrantRequest struct {
	ItemID         string `json:"itemId"`
	Quantity       int    `json:"quantity"`
	Reason         string `json:"reason"`
	IdempotencyKey string `json:"idempotencyKey"`
}

func (s *Server) createItemGrant(w http.ResponseWriter, r *http.Request) {
	capability := s.itemSpool.Capability(s.cfg.ItemGrantsEnabled, s.itemCatalog, time.Now().UTC())
	if !capability.Ready {
		writeError(w, http.StatusServiceUnavailable, "item_grants_unavailable", capability.Reason)
		return
	}
	var req createGrantRequest
	if !decode(w, r, &req) {
		return
	}
	req.ItemID, req.Reason, req.IdempotencyKey = strings.TrimSpace(req.ItemID), strings.TrimSpace(req.Reason), strings.TrimSpace(req.IdempotencyKey)
	if !idempotencyKeyPattern.MatchString(req.IdempotencyKey) {
		writeError(w, 400, "invalid_idempotency_key", "idempotencyKey must be 8 to 128 safe characters.")
		return
	}
	if len(req.Reason) < 3 || len(req.Reason) > 200 {
		writeError(w, 400, "invalid_reason", "reason must be from 3 to 200 characters.")
		return
	}
	item, ok := s.itemCatalog.Item(req.ItemID)
	if !ok || !item.Grantable {
		writeError(w, 400, "item_not_grantable", "That item is not in the grant allowlist.")
		return
	}
	if req.Quantity < 1 || req.Quantity > item.MaxQuantity {
		writeError(w, 400, "invalid_quantity", fmt.Sprintf("quantity must be from 1 to %d for this item.", item.MaxQuantity))
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
		writeError(w, 409, "player_offline", "The player must be online to receive an item safely.")
		return
	}
	now := time.Now().UTC()
	requestID, err := itemgrant.NewRequestID()
	if err != nil {
		internal(w, err)
		return
	}
	grant := store.ItemGrant{RequestID: requestID, IdempotencyKey: req.IdempotencyKey, CreatedAt: now, UpdatedAt: now, Actor: principalFrom(r).Username, PlayerUID: player.UID, PlayerName: player.Name, ItemID: item.ID, ItemName: item.Name, Quantity: req.Quantity, Reason: req.Reason, Status: "queued"}
	grant, created, err := s.store.CreateItemGrant(r.Context(), grant)
	if err != nil {
		internal(w, err)
		return
	}
	if !created {
		if grant.PlayerUID != player.UID || grant.ItemID != item.ID || grant.Quantity != req.Quantity {
			writeError(w, 409, "idempotency_conflict", "That idempotency key was already used for a different grant.")
			return
		}
		grant = s.reconcileItemGrant(r, grant)
		writeJSON(w, http.StatusOK, grant)
		return
	}
	bridgeReq := itemgrant.Request{ProtocolVersion: itemgrant.BridgeProtocolVersion, RequestID: requestID, CreatedAt: now.Format(time.RFC3339), PlayerUID: player.UID, PlayerName: player.Name, ItemID: item.ID, Quantity: req.Quantity}
	if err := s.itemSpool.Submit(bridgeReq); err != nil {
		_ = s.store.CompleteItemGrant(r.Context(), requestID, "failed", 0, "bridge_submit_failed", "The bridge could not accept the request.", now)
		if errors.Is(err, itemgrant.ErrBridgeBusy) {
			writeError(w, http.StatusConflict, "item_grant_busy", "Another item grant is already being processed; wait for it to finish.")
			return
		}
		internal(w, err)
		return
	}
	s.audit(r, "item_grant", fmt.Sprintf("Queued %d × %s for %s", req.Quantity, item.Name, player.Name), map[string]any{"requestId": requestID, "playerUid": player.UID, "itemId": item.ID, "quantity": req.Quantity})
	writeJSON(w, http.StatusAccepted, grant)
}

func (s *Server) getItemGrant(w http.ResponseWriter, r *http.Request) {
	grant, err := s.store.ItemGrant(r.Context(), chi.URLParam(r, "requestId"))
	if errors.Is(err, sql.ErrNoRows) {
		writeError(w, 404, "grant_not_found", "Item grant not found.")
		return
	}
	if err != nil {
		internal(w, err)
		return
	}
	writeJSON(w, 200, s.reconcileItemGrant(r, grant))
}

func (s *Server) listItemGrants(w http.ResponseWriter, r *http.Request) {
	grants, err := s.store.ItemGrants(r.Context(), 100)
	if err != nil {
		internal(w, err)
		return
	}
	for i := range grants {
		grants[i] = s.reconcileItemGrant(r, grants[i])
	}
	writeJSON(w, 200, map[string]any{"grants": grants})
}

func (s *Server) reconcileItemGrant(r *http.Request, grant store.ItemGrant) store.ItemGrant {
	if grant.Status != "queued" {
		return grant
	}
	result, err := s.itemSpool.Result(grant.RequestID)
	if err != nil {
		return grant
	}
	completedAt := time.Now().UTC()
	if parsed, parseErr := time.Parse(time.RFC3339, result.CompletedAt); parseErr == nil && parsed.After(completedAt.Add(-5*time.Minute)) && parsed.Before(completedAt.Add(5*time.Minute)) {
		completedAt = parsed
	}
	if (result.Status == "succeeded" && (result.GrantedQuantity < 1 || result.GrantedQuantity > grant.Quantity)) || (result.Status == "failed" && result.GrantedQuantity != 0) {
		_ = s.store.CompleteItemGrant(r.Context(), grant.RequestID, "failed", 0, "invalid_bridge_result", "The bridge returned an invalid quantity; the mutation outcome is unknown and was not retried.", completedAt)
		updated, updateErr := s.store.ItemGrant(r.Context(), grant.RequestID)
		if updateErr == nil {
			return updated
		}
		return grant
	}
	_ = s.store.CompleteItemGrant(r.Context(), grant.RequestID, result.Status, result.GrantedQuantity, result.ErrorCode, result.ErrorMessage, completedAt)
	updated, err := s.store.ItemGrant(r.Context(), grant.RequestID)
	if err == nil {
		return updated
	}
	return grant
}
