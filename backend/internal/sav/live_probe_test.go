package sav

import (
	"os"
	"testing"
)

// TestLiveSaveProbe parses a real Palworld 1.0 Level.sav end-to-end (through the
// Oodle container) and asserts the recovered entity counts. It is optional: set
// PROBE_SAV to the absolute path of a Level.sav to run it. Because a real save
// contains personal data it is never committed; this guards the 1.0 fixes
// against regressions on genuine game output during local development.
func TestLiveSaveProbe(t *testing.T) {
	path := os.Getenv("PROBE_SAV")
	if path == "" {
		t.Skip("set PROBE_SAV=/path/to/Level.sav to run the live-save probe")
	}
	w, err := ParseLevel(path, Options{})
	if err != nil {
		t.Fatalf("ParseLevel: %v", err)
	}
	t.Logf("players=%d pals=%d guilds=%d skippedStructs=%d skippedProps=%d failures=%v",
		len(w.Players), len(w.Pals), len(w.Guilds),
		w.Stats.SkippedStructs, w.Stats.SkippedProperties, w.Stats.DecodeFailures)
	progressPlayers := 0
	for _, p := range w.Players {
		if p.CaptureTotal != nil && p.UniquePalsCaptured != nil && p.PaldeckUnlocked != nil {
			progressPlayers++
		}
	}
	t.Logf("players with decoded capture progression: %d/%d", progressPlayers, len(w.Players))
	if progressPlayers == 0 {
		t.Fatal("no per-player RecordData capture progression decoded")
	}

	if w.Stats.SkippedStructs != 0 {
		t.Fatalf("skippedStructs=%d, want 0 (worldSaveData must decode fully)", w.Stats.SkippedStructs)
	}
	if w.Stats.SkippedProperties != 0 {
		t.Fatalf("skippedProperties=%d, want 0 (guild tail must decode with zero tolerance): %v",
			w.Stats.SkippedProperties, w.Stats.SkippedDetails)
	}
	if len(w.Players) < 1 {
		t.Fatalf("players=%d, want >=1", len(w.Players))
	}
	if len(w.Pals) < 1 {
		t.Fatalf("pals=%d, want >=1", len(w.Pals))
	}
	nonzero := 0
	for _, p := range w.Pals {
		if p.Level > 0 {
			nonzero++
		}
	}
	if nonzero == 0 {
		t.Fatal("no pal reported a nonzero level (ByteProperty level decode regressed)")
	}
	// At least one guild must decode with a base camp and members. Any real
	// world with an established guild satisfies this; no specific name is assumed.
	var camp *Guild
	for i := range w.Guilds {
		if w.Guilds[i].BaseCampLevel >= 1 && len(w.Guilds[i].BaseIDs) >= 1 && len(w.Guilds[i].Members) >= 1 {
			camp = &w.Guilds[i]
			break
		}
	}
	if camp == nil {
		t.Fatalf("no guild decoded with baseCampLevel>=1, a base camp, and members among %d guilds", len(w.Guilds))
	}
	// At least one decoded player must be linked to a decoded guild.
	guildIDs := make(map[string]bool, len(w.Guilds))
	for i := range w.Guilds {
		guildIDs[w.Guilds[i].ID] = true
	}
	linked := 0
	for _, p := range w.Players {
		if p.GuildID != "" && guildIDs[p.GuildID] {
			linked++
		}
	}
	if linked == 0 {
		t.Fatal("no player linked to a decoded guild (player.GuildID / guild.ID mismatch)")
	}
	t.Logf("established guild: baseCampLevel=%d baseIDs=%d members=%d; players linked to guilds=%d",
		camp.BaseCampLevel, len(camp.BaseIDs), len(camp.Members), linked)

	// Container placement: owned pals must carry a non-empty ContainerID and a
	// non-negative SlotIndex once the SlotId struct is extracted. Wild/NPC pals
	// legitimately lack one, so this asserts the aggregate, not every pal.
	placed, minSlot := 0, -1
	for _, p := range w.Pals {
		if p.ContainerID != "" && p.SlotIndex >= 0 {
			placed++
			if minSlot == -1 || p.SlotIndex < minSlot {
				minSlot = p.SlotIndex
			}
		}
	}
	t.Logf("pals with container placement: %d/%d (min slot=%d)", placed, len(w.Pals), minSlot)
	if placed == 0 {
		t.Fatal("no pal resolved a ContainerID/SlotIndex (SlotId extraction regressed)")
	}

	// The party (Otomo) container id of a player should match the ContainerID of a
	// small number (<=5) of that player's pals — the in-party pals.
	for i := range w.Players {
		pl := &w.Players[i]
		if pl.OtomoContainerID == "" && pl.PalStorageContainerID == "" {
			continue
		}
		party := 0
		for _, p := range w.Pals {
			if pl.OtomoContainerID != "" && p.ContainerID == pl.OtomoContainerID {
				party++
			}
		}
		t.Logf("player %q (uid=%s): otomo=%v palbox=%v partyMembersByContainer=%d",
			pl.Nickname, pl.UID, pl.OtomoContainerID != "", pl.PalStorageContainerID != "", party)
		if party > 5 {
			t.Fatalf("player %q party container matched %d pals, want <=5", pl.Nickname, party)
		}
	}
}
