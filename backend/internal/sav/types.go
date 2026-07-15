package sav

// Options controls optional data loaded alongside Level.sav.
type Options struct {
	// PlayersDir overrides the sibling Players directory. An empty value uses
	// "Players" beside Level.sav. A missing directory is not an error.
	PlayersDir string
}

// World is the typed, read-only view of a Palworld world save.
type World struct {
	Meta    WorldMeta  `json:"meta"`
	Players []Player   `json:"players"`
	Pals    []Pal      `json:"pals"`
	Guilds  []Guild    `json:"guilds"`
	Bases   []BaseCamp `json:"bases"`
	Stats   ParseStats `json:"stats"`
}

// WorldMeta contains the stable metadata fields found in LevelMeta.sav.
type WorldMeta struct {
	WorldName string `json:"worldName,omitempty"`
	Day       int64  `json:"day,omitempty"`
	Timestamp int64  `json:"timestamp,omitempty"`
	Version   int64  `json:"version,omitempty"`
}

// Vector is an Unreal world-space position.
type Vector struct {
	X float64 `json:"x"`
	Y float64 `json:"y"`
	Z float64 `json:"z"`
}

// Player describes a player character.
type Player struct {
	UID        string  `json:"uid"`
	Nickname   string  `json:"nickname,omitempty"`
	Level      int32   `json:"level,omitempty"`
	Exp        int64   `json:"exp,omitempty"`
	HP         float64 `json:"hp,omitempty"`
	LastOnline int64   `json:"lastOnline,omitempty"`
	GuildID    string  `json:"guildId,omitempty"`
	Location   *Vector `json:"location,omitempty"`
	// OtomoContainerID is the GUID of the player's party (Otomo) container and
	// PalStorageContainerID is the GUID of the player's pal-box container. Both
	// are read from the per-player .sav file and are empty when that file is
	// missing or unreadable. The store derives party/box membership by matching
	// these against each pal's ContainerID; the raw GUIDs are not exposed by the
	// API.
	OtomoContainerID      string `json:"otomoContainerId,omitempty"`
	PalStorageContainerID string `json:"palStorageContainerId,omitempty"`
	// CaptureTotal is RecordData.TribeCaptureCount: the game's lifetime Pal
	// capture counter for this character. UniquePalsCaptured counts positive
	// entries in RecordData.PalCaptureCount; PaldeckUnlocked counts true entries
	// in RecordData.PaldeckUnlockFlag (seen/unlocked, not necessarily captured).
	// Pointers preserve the important distinction between a real zero and an
	// unavailable/undecodable RecordData block.
	CaptureTotal       *int64 `json:"captureTotal,omitempty"`
	UniquePalsCaptured *int   `json:"uniquePalsCaptured,omitempty"`
	PaldeckUnlocked    *int   `json:"paldeckUnlocked,omitempty"`
	// PalCaptureCounts and PaldeckUnlockFlags retain the authoritative
	// CharacterID-keyed RecordData maps for Paldeck progression. A nil map means
	// unavailable; an empty map is an authoritative zero-entry map. The
	// Truncated flags are defensive parser bounds and must be surfaced by any
	// consumer rather than silently treating a partial map as complete.
	PalCaptureCounts            map[string]int64 `json:"palCaptureCounts,omitempty"`
	PaldeckUnlockFlags          map[string]bool  `json:"paldeckUnlockFlags,omitempty"`
	PalCaptureCountsTruncated   bool             `json:"palCaptureCountsTruncated,omitempty"`
	PaldeckUnlockFlagsTruncated bool             `json:"paldeckUnlockFlagsTruncated,omitempty"`
}

