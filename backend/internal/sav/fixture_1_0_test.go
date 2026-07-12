package sav

import (
	"encoding/binary"
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

// This file builds a fully synthetic Palworld 1.0 worldSaveData GVAS body and
// asserts the parser recovers players, pals and guilds from it. Every string and
// GUID here is invented so the committed fixture contains no real player data.
//
// The builders mirror the reader functions in reader.go/props.go byte-for-byte:
// a property is name+type+size(u64)+preamble+body, where the declared size counts
// only the value body that follows the type-specific tag preamble.

type gw struct{ b []byte }

func (w *gw) bytes(p []byte)  { w.b = append(w.b, p...) }
func (w *gw) u8(v uint8)      { w.b = append(w.b, v) }
func (w *gw) u32(v uint32)    { w.b = binary.LittleEndian.AppendUint32(w.b, v) }
func (w *gw) i32(v int32)     { w.u32(uint32(v)) }
func (w *gw) u64(v uint64)    { w.b = binary.LittleEndian.AppendUint64(w.b, v) }
func (w *gw) i64(v int64)     { w.u64(uint64(v)) }
func (w *gw) guid(b [16]byte) { w.b = append(w.b, b[:]...) }
func (w *gw) optGUIDAbsent()  { w.u8(0) }
func (w *gw) fstr(s string) {
	if s == "" {
		w.i32(0)
		return
	}
	w.i32(int32(len(s) + 1))
	w.bytes([]byte(s))
	w.u8(0)
}

// testGUID makes a deterministic, obviously-synthetic 16-byte GUID.
func testGUID(tag byte) [16]byte {
	var g [16]byte
	for i := range g {
		g[i] = tag
	}
	return g
}

// --- property builders (each returns the full name..body byte sequence) ---

func propHeader(name, typ string, size uint64) []byte {
	w := &gw{}
	w.fstr(name)
	w.fstr(typ)
	w.u64(size)
	return w.b
}

func boolProp(name string, v bool) []byte {
	w := &gw{}
	w.bytes(propHeader(name, "BoolProperty", 0))
	if v {
		w.u8(1)
	} else {
		w.u8(0)
	}
	w.optGUIDAbsent()
	return w.b
}

func strProp(name, typ, s string) []byte {
	body := &gw{}
	body.optGUIDAbsent()
	body.fstr(s)
	w := &gw{}
	w.bytes(propHeader(name, typ, uint64(len(body.b))))
	w.bytes(body.b)
	return w.b
}

func intProp(name string, v int32) []byte {
	w := &gw{}
	w.bytes(propHeader(name, "IntProperty", 5))
	w.optGUIDAbsent()
	w.i32(v)
	return w.b
}

func int64Prop(name string, v int64) []byte {
	w := &gw{}
	w.bytes(propHeader(name, "Int64Property", 9))
	w.optGUIDAbsent()
	w.i64(v)
	return w.b
}

// byteProp encodes a ByteProperty whose enum type is "None" and whose value is a
// raw byte, matching Palworld 1.0 pal Level / Talent_* serialization.
func byteProp(name string, v uint8) []byte {
	body := &gw{}
	body.fstr("None")
	body.optGUIDAbsent()
	body.u8(v)
	w := &gw{}
	w.bytes(propHeader(name, "ByteProperty", uint64(len(body.b))))
	w.bytes(body.b)
	return w.b
}

func enumProp(name, enumType, value string) []byte {
	body := &gw{}
	body.fstr(enumType)
	body.optGUIDAbsent()
	body.fstr(value)
	w := &gw{}
	w.bytes(propHeader(name, "EnumProperty", uint64(len(body.b))))
	w.bytes(body.b)
	return w.b
}

// structProp encodes a StructProperty whose value bytes are supplied directly.
func structProp(name, structType string, value []byte) []byte {
	preamble := &gw{}
	preamble.fstr(structType)
	preamble.guid([16]byte{}) // struct header guid
	preamble.optGUIDAbsent()
	w := &gw{}
	w.bytes(propHeader(name, "StructProperty", uint64(len(value))))
	w.bytes(preamble.b)
	w.bytes(value)
	return w.b
}

// guidStructProp encodes a StructProperty of struct-type "Guid" carrying a GUID.
func guidStructProp(name string, g [16]byte) []byte {
	return structProp(name, "Guid", g[:])
}

// hpStructProp encodes the 1.0 pal/player HP field: a StructProperty of type
// FixedPoint64 wrapping an Int64Property named "Value".
func hpStructProp(name string, hp int64) []byte {
	inner := &gw{}
	inner.bytes(int64Prop("Value", hp))
	inner.fstr("None")
	return structProp(name, "FixedPoint64", inner.b)
}

func byteArrayProp(name string, data []byte) []byte {
	body := &gw{}
	body.u32(uint32(len(data)))
	body.bytes(data)
	preamble := &gw{}
	preamble.fstr("ByteProperty")
	preamble.optGUIDAbsent()
	w := &gw{}
	w.bytes(propHeader(name, "ArrayProperty", uint64(len(body.b))))
	w.bytes(preamble.b)
	w.bytes(body.b)
	return w.b
}

func stringArrayProp(name, elementType string, values ...string) []byte {
	body := &gw{}
	body.u32(uint32(len(values)))
	for _, value := range values {
		body.fstr(value)
	}
	preamble := &gw{}
	preamble.fstr(elementType)
	preamble.optGUIDAbsent()
	w := &gw{}
	w.bytes(propHeader(name, "ArrayProperty", uint64(len(body.b))))
	w.bytes(preamble.b)
	w.bytes(body.b)
	return w.b
}

// mapProp encodes a MapProperty: key/value types plus the concatenated entry
// bytes. Entry encoding depends on the reader's per-type behaviour and the
// registered type hints, so callers pass already-serialized entries.
func mapProp(name, keyType, valueType string, count uint32, entries []byte) []byte {
	body := &gw{}
	body.u32(0) // reserved/pad word the reader discards
	body.u32(count)
	body.bytes(entries)
	preamble := &gw{}
	preamble.fstr(keyType)
	preamble.fstr(valueType)
	preamble.optGUIDAbsent()
	w := &gw{}
	w.bytes(propHeader(name, "MapProperty", uint64(len(body.b))))
	w.bytes(preamble.b)
	w.bytes(body.b)
	return w.b
}

// --- composite blobs ---

func characterBlob(saveParamInner []byte, group [16]byte) []byte {
	obj := &gw{}
	obj.bytes(structProp("SaveParameter", "PalIndividualCharacterSaveParameter", saveParamInner))
	obj.fstr("None")
	w := &gw{}
	w.bytes(obj.b)
	w.u32(0) // trailing header word decodeCharacter skips
	w.guid(group)
	w.u32(0) // trailing footer word decodeCharacter skips
	return w.b
}

func characterEntry(playerUID, instanceID [16]byte, saveParamInner []byte, group [16]byte) []byte {
	key := &gw{}
	key.bytes(guidStructProp("PlayerUId", playerUID))
	key.bytes(guidStructProp("InstanceId", instanceID))
	key.fstr("None")
	val := &gw{}
	val.bytes(byteArrayProp("RawData", characterBlob(saveParamInner, group)))
	val.fstr("None")
	w := &gw{}
	w.bytes(key.b)
	w.bytes(val.b)
	return w.b
}

func playerSaveParam(nick string, level int32, hp int64, owner [16]byte) []byte {
	w := &gw{}
	w.bytes(boolProp("IsPlayer", true))
	w.bytes(strProp("NickName", "StrProperty", nick))
	w.bytes(intProp("Level", level))
	w.bytes(int64Prop("Exp", 1000))
	w.bytes(hpStructProp("Hp", hp))
	w.bytes(guidStructProp("OwnerPlayerUId", owner))
	w.fstr("None")
	return w.b
}

// containerIDStruct encodes a PalContainerId: a StructProperty (struct type
// PalContainerId) wrapping a Guid-typed "ID". Used by both a pal's SlotId and a
// player's party/box container ids.
func containerIDStruct(name string, container [16]byte) []byte {
	inner := &gw{}
	inner.bytes(guidStructProp("ID", container))
	inner.fstr("None")
	return structProp(name, "PalContainerId", inner.b)
}

// slotIDStruct encodes the pal SlotId field: a StructProperty (struct type
// PalCharacterSlotId) carrying a ContainerId (PalContainerId) and an IntProperty
// SlotIndex, matching the live 1.0 layout.
func slotIDStruct(container [16]byte, slotIndex int32) []byte {
	inner := &gw{}
	inner.bytes(containerIDStruct("ContainerId", container))
	inner.bytes(intProp("SlotIndex", slotIndex))
	inner.fstr("None")
	return structProp("SlotId", "PalCharacterSlotId", inner.b)
}

// palSaveParam builds a pal's SaveParameter list. When slot is non-nil it is
// appended (a SlotId struct); wild/NPC pals pass nil so no SlotId is present.
func palSaveParam(charID string, level uint8, hp int64, talentHP uint8, owner [16]byte, slot []byte) []byte {
	w := &gw{}
	w.bytes(strProp("CharacterID", "NameProperty", charID))
	w.bytes(byteProp("Level", level))
	w.bytes(hpStructProp("Hp", hp))
	w.bytes(byteProp("Talent_HP", talentHP))
	w.bytes(byteProp("Talent_Shot", 70))
	w.bytes(byteProp("Talent_Defense", 60))
	w.bytes(enumProp("Gender", "EPalGenderType", "EPalGenderType::Female"))
	w.bytes(stringArrayProp("PassiveSkillList", "NameProperty", "CraftSpeed_up2", "PAL_ALLAttack_up1"))
	w.bytes(stringArrayProp("EquipWaza", "EnumProperty", "EPalWazaID::AirCanon", "EPalWazaID::StoneShotgun"))
	w.bytes(guidStructProp("OwnerPlayerUId", owner))
	if slot != nil {
		w.bytes(slot)
	}
	w.fstr("None")
	return w.b
}

type guildMemberFix struct {
	uid  [16]byte
	name string
	last int64
}

// guildBlob encodes the EPalGroupType::Guild raw data in the proven retail
// Palworld 1.x layout: after the shared prefix come base ids, base-camp level,
// the guild name, the last-name-modifier uid, a 14-byte opaque block, the admin
// uid, the player list, and a 31-byte opaque trailer. The opaque blocks are the
// widths observed in live saves; their contents are irrelevant to the decoder.
func guildBlob(id [16]byte, guildName string, baseCampLevel int32, baseID [16]byte, admin [16]byte, members []guildMemberFix) []byte {
	w := &gw{}
	w.guid(id)
	w.fstr("") // group_name (empty; the guild name is carried separately)
	w.u32(0)   // individual_character_handle_ids count
	w.u8(0)    // org_type byte (present for org/guild/independent)
	w.u32(0)   // leading_bytes (4 reserved)
	w.u32(1)   // base_ids count
	w.guid(baseID)
	w.i32(0) // unknown_1
	w.i32(baseCampLevel)
	w.u32(0) // map_object_instance_ids_base_camp_points count
	w.fstr(guildName)
	w.guid(admin)               // last_guild_name_modifier_player_uid
	w.bytes(make([]byte, 14))   // unknown_2 (opaque)
	w.guid(admin)               // admin_player_uid
	w.u32(uint32(len(members))) // players count
	for _, m := range members {
		w.guid(m.uid)
		w.i64(m.last)
		w.fstr(m.name)
	}
	w.bytes(make([]byte, 31)) // trailing_bytes (opaque)
	return w.b
}

func guildEntry(key [16]byte, blob []byte) []byte {
	w := &gw{}
	w.guid(key) // GUID-keyed (GroupSaveDataMap.Key hint = "Guid")
	val := &gw{}
	val.bytes(enumProp("GroupType", "EPalGroupType", groupGuild))
	val.bytes(byteArrayProp("RawData", blob))
	val.fstr("None")
	w.bytes(val.b)
	return w.b
}

// build10World assembles a complete synthetic 1.0 GVAS file. When withDrift is
// set it injects an undecodable GUID-keyed map with no type hint, so the
// resilient per-property skip path is exercised.
func build10World(withDrift bool) []byte {
	ownerUID := testGUID(0x11)
	playerInstance := testGUID(0x12)
	pal1Instance := testGUID(0x21)
	pal2Instance := testGUID(0x22)
	pal3Instance := testGUID(0x23)
	palBoxContainer := testGUID(0x61)
	guildKey := testGUID(0x31)
	guildID := testGUID(0x32)
	adminUID := ownerUID
	memberUID := ownerUID
	group := guildID

	// worldSaveData inner property list.
	inner := &gw{}

	// A GUID-keyed map that only decodes because of the InvaderSaveData hint.
	invaderEntry := &gw{}
	invaderEntry.guid(testGUID(0x41)) // key (Guid hint)
	invaderEntry.fstr("None")         // value: empty struct property list
	inner.bytes(mapProp("InvaderSaveData", "StructProperty", "StructProperty", 1, invaderEntry.b))

	if withDrift {
		// No type hint exists for this path, so its struct key (a bare GUID) is
		// undecodable and the property must be skipped, not fatal.
		drift := &gw{}
		drift.guid(testGUID(0x51))
		drift.fstr("None")
		inner.bytes(mapProp("FutureUnknownMap", "StructProperty", "StructProperty", 1, drift.b))
	}

	// CharacterSaveParameterMap: 1 player + 3 pals. Grassmon and Rockmon share one
	// container at slots 0 and 1; Wildmon carries no SlotId (like a wild/NPC
	// character) so it must extract to an empty ContainerID and SlotIndex -1.
	chars := &gw{}
	chars.bytes(characterEntry(ownerUID, playerInstance,
		playerSaveParam("Ada", 5, 570000, ownerUID), group))
	chars.bytes(characterEntry(ownerUID, pal1Instance,
		palSaveParam("Grassmon", 12, 1500, 50, ownerUID, slotIDStruct(palBoxContainer, 0)), group))
	chars.bytes(characterEntry(ownerUID, pal2Instance,
		palSaveParam("Rockmon", 7, 900, 30, ownerUID, slotIDStruct(palBoxContainer, 1)), group))
	chars.bytes(characterEntry(ownerUID, pal3Instance,
		palSaveParam("Wildmon", 3, 300, 10, ownerUID, nil), group))
	inner.bytes(mapProp("CharacterSaveParameterMap", "StructProperty", "StructProperty", 4, chars.b))

	// GroupSaveDataMap: one guild in the retail 1.x layout with two members and a
	// base id. Member 0 is the player (uid == ownerUID) so player.GuildID links to
	// the guild; member 1 is a second guild member.
	guilds := &gw{}
	guilds.bytes(guildEntry(guildKey,
		guildBlob(guildID, "Ironpaw", 4, testGUID(0x33), adminUID, []guildMemberFix{
			{uid: memberUID, name: "Ada", last: 638000000000000000},
			{uid: testGUID(0x34), name: "Bo", last: 638000000000000001},
		})))
	inner.bytes(mapProp("GroupSaveDataMap", "StructProperty", "StructProperty", 1, guilds.b))

	inner.fstr("None")

	// worldSaveData StructProperty wrapping the inner list, then top-level None.
	top := &gw{}
	top.bytes(structProp("worldSaveData", "PalWorldSaveData", inner.b))
	top.fstr("None")

	// GVAS header.
	h := &gw{}
	h.u32(0x53415647)                                   // "GVAS"
	h.i32(3)                                            // save game version
	h.i32(0)                                            // package UE4
	h.i32(0)                                            // package UE5
	h.bytes([]byte{0x05, 0x00, 0x01, 0x00, 0x01, 0x00}) // engine 5.1.1
	h.u32(0)                                            // engine change
	h.fstr("++UE5+Release-5.1")                         // engine branch
	h.i32(3)                                            // custom version format
	h.u32(0)                                            // custom version count
	h.fstr("/Script/Pal.PalWorldSaveGame")              // class name

	out := &gw{}
	out.bytes(h.b)
	out.bytes(top.b)
	return out.b
}

func parseSyntheticWorld(t *testing.T, data []byte) *World {
	t.Helper()
	stats := newStats()
	g, err := parseGVAS(data, &stats)
	if err != nil {
		t.Fatalf("parseGVAS: %v", err)
	}
	// stats already carries any SkippedProperties/SkippedStructs from parseGVAS;
	// extractWorldSaveData accumulates further decode failures through &w.Stats.
	w := &World{Players: []Player{}, Pals: []Pal{}, Guilds: []Guild{}, Bases: []BaseCamp{}, Stats: stats}
	extractWorldSaveData(w, g.Properties)
	return w
}

func TestParseSynthetic1_0Fixture(t *testing.T) {
	path := filepath.Join("testdata", "World_1_0.gvas")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read fixture (regenerate with WRITE_FIXTURE=1): %v", err)
	}
	w := parseSyntheticWorld(t, data)

	if w.Stats.SkippedStructs != 0 || w.Stats.SkippedProperties != 0 {
		t.Fatalf("clean 1.0 fixture reported drift: structs=%d props=%d details=%v",
			w.Stats.SkippedStructs, w.Stats.SkippedProperties, w.Stats.SkippedDetails)
	}
	if len(w.Players) != 1 {
		t.Fatalf("players=%d, want 1", len(w.Players))
	}
	p := w.Players[0]
	if p.Nickname != "Ada" || p.Level != 5 || p.HP != 570 {
		t.Fatalf("player = %+v, want nick=Ada level=5 hp=570", p)
	}
	if len(w.Pals) != 3 {
		t.Fatalf("pals=%d, want 3", len(w.Pals))
	}
	var grass, rock, wild *Pal
	for i := range w.Pals {
		switch w.Pals[i].CharacterID {
		case "Grassmon":
			grass = &w.Pals[i]
		case "Rockmon":
			rock = &w.Pals[i]
		case "Wildmon":
			wild = &w.Pals[i]
		}
		if w.Pals[i].Level == 0 {
			t.Fatalf("pal %q has level 0 (ByteProperty level not decoded)", w.Pals[i].CharacterID)
		}
	}
	if grass == nil || rock == nil || wild == nil {
		t.Fatalf("missing pal: grass=%v rock=%v wild=%v", grass, rock, wild)
	}
	if grass.Level != 12 || grass.HP != 1.5 || grass.Talents["Talent_HP"] != 50 {
		t.Fatalf("grass pal = %+v, want level=12 hp=1.5 talentHP=50", grass)
	}
	if grass.Gender != "female" || !reflect.DeepEqual(grass.PassiveSkillIDs, []string{"CraftSpeed_up2", "PAL_ALLAttack_up1"}) || !reflect.DeepEqual(grass.EquippedSkillIDs, []string{"AirCanon", "StoneShotgun"}) {
		t.Fatalf("grass rich detail = %+v", grass)
	}
	// Container placement: Grassmon and Rockmon share one container at slots 0/1.
	const wantContainer = "61616161-6161-6161-6161-616161616161" // testGUID(0x61) normalized
	if grass.ContainerID != wantContainer || grass.SlotIndex != 0 {
		t.Fatalf("grass container = %q slot %d, want %q slot 0", grass.ContainerID, grass.SlotIndex, wantContainer)
	}
	if rock.ContainerID != wantContainer || rock.SlotIndex != 1 {
		t.Fatalf("rock container = %q slot %d, want %q slot 1", rock.ContainerID, rock.SlotIndex, wantContainer)
	}
	// Wildmon has no SlotId: empty container id and the -1 default slot.
	if wild.ContainerID != "" || wild.SlotIndex != -1 {
		t.Fatalf("wild container = %q slot %d, want empty/-1", wild.ContainerID, wild.SlotIndex)
	}
	if len(w.Guilds) != 1 {
		t.Fatalf("guilds=%d, want 1", len(w.Guilds))
	}
	gu := w.Guilds[0]
	if gu.Name != "Ironpaw" || gu.BaseCampLevel != 4 {
		t.Fatalf("guild = %+v, want name=Ironpaw baseCampLevel=4", gu)
	}
	if len(gu.BaseIDs) != 1 {
		t.Fatalf("guild BaseIDs=%v, want exactly 1", gu.BaseIDs)
	}
	if len(gu.Members) != 2 {
		t.Fatalf("guild members = %+v, want 2", gu.Members)
	}
	var ada *GuildMember
	for i := range gu.Members {
		if gu.Members[i].Name == "Ada" {
			ada = &gu.Members[i]
		}
	}
	if ada == nil {
		t.Fatalf("guild members = %+v, want one named Ada", gu.Members)
	}
	if len(gu.MemberUIDs) != 2 {
		t.Fatalf("guild MemberUIDs=%v, want 2", gu.MemberUIDs)
	}
	// End-to-end player->guild linking: the player's GuildID (from its character
	// group id) must match the guild that lists the player as a member.
	if p.GuildID != gu.ID {
		t.Fatalf("player GuildID=%q, want guild ID %q", p.GuildID, gu.ID)
	}
	if ada.UID != p.UID {
		t.Fatalf("guild member Ada uid=%q, want player uid %q", ada.UID, p.UID)
	}
}

