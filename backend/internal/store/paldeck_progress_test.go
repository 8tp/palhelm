package store

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/8tp/palhelm/internal/sav"
)

func TestPaldeckProgressPersistsAuthoritativeMapsAndNormalizesBossSpecies(t *testing.T) {
	s, err := Open(filepath.Join(t.TempDir(), "paldeck.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	ctx := context.Background()
	at := time.Date(2026, 7, 15, 12, 0, 0, 0, time.UTC)
	totalA, uniqueA, unlockedA := int64(10), 3, 2
	totalB, uniqueB, unlockedB := int64(7), 1, 1
	world := &sav.World{Players: []sav.Player{
		{UID: "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", Nickname: "A", CaptureTotal: &totalA, UniquePalsCaptured: &uniqueA, PaldeckUnlocked: &unlockedA,
			PalCaptureCounts: map[string]int64{"SheepBall": 2, "BOSS_Anubis": 1, "Unknown_One": 4}, PaldeckUnlockFlags: map[string]bool{"SheepBall": true, "BOSS_Anubis": true}},
		{UID: "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb", Nickname: "B", CaptureTotal: &totalB, UniquePalsCaptured: &uniqueB, PaldeckUnlocked: &unlockedB,
			PalCaptureCounts: map[string]int64{"sheepball": 3}, PaldeckUnlockFlags: map[string]bool{"SheepBall": true}},
	}}
	if err = s.ReplaceWorld(ctx, world, at, time.Millisecond); err != nil {
		t.Fatal(err)
	}

	server, err := s.ServerPaldeck(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if server.Coverage.Source != paldeckCoverageSource || server.Coverage.PlayersTotal != 2 || server.Coverage.PlayersWithCaptureCounts != 2 || server.Coverage.PlayersWithUnlockFlags != 2 {
		t.Fatalf("coverage = %#v", server.Coverage)
	}
	if server.CaptureTotal == nil || *server.CaptureTotal != 17 || server.Catalog.KnownSpecies == 0 || server.Catalog.ObservedUnknownSpecies != 1 || len(server.Species) != server.Catalog.KnownSpecies+1 {
		t.Fatalf("server paldeck = %#v", server)
	}
	byID := map[string]PaldeckSpecies{}
	for _, species := range server.Species {
		byID[species.CharacterID] = species
	}
	if got := byID["sheepball"]; got.CaptureCount == nil || *got.CaptureCount != 5 || got.CapturedByPlayers == nil || *got.CapturedByPlayers != 2 || got.UnlockedByPlayers == nil || *got.UnlockedByPlayers != 2 || !got.Known {
		t.Fatalf("Lamball aggregate = %#v", got)
	}
	if got := byID["anubis"]; got.CaptureCount == nil || *got.CaptureCount != 1 {
		t.Fatalf("boss-normalized Anubis = %#v", got)
	}
	if _, exists := byID["boss_anubis"]; exists {
		t.Fatal("boss variant leaked as a duplicate Paldeck species")
	}
	if got := byID["unknown_one"]; got.Known || got.CaptureCount == nil || *got.CaptureCount != 4 {
		t.Fatalf("unknown observed species = %#v", got)
	}

	player, err := s.PlayerPaldeck(ctx, world.Players[0].UID)
	if err != nil {
		t.Fatal(err)
	}
	if !player.Coverage.CaptureCountsAvailable || !player.Coverage.UnlockFlagsAvailable || player.Coverage.CaptureObservedAt == nil || !player.Coverage.CaptureObservedAt.Equal(at) {
		t.Fatalf("player coverage = %#v", player.Coverage)
	}
	playerByID := map[string]PlayerPaldeckSpecies{}
	for _, species := range player.Species {
		playerByID[species.CharacterID] = species
	}
	if absent := playerByID["alpaca"]; absent.CaptureCount == nil || *absent.CaptureCount != 0 || absent.Unlocked == nil || *absent.Unlocked {
		t.Fatalf("known absent species must be authoritative zero/false: %#v", absent)
	}
}

func TestPaldeckProgressPreservesUnavailableAndClearsAuthoritativeEmptyMap(t *testing.T) {
	s, err := Open(filepath.Join(t.TempDir(), "paldeck.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	ctx := context.Background()
	uid := "cccccccccccccccccccccccccccccccc"
	first := time.Date(2026, 7, 15, 12, 0, 0, 0, time.UTC)
	if err = s.ReplaceWorld(ctx, &sav.World{Players: []sav.Player{{UID: uid, PalCaptureCounts: map[string]int64{"SheepBall": 2}, PaldeckUnlockFlags: map[string]bool{"SheepBall": true}}}}, first, 0); err != nil {
		t.Fatal(err)
	}
	// A missing map is not an authoritative clear, matching the aggregate progression contract.
	if err = s.ReplaceWorld(ctx, &sav.World{Players: []sav.Player{{UID: uid}}}, first.Add(time.Hour), 0); err != nil {
		t.Fatal(err)
	}
	got, err := s.PlayerPaldeck(ctx, uid)
	if err != nil || got.Coverage.CaptureObservedAt == nil || !got.Coverage.CaptureObservedAt.Equal(first) {
		t.Fatalf("unavailable map erased prior observation: %#v, %v", got, err)
	}
	// An empty non-nil map is authoritative and clears capture counts while retaining unlocks.
	second := first.Add(2 * time.Hour)
	if err = s.ReplaceWorld(ctx, &sav.World{Players: []sav.Player{{UID: uid, PalCaptureCounts: map[string]int64{}}}}, second, 0); err != nil {
		t.Fatal(err)
	}
	got, err = s.PlayerPaldeck(ctx, uid)
	if err != nil {
		t.Fatal(err)
	}
	if got.Coverage.CaptureObservedAt == nil || !got.Coverage.CaptureObservedAt.Equal(second) || got.Coverage.UnlockObservedAt == nil || !got.Coverage.UnlockObservedAt.Equal(first) {
		t.Fatalf("independent map observations = %#v", got.Coverage)
	}
	for _, species := range got.Species {
		if species.CharacterID == "sheepball" && (species.CaptureCount == nil || *species.CaptureCount != 0 || species.Unlocked == nil || !*species.Unlocked) {
			t.Fatalf("independent capture clear/unlock preservation = %#v", species)
		}
	}
}
