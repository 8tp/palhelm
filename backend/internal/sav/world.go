package sav

import (
	"errors"
	"math"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

// ParseLevel parses Level.sav and optional player saves into a typed World.
func ParseLevel(path string, opts Options) (*World, error) {
	raw, err := readSave(path)
	if err != nil {
		return nil, err
	}
	stats := newStats()
	g, err := parseGVAS(raw, &stats)
	if err != nil {
		return nil, err
	}
	w := &World{Players: []Player{}, Pals: []Pal{}, Guilds: []Guild{}, Bases: []BaseCamp{}, Stats: stats}
	w.Meta = metaFromProperties(g.Properties)
	metaPath := filepath.Join(filepath.Dir(path), "LevelMeta.sav")
	if !strings.EqualFold(filepath.Base(path), "LevelMeta.sav") {
		if meta, metaErr := ParseLevelMeta(metaPath); metaErr == nil {
			w.Meta = *meta
		} else if !errors.Is(metaErr, os.ErrNotExist) {
			w.Stats.DecodeFailures["meta"]++
		}
	}
	extractWorldSaveData(w, g.Properties)
	loadPlayerDirectory(w, path, opts)
	return w, nil
}

// extractWorldSaveData derives players, pals, guilds and bases from the parsed
// worldSaveData property tree. Split out from ParseLevel so tests can exercise
// it against synthetic GVAS bytes without the Oodle container layer.
func extractWorldSaveData(w *World, props propertyMap) {
	root, ok := propertyProperties(props, "worldSaveData")
	if !ok {
		return
	}
	if p := root["GroupSaveDataMap"]; p != nil {
		entries, ok := p.Value.([]mapEntry)
		if !ok {
			w.Stats.DecodeFailures["guilds"]++
		} else {
			for _, e := range entries {
				decodeGuildEntry(w, e)
			}
		}
	}
	if p := root["CharacterSaveParameterMap"]; p != nil {
		entries, ok := p.Value.([]mapEntry)
		if !ok {
			w.Stats.DecodeFailures["characters"]++
		} else {
			for _, e := range entries {
				pl, pal, e2 := characterFromEntry(e, &w.Stats)
				if e2 != nil {
					w.Stats.DecodeFailures["characters"]++
					continue
				}
				if pl != nil {
					w.Players = append(w.Players, *pl)
				}
				if pal != nil {
					w.Pals = append(w.Pals, *pal)
				}
			}
		}
	}
	if p := root["BaseCampSaveData"]; p != nil {
		if entries, ok := p.Value.([]mapEntry); ok {
			for _, e := range entries {
				w.Bases = append(w.Bases, baseFromEntry(e, &w.Stats))
			}
		} else {
			w.Stats.DecodeFailures["bases"]++
		}
	}
	assignBaseGuilds(w)
	assignBaseWorkers(w)
}

// ParseLevelMeta parses a LevelMeta.sav file.
func ParseLevelMeta(path string) (*WorldMeta, error) {
	raw, err := readSave(path)
	if err != nil {
		return nil, err
	}
	stats := newStats()
	g, err := parseGVAS(raw, &stats)
	if err != nil {
		return nil, err
	}
	m := metaFromProperties(g.Properties)
	return &m, nil
}

func decodeGuildEntry(w *World, e mapEntry) {
	v, ok := asProperties(e.Value)
	if !ok {
		w.Stats.DecodeFailures["guilds"]++
		return
	}
	t := firstString(v, "GroupType")
	rawProp := v["RawData"]
	if rawProp == nil {
		w.Stats.DecodeFailures["guilds"]++
		return
	}
	raw, ok := rawProp.Value.([]byte)
	if !ok {
		w.Stats.DecodeFailures["guilds"]++
		return
	}
	g, err := decodeGroup(raw, t, &w.Stats)
	if err != nil {
		w.Stats.DecodeFailures["guilds"]++
		return
	}
	w.Guilds = append(w.Guilds, g)
}

func baseFromEntry(e mapEntry, stats *ParseStats) BaseCamp {
	b := BaseCamp{}
	if id, ok := e.Key.(string); ok {
		b.ID = id
	}
	if v, ok := asProperties(e.Value); ok {
		b.GuildID = firstString(v, "GroupIdBelongTo", "GroupID", "GuildId", "GuildID")
		// The base's name and world transform live inside PalBaseCampSaveData.RawData;
		// neither is exposed as an ordinary property, so decode them from the raw
		// bytes. A pre-1.0 save that instead carries a plain vector property is
		// still honored as a fallback.
		if raw, ok := propertyBytes(v, "RawData"); ok {
			name, loc, ok := decodeBaseRaw(raw, b.ID)
			if ok {
				b.Name = normalizeBaseName(name)
			}
			if loc != nil {
				b.Position = loc
			} else {
				stats.recordSkip("worldSaveData.BaseCampSaveData.Value.RawData.transform", "tolerated")
			}
		}
		if b.Position == nil {
			if p, ok := firstVector(v, "Position", "Location"); ok {
				b.Position = &p
			}
		}
		if worker, ok := propertyProperties(v, "WorkerDirector"); ok {
			if raw, ok := propertyBytes(worker, "RawData"); ok {
				b.WorkerContainerID = workerContainerID(raw, b.ID)
			}
		}
	}
	return b
}

// decodeBaseRaw decodes the name and world-space translation of a base camp
// from PalBaseCampSaveData.RawData. The proven retail 1.x prefix is:
//
//	id                 GUID (16 bytes) — must match the map key
//	name               fstring (UTF-16 in retail saves)
//	state              1 byte (EPalBaseCampWorkerStateType)
//	transform          FTransform: rotation quaternion (4 f64) + translation
//	                   (3 f64) + scale3d (3 f64); modern 1.x saves store each
//	                   component as f64
//	area_range         f32
//	group_id_belong_to GUID
//	... (worker/module data this decoder ignores)
//
// ok reports whether the structural prefix (GUID + name) decoded; the raw name
// is returned as stored (normalizeBaseName decides what is displayable). The
// location is nil — served as null, never a misleading (0,0) — on any
// structural drift past the name: a short buffer, a read error, or a
// non-finite/implausibly large component. A GUID that does not match the map
// key fails the whole decode. Verified against a live 1.0 world: all 20 bases
// decoded to within <1 cm of the guild's in-game PalBox.
func decodeBaseRaw(raw []byte, baseID string) (name string, loc *Vector, ok bool) {
	r := newReader(raw)
	embedded, err := readGUID(r)
	if err != nil || (baseID != "" && !strings.EqualFold(embedded, baseID)) {
		return "", nil, false
	}
	if name, err = r.fstring(); err != nil {
		return "", nil, false
	}
	// state byte, then the rotation quaternion (4 f64) we do not need.
	if err = r.skip(1 + 4*8); err != nil {
		return name, nil, true
	}
	x, err := r.f64()
	if err != nil {
		return name, nil, true
	}
	y, err := r.f64()
	if err != nil {
		return name, nil, true
	}
	z, err := r.f64()
	if err != nil {
		return name, nil, true
	}
	if !finiteBaseCoord(x) || !finiteBaseCoord(y) || !finiteBaseCoord(z) {
		return name, nil, true
	}
	return name, &Vector{X: x, Y: y, Z: z}, true
}

// baseNamePlaceholderPrefix is the engine-side default written into every base
// camp the player never renamed: "新規生成拠点テンプレート名<n>(仮)" — literally
// "newly generated base template name <n> (tentative)". Palworld writes this
// placeholder regardless of the server's locale (the in-game UI substitutes a
// localized label), so it is not a player-chosen name and must not be shown.
const baseNamePlaceholderPrefix = "新規生成拠点テンプレート名"

// normalizeBaseName maps a raw stored base name to its displayable form: empty
// when the base is effectively unnamed. Whitespace-only names and the engine's
// untranslated placeholder template collapse to "" so every downstream surface
// can apply one rule — empty means absent means null, never a synthetic value.
func normalizeBaseName(name string) string {
	name = strings.TrimSpace(name)
	if strings.HasPrefix(name, baseNamePlaceholderPrefix) {
		return ""
	}
	return name
}

// finiteBaseCoord rejects NaN, infinities, and coordinates far outside any
// plausible Palworld world extent (~±700 km in cm), which would indicate the
// transform read landed on misaligned bytes rather than a real translation.
func finiteBaseCoord(v float64) bool {
	const maxWorldCoord = 1e10
	return !math.IsNaN(v) && !math.IsInf(v, 0) && v >= -maxWorldCoord && v <= maxWorldCoord
}

// workerContainerID decodes PalBaseCampSaveData_WorkerDirector.RawData. The
// stable prefix is [base GUID][FTransform: 10 float64][order byte][battle byte]
// [worker-container GUID]. Palworld 1.0 appends four version bytes, so trailing
// data is tolerated while every structural field before the container remains
// fixed-width and the embedded base GUID must match the map key.
func workerContainerID(raw []byte, baseID string) string {
	const containerOffset = 16 + (10 * 8) + 2
	if len(raw) < containerOffset+16 {
		return ""
	}
	reader := newReader(raw)
	embeddedBase, err := readGUID(reader)
	if err != nil || (baseID != "" && !strings.EqualFold(embeddedBase, baseID)) {
		return ""
	}
	if err = reader.skip((10 * 8) + 2); err != nil {
		return ""
	}
	containerID, err := readGUID(reader)
	if err != nil {
		return ""
	}
	return containerID
}

func propertyBytes(p propertyMap, name string) ([]byte, bool) {
	q := p[name]
	if q == nil {
		return nil, false
	}
	value, ok := q.Value.([]byte)
	return value, ok
}

func assignBaseWorkers(w *World) {
	byContainer := make(map[string]string, len(w.Bases))
	for _, base := range w.Bases {
		if base.WorkerContainerID != "" {
			byContainer[strings.ToLower(base.WorkerContainerID)] = base.ID
		}
	}
	for i := range w.Pals {
		w.Pals[i].BaseID = byContainer[strings.ToLower(w.Pals[i].ContainerID)]
	}
}

// assignBaseGuilds joins the decoded guild BaseIDs to BaseCampSaveData keys.
// Palworld 1.0 does not expose GroupIdBelongTo as an ordinary property on every
// base entry, while the guild's exact base-id array is structural and proven.
func assignBaseGuilds(w *World) {
	owners := make(map[string]string, len(w.Bases))
	ambiguous := make(map[string]bool)
	for _, guild := range w.Guilds {
		for _, rawBaseID := range guild.BaseIDs {
			baseID := strings.ToLower(rawBaseID)
			if current, exists := owners[baseID]; exists && !strings.EqualFold(current, guild.ID) {
				ambiguous[baseID] = true
				continue
			}
			owners[baseID] = guild.ID
		}
	}
	for i := range w.Bases {
		key := strings.ToLower(w.Bases[i].ID)
		if w.Bases[i].GuildID == "" && !ambiguous[key] {
			w.Bases[i].GuildID = owners[key]
		}
	}
}

func metaFromProperties(p propertyMap) WorldMeta {
	m := WorldMeta{WorldName: firstStringRecursive(p, "WorldName", "world_name", "Name")}
	m.Day = firstIntRecursive(p, "Day", "GameDay", "InGameDay")
	m.Timestamp = firstIntRecursive(p, "Timestamp")
	if q := p["Timestamp"]; q != nil {
		if s, ok := q.Value.(structData); ok {
			if v, ok := s.Value.(uint64); ok {
				m.Timestamp = int64(v)
			}
		}
	}
	m.Version = firstIntRecursive(p, "Version")
	return m
}

func loadPlayerDirectory(w *World, levelPath string, opts Options) {
	dir := opts.PlayersDir
	if dir == "" {
		dir = filepath.Join(filepath.Dir(levelPath), "Players")
	}
	entries, err := os.ReadDir(dir)
	if os.IsNotExist(err) {
		return
	}
	if err != nil {
		w.Stats.DecodeFailures["players"]++
		return
	}
	for _, de := range entries {
		if de.IsDir() || !strings.HasSuffix(strings.ToLower(de.Name()), ".sav") {
			continue
		}
		raw, e := readSave(filepath.Join(dir, de.Name()))
		if e != nil {
			w.Stats.DecodeFailures["players"]++
			continue
		}
		g, e := parseGVAS(raw, &w.Stats)
		if e != nil {
			w.Stats.DecodeFailures["players"]++
			continue
		}
		data := g.Properties
		if nested, ok := propertyProperties(data, "SaveData"); ok {
			data = nested
		}
		uid := strings.TrimSuffix(de.Name(), filepath.Ext(de.Name()))
		p := Player{UID: uid, Nickname: firstStringRecursive(data, "NickName", "Nickname"), Level: int32(firstIntRecursive(data, "Level"))}
		decodePlayerProgress(data, &p)
		if loc, ok := firstVectorRecursive(data, "Location", "Position"); ok {
			p.Location = &loc
		}
		// Party and pal-box container GUIDs live at the SaveData top level as
		// PalContainerId structs wrapping a Guid "ID". Absent ids leave the
		// fields empty, matching how the rest of this loader degrades.
		p.OtomoContainerID = containerGUID(data, "OtomoCharacterContainerId")
		p.PalStorageContainerID = containerGUID(data, "PalStorageContainerId")
		mergePlayer(w, p)
	}
}

func mergePlayer(w *World, p Player) {
	for i := range w.Players {
		if strings.EqualFold(w.Players[i].UID, p.UID) {
			if p.Nickname != "" {
				w.Players[i].Nickname = p.Nickname
			}
			if p.Level != 0 {
				w.Players[i].Level = p.Level
			}
			if p.Location != nil {
				w.Players[i].Location = p.Location
			}
			if p.OtomoContainerID != "" {
				w.Players[i].OtomoContainerID = p.OtomoContainerID
			}
			if p.PalStorageContainerID != "" {
				w.Players[i].PalStorageContainerID = p.PalStorageContainerID
			}
			if p.CaptureTotal != nil {
				w.Players[i].CaptureTotal = p.CaptureTotal
			}
			if p.UniquePalsCaptured != nil {
				w.Players[i].UniquePalsCaptured = p.UniquePalsCaptured
			}
			if p.PaldeckUnlocked != nil {
				w.Players[i].PaldeckUnlocked = p.PaldeckUnlocked
			}
			if p.PalCaptureCounts != nil {
				w.Players[i].PalCaptureCounts = p.PalCaptureCounts
				w.Players[i].PalCaptureCountsTruncated = p.PalCaptureCountsTruncated
			}
			if p.PaldeckUnlockFlags != nil {
				w.Players[i].PaldeckUnlockFlags = p.PaldeckUnlockFlags
				w.Players[i].PaldeckUnlockFlagsTruncated = p.PaldeckUnlockFlagsTruncated
			}
			return
		}
	}
	w.Players = append(w.Players, p)
}

// decodePlayerProgress reads only documented, typed fields from the per-player
// SaveData.RecordData struct. It deliberately does not estimate lifetime catches
// from the current world roster. Missing maps stay nil so API consumers can say
// "unavailable" instead of presenting a misleading zero.
func decodePlayerProgress(data propertyMap, p *Player) {
	const maxPaldeckEntries = 2048
	record, ok := propertyProperties(data, "RecordData")
	if !ok {
		return
	}
	if v, ok := propertyInt(record, "TribeCaptureCount"); ok && v >= 0 {
		p.CaptureTotal = int64Ptr(v)
	}
	if entries, ok := propertyMapEntries(record, "PalCaptureCount"); ok {
		count := 0
		p.PalCaptureCounts = make(map[string]int64, min(len(entries), maxPaldeckEntries))
		for _, entry := range entries {
			if v, ok := numericValue(entry.Value); ok && v > 0 {
				count++
			}
			key, validKey := entry.Key.(string)
			value, validValue := numericValue(entry.Value)
			key = strings.TrimSpace(key)
			if !validKey || key == "" || !validValue || value < 0 {
				continue
			}
			if _, exists := p.PalCaptureCounts[key]; !exists && len(p.PalCaptureCounts) == maxPaldeckEntries {
				p.PalCaptureCountsTruncated = true
				continue
			}
			p.PalCaptureCounts[key] = value
		}
		p.UniquePalsCaptured = intPtr(count)
	}
	if entries, ok := propertyMapEntries(record, "PaldeckUnlockFlag"); ok {
		count := 0
		p.PaldeckUnlockFlags = make(map[string]bool, min(len(entries), maxPaldeckEntries))
		for _, entry := range entries {
			if v, ok := entry.Value.(bool); ok && v {
				count++
			}
			key, validKey := entry.Key.(string)
			value, validValue := entry.Value.(bool)
			key = strings.TrimSpace(key)
			if !validKey || key == "" || !validValue {
				continue
			}
			if _, exists := p.PaldeckUnlockFlags[key]; !exists && len(p.PaldeckUnlockFlags) == maxPaldeckEntries {
				p.PaldeckUnlockFlagsTruncated = true
				continue
			}
			p.PaldeckUnlockFlags[key] = value
		}
		p.PaldeckUnlocked = intPtr(count)
	}
}

func propertyMapEntries(p propertyMap, name string) ([]mapEntry, bool) {
	q := p[name]
	if q == nil {
		return nil, false
	}
	v, ok := q.Value.([]mapEntry)
	return v, ok
}

func numericValue(v any) (int64, bool) {
	switch n := v.(type) {
	case int32:
		return int64(n), true
	case int64:
		return n, true
	case uint32:
		return int64(n), true
	case uint64:
		if n <= uint64(^uint64(0)>>1) {
			return int64(n), true
		}
	}
	return 0, false
}

func intPtr(v int) *int       { return &v }
func int64Ptr(v int64) *int64 { return &v }

func asProperties(v any) (propertyMap, bool) {
	switch x := v.(type) {
	case propertyMap:
		return x, true
	case structData:
		p, ok := x.Value.(propertyMap)
		return p, ok
	default:
		return nil, false
	}
}
func propertyProperties(p propertyMap, name string) (propertyMap, bool) {
	q := p[name]
	if q == nil {
		return nil, false
	}
	return asProperties(q.Value)
}
func propertyInt(p propertyMap, name string) (int64, bool) {
	q := p[name]
	if q == nil {
		return 0, false
	}
	switch v := q.Value.(type) {
	case int32:
		return int64(v), true
	case int64:
		return v, true
	case uint16:
		return int64(v), true
	case uint32:
		return int64(v), true
	case uint64:
		return int64(v), true
	case enumData:
		// Palworld 1.0 serializes small integers such as pal Level and the
		// Talent_* stats as a ByteProperty, which decodes to enumData{Type:"None"}
		// carrying the numeric byte as a string. A named enum (e.g. Gender) is not
		// numeric and simply fails the parse, yielding (0,false).
		if n, err := strconv.ParseInt(v.Value, 10, 64); err == nil {
			return n, true
		}
	}
	return 0, false
}
func firstInt(p propertyMap, names ...string) int64 {
	for _, n := range names {
		if v, ok := propertyInt(p, n); ok {
			return v
		}
	}
	return 0
}
func firstNumber(p propertyMap, names ...string) float64 {
	for _, n := range names {
		q := p[n]
		if q == nil {
			continue
		}
		switch v := q.Value.(type) {
		case int32:
			return float64(v)
		case int64:
			return float64(v)
		case uint32:
			return float64(v)
		case uint64:
			return float64(v)
		case float32:
			return float64(v)
		case float64:
			return v
		case structData:
			if nested, ok := v.Value.(propertyMap); ok {
				return firstNumber(nested, "Value", "Current", "HP")
			}
		}
	}
	return 0
}
func firstBool(p propertyMap, names ...string) bool {
	for _, n := range names {
		if q := p[n]; q != nil {
			if v, ok := q.Value.(bool); ok {
				return v
			}
		}
	}
	return false
}
func firstString(p propertyMap, names ...string) string {
	for _, n := range names {
		if q := p[n]; q != nil {
			switch v := q.Value.(type) {
			case string:
				return v
			case enumData:
				return v.Value
			case structData:
				if s, ok := v.Value.(string); ok {
					return s
				}
			}
		}
	}
	return ""
}

func propertyStringArray(p propertyMap, name string) []string {
	q := p[name]
	if q == nil {
		return nil
	}
	values, ok := q.Value.([]any)
	if !ok {
		return nil
	}
	out := make([]string, 0, len(values))
	for _, value := range values {
		switch typed := value.(type) {
		case string:
			if typed != "" {
				out = append(out, typed)
			}
		case enumData:
			if typed.Value != "" {
				out = append(out, typed.Value)
			}
		}
	}
	if len(out) == 0 {
		return nil
	}
	return out
}
func firstGUID(p propertyMap, names ...string) string { return firstString(p, names...) }

// containerGUID reads a PalContainerId-shaped property: a StructProperty wrapping
// a Guid-typed "ID". Both a pal's SlotId.ContainerId and a player's party/box
// container ids use this shape. Returns the normalized GUID, or "" when the named
// property is absent or not a struct.
func containerGUID(p propertyMap, name string) string {
	inner, ok := propertyProperties(p, name)
	if !ok {
		return ""
	}
	return firstString(inner, "ID")
}
func firstVector(p propertyMap, names ...string) (Vector, bool) {
	for _, n := range names {
		if q := p[n]; q != nil {
			if s, ok := q.Value.(structData); ok {
				if v, ok := s.Value.(Vector); ok {
					return v, true
				}
			}
		}
	}
	return Vector{}, false
}

func firstStringRecursive(p propertyMap, names ...string) string {
	if v := firstString(p, names...); v != "" {
		return v
	}
	for _, q := range p {
		if n, ok := asProperties(q.Value); ok {
			if v := firstStringRecursive(n, names...); v != "" {
				return v
			}
		}
	}
	return ""
}
func firstIntRecursive(p propertyMap, names ...string) int64 {
	if v := firstInt(p, names...); v != 0 {
		return v
	}
	for _, q := range p {
		if n, ok := asProperties(q.Value); ok {
			if v := firstIntRecursive(n, names...); v != 0 {
				return v
			}
		}
	}
	return 0
}
func firstVectorRecursive(p propertyMap, names ...string) (Vector, bool) {
	if v, ok := firstVector(p, names...); ok {
		return v, true
	}
	for _, q := range p {
		if n, ok := asProperties(q.Value); ok {
			if v, ok := firstVectorRecursive(n, names...); ok {
				return v, true
			}
		}
	}
	return Vector{}, false
}
