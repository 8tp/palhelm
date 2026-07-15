package store

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/8tp/palhelm/internal/sav"
)

func TestGuildDetailUsesExactCurrentSaveLinksAndClampedObservedActivity(t *testing.T) {
	s, err := Open(filepath.Join(t.TempDir(), "guild.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	ctx := context.Background()
	now := time.Date(2026, 7, 15, 12, 0, 0, 0, time.UTC)
	member, outsider := "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"
	guildID, baseID := "cccccccccccccccccccccccccccccccc", "dddddddddddddddddddddddddddddddd"
	world := &sav.World{
		Players: []sav.Player{{UID: member, Nickname: "Member", GuildID: guildID}, {UID: outsider, Nickname: "Outsider"}},
		Guilds:  []sav.Guild{{ID: guildID, Name: "Guild", AdminUID: member, Members: []sav.GuildMember{{UID: member, Name: "Member"}}}},
		Bases:   []sav.BaseCamp{{ID: baseID, GuildID: guildID, Position: &sav.Vector{X: 100, Y: 200}}},
		Pals: []sav.Pal{
			{InstanceID: "11111111111111111111111111111111", CharacterID: "SheepBall", OwnerUID: member, SlotIndex: -1},
			{InstanceID: "22222222222222222222222222222222", CharacterID: "BOSS_Anubis", OwnerUID: outsider, BaseID: baseID, SlotIndex: -1},
			{InstanceID: "33333333333333333333333333333333", CharacterID: "ChickenPal", OwnerUID: outsider, SlotIndex: -1},
		},
	}
	if err = s.ReplaceWorld(ctx, world, now, 0); err != nil {
		t.Fatal(err)
	}
	if _, err = s.db.Exec(`INSERT INTO sessions(player_uid,join_at,leave_at) VALUES(?,?,?),(?,?,?)`, member, now.Add(-40*24*time.Hour).Unix(), now.Add(-20*24*time.Hour).Unix(), member, now.Add(-time.Hour).Unix(), nil); err != nil {
		t.Fatal(err)
	}

	detail, err := s.GuildDetail(ctx, guildID, now)
	if err != nil {
		t.Fatal(err)
	}
	if detail.Name != "Guild" || detail.MemberCount != 1 || len(detail.Bases) != 1 || detail.Bases[0].Location == nil || detail.Bases[0].Location.X != 100 || detail.Bases[0].PalCount != 1 {
		t.Fatalf("guild detail = %#v", detail)
	}
	if detail.PalCount != 2 || len(detail.Pals) != 2 || detail.PalsTruncated {
		t.Fatalf("associated pals = count %d rows %#v truncated=%v", detail.PalCount, detail.Pals, detail.PalsTruncated)
	}
	associations := map[string]string{}
	for _, pal := range detail.Pals {
		associations[pal.CharacterID] = pal.Association
	}
	if associations["SheepBall"] != "current_member_owner" || associations["BOSS_Anubis"] != "guild_base" || associations["ChickenPal"] != "" {
		t.Fatalf("associations = %#v", associations)
	}
	wantDuration := int64((10*24*time.Hour + time.Hour) / time.Second)
	if detail.Activity.Coverage != "panel_observed_sessions" || detail.Activity.Attribution != "current_guild_membership" || detail.Activity.DurationSec != wantDuration || detail.Activity.SessionCount != 2 || detail.Activity.ActivePlayers != 1 {
		t.Fatalf("activity = %#v, want duration %d", detail.Activity, wantDuration)
	}
	if detail.Members[0].ObservedDurationSec != wantDuration || detail.Members[0].ObservedSessionCount != 2 || !detail.Members[0].CurrentSession {
		t.Fatalf("member activity = %#v", detail.Members[0])
	}
}
