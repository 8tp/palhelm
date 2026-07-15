package server

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/8tp/palhelm/internal/sav"
)

func TestSessionPalsFiltersPaginatesAndMarksBosses(t *testing.T) {
	_, h, st := newKeyManagementTestServer(t)
	w := &sav.World{
		Players: []sav.Player{{UID: "owner-a", Nickname: "Owner A", PalStorageContainerID: "box-a"}},
		Pals: []sav.Pal{
			{InstanceID: "a-boss", CharacterID: "BOSS_GrassMammoth", OwnerUID: "owner-a", ContainerID: "box-a", SlotIndex: 0, Level: 35},
			{InstanceID: "b-pal", CharacterID: "Anubis", OwnerUID: "owner-a", ContainerID: "box-a", SlotIndex: 1, Level: 30},
			{InstanceID: "c-pal", CharacterID: "SheepBall", OwnerUID: "owner-a", ContainerID: "box-a", SlotIndex: 2, Level: 5, IsLucky: true},
		},
	}
	if err := st.ReplaceWorld(context.Background(), w, time.Now(), 0); err != nil {
		t.Fatal(err)
	}
	viewer := loginAs(t, h, "viewerpass")
	request := func(method, path string) *httptest.ResponseRecorder {
		return sessionRequest(h, method, path, "", viewer)
	}

	first := request(http.MethodGet, "/api/v1/pals?q=owner+a&placement=box&minLevel=20&limit=1")
	if first.Code != http.StatusOK {
		t.Fatalf("first status=%d body=%s", first.Code, first.Body.String())
	}
	var page sessionPalExplorerPage
	if err := json.Unmarshal(first.Body.Bytes(), &page); err != nil {
		t.Fatal(err)
	}
	if len(page.Data) != 1 || page.Data[0].DisplayName != "Mammorest" || !page.Data[0].IsBoss || page.Data[0].OwnerName != "Owner A" || page.Data[0].Placement != "box" || page.NextCursor == nil {
		t.Fatalf("first page = %#v", page)
	}
	if page.Data[0].PassiveSkillIDs == nil || page.Data[0].EquippedSkillIDs == nil {
		t.Fatalf("skill arrays must be non-null: %#v", page.Data[0])
	}

	second := request(http.MethodGet, "/api/v1/pals?q=owner+a&placement=box&minLevel=20&limit=1&cursor="+*page.NextCursor)
	if second.Code != http.StatusOK {
		t.Fatalf("second status=%d body=%s", second.Code, second.Body.String())
	}
	if err := json.Unmarshal(second.Body.Bytes(), &page); err != nil {
		t.Fatal(err)
	}
	if len(page.Data) != 1 || page.Data[0].DisplayName != "Anubis" || page.Data[0].IsBoss || page.NextCursor != nil {
		t.Fatalf("second page = %#v", page)
	}
	for _, forbidden := range []string{"raw_json", "steamId", "accountName"} {
		if contains := jsonContainsKey(second.Body.Bytes(), forbidden); contains {
			t.Errorf("session Pal roster leaked %q: %s", forbidden, second.Body.String())
		}
	}
}

func TestSessionPalsRejectsUnboundedOrInvalidFilters(t *testing.T) {
	_, h, _ := newKeyManagementTestServer(t)
	viewer := loginAs(t, h, "viewerpass")
	request := func(method, path string) *httptest.ResponseRecorder {
		return sessionRequest(h, method, path, "", viewer)
	}
	for _, path := range []string{
		"/api/v1/pals?limit=101",
		"/api/v1/pals?placement=inventory",
		"/api/v1/pals?specimen=shiny",
		"/api/v1/pals?ownerSource=guessed",
		"/api/v1/pals?minLevel=40&maxLevel=20",
	} {
		rr := request(http.MethodGet, path)
		if rr.Code != http.StatusBadRequest {
			t.Errorf("%s status=%d body=%s", path, rr.Code, rr.Body.String())
		}
	}

	unauthenticated := integrationRequest(h, http.MethodGet, "/api/v1/pals", "")
	if unauthenticated.Code != http.StatusUnauthorized {
		t.Fatalf("unauthenticated status=%d body=%s", unauthenticated.Code, unauthenticated.Body.String())
	}
}

func TestOpenAPIDocumentsSessionPalExplorer(t *testing.T) {
	_, h, _ := newIntegrationTestServer(t, nil)
	rr := integrationRequest(h, http.MethodGet, "/api/openapi.json", "")
	if rr.Code != http.StatusOK {
		t.Fatalf("openapi status=%d body=%s", rr.Code, rr.Body.String())
	}
	var doc struct {
		Paths map[string]any `json:"paths"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &doc); err != nil {
		t.Fatal(err)
	}
	if _, ok := doc.Paths["/api/v1/pals"]; !ok {
		t.Fatal("OpenAPI is missing /api/v1/pals")
	}
}

func jsonContainsKey(body []byte, key string) bool {
	var value any
	if json.Unmarshal(body, &value) != nil {
		return false
	}
	var walk func(any) bool
	walk = func(node any) bool {
		switch typed := node.(type) {
		case map[string]any:
			for candidate, child := range typed {
				if candidate == key || walk(child) {
					return true
				}
			}
		case []any:
			for _, child := range typed {
				if walk(child) {
					return true
				}
			}
		}
		return false
	}
	return walk(value)
}
