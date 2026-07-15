package palworld

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func TestGameDataDecodesDocumentedWireShapeAndDropsPrivateFields(t *testing.T) {
	var sawAuth bool
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/api/game-data" || r.Method != http.MethodGet {
			t.Fatalf("request = %s %s", r.Method, r.URL.Path)
		}
		user, password, ok := r.BasicAuth()
		sawAuth = ok && user == "admin" && password == "secret"
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
          "Time":"2026-07-14 12:34:56","FPS":57.5,"AverageFPS":54.25,
          "ActorData":[
            {"Type":"Character","UnitType":"Player","InstanceID":"player-id","NickName":"Player One","userid":"steam_private","ip":"192.0.2.10","level":35,"HP":900,"MaxHP":1000,"LocationX":1,"LocationY":2,"LocationZ":3,"IsActive":"true","futureField":"ignored"},
            {"Type":"PalBox","GuildID":"guild-id","GuildName":"Example Guild","Class":"PalBox","LocationX":4,"LocationY":5,"LocationZ":6}
          ],"futureRoot":"ignored"
        }`))
	}))
	defer srv.Close()

	got, err := NewClient(srv.URL, "admin", "secret").GameData(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if !sawAuth {
		t.Fatal("request did not use expected Basic auth")
	}
	if got.Time != "2026-07-14 12:34:56" || got.FPS != 57.5 || len(got.ActorData) != 2 {
		t.Fatalf("snapshot = %#v", got)
	}
	if got.ActorData[0].Level != 35 || got.ActorData[0].Active() == nil || !*got.ActorData[0].Active() {
		t.Fatalf("player actor = %#v", got.ActorData[0])
	}
	// The wire DTO has no IP/UserID fields by construction. This string-level assertion also
	// catches a future accidental field addition with those exact JSON tags.
	b, err := json.Marshal(got)
	if err != nil {
		t.Fatal(err)
	}
	encoded := strings.ToLower(string(b))
	for _, private := range []string{"steam_private", "192.0.2.10", `"ip"`, `"userid"`} {
		if strings.Contains(encoded, strings.ToLower(private)) {
			t.Fatalf("private field %q survived credential-boundary decode: %s", private, encoded)
		}
	}
}

func TestGameDataClassifiesUnsupportedAndUnauthorized(t *testing.T) {
	for _, tc := range []struct {
		name   string
		status int
		kind   ErrorKind
	}{{"unsupported", http.StatusNotFound, ErrorUnsupported}, {"unauthorized", http.StatusUnauthorized, ErrorUnauthorized}} {
		t.Run(tc.name, func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(tc.status)
				_, _ = w.Write([]byte(`{"ip":"must-not-escape"}`))
			}))
			defer srv.Close()
			_, err := NewClient(srv.URL, "admin", "secret").GameData(context.Background())
			if !IsKind(err, tc.kind) {
				t.Fatalf("error = %v, want kind %s", err, tc.kind)
			}
			if strings.Contains(err.Error(), "must-not-escape") {
				t.Fatalf("upstream body escaped in error: %v", err)
			}
		})
	}
}

func TestGameDataRejectsOversizedAndMalformedBodies(t *testing.T) {
	for _, tc := range []struct {
		name   string
		body   string
		length int64
	}{{name: "content length", body: `{}`, length: GameDataMaxResponseBytes + 1}, {name: "malformed", body: `{`, length: -1}} {
		t.Run(tc.name, func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				if tc.length >= 0 {
					w.Header().Set("Content-Length", "33554433")
				}
				_, _ = w.Write([]byte(tc.body))
			}))
			defer srv.Close()
			_, err := NewClient(srv.URL, "", "").GameData(context.Background())
			if !IsKind(err, ErrorResponse) {
				t.Fatalf("error = %v, want response kind", err)
			}
		})
	}
}

func TestGameDataUsesIndependentTimeout(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		time.Sleep(100 * time.Millisecond)
		_, _ = w.Write([]byte(`{"Time":"x","FPS":1,"AverageFPS":1,"ActorData":[]}`))
	}))
	defer srv.Close()
	client := NewClient(srv.URL, "", "")
	client.SetGameDataTimeout(20 * time.Millisecond)
	_, err := client.GameData(context.Background())
	if err == nil || !IsKind(err, ErrorUnreachable) {
		t.Fatalf("error = %v, want timeout/unreachable", err)
	}
	if !errors.Is(err, context.DeadlineExceeded) {
		var apiErr *APIError
		if !errors.As(err, &apiErr) || apiErr.Err == nil {
			t.Fatalf("timeout error lost cause: %v", err)
		}
	}
}

func TestGameDataActiveRejectsUnknownValues(t *testing.T) {
	if got := (GameDataActor{IsActive: "yes"}).Active(); got != nil {
		t.Fatalf("Active() = %v, want nil", *got)
	}
}

func TestGameDataRefusesRedirectWithoutForwardingCredentials(t *testing.T) {
	var destinationCalled atomic.Bool
	destination := httptest.NewServer(http.HandlerFunc(func(_ http.ResponseWriter, _ *http.Request) {
		destinationCalled.Store(true)
	}))
	defer destination.Close()
	source := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		user, password, ok := r.BasicAuth()
		if !ok || user != "admin" || password != "secret" {
			t.Fatalf("source auth = %q/%q ok=%v", user, password, ok)
		}
		http.Redirect(w, r, destination.URL, http.StatusFound)
	}))
	defer source.Close()

	_, err := NewClient(source.URL, "admin", "secret").GameData(context.Background())
	if !IsKind(err, ErrorResponse) {
		t.Fatalf("error = %v, want response kind", err)
	}
	if destinationCalled.Load() {
		t.Fatal("redirect destination was called; credentials were not pinned to configured host")
	}
}

func TestGameDataRejectsChunkedBodyOverLimit(t *testing.T) {
	chunk := bytes.Repeat([]byte(" "), 1<<20)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if flusher, ok := w.(http.Flusher); ok {
			flusher.Flush() // commits a response with unknown Content-Length
		}
		for i := 0; i < 33; i++ {
			if _, err := w.Write(chunk); err != nil {
				return
			}
		}
	}))
	defer srv.Close()
	_, err := NewClient(srv.URL, "", "").GameData(context.Background())
	if !IsKind(err, ErrorResponse) || !strings.Contains(err.Error(), "size limit") {
		t.Fatalf("error = %v, want size-limit response", err)
	}
}

func TestGameDataRejectsActorCountOverLimit(t *testing.T) {
	body := `{"Time":"x","FPS":1,"AverageFPS":1,"ActorData":[`
	body += strings.Repeat("{},", GameDataMaxActors) + `{}` + `]}`
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(body))
	}))
	defer srv.Close()
	_, err := NewClient(srv.URL, "", "").GameData(context.Background())
	if !IsKind(err, ErrorResponse) || !strings.Contains(err.Error(), "actor count") {
		t.Fatalf("error = %v, want actor-count response", err)
	}
}

func TestGameDataRejectsNonFiniteNumbers(t *testing.T) {
	for _, body := range []string{
		`{"Time":"x","FPS":1e1000,"AverageFPS":1,"ActorData":[]}`,
		`{"Time":"x","FPS":1,"AverageFPS":1,"ActorData":[{"LocationX":1e1000}]}`,
	} {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			_, _ = w.Write([]byte(body))
		}))
		_, err := NewClient(srv.URL, "", "").GameData(context.Background())
		srv.Close()
		if !IsKind(err, ErrorResponse) {
			t.Fatalf("error = %v, want response kind", err)
		}
	}
}
