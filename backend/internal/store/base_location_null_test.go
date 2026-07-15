package store

import (
	"context"
	"encoding/json"
	"path/filepath"
	"testing"
	"time"

	"github.com/8tp/palhelm/internal/sav"
)

// TestBaseNullLocationStaysNull proves the honesty contract end to end: a base
// whose transform was never decoded (Position == nil) is stored and served as a
// null location on every guild surface, never a misleading (0,0).
func TestBaseNullLocationStaysNull(t *testing.T) {
	s, err := Open(filepath.Join(t.TempDir(), "base-null.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	ctx := context.Background()
	now := time.Date(2026, 7, 15, 12, 0, 0, 0, time.UTC)
	member := "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
	guildID := "cccccccccccccccccccccccccccccccc"
	placed := "dddddddddddddddddddddddddddddddd"
	unplaced := "eeeeeeeeeeeeeeeeeeeeeeeeeeeeeeee0"
	world := &sav.World{
		Players: []sav.Player{{UID: member, Nickname: "Member", GuildID: guildID}},
		Guilds:  []sav.Guild{{ID: guildID, Name: "Guild", AdminUID: member, Members: []sav.GuildMember{{UID: member, Name: "Member"}}}},
		Bases: []sav.BaseCamp{
			{ID: placed, GuildID: guildID, Position: &sav.Vector{X: 100, Y: 200, Z: 300}},
			{ID: unplaced, GuildID: guildID, Position: nil}, // transform never decoded
		},
	}
	if err = s.ReplaceWorld(ctx, world, now, 0); err != nil {
		t.Fatal(err)
	}

	// Guild detail: the placed base carries a location, the unplaced one is null.
	detail, err := s.GuildDetail(ctx, guildID, now)
	if err != nil {
		t.Fatal(err)
	}
	byID := map[string]*GuildDetailLocation{}
	for _, b := range detail.Bases {
		byID[b.ID] = b.Location
	}
	if loc := byID[placed]; loc == nil || loc.X != 100 || loc.Y != 200 {
		t.Fatalf("placed base detail location = %#v, want {100,200}", loc)
	}
	if loc, ok := byID[unplaced]; !ok || loc != nil {
		t.Fatalf("unplaced base detail location = %#v, want present-but-null", loc)
	}

	// Typed integration surface: HasLocation distinguishes null from (0,0).
	guilds, err := s.Guilds(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(guilds) != 1 {
		t.Fatalf("Guilds = %d, want 1", len(guilds))
	}
	seen := 0
	for _, b := range guilds[0].Bases {
		switch b.ID {
		case placed:
			if !b.HasLocation || b.X != 100 || b.Y != 200 {
				t.Fatalf("placed GuildBase = %#v", b)
			}
			seen++
		case unplaced:
			if b.HasLocation {
				t.Fatalf("unplaced GuildBase reported HasLocation; must be null")
			}
			seen++
		}
	}
	if seen != 2 {
		t.Fatalf("expected both bases in typed guild, saw %d", seen)
	}

	// Session JSON surface (GuildJSON): the unplaced base's "location" is JSON null.
	guildObjects, err := s.GuildJSON(ctx)
	if err != nil {
		t.Fatal(err)
	}
	raw, err := json.Marshal(guildObjects)
	if err != nil {
		t.Fatal(err)
	}
	var decoded []struct {
		Bases []struct {
			ID       string `json:"id"`
			Location *struct {
				X, Y float64
			} `json:"location"`
		} `json:"bases"`
	}
	if err = json.Unmarshal(raw, &decoded); err != nil {
		t.Fatalf("GuildJSON unmarshal: %v (body %s)", err, raw)
	}
	if len(decoded) != 1 {
		t.Fatalf("GuildJSON guilds = %d, want 1", len(decoded))
	}
	for _, b := range decoded[0].Bases {
		if b.ID == unplaced && b.Location != nil {
			t.Fatalf("GuildJSON unplaced base location = %#v, want null", b.Location)
		}
		if b.ID == placed && (b.Location == nil || b.Location.X != 100) {
			t.Fatalf("GuildJSON placed base location = %#v, want {100,200}", b.Location)
		}
	}
}
