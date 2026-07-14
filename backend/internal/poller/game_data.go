package poller

import (
	"context"
	"errors"
	"strings"
	"time"
	"unicode"

	"github.com/8tp/palhelm/internal/palworld"
)

const maxCachedLiveActors = 2_048

// ErrGameDataCollapsed indicates that one successful response was drastically smaller than
// the last accepted generation. A second consecutive collapsed response is required before it
// replaces last-good data.
var ErrGameDataCollapsed = errors.New("game-data actor population collapsed; awaiting confirmation")

// GameDataSource is the narrow upstream seam used by the optional live-world poller.
type GameDataSource interface {
	GameData(context.Context) (palworld.GameDataSnapshot, error)
}

// GameDataState describes capability and freshness without turning an optional endpoint into
// global REST health. Unsupported and unauthorized are terminal until process restart.
type GameDataState string

const (
	GameDataDisabled     GameDataState = "disabled"
	GameDataPending      GameDataState = "pending"
	GameDataReady        GameDataState = "ready"
	GameDataStale        GameDataState = "stale"
	GameDataUnsupported  GameDataState = "unsupported"
	GameDataUnauthorized GameDataState = "unauthorized"
	GameDataUnavailable  GameDataState = "unavailable"
)

// GameDataCounts contains aggregate actor counts computed once per accepted upstream poll.
type GameDataCounts struct {
	Players   int `json:"players"`
	PartyPals int `json:"partyPals"`
	BasePals  int `json:"basePals"`
	WildPals  int `json:"wildPals"`
	NPCs      int `json:"npcs"`
	PalBoxes  int `json:"palBoxes"`
	Unknown   int `json:"unknown"`
}

func (c GameDataCounts) total() int {
	return c.Players + c.PartyPals + c.BasePals + c.WildPals + c.NPCs + c.PalBoxes + c.Unknown
}

// LiveWorldActor is the bounded, sanitized projection cached for the authenticated panel. Raw
// actor/trainer IDs, class/action strings, rotations, Stage, IP, and userid never enter it.
type LiveWorldActor struct {
	Kind        string   `json:"kind"`
	CharacterID string   `json:"characterId,omitempty"`
	IsBoss      bool     `json:"isBoss,omitempty"`
	Name        string   `json:"name,omitempty"`
	TrainerName string   `json:"trainerName,omitempty"`
	GuildName   string   `json:"guildName,omitempty"`
	Level       int      `json:"level,omitempty"`
	HPPercent   *float64 `json:"hpPercent,omitempty"`
	Active      *bool    `json:"active,omitempty"`
	Activity    string   `json:"activity"`
	X           float64  `json:"-"`
	Y           float64  `json:"-"`
	Z           float64  `json:"-"`
}

// CachedGameData is a bounded, memory-only panel view of the most recent acceptable snapshot.
type CachedGameData struct {
	State         GameDataState
	CapturedAt    time.Time
	LastAttemptAt time.Time
	SourceTime    string
	FPS           float64
	FPSAvg        float64
	Counts        GameDataCounts
	Actors        []LiveWorldActor
	Truncated     bool
}

// GameDataSummary is the O(1) aggregate view used by bearer-token requests.
type GameDataSummary struct {
	State         GameDataState
	CapturedAt    time.Time
	LastAttemptAt time.Time
	FPS           float64
	FPSAvg        float64
	Counts        GameDataCounts
}

// ConfigureGameData enables the optional fourth poller. It is intentionally a separate call
// so existing unit-test constructors and installs remain source-compatible and disabled.
func (s *Service) ConfigureGameData(enabled bool, every time.Duration) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.gameDataEnabled = enabled
	s.gameDataEvery = every
	s.gameDataStaleAfter = 3 * every
	if s.gameDataStaleAfter < 2*time.Minute {
		s.gameDataStaleAfter = 2 * time.Minute
	}
	if s.gameDataStaleAfter > 10*time.Minute {
		s.gameDataStaleAfter = 10 * time.Minute
	}
	s.clearGameDataLocked()
	if enabled {
		s.gameDataState = GameDataPending
	} else {
		s.gameDataState = GameDataDisabled
	}
}

func (s *Service) gameDataLoop(ctx context.Context) {
	backoff := s.gameDataEvery
	if backoff < 15*time.Second {
		backoff = 15 * time.Second
	}
	for {
		err := s.pollGameData(ctx)
		if ctx.Err() != nil {
			return
		}
		state := s.gameDataStateSnapshot()
		if state == GameDataUnsupported || state == GameDataUnauthorized {
			return
		}
		if err == nil || errors.Is(err, ErrGameDataCollapsed) {
			backoff = s.gameDataEvery
			if backoff < 15*time.Second {
				backoff = 15 * time.Second
			}
		} else {
			backoff *= 2
			if backoff > 10*time.Minute {
				backoff = 10 * time.Minute
			}
		}
		timer := time.NewTimer(backoff)
		select {
		case <-ctx.Done():
			if !timer.Stop() {
				<-timer.C
			}
			return
		case <-timer.C:
		}
	}
}

