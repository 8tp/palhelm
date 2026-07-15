package server

import (
	"net/http"

	"github.com/8tp/palhelm/internal/poller"
)

type gameDataCountsView struct {
	Players   int `json:"players"`
	PartyPals int `json:"partyPals"`
	BasePals  int `json:"basePals"`
	WildPals  int `json:"wildPals"`
	NPCs      int `json:"npcs"`
	PalBoxes  int `json:"palBoxes"`
	Unknown   int `json:"unknown"`
}

type gameDataLocationView struct {
	X float64 `json:"x"`
	Y float64 `json:"y"`
	Z float64 `json:"z"`
}

// sessionLiveActorView is intentionally narrower than palworld.GameDataActor. The credential
// boundary already discards IP/userid; this second allowlist also excludes raw instance/trainer
// IDs, rotation, Stage, Class, Action, and AI_Action from browser responses.
type sessionLiveActorView struct {
	Kind        string               `json:"kind"`
	CharacterID string               `json:"characterId,omitempty"`
	IsBoss      bool                 `json:"isBoss,omitempty"`
	Name        string               `json:"name,omitempty"`
	TrainerName string               `json:"trainerName,omitempty"`
	GuildName   string               `json:"guildName,omitempty"`
	Level       int                  `json:"level,omitempty"`
	HPPercent   *float64             `json:"hpPercent,omitempty"`
	Active      *bool                `json:"active,omitempty"`
	Activity    string               `json:"activity"`
	Location    gameDataLocationView `json:"location"`
}

type sessionWorldSnapshotView struct {
	State         poller.GameDataState    `json:"state"`
	CapturedAt    any                     `json:"capturedAt"`
	LastAttemptAt any                     `json:"lastAttemptAt"`
	SourceTime    string                  `json:"sourceTime,omitempty"`
	FPS           float64                 `json:"fps"`
	FPSAvg        float64                 `json:"fpsAvg"`
	Counts        gameDataCountsView      `json:"counts"`
	Actors        []sessionLiveActorView  `json:"actors"`
	Truncated     bool                    `json:"truncated"`
	Diagnostics   gameDataDiagnosticsView `json:"diagnostics"`
}

type gameDataDiagnosticsView struct {
	LastRequestDurationMS  int64                        `json:"lastRequestDurationMs"`
	LastAcceptedActorCount int                          `json:"lastAcceptedActorCount"`
	LastErrorCategory      poller.GameDataErrorCategory `json:"lastErrorCategory"`
	ScheduledDelayMS       int64                        `json:"scheduledDelayMs"`
	NextAttemptAt          any                          `json:"nextAttemptAt"`
}

type integrationWorldSummaryView struct {
	State         poller.GameDataState `json:"state"`
	CapturedAt    any                  `json:"capturedAt"`
	LastAttemptAt any                  `json:"lastAttemptAt"`
	FPS           float64              `json:"fps"`
	FPSAvg        float64              `json:"fpsAvg"`
	Counts        gameDataCountsView   `json:"counts"`
}

func (s *Server) worldSnapshot(w http.ResponseWriter, _ *http.Request) {
	cached := s.poll.GameData()
	actors := make([]sessionLiveActorView, 0, len(cached.Actors))
	for _, actor := range cached.Actors {
		actors = append(actors, sessionLiveActorView{
			Kind: actor.Kind, CharacterID: actor.CharacterID, IsBoss: actor.IsBoss,
			Name: actor.Name, TrainerName: actor.TrainerName, GuildName: actor.GuildName,
			Level: actor.Level, HPPercent: actor.HPPercent, Active: actor.Active, Activity: actor.Activity,
			Location: gameDataLocationView{X: actor.X, Y: actor.Y, Z: actor.Z},
		})
	}
	writeJSON(w, http.StatusOK, sessionWorldSnapshotView{
		State: cached.State, CapturedAt: nullableTime(cached.CapturedAt), LastAttemptAt: nullableTime(cached.LastAttemptAt),
		SourceTime: cached.SourceTime, FPS: cached.FPS, FPSAvg: cached.FPSAvg,
		Counts: newGameDataCountsView(cached.Counts), Actors: actors, Truncated: cached.Truncated,
		Diagnostics: gameDataDiagnosticsView{
			LastRequestDurationMS:  cached.Diagnostics.LastRequestDuration.Milliseconds(),
			LastAcceptedActorCount: cached.Diagnostics.LastAcceptedActorCount,
			LastErrorCategory:      cached.Diagnostics.LastErrorCategory,
			ScheduledDelayMS:       cached.Diagnostics.ScheduledDelay.Milliseconds(),
			NextAttemptAt:          nullableTime(cached.Diagnostics.NextAttemptAt),
		},
	})
}

// integrationWorldSummary exposes only cached aggregates. It contains no actor names, IDs,
// locations, health, guilds, trainer linkage, or raw activity strings, and never calls upstream.
func (s *Server) integrationWorldSummary(w http.ResponseWriter, r *http.Request) {
	cached := s.poll.GameDataSummary()
	s.writeIntegration(w, r, integrationEnvelope{Data: integrationWorldSummaryView{
		State: cached.State, CapturedAt: nullableTime(cached.CapturedAt), LastAttemptAt: nullableTime(cached.LastAttemptAt),
		FPS: cached.FPS, FPSAvg: cached.FPSAvg, Counts: newGameDataCountsView(cached.Counts),
	}})
}

func newGameDataCountsView(counts poller.GameDataCounts) gameDataCountsView {
	return gameDataCountsView{
		Players: counts.Players, PartyPals: counts.PartyPals, BasePals: counts.BasePals,
		WildPals: counts.WildPals, NPCs: counts.NPCs, PalBoxes: counts.PalBoxes, Unknown: counts.Unknown,
	}
}
