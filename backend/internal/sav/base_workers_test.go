package sav

import (
	"os"
	"testing"
)

func TestWorkerContainerIDDecodesVersionedRawData(t *testing.T) {
	base := testGUID(0x41)
	container := testGUID(0x72)
	raw := &gw{}
	raw.guid(base)
	raw.bytes(make([]byte, 10*8)) // FTransform
	raw.u8(2)                     // current order type
	raw.u8(3)                     // current battle type
	raw.guid(container)
	raw.u32(1) // Palworld 1.0 trailing version field

	baseID, err := readGUID(newReader(base[:]))
	if err != nil {
		t.Fatal(err)
	}
	want, err := readGUID(newReader(container[:]))
	if err != nil {
		t.Fatal(err)
	}
	if got := workerContainerID(raw.b, baseID); got != want {
		t.Fatalf("workerContainerID = %q, want %q", got, want)
	}
	if got := workerContainerID(raw.b, "not-the-embedded-base"); got != "" {
		t.Fatalf("mismatched embedded base decoded container %q", got)
	}
}

func TestAssignBaseWorkersJoinsOnlyExactContainer(t *testing.T) {
	w := &World{
		Bases: []BaseCamp{{ID: "base-a", WorkerContainerID: "container-a"}},
		Pals: []Pal{
			{InstanceID: "worker", ContainerID: "CONTAINER-A"},
			{InstanceID: "boxed", ContainerID: "container-b"},
		},
	}
	assignBaseWorkers(w)
	if w.Pals[0].BaseID != "base-a" || w.Pals[1].BaseID != "" {
		t.Fatalf("base assignment = %#v", w.Pals)
	}
}

func TestAssignBaseGuildsUsesStructuralGuildBaseIDs(t *testing.T) {
	w := &World{
		Guilds: []Guild{{ID: "guild-a", BaseIDs: []string{"BASE-A"}}},
		Bases:  []BaseCamp{{ID: "base-a"}, {ID: "base-b"}},
	}
	assignBaseGuilds(w)
	if w.Bases[0].GuildID != "guild-a" || w.Bases[1].GuildID != "" {
		t.Fatalf("base guild assignment = %#v", w.Bases)
	}
}

// Optional read-only proof against a live Palworld 1.0 save. It logs aggregate
// coverage only—never player, Pal, base, guild, or container identifiers.
func TestLiveBaseWorkerProbe(t *testing.T) {
	path := os.Getenv("PROBE_BASE_SAV")
	if path == "" {
		t.Skip("set PROBE_BASE_SAV=/path/to/Level.sav to run the base-worker probe")
	}
	w, err := ParseLevel(path, Options{})
	if err != nil {
		t.Fatalf("ParseLevel: %v", err)
	}
	containers, guildLinkedBases, basesWithWorkers, workers := 0, 0, 0, 0
	workersByBase := map[string]int{}
	for _, base := range w.Bases {
		if base.WorkerContainerID != "" {
			containers++
		}
		if base.GuildID != "" {
			guildLinkedBases++
		}
	}
	for _, pal := range w.Pals {
		if pal.BaseID != "" {
			workers++
			workersByBase[pal.BaseID]++
		}
	}
	for _, count := range workersByBase {
		if count > 0 {
			basesWithWorkers++
		}
	}
	t.Logf("base-worker coverage: bases=%d decodedContainers=%d guildLinkedBases=%d basesWithWorkers=%d workers=%d pals=%d", len(w.Bases), containers, guildLinkedBases, basesWithWorkers, workers, len(w.Pals))
	if len(w.Bases) == 0 || containers == 0 || guildLinkedBases == 0 || workers == 0 {
		t.Fatalf("base-worker mapping has implausibly empty coverage")
	}
}