func (s *Service) pollGameData(ctx context.Context) error {
	if !s.gameDataInFlight.CompareAndSwap(false, true) {
		return nil
	}
	defer s.gameDataInFlight.Store(false)
	snapshot, err := s.gameDataSource.GameData(ctx)
	completedAt := time.Now().UTC()
	s.mu.Lock()
	defer s.mu.Unlock()
	s.gameDataLastAttempt = completedAt
	if err == nil {
		counts, actors, truncated := projectGameData(snapshot.ActorData)
		previousTotal := s.gameDataCounts.total()
		collapsed := !s.gameDataCapturedAt.IsZero() && previousTotal >= 10 && counts.total()*4 < previousTotal
		if collapsed && !s.gameDataCollapsePending {
			s.gameDataCollapsePending = true
			s.gameDataState = GameDataStale
			return ErrGameDataCollapsed
		}
		s.gameDataCollapsePending = false
		s.gameDataSourceTime = boundedClean(snapshot.Time, 32)
		s.gameDataFPS, s.gameDataFPSAvg = snapshot.FPS, snapshot.AverageFPS
		s.gameDataCounts, s.gameDataActors, s.gameDataTruncated = counts, actors, truncated
		s.gameDataCapturedAt = completedAt
		s.gameDataState = GameDataReady
		return nil
	}
	s.gameDataCollapsePending = false
	switch {
	case palworld.IsKind(err, palworld.ErrorUnsupported):
		s.clearGameDataLocked()
		s.gameDataLastAttempt = completedAt
		s.gameDataState = GameDataUnsupported
	case palworld.IsKind(err, palworld.ErrorUnauthorized):
		s.clearGameDataLocked()
		s.gameDataLastAttempt = completedAt
		s.gameDataState = GameDataUnauthorized
	case !s.gameDataCapturedAt.IsZero():
		s.gameDataState = GameDataStale
	default:
		s.gameDataState = GameDataUnavailable
	}
	return err
}

func (s *Service) clearGameDataLocked() {
	s.gameDataCapturedAt = time.Time{}
	s.gameDataSourceTime = ""
	s.gameDataFPS, s.gameDataFPSAvg = 0, 0
	s.gameDataCounts = GameDataCounts{}
	s.gameDataActors = nil
	s.gameDataTruncated = false
	s.gameDataCollapsePending = false
}

func (s *Service) effectiveGameDataStateLocked(now time.Time) GameDataState {
	state := s.gameDataState
	if (state == GameDataReady || state == GameDataStale) && !s.gameDataCapturedAt.IsZero() {
		age := now.Sub(s.gameDataCapturedAt)
		if age < 0 || age > s.gameDataStaleAfter {
			return GameDataStale
		}
	}
	return state
}

func (s *Service) gameDataStateSnapshot() GameDataState {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.effectiveGameDataStateLocked(time.Now().UTC())
}

// GameData returns only the bounded sanitized projection. Exact actors are withheld once the
// server-side freshness ceiling expires or capability/auth becomes terminal.
func (s *Service) GameData() CachedGameData {
	s.mu.RLock()
	defer s.mu.RUnlock()
	state := s.effectiveGameDataStateLocked(time.Now().UTC())
	actors := s.gameDataActors
	if state != GameDataReady && state != GameDataStale {
		actors = nil
	} else {
		age := time.Since(s.gameDataCapturedAt)
		if age < 0 || age > s.gameDataStaleAfter {
			actors = nil
		}
	}
	actors = append([]LiveWorldActor(nil), actors...)
	for i := range actors {
		if actors[i].HPPercent != nil {
			value := *actors[i].HPPercent
			actors[i].HPPercent = &value
		}
		if actors[i].Active != nil {
			value := *actors[i].Active
			actors[i].Active = &value
		}
	}
	return CachedGameData{
		State: state, CapturedAt: s.gameDataCapturedAt, LastAttemptAt: s.gameDataLastAttempt,
		SourceTime: s.gameDataSourceTime, FPS: s.gameDataFPS, FPSAvg: s.gameDataFPSAvg,
		Counts: s.gameDataCounts, Actors: actors, Truncated: s.gameDataTruncated,
	}
}

// GameDataSummary is an O(1) accessor: bearer requests never clone or scan actor arrays.
func (s *Service) GameDataSummary() GameDataSummary {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return GameDataSummary{
		State: s.effectiveGameDataStateLocked(time.Now().UTC()), CapturedAt: s.gameDataCapturedAt,
		LastAttemptAt: s.gameDataLastAttempt, FPS: s.gameDataFPS, FPSAvg: s.gameDataFPSAvg, Counts: s.gameDataCounts,
	}
}

// RefreshGameData performs one coalesced fetch. Production uses the background loop; keeping
// this seam exported lets capability probes and tests exercise the same cache transition
// without adding a per-request upstream path.
func (s *Service) RefreshGameData(ctx context.Context) error { return s.pollGameData(ctx) }

