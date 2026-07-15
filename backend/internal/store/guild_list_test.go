package store

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/8tp/palhelm/internal/sav"
)

// TestGuildJSONListsOnlyGuildsWithBaseAndConfirmedMember proves the guild-list filter:
// Palworld records a group for things that are not player guilds (solo auto-orgs, other
// non-guild groups), which decode into rows with no base placed and/or no member whose
// save identity resolves to a known player. The list must drop those, but the detail
// endpoint must still resolve a filtered guild so a player row can link through to it.
func TestGuildJSONListsOnlyGuildsWithBaseAndConfirmedMember(t *testing.T) {
	s, err := Open(filepath.Join(t.TempDir(), "guild-list.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	ctx := context.Background()
	now := time.Date(2026, 7, 15, 12, 0, 0, 0, time.UTC)

	realMember := "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
	backrefPlayer := "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"
	ghostUID := "eeeeeeeeeeeeeeeeeeeeeeeeeeeeeeee"
	realGuild := "11111111111111111111111111111111"
	orgGuild := "22222222222222222222222222222222"
	ghostGuild := "33333333333333333333333333333333"
	emptyGuild := "44444444444444444444444444444444"
	backrefGuild := "55555555555555555555555555555555"

	world := &sav.World{
		Players: []sav.Player{
			{UID: realMember, Nickname: "Member", GuildID: realGuild},
			// Real 1.0 saves carry base-owning guilds whose group roster decodes empty
			// while the players themselves still point at the guild via guild_id.
			{UID: backrefPlayer, Nickname: "Backref", GuildID: backrefGuild},
		},
		Guilds: []sav.Guild{
			// Real player guild: a base is placed and a confirmed player is a member.
			{ID: realGuild, Name: "Real Guild", AdminUID: realMember, Members: []sav.GuildMember{{UID: realMember, Name: "Member"}}},
			// Solo auto-org: confirmed member but no base placed.
			{ID: orgGuild, Name: "Solo Org", AdminUID: realMember, Members: []sav.GuildMember{{UID: realMember, Name: "Member"}}},
			// Group with a base but only an unresolved (non-player) member.
			{ID: ghostGuild, Name: "Ghost Group", AdminUID: ghostUID, Members: []sav.GuildMember{{UID: ghostUID, Name: ""}}},
			// Group with a base but no members at all.
			{ID: emptyGuild, Name: "Empty Group", AdminUID: ""},
			// Base placed, empty group roster, but a known player references the guild.
			{ID: backrefGuild, Name: "Backref Guild", AdminUID: ""},
		},
		Bases: []sav.BaseCamp{
			{ID: "b1b1b1b1b1b1b1b1b1b1b1b1b1b1b1b1", GuildID: realGuild, Position: &sav.Vector{X: 1, Y: 2}},
			{ID: "b3b3b3b3b3b3b3b3b3b3b3b3b3b3b3b3", GuildID: ghostGuild, Position: &sav.Vector{X: 3, Y: 4}},
			{ID: "b4b4b4b4b4b4b4b4b4b4b4b4b4b4b4b4", GuildID: emptyGuild, Position: &sav.Vector{X: 5, Y: 6}},
			{ID: "b5b5b5b5b5b5b5b5b5b5b5b5b5b5b5b5", GuildID: backrefGuild, Position: &sav.Vector{X: 7, Y: 8}},
		},
	}
	if err = s.ReplaceWorld(ctx, world, now, 0); err != nil {
		t.Fatal(err)
	}

	list, err := s.GuildJSON(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(list) != 2 {
		t.Fatalf("guild list = %d guilds, want 2: %#v", len(list), list)
	}
	listed := map[any]bool{list[0]["id"]: true, list[1]["id"]: true}
	if !listed[NormalizeUID(realGuild)] || !listed[NormalizeUID(backrefGuild)] {
		t.Fatalf("listed guilds = %#v, want the real guild and the player-backref guild", list)
	}

	// Every filtered-out guild must still resolve through the detail endpoint so a player
	// row can link to it without a 404.
	for _, id := range []string{orgGuild, ghostGuild, emptyGuild} {
		detail, derr := s.GuildDetail(ctx, id, now)
		if derr != nil {
			t.Fatalf("GuildDetail(%s) filtered from list must still resolve: %v", id, derr)
		}
		if detail.ID != NormalizeUID(id) {
			t.Fatalf("GuildDetail(%s) = %#v", id, detail)
		}
	}
}
