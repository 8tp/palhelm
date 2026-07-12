package server

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/palhelm/palhelm/internal/config"
	"github.com/palhelm/palhelm/internal/store"
)

func TestRESTIntegration(t *testing.T) {
	var kicked atomic.Bool
	fake := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if user, pass, ok := r.BasicAuth(); !ok || user != "admin" || pass != "gamepass" {
			http.Error(w, "bad auth", 401)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/v1/api/info":
			_, _ = io.WriteString(w, `{"servername":"Test","version":"1","worldguid":"abc","uptime":12}`)
		case "/v1/api/metrics":
			_, _ = io.WriteString(w, `{"serverfps":60,"serverframetime":16.6,"currentplayernum":1,"maxplayernum":32,"uptime":12,"days":3,"basecampnum":1}`)
		case "/v1/api/players":
			_, _ = io.WriteString(w, `{"players":[{"name":"Hunter","accountName":"h","playerId":"84C20A31-0000","userId":"steam_123","level":9}]}`)
		case "/v1/api/kick":
			var v map[string]any
			_ = json.NewDecoder(r.Body).Decode(&v)
			if v["userid"] != "steam_123" {
				t.Errorf("userid=%v", v["userid"])
			}
			kicked.Store(true)
			_, _ = io.WriteString(w, `{}`)
		default:
			http.NotFound(w, r)
		}
	}))
	defer fake.Close()
	st, err := store.Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	cfg := config.Config{AdminPassword: "panelpass", SessionSecret: strings.Repeat("s", 48), RESTURL: fake.URL, RESTUser: "admin", PalworldPassword: "gamepass", MetricsInterval: 10 * time.Millisecond, PlayersInterval: 10 * time.Millisecond, SaveSyncInterval: time.Hour}
	app, h := New(cfg, st, slog.New(slog.NewTextHandler(io.Discard, nil)))
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go app.RunPollers(ctx)
	login := httptest.NewRequest("POST", "/api/v1/auth/login", bytes.NewBufferString(`{"password":"panelpass"}`))
	login.Header.Set("Content-Type", "application/json")
	lr := httptest.NewRecorder()
	h.ServeHTTP(lr, login)
	if lr.Code != 200 {
		t.Fatalf("login=%d %s", lr.Code, lr.Body.String())
	}
	cookie := lr.Result().Cookies()[0]
	request := func(method, path, body string) *httptest.ResponseRecorder {
		req := httptest.NewRequest(method, path, strings.NewReader(body))
		req.AddCookie(cookie)
		if body != "" {
			req.Header.Set("Content-Type", "application/json")
		}
		rr := httptest.NewRecorder()
		h.ServeHTTP(rr, req)
		return rr
	}
	deadline := time.Now().Add(time.Second)
	for {
		rr := request("GET", "/api/v1/players", "")
		if rr.Code == 200 && strings.Contains(rr.Body.String(), "Hunter") {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("players never appeared: %s", rr.Body.String())
		}
		time.Sleep(10 * time.Millisecond)
	}
	for _, path := range []string{"/api/v1/metrics/current", "/api/v1/metrics/history?window=1h", "/api/v1/players"} {
		if rr := request("GET", path, ""); rr.Code != 200 {
			t.Fatalf("%s=%d %s", path, rr.Code, rr.Body.String())
		}
	}
	if rr := request("POST", "/api/v1/players/84c20a310000/kick", `{"message":"bye"}`); rr.Code != 200 {
		t.Fatalf("kick=%d %s", rr.Code, rr.Body.String())
	}
	if !kicked.Load() {
		t.Fatal("fake did not receive kick")
	}
	if rr := request("GET", "/api/v1/events?limit=10", ""); rr.Code != 200 || !strings.Contains(rr.Body.String(), "join") {
		t.Fatalf("events=%d %s", rr.Code, rr.Body.String())
	}
}

func TestEmbeddedOpenAPI(t *testing.T) {
	var doc struct {
		OpenAPI string                     `json:"openapi"`
		Paths   map[string]json.RawMessage `json:"paths"`
	}
	if err := json.Unmarshal(openapi, &doc); err != nil {
		t.Fatal(err)
	}
	if doc.OpenAPI != "3.1.0" {
		t.Fatalf("version=%q", doc.OpenAPI)
	}
	for _, path := range []string{"/api/v1/server", "/api/v1/players/{uid}/kick", "/api/v1/events/stream", "/api/v1/backups", "/api/v1/config"} {
		if _, ok := doc.Paths[path]; !ok {
			t.Errorf("missing path %s", path)
		}
	}
}
