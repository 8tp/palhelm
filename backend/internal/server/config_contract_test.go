package server

import (
	"bytes"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/8tp/palhelm/internal/config"
	"github.com/8tp/palhelm/internal/gameconfig"
	"github.com/8tp/palhelm/internal/store"
)

// TestConfigFrontendContractAgainstRealHandler exercises the exact GET/PUT/error documents
// consumed by frontend/src/api/client.ts. It deliberately uses the real router and editor,
// rather than the frontend mock that allowed the v0.2 contract inversion to escape.
func TestConfigFrontendContractAgainstRealHandler(t *testing.T) {
	dir := t.TempDir()
	composePath := filepath.Join(dir, "docker-compose.yml")
	compose := `services:
  palworld:
    environment:
      SERVER_NAME: "contract fixture"
      SERVER_PASSWORD: "join-secret"
      ADMIN_PASSWORD: "admin-secret"
      PLAYERS: 16
      EXP_RATE: 1.5
      RCON_PORT: 25575
`
	if err := os.WriteFile(composePath, []byte(compose), 0o640); err != nil {
		t.Fatal(err)
	}
	st, err := store.Open(filepath.Join(dir, "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	cfg := config.Config{
		AdminPassword: "panelpass",
		SessionSecret: strings.Repeat("s", 48),
		DataDir:       filepath.Join(dir, "data"),
		ComposeFile:   composePath,
		GameService:   "palworld",
		SaveDir:       filepath.Join(dir, "saved"),
	}
	_, handler := New(cfg, st, slog.New(slog.NewTextHandler(io.Discard, nil)))

	login := httptest.NewRequest(http.MethodPost, "/api/v1/auth/login", bytes.NewBufferString(`{"password":"panelpass"}`))
	login.Header.Set("Content-Type", "application/json")
	lr := httptest.NewRecorder()
	handler.ServeHTTP(lr, login)
	if lr.Code != http.StatusOK {
		t.Fatalf("login = %d: %s", lr.Code, lr.Body.String())
	}
	cookie := lr.Result().Cookies()[0]
	request := func(method, path, body string) *httptest.ResponseRecorder {
		req := httptest.NewRequest(method, path, strings.NewReader(body))
		req.AddCookie(cookie)
		if body != "" {
			req.Header.Set("Content-Type", "application/json")
		}
		rr := httptest.NewRecorder()
		handler.ServeHTTP(rr, req)
		return rr
	}

	get := request(http.MethodGet, "/api/v1/config", "")
	if get.Code != http.StatusOK {
		t.Fatalf("GET /config = %d: %s", get.Code, get.Body.String())
	}
	if strings.Contains(get.Body.String(), "join-secret") || strings.Contains(get.Body.String(), "admin-secret") {
		t.Fatalf("structured Config leaked a password: %s", get.Body.String())
	}
	var doc gameconfig.Response
	if err := json.Unmarshal(get.Body.Bytes(), &doc); err != nil {
		t.Fatal(err)
	}
	if doc.Source != "compose" || doc.Version == "" || !doc.Capabilities.Write.Available || doc.Capabilities.Apply.Available {
		t.Fatalf("GET document does not match frontend contract: %#v", doc)
	}

	body, err := json.Marshal(map[string]any{
		"version": doc.Version,
		"changes": map[string]any{"SERVER_DESCRIPTION": "colon: # literal"},
	})
	if err != nil {
		t.Fatal(err)
	}
	put := request(http.MethodPut, "/api/v1/config", string(body))
	if put.Code != http.StatusOK {
		t.Fatalf("PUT /config = %d: %s", put.Code, put.Body.String())
	}
	var updated gameconfig.Response
	if err := json.Unmarshal(put.Body.Bytes(), &updated); err != nil {
		t.Fatal(err)
	}
	if updated.Version == "" || updated.Version == doc.Version || len(updated.Settings) == 0 {
		t.Fatalf("PUT did not return a complete updated ConfigDoc: %#v", updated)
	}
	written, _ := os.ReadFile(composePath)
	if !strings.Contains(string(written), `SERVER_DESCRIPTION: "colon: # literal"`) {
		t.Fatalf("PUT did not YAML-encode the frontend value: %s", written)
	}

	conflict := request(http.MethodPut, "/api/v1/config", string(body))
	if conflict.Code != http.StatusConflict || !strings.Contains(conflict.Body.String(), `"code":"config_conflict"`) {
		t.Fatalf("stale PUT = %d: %s", conflict.Code, conflict.Body.String())
	}

	apply := request(http.MethodPost, "/api/v1/config/apply", "")
	if apply.Code != http.StatusNotImplemented {
		t.Fatalf("POST /config/apply = %d: %s", apply.Code, apply.Body.String())
	}
	var envelope struct {
		Error struct {
			Code          string `json:"code"`
			ManualCommand string `json:"manualCommand"`
		} `json:"error"`
	}
	if err := json.Unmarshal(apply.Body.Bytes(), &envelope); err != nil {
		t.Fatal(err)
	}
	if envelope.Error.Code != "docker_apply_disabled" || envelope.Error.ManualCommand != "docker compose up -d palworld" {
		t.Fatalf("manual apply envelope drifted: %s", apply.Body.String())
	}
}

func TestConfigOpenAPIContract(t *testing.T) {
	var doc struct {
		Paths map[string]map[string]struct {
			Responses map[string]json.RawMessage `json:"responses"`
		} `json:"paths"`
		Components struct {
			Schemas map[string]json.RawMessage `json:"schemas"`
		} `json:"components"`
	}
	if err := json.Unmarshal(openapi, &doc); err != nil {
		t.Fatal(err)
	}
	want := map[string]map[string][]string{
		"/api/v1/config":       {"get": {"200"}, "put": {"200", "400", "409"}},
		"/api/v1/config/raw":   {"get": {"200", "404"}},
		"/api/v1/config/apply": {"post": {"501"}},
	}
	for path, methods := range want {
		for method, statuses := range methods {
			op, ok := doc.Paths[path][method]
			if !ok {
				t.Errorf("OpenAPI missing %s %s", method, path)
				continue
			}
			for _, status := range statuses {
				if _, ok := op.Responses[status]; !ok {
					t.Errorf("OpenAPI %s %s missing %s response", method, path, status)
				}
			}
		}
	}
	for _, schema := range []string{"Error", "ConfigCapability", "ConfigSetting", "ConfigDoc", "ConfigUpdate"} {
		if _, ok := doc.Components.Schemas[schema]; !ok {
			t.Errorf("OpenAPI missing %s schema", schema)
		}
	}
}
