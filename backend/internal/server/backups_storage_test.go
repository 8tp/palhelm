package server

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/8tp/palhelm/internal/config"
	"github.com/8tp/palhelm/internal/store"
)

// storageTestServer builds a logged-in panel and returns an authenticated request helper.
func storageTestServer(t *testing.T) (*Server, func(method, path string) *httptest.ResponseRecorder) {
	t.Helper()
	dir := t.TempDir()
	st, err := store.Open(filepath.Join(dir, "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { st.Close() })
	cfg := config.Config{
		DataDir: dir, SaveDir: filepath.Join(dir, "Saved"), AdminPassword: "panelpass",
		SessionSecret:   strings.Repeat("s", 48),
		MetricsInterval: time.Hour, PlayersInterval: time.Hour, SaveSyncInterval: time.Hour,
	}
	app, handler := New(cfg, st, slog.New(slog.NewTextHandler(io.Discard, nil)))

	login := httptest.NewRequest(http.MethodPost, "/api/v1/auth/login", bytes.NewBufferString(`{"password":"panelpass"}`))
	login.Header.Set("Content-Type", "application/json")
	lr := httptest.NewRecorder()
	handler.ServeHTTP(lr, login)
	if lr.Code != http.StatusOK {
		t.Fatalf("login = %d: %s", lr.Code, lr.Body.String())
	}
	cookie := lr.Result().Cookies()[0]
	request := func(method, path string) *httptest.ResponseRecorder {
		req := httptest.NewRequest(method, path, nil)
		req.AddCookie(cookie)
		rr := httptest.NewRecorder()
		handler.ServeHTTP(rr, req)
		return rr
	}
	return app, request
}

func TestBackupStorageReportsRealCapacity(t *testing.T) {
	app, request := storageTestServer(t)
	// The default statfs implementation is wired in New; assert it returns a sane,
	// self-consistent capacity for the real backup filesystem.
	app.diskStat = statfsDiskUsage

	rr := request(http.MethodGet, "/api/v1/backups/storage")
	if rr.Code != http.StatusOK {
		t.Fatalf("GET /backups/storage = %d: %s", rr.Code, rr.Body.String())
	}
	var body struct {
		TotalBytes *uint64 `json:"totalBytes"`
		FreeBytes  *uint64 `json:"freeBytes"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if body.TotalBytes == nil || body.FreeBytes == nil {
		t.Fatalf("expected real capacity, got %s", rr.Body.String())
	}
	if *body.TotalBytes == 0 {
		t.Fatalf("totalBytes should be positive: %s", rr.Body.String())
	}
	if *body.FreeBytes > *body.TotalBytes {
		t.Fatalf("freeBytes %d exceeds totalBytes %d", *body.FreeBytes, *body.TotalBytes)
	}
}

func TestBackupStorageStatFailureReportsNull(t *testing.T) {
	app, request := storageTestServer(t)
	// Inject a failing stat: the endpoint must degrade to null fields, never a fabricated
	// capacity, and never surface the host path.
	app.diskStat = func(string) (uint64, uint64, error) { return 0, 0, errors.New("statfs: no such file or directory") }

	rr := request(http.MethodGet, "/api/v1/backups/storage")
	if rr.Code != http.StatusOK {
		t.Fatalf("GET /backups/storage = %d: %s", rr.Code, rr.Body.String())
	}
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(rr.Body.Bytes(), &raw); err != nil {
		t.Fatal(err)
	}
	for _, field := range []string{"totalBytes", "freeBytes"} {
		v, ok := raw[field]
		if !ok {
			t.Fatalf("response missing %q: %s", field, rr.Body.String())
		}
		if string(v) != "null" {
			t.Fatalf("%s = %s, want null", field, string(v))
		}
	}
}

func TestServerInfoExposesRuntimeConfig(t *testing.T) {
	dir := t.TempDir()
	st, err := store.Open(filepath.Join(dir, "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	cfg := config.Config{
		DataDir: dir, SaveDir: filepath.Join(dir, "Saved"), AdminPassword: "panelpass",
		SessionSecret:   strings.Repeat("s", 48),
		MetricsInterval: time.Hour, PlayersInterval: time.Hour, SaveSyncInterval: 10 * time.Minute,
		SessionDays: 14,
	}
	_, handler := New(cfg, st, slog.New(slog.NewTextHandler(io.Discard, nil)))
	login := httptest.NewRequest(http.MethodPost, "/api/v1/auth/login", bytes.NewBufferString(`{"password":"panelpass"}`))
	login.Header.Set("Content-Type", "application/json")
	lr := httptest.NewRecorder()
	handler.ServeHTTP(lr, login)
	if lr.Code != http.StatusOK {
		t.Fatalf("login = %d", lr.Code)
	}
	cookie := lr.Result().Cookies()[0]
	req := httptest.NewRequest(http.MethodGet, "/api/v1/server", nil)
	req.AddCookie(cookie)
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("GET /server = %d: %s", rr.Code, rr.Body.String())
	}
	var body struct {
		SessionDays     int `json:"sessionDays"`
		SaveSyncMinutes int `json:"saveSyncMinutes"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if body.SessionDays != 14 {
		t.Errorf("sessionDays = %d, want 14", body.SessionDays)
	}
	if body.SaveSyncMinutes != 10 {
		t.Errorf("saveSyncMinutes = %d, want 10", body.SaveSyncMinutes)
	}
}
