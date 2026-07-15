package server

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"
	"time"

	"github.com/8tp/palhelm/internal/sav"
)

// TestGuildsListHidesPlaceholderGroupsButDetailStillResolves verifies the /api/v1/guilds
// endpoint only lists genuine player guilds (a placed base and a confirmed player member)
// while the /api/v1/guilds/{id} detail endpoint still resolves a filtered-out group, so a
// player row that links to its guild never 404s.
func TestGuildsListHidesPlaceholderGroupsButDetailStillResolves(t *testing.T) {
	_, h, st := newKeyManagementTestServer(t)
	ctx := context.Background()
	now := time.Now().UTC().Truncate(time.Second)

	member := "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
	realGuild := "11111111111111111111111111111111"
	orgGuild := "22222222222222222222222222222222"

	world := &sav.World{
		Players: []sav.Player{{UID: member, Nickname: "Member", GuildID: realGuild}},
		Guilds: []sav.Guild{
			{ID: realGuild, Name: "Real Guild", AdminUID: member, Members: []sav.GuildMember{{UID: member, Name: "Member"}}},
			{ID: orgGuild, Name: "Solo Org", AdminUID: member, Members: []sav.GuildMember{{UID: member, Name: "Member"}}},
		},
		Bases: []sav.BaseCamp{{ID: "b1b1b1b1b1b1b1b1b1b1b1b1b1b1b1b1", GuildID: realGuild, Position: &sav.Vector{X: 1, Y: 2}}},
	}
	if err := st.ReplaceWorld(ctx, world, now, time.Millisecond); err != nil {
		t.Fatal(err)
	}

	viewer := loginAs(t, h, "viewerpass")
	listRR := sessionRequest(h, http.MethodGet, "/api/v1/guilds", "", viewer)
	if listRR.Code != http.StatusOK {
		t.Fatalf("guild list status=%d body=%s", listRR.Code, listRR.Body.String())
	}
	var list []map[string]any
	if err := json.Unmarshal(listRR.Body.Bytes(), &list); err != nil {
		t.Fatal(err)
	}
	if len(list) != 1 || list[0]["name"] != "Real Guild" {
		t.Fatalf("guild list = %#v, want only the real guild", list)
	}

	// The solo org is filtered from the list but must still open by id.
	detailRR := sessionRequest(h, http.MethodGet, "/api/v1/guilds/"+orgGuild, "", viewer)
	if detailRR.Code != http.StatusOK {
		t.Fatalf("filtered guild detail status=%d body=%s", detailRR.Code, detailRR.Body.String())
	}
}
