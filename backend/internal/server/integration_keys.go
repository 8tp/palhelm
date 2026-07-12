// Admin key-management API for the bearer-token integration surface (spec §2, §9):
// POST/GET/DELETE /api/v1/integration-keys. These routes are session-authenticated,
// admin-only, and live in the existing adminOnly group in server.go's routes() - they are
// never reachable with a bearer token and share nothing with integrationRouter().
package server

import (
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"net/http"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/go-chi/chi/v5"
	"github.com/palhelm/palhelm/internal/store"
)

// maxActiveIntegrationKeys bounds active keys, and by extension the integrationAuth cache
// and its limiter cardinality (spec §2.6, §8.1). Creation past this cap is 409. Enforcement
// is atomic inside store.CreateAPIKey (store.MaxActiveAPIKeys); this alias exists so
// HTTP-layer messages and tests share the same literal instead of a second hardcoded 100.
const maxActiveIntegrationKeys = store.MaxActiveAPIKeys

// maxIntegrationKeyLabelLength is the trimmed-length ceiling of spec §9's label validation.
const maxIntegrationKeyLabelLength = 64

// maxKeyIDAttempts bounds the retry loop on the astronomically unlikely id collision
// (spec §2.1): 8 hex chars is 32 bits of space, so a second collision inside a handful of
// retries is not a realistic outcome - this just keeps a pathological case from looping
// forever.
const maxKeyIDAttempts = 5

// newIntegrationKeyID is overridable in tests to force an id collision and exercise the
// retry loop in createIntegrationKey without waiting on astronomical odds.
var newIntegrationKeyID = func() (string, error) {
	b := make([]byte, 4)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}

// newIntegrationKeyToken mints one candidate id + full plaintext token per spec §2.1:
// phk_<8 lowercase hex>_<43 base64url chars>, 56 bytes total.
func newIntegrationKeyToken() (id, token string, err error) {
	id, err = newIntegrationKeyID()
	if err != nil {
		return "", "", err
	}
	secret := make([]byte, 32)
	if _, err = rand.Read(secret); err != nil {
		return "", "", err
	}
	return id, "phk_" + id + "_" + base64.RawURLEncoding.EncodeToString(secret), nil
}

// isUniqueConstraintErr reports whether err is a SQLite UNIQUE constraint violation, the
// only expected failure mode for a colliding key id or (vanishingly less likely) digest.
func isUniqueConstraintErr(err error) bool {
	return err != nil && strings.Contains(err.Error(), "UNIQUE constraint")
}

// validIntegrationKeyLabel enforces spec §9: required, trimmed length 1-64, no control
// characters. Callers pass the already-trimmed label.
func validIntegrationKeyLabel(label string) bool {
	if label == "" || utf8.RuneCountInString(label) > maxIntegrationKeyLabelLength {
		return false
	}
	for _, r := range label {
		if unicode.IsControl(r) {
			return false
		}
	}
	return true
}

// integrationKeyCreated is the 201 create response: the stored record plus the plaintext
// key, which appears here and nowhere else, ever (spec §2.2, §9).
type integrationKeyCreated struct {
	store.APIKey
	Key string `json:"key"`
}

func (s *Server) createIntegrationKey(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Label string `json:"label"`
	}
	if !decode(w, r, &req) {
		return
	}
	label := strings.TrimSpace(req.Label)
	if !validIntegrationKeyLabel(label) {
		writeError(w, http.StatusBadRequest, "invalid_request", "label is required, must be 1-64 characters after trimming, and must not contain control characters.")
		return
	}
	now := time.Now().UTC()
	var rec store.APIKey
	var token string
	for attempt := 0; ; attempt++ {
		var id string
		var err error
		id, token, err = newIntegrationKeyToken()
		if err != nil {
			internal(w, err)
			return
		}
		hash := sha256.Sum256([]byte(token))
		// store.CreateAPIKey is the authoritative, atomic enforcement of the 100-active
		// cap (count-then-insert in one transaction; spec §2.6) - the HTTP layer no longer
		// pre-checks with a separate ActiveAPIKeys read, which was the TOCTOU an audit
		// found: concurrent creates at 99 active could each read count<100 before either
		// inserted.
		rec, err = s.store.CreateAPIKey(r.Context(), id, hash, label, now)
		if err == nil {
			// Cache is synchronized only after the store write succeeds (spec §2.3's
			// cache is authoritative for validation, so it must never contain a key
			// absent from durable storage).
			s.integration.Add(id, hash, label)
			break
		}
		if errors.Is(err, store.ErrAPIKeyCapReached) {
			writeError(w, http.StatusConflict, "too_many_keys", "The active integration key limit (100) has been reached.")
			return
		}
		if !isUniqueConstraintErr(err) || attempt >= maxKeyIDAttempts-1 {
			internal(w, err)
			return
		}
	}
	s.audit(r, "panel", "created integration key", map[string]any{"id": rec.ID, "label": rec.Label})
	writeJSON(w, http.StatusCreated, integrationKeyCreated{APIKey: rec, Key: token})
}

func (s *Server) listIntegrationKeys(w http.ResponseWriter, r *http.Request) {
	keys, err := s.store.ListAPIKeys(r.Context())
	if err != nil {
		internal(w, err)
		return
	}
	for i, k := range keys {
		if last, ok := s.integration.LastUsed(k.ID); ok {
			t := last.UTC()
			keys[i].LastUsedAt = &t
		}
	}
	writeJSON(w, http.StatusOK, keys)
}

func (s *Server) revokeIntegrationKey(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	rec, err := s.store.RevokeAPIKey(r.Context(), id, time.Now().UTC())
	if errors.Is(err, sql.ErrNoRows) {
		writeError(w, http.StatusNotFound, "not_found", "Integration key not found.")
		return
	}
	if err != nil {
		internal(w, err)
		return
	}
	// Store first, then the cache flip (spec §2.6): the next request for this id fails
	// validation immediately, with no restart and no staleness window (spec §12.8).
	s.integration.Revoke(id)
	s.audit(r, "panel", "revoked integration key", map[string]any{"id": rec.ID, "label": rec.Label})
	writeJSON(w, http.StatusOK, rec)
}