func projectGameData(raw []palworld.GameDataActor) (GameDataCounts, []LiveWorldActor, bool) {
	counts := countGameData(raw)
	actors := make([]LiveWorldActor, 0, min(len(raw), maxCachedLiveActors))
	// Players first: a dense base cannot truncate useful live-coordinate candidates.
	for _, wanted := range []string{"Player", "OtomoPal", "BaseCampPal", "PalBox"} {
		for _, actor := range raw {
			kind := ""
			switch actor.Type {
			case "PalBox":
				kind = "PalBox"
			case "Character":
				kind = actor.UnitType
			default:
				continue
			}
			if kind != wanted {
				continue
			}
			if len(actors) == maxCachedLiveActors {
				return counts, actors, true
			}
			actors = append(actors, projectLiveWorldActor(actor, kind))
		}
	}
	return counts, actors, false
}

func countGameData(raw []palworld.GameDataActor) GameDataCounts {
	var counts GameDataCounts
	for _, actor := range raw {
		if actor.Type == "PalBox" {
			counts.PalBoxes++
			continue
		}
		if actor.Type != "Character" {
			counts.Unknown++
			continue
		}
		switch actor.UnitType {
		case "Player":
			counts.Players++
		case "OtomoPal":
			counts.PartyPals++
		case "BaseCampPal":
			counts.BasePals++
		case "WildPal":
			counts.WildPals++
		case "NPC":
			counts.NPCs++
		default:
			counts.Unknown++
		}
	}
	return counts
}

func projectLiveWorldActor(actor palworld.GameDataActor, kind string) LiveWorldActor {
	characterID, boss := "", false
	if kind == "OtomoPal" || kind == "BaseCampPal" {
		characterID, boss = normalizeGameDataClass(actor.Class)
	}
	var hpPercent *float64
	if actor.MaxHP > 0 && actor.HP >= 0 {
		value := 100 * float64(actor.HP) / float64(actor.MaxHP)
		if value > 100 {
			value = 100
		}
		hpPercent = &value
	}
	return LiveWorldActor{
		Kind: kind, CharacterID: characterID, IsBoss: boss,
		Name: boundedClean(actor.NickName, 100), TrainerName: boundedClean(actor.TrainerNickName, 100),
		GuildName: boundedClean(actor.GuildName, 100), Level: actor.Level, HPPercent: hpPercent,
		Active: actor.Active(), Activity: gameDataActivity(actor), X: actor.LocationX, Y: actor.LocationY, Z: actor.LocationZ,
	}
}

func gameDataActivity(actor palworld.GameDataActor) string {
	raw := strings.ToLower(boundedClean(actor.Action, 128) + " " + boundedClean(actor.AIAction, 128) + " " + boundedClean(actor.Stage, 128))
	for _, rule := range []struct {
		category string
		needles  []string
	}{
		{"incapacitated", []string{"down", "dead", "knockout", "incapacitat"}},
		{"combat", []string{"attack", "battle", "combat"}},
		{"sleeping", []string{"sleep"}},
		{"eating", []string{"eat", "food"}},
		{"transporting", []string{"transport", "carry"}},
		{"working", []string{"work", "craft", "build", "generate", "harvest", "mine", "logging"}},
		{"moving", []string{"move", "walk", "run", "return"}},
	} {
		for _, needle := range rule.needles {
			if strings.Contains(raw, needle) {
				return rule.category
			}
		}
	}
	if active := actor.Active(); active != nil {
		if *active {
			return "idle"
		}
		return "inactive"
	}
	return "unknown"
}

func normalizeGameDataClass(value string) (string, bool) {
	value = strings.TrimSpace(value)
	if i := strings.LastIndexAny(value, "/."); i >= 0 {
		value = value[i+1:]
	}
	value = strings.TrimPrefix(value, "Default__")
	value = strings.TrimSuffix(value, "_C")
	boss := len(value) >= len("BOSS_") && strings.EqualFold(value[:len("BOSS_")], "BOSS_")
	if boss {
		value = value[len("BOSS_"):]
	}
	return boundedClean(value, 100), boss
}

// boundedClean removes control characters, collapses whitespace, and stops decoding after a
// bounded number of runes. It never allocates in proportion to an attacker-controlled 32 MiB
// string merely to render a 100-character label.
func boundedClean(value string, maxRunes int) string {
	var b strings.Builder
	b.Grow(min(len(value), maxRunes*4))
	written, pendingSpace := 0, false
	for _, r := range value {
		if written >= maxRunes {
			break
		}
		if unicode.IsSpace(r) || unicode.IsControl(r) {
			if written > 0 {
				pendingSpace = true
			}
			continue
		}
		if pendingSpace {
			b.WriteByte(' ')
			pendingSpace = false
		}
		b.WriteRune(r)
		written++
	}
	return b.String()
}