func TestResilientSkipRecoversUnknownProperty(t *testing.T) {
	w := parseSyntheticWorld(t, build10World(true))

	if w.Stats.SkippedProperties != 1 {
		t.Fatalf("skippedProperties=%d, want 1 (details=%v)", w.Stats.SkippedProperties, w.Stats.SkippedDetails)
	}
	if len(w.Stats.SkippedDetails) != 1 ||
		!containsSub(w.Stats.SkippedDetails[0], "FutureUnknownMap") {
		t.Fatalf("skip details = %v, want the unknown map recorded", w.Stats.SkippedDetails)
	}
	// The rest of the world must still decode around the skipped property.
	if len(w.Players) != 1 || len(w.Pals) != 3 || len(w.Guilds) != 1 {
		t.Fatalf("post-skip world = %d players / %d pals / %d guilds, want 1/3/1",
			len(w.Players), len(w.Pals), len(w.Guilds))
	}
}

func TestDungeonRewardMapUsesGUIDStructKeyHint(t *testing.T) {
	entry := &gw{}
	entry.guid(testGUID(0x71))
	entry.bytes(intProp("RewardRank", 3))
	entry.fstr("None")
	body := &gw{}
	body.fstr("StructProperty")
	body.fstr("StructProperty")
	body.optGUIDAbsent()
	body.u32(0) // keys removed
	body.u32(1)
	body.bytes(entry.b)

	stats := newStats()
	got, err := readMap(newReaderWithStats(body.b, &stats),
		".worldSaveData.DungeonSaveData.DungeonSaveData.RewardSaveDataMap", &stats)
	if err != nil {
		t.Fatalf("readMap: %v", err)
	}
	if len(got) != 1 || got[0].Key != "71717171-7171-7171-7171-717171717171" {
		t.Fatalf("reward map = %#v, want one GUID-keyed entry", got)
	}
	value, ok := got[0].Value.(propertyMap)
	if !ok || firstInt(value, "RewardRank") != 3 {
		t.Fatalf("reward value = %#v, want RewardRank=3", got[0].Value)
	}
}

func containsSub(s, sub string) bool {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}

func TestWrite1_0Fixture(t *testing.T) {
	if os.Getenv("WRITE_FIXTURE") == "" {
		t.Skip("set WRITE_FIXTURE=1 to (re)generate testdata/World_1_0.gvas")
	}
	path := filepath.Join("testdata", "World_1_0.gvas")
	if err := os.WriteFile(path, build10World(false), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Logf("wrote %s", path)
}
