package store

import (
	"context"
	"path/filepath"
	"testing"
	"time"
)

func TestServerActivityClampsRanksAndAttributesCurrentGuild(t *testing.T) {
	s, err := Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	ctx := context.Background()
	now := time.Date(2026, 7, 15, 18, 0, 0, 0, time.UTC)
	for _, player := range []Player{{UID: "a", Name: "A"}, {UID: "b", Name: "B"}, {UID: "c", Name: "C"}} {
		if err = s.UpsertLivePlayer(ctx, player, now); err != nil {
			t.Fatal(err)
		}
	}
	if _, err = s.db.Exec("UPDATE players SET guild_id='guild-one',guild_name='Guild One' WHERE uid IN ('a','b')"); err != nil {
		t.Fatal(err)
	}
	unix := func(d time.Duration) int64 { return now.Add(d).Unix() }
	for _, args := range [][]any{
		{"a", unix(-40 * 24 * time.Hour), unix(-39 * 24 * time.Hour)},
		{"a", unix(-2 * time.Hour), nil},
		{"b", unix(-30 * time.Minute), nil},
		{"c", unix(-90 * time.Minute), unix(-30 * time.Minute)},
	} {
		if _, err = s.db.Exec("INSERT INTO sessions(player_uid,join_at,leave_at) VALUES(?,?,?)", args...); err != nil {
			t.Fatal(err)
		}
	}

	got, err := s.ServerActivity(ctx, now, 24*time.Hour, time.Hour, "24h", 2)
	if err != nil {
		t.Fatal(err)
	}
	if got.Coverage != "panel_observed_sessions" || got.TrackingSince == nil || !got.TrackingSince.Equal(now.Add(-40*24*time.Hour)) || got.AnalysisTruncated {
		t.Fatalf("coverage = %#v", got)
	}
	if got.ActivePlayers != 3 || got.NewPlayers != 2 || got.ReturningPlayers != 1 {
		t.Fatalf("player classification = active %d new %d returning %d", got.ActivePlayers, got.NewPlayers, got.ReturningPlayers)
	}
	if got.PeakConcurrency != 2 || got.PeakAt == nil || len(got.Concurrency) != 24 {
		t.Fatalf("concurrency = peak %d at %v buckets %d", got.PeakConcurrency, got.PeakAt, len(got.Concurrency))
	}
	if len(got.Players) != 2 || got.Players[0].UID != "a" || got.Players[0].DurationSec != int64(2*time.Hour/time.Second) || !got.Players[0].CurrentSession || got.Players[1].UID != "c" {
		t.Fatalf("bounded player ranking = %#v", got.Players)
	}
	if len(got.Guilds) != 1 || got.Guilds[0].GuildName != "Guild One" || got.Guilds[0].DurationSec != int64(150*time.Minute/time.Second) || got.Guilds[0].ActivePlayers != 2 || got.GuildAttribution != "current_player_guild" {
		t.Fatalf("guild ranking = %#v attribution=%q", got.Guilds, got.GuildAttribution)
	}
	if got.UnattributedPlayers != 1 || got.UnattributedDurationSec != int64(time.Hour/time.Second) {
		t.Fatalf("unattributed = %d players %d sec", got.UnattributedPlayers, got.UnattributedDurationSec)
	}
}

func TestActivityConcurrencyUsesAverageAndPeakPerBucket(t *testing.T) {
	since := time.Date(2026, 7, 15, 0, 0, 0, 0, time.UTC)
	through := since.Add(2 * time.Hour)
	boundaries := []activityBoundary{
		{at: since.Unix(), delta: 1},
		{at: since.Add(30 * time.Minute).Unix(), delta: 1},
		{at: since.Add(90 * time.Minute).Unix(), delta: -1},
		{at: through.Unix(), delta: -1},
	}
	buckets, peak, peakAt := activityConcurrency(boundaries, since, through, time.Hour)
	if len(buckets) != 2 || peak != 2 || peakAt == nil || buckets[0].PeakPlayers != 2 || buckets[0].AveragePlayers != 1.5 || buckets[1].AveragePlayers != 1.5 {
		t.Fatalf("buckets=%#v peak=%d at=%v", buckets, peak, peakAt)
	}
}
