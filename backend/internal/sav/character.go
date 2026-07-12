package sav

import (
	"fmt"
	"strings"
)

type characterRaw struct {
	Object  propertyMap
	GroupID string
}

func decodeCharacter(raw []byte, stats *ParseStats) (characterRaw, error) {
	r := newReaderWithStats(raw, stats)
	obj, err := readProperties(r, ".worldSaveData.CharacterSaveParameterMap.Value.RawData", stats)
	if err != nil {
		return characterRaw{}, err
	}
	if err = r.skip(4); err != nil {
		return characterRaw{}, err
	}
	group, err := readGUID(r)
	if err != nil {
		return characterRaw{}, err
	}
	if err = r.skip(4); err != nil {
		return characterRaw{}, err
	}
	if r.remaining() != 0 {
		return characterRaw{}, fmt.Errorf("%d trailing character bytes", r.remaining())
	}
	return characterRaw{Object: obj, GroupID: group}, nil
}

func characterFromEntry(e mapEntry, stats *ParseStats) (*Player, *Pal, error) {
	value, ok := asProperties(e.Value)
	if !ok {
		return nil, nil, fmt.Errorf("character value is not a struct")
	}
	rawProp := value["RawData"]
	if rawProp == nil {
		return nil, nil, fmt.Errorf("character has no RawData")
	}
	raw, ok := rawProp.Value.([]byte)
	if !ok {
		return nil, nil, fmt.Errorf("character RawData is %T", rawProp.Value)
	}
	c, err := decodeCharacter(raw, stats)
	if err != nil {
		return nil, nil, err
	}
	sp := c.Object
	if nested, ok := propertyProperties(sp, "SaveParameter"); ok {
		sp = nested
	}
	key, _ := asProperties(e.Key)
	uid := firstGUID(key, "PlayerUId", "PlayerUID", "PlayerUid")
	instance := firstGUID(key, "InstanceId", "InstanceID")
	if instance == "" {
		instance = firstString(sp, "InstanceId", "InstanceID")
	}
	if firstBool(sp, "IsPlayer") {
		p := &Player{UID: uid, Nickname: firstString(sp, "NickName", "Nickname"), Level: int32(firstInt(sp, "Level")), Exp: firstInt(sp, "Exp"), HP: fixedPointDisplay(firstNumber(sp, "HP", "Hp")), GuildID: c.GroupID}
		if p.UID == "" {
			p.UID = firstString(sp, "PlayerUId", "PlayerUID")
		}
		if loc, ok := firstVector(sp, "Location", "Position"); ok {
			p.Location = &loc
		}
		return p, nil, nil
	}
	pal := &Pal{InstanceID: instance, CharacterID: firstString(sp, "CharacterID", "CharacterId", "character_id"), Level: int32(firstInt(sp, "Level")), Exp: firstInt(sp, "Exp"), HP: fixedPointDisplay(firstNumber(sp, "HP", "Hp")), OwnerUID: firstString(sp, "OwnerPlayerUId", "OwnerPlayerUID"), IsLucky: firstBool(sp, "IsRarePal", "IsLucky"), IsBoss: firstBool(sp, "IsBoss"), Talents: map[string]int{}, SlotIndex: -1}
	pal.Gender = normalizedGender(firstString(sp, "Gender"))
	pal.PassiveSkillIDs = propertyStringArray(sp, "PassiveSkillList")
	pal.EquippedSkillIDs = normalizedEnumArray(propertyStringArray(sp, "EquipWaza"), "EPalWazaID::")
	// Container placement: the SlotId struct (PalCharacterSlotId) carries the
	// pal's ContainerId (a PalContainerId wrapping a Guid "ID") and SlotIndex.
	// Wild and NPC characters lack SlotId entirely, so a missing struct is not an
	// error: ContainerID stays empty and SlotIndex stays -1.
	if slot, ok := propertyProperties(sp, "SlotId"); ok {
		pal.ContainerID = containerGUID(slot, "ContainerId")
		if idx, ok := propertyInt(slot, "SlotIndex"); ok {
			pal.SlotIndex = int(idx)
		}
	}
	for _, name := range []string{"Talent_HP", "Talent_Melee", "Talent_Shot", "Talent_Defense"} {
		if v, ok := propertyInt(sp, name); ok {
			pal.Talents[name] = int(v)
		}
	}
	if len(pal.Talents) == 0 {
		pal.Talents = nil
	}
	return nil, pal, nil
}

func fixedPointDisplay(value float64) float64 {
	if value == 0 {
		return 0
	}
	return value / 1000
}

func normalizedEnumSuffix(value string) string {
	if index := strings.LastIndex(value, "::"); index >= 0 {
		value = value[index+2:]
	}
	return strings.ToLower(strings.TrimSpace(value))
}

func normalizedGender(value string) string {
	if value == "" {
		return ""
	}
	switch normalizedEnumSuffix(value) {
	case "male":
		return "male"
	case "female":
		return "female"
	default:
		return "unknown"
	}
}

func normalizedEnumArray(values []string, prefix string) []string {
	for index, value := range values {
		values[index] = strings.TrimPrefix(value, prefix)
	}
	return values
}
