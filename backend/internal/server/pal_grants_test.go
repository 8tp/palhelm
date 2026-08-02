package server

import (
	"context"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/8tp/palhelm/internal/store"
)

func TestPalGrantCatalogIsSafeAndMutationDefaultsFailClosed(t *testing.T) {
	h, st := newItemGrantTestServer(t)
	uid := "abcdef12000000000000000000000000"
	if err := st.UpsertLivePlayer(context.Background(), store.Player{UID: uid, Name: "Player"}, time.Now().UTC()); err != nil {
		t.Fatal(err)
	}
	viewer := loginAs(t, h, "viewerpass")
	admin := loginAs(t, h, "panelpass")
	catalog := sessionRequest(h, http.MethodGet, "/api/v1/pal-grants/catalog?q=anub", "", viewer)
	if catalog.Code != 200 || !strings.Contains(catalog.Body.String(), `"displayName":"Anubis"`) || strings.Contains(catalog.Body.String(), "Hunter_Rifle") {
		t.Fatalf("catalog status=%d body=%s", catalog.Code, catalog.Body.String())
	}
	passives := sessionRequest(h, http.MethodGet, "/api/v1/pal-grants/passives?q=remarkable", "", viewer)
	if passives.Code != 200 || !strings.Contains(passives.Body.String(), `"id":"CraftSpeed_up3"`) || !strings.Contains(passives.Body.String(), `"rank":4`) {
		t.Fatalf("passive catalog status=%d body=%s", passives.Code, passives.Body.String())
	}
	capability := sessionRequest(h, http.MethodGet, "/api/v1/pal-grants/capability", "", viewer)
	if capability.Code != 200 || !strings.Contains(capability.Body.String(), `"ready":false`) {
		t.Fatalf("capability status=%d body=%s", capability.Code, capability.Body.String())
	}
	body := `{"characterId":"anubis","spec":{"mode":"natural","levelMin":20,"levelMax":25,"gender":"random","alpha":false,"lucky":false,"ivMode":"natural","passiveSkillIds":[],"condensationStars":0},"reason":"event reward","idempotencyKey":"pal-test-123"}`
	if got := sessionRequest(h, http.MethodPost, "/api/v1/players/"+uid+"/pal-grants", body, viewer); got.Code != http.StatusForbidden {
		t.Fatalf("viewer mutation status=%d body=%s", got.Code, got.Body.String())
	}
	if got := sessionRequest(h, http.MethodPost, "/api/v1/players/"+uid+"/pal-grants", body, admin); got.Code != http.StatusServiceUnavailable || !strings.Contains(got.Body.String(), "pal_grants_unavailable") {
		t.Fatalf("disabled mutation status=%d body=%s", got.Code, got.Body.String())
	}
}
