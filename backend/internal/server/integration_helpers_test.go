package server

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/8tp/palhelm/internal/config"
	"github.com/8tp/palhelm/internal/store"
)

// newIntegrationTestServer builds a fully wired Server against a temp SQLite store and an
// optional fake Palworld REST backend (nil leaves RESTURL empty, i.e. permanently
// unreachable - the shape every /server-unreachable test wants).
func newIntegrationTestServer(t *testing.T, rest http.Handler) (*Server, http.Handler, *store.Store) {
	t.Helper()
	st, err := store.Open(filepath.Join(t.TempDir(), "test.db"))
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
		IntegrationRateLimit: 60,
	}
	s, h := New(cfg, st, testLogger())
	return s, h, st
}

// issueTestAPIKey creates an active key row in the store and mirrors it into the running
// server's in-memory cache via the same Add call the (separately built) admin create
// handler will use, so tests can mint usable tokens regardless of whether New() already ran.
func issueTestAPIKey(t *testing.T, s *Server, st *store.Store, id, label string) string {
	t.Helper()
	secret := make([]byte, 32)
	if _, err := rand.Read(secret); err != nil {
		t.Fatal(err)
	}
	token := "phk_" + id + "_" + base64.RawURLEncoding.EncodeToString(secret)
	hash := sha256.Sum256([]byte(token))
	if _, err := st.CreateAPIKey(context.Background(), id, hash, label, time.Now().UTC()); err != nil {
		t.Fatal(err)
	}
	s.integration.Add(id, hash, label)
	return token
}

// integrationRequest issues a request against h with the given bearer token (empty string
// omits the Authorization header entirely).
func integrationRequest(h http.Handler, method, path, token string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(method, path, nil)
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)
	return rr
}
