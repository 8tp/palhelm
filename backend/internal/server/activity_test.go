package server

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/8tp/palhelm/internal/store"
)

func TestActivityIsViewerSafeBoundedAndValidatesWindow(t *testing.T) {
	_, h, st := newKeyManagementTestServer(t)
	ctx := context.Background()
	now := time.Now().UTC()
	if err := st.UpsertLivePlayer(ctx, store.Player{UID: "activity-user", Name: "Activity User"}, now); err != nil {
		t.Fatal(err)
	}
	if err := st.StartSession(ctx, "activity-user", now.Add(-time.Hour)); err != nil {
		t.Fatal(err)
	}
	viewer := loginAs(t, h, "viewerpass")
	rr := sessionRequest(h, http.MethodGet, "/api/v1/activity?window=24h", "", viewer)
	if rr.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", rr.Code, rr.Body.String())
	}
	var doc store.ServerActivity
	if err := json.Unmarshal(rr.Body.Bytes(), &doc); err != nil {
		t.Fatal(err)
	}
	if doc.Window != "24h" || doc.Coverage != "panel_observed_sessions" || len(doc.Concurrency) != 24 || doc.ActivePlayers != 1 || len(doc.Players) != 1 || len(doc.Players) > 25 || len(doc.Guilds) > 25 {
		t.Fatalf("activity = %#v", doc)
	}
	body := strings.ToLower(rr.Body.String())
	for _, forbidden := range []string{"steamid", "accountname", "raw_json", "platform", "location", "joinedat", "leftat"} {
		if strings.Contains(body, forbidden) {
			t.Errorf("activity leaked %q: %s", forbidden, rr.Body.String())
		}
	}
	bad := sessionRequest(h, http.MethodGet, "/api/v1/activity?window=1h", "", viewer)
	if bad.Code != http.StatusBadRequest || !strings.Contains(bad.Body.String(), "invalid_window") {
		t.Fatalf("bad window status=%d body=%s", bad.Code, bad.Body.String())
	}
	unauthenticated := sessionRequest(h, http.MethodGet, "/api/v1/activity", "", nil)
	if unauthenticated.Code != http.StatusUnauthorized {
		t.Fatalf("unauthenticated status=%d", unauthenticated.Code)
	}
}