// Pal describes a non-player character from CharacterSaveParameterMap.
type Pal struct {
	InstanceID       string         `json:"instanceId"`
	CharacterID      string         `json:"characterId,omitempty"`
	Level            int32          `json:"level,omitempty"`
	Exp              int64          `json:"exp,omitempty"`
	HP               float64        `json:"hp,omitempty"`
	OwnerUID         string         `json:"ownerUid,omitempty"`
	IsLucky          bool           `json:"isLucky,omitempty"`
	IsBoss           bool           `json:"isBoss,omitempty"`
	Talents          map[string]int `json:"talents,omitempty"`
	Gender           string         `json:"gender,omitempty"`
	PassiveSkillIDs  []string       `json:"passiveSkillIds,omitempty"`
	EquippedSkillIDs []string       `json:"equippedSkillIds,omitempty"`
	// ContainerID is the GUID of the CharacterContainer this pal occupies (party,
	// pal box, base, viewing cage, …) and SlotIndex is its slot within that
	// container. Read from the pal's SlotId struct. ContainerID is empty and
	// SlotIndex is -1 when the character carries no SlotId (e.g. wild or NPC
	// characters). The store derives inParty/boxPage/boxSlot from these.
	ContainerID string `json:"containerId,omitempty"`
	SlotIndex   int    `json:"slotIndex"`
	// BaseID is derived by joining ContainerID to BaseCamp.WorkerContainerID.
	// It is safe to expose through public APIs; the raw container GUID is not.
	BaseID string `json:"baseId,omitempty"`
}

// Guild describes one GroupSaveDataMap entry. GroupType is retained because
// Palworld stores organizations and independent guilds in the same map.
type Guild struct {
	ID            string        `json:"id"`
	Name          string        `json:"name,omitempty"`
	GroupType     string        `json:"groupType,omitempty"`
	AdminUID      string        `json:"adminUid,omitempty"`
	MemberUIDs    []string      `json:"memberUids"`
	Members       []GuildMember `json:"members,omitempty"`
	BaseIDs       []string      `json:"baseIds"`
	BaseCampLevel int32         `json:"baseCampLevel,omitempty"`
}

// GuildMember contains the player details embedded in guild raw data.
type GuildMember struct {
	UID        string `json:"uid"`
	Name       string `json:"name,omitempty"`
	LastOnline int64  `json:"lastOnline,omitempty"`
}

// BaseCamp describes base information available without decoding its RawData.
type BaseCamp struct {
	ID       string  `json:"id"`
	GuildID  string  `json:"guildId,omitempty"`
	Position *Vector `json:"position,omitempty"`
	// WorkerContainerID is decoded from WorkerDirector.RawData and retained only
	// for internal joins. Public projections expose BaseID, never this raw GUID.
	WorkerContainerID string `json:"workerContainerId,omitempty"`
}

// ParseStats reports data skipped or isolated while tolerantly decoding.
type ParseStats struct {
	SkippedProperties int            `json:"skippedProperties"`
	SkippedStructs    int            `json:"skippedStructs"`
	DecodeFailures    map[string]int `json:"decodeFailures"`
	// SkippedDetails records "path (type)" for the first few properties skipped
	// by the resilient decoder so unexpected format drift is diagnosable without
	// growing unbounded on a hostile input.
	SkippedDetails []string `json:"skippedDetails,omitempty"`
	propertyCount  uint64
	decodedNodes   uint64
	decodedBytes   uint64
}

// maxSkippedDetails caps the diagnostic detail slice so a pathological file
// cannot drive unbounded memory growth through the skip accounting.
const maxSkippedDetails = 20

// recordSkip increments the skipped-property counter and, while under the cap,
// appends a human-readable "path (type)" note for later diagnosis.
func (s *ParseStats) recordSkip(path, typ string) {
	if s == nil {
		return
	}
	s.SkippedProperties++
	if len(s.SkippedDetails) < maxSkippedDetails {
		s.SkippedDetails = append(s.SkippedDetails, path+" ("+typ+")")
	}
}

func newStats() ParseStats { return ParseStats{DecodeFailures: make(map[string]int)} }
