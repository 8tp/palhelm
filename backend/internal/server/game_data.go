package server

import (
	"net/http"
	"time"

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
	InstanceID  string               `json:"instanceId,omitempty"`
	BaseID      string               `json:"baseId,omitempty"`
	OwnerUID    string               `json:"ownerUid,omitempty"`
	OwnerName   string               `json:"ownerName,omitempty"`
	OwnerSource string               `json:"ownerSource,omitempty"`
	Linked      bool                 `json:"linked,omitempty"`
	Location    gameDataLocationView `json:"location"`
}

type sessionWorldSnapshotView struct {
	State         poller.GameDataState          `json:"state"`
	CapturedAt    any                           `json:"capturedAt"`
	LastAttemptAt any                           `json:"lastAttemptAt"`
	SourceTime    string                        `json:"sourceTime,omitempty"`
	FPS           float64                       `json:"fps"`
	FPSAvg        float64                       `json:"fpsAvg"`
	Counts        gameDataCountsView            `json:"counts"`
	Activity      poller.GameDataActivityCounts `json:"activity"`
	Actors        []sessionLiveActorView        `json:"actors"`
	Truncated     bool                          `json:"truncated"`
	Diagnostics   gameDataDiagnosticsView       `json:"diagnostics"`
}

type gameDataDiagnosticsView struct {
	LastRequestDurationMS  int64                        `json:"lastRequestDurationMs"`
	LastAcceptedActorCount int                          `json:"lastAcceptedActorCount"`
	LastErrorCategory      poller.GameDataErrorCategory `json:"lastErrorCategory"`
	LinkedBasePals         int                          `json:"linkedBasePals"`
	UnresolvedBasePals     int                          `json:"unresolvedBasePals"`
	LinkLookupFailed       bool                         `json:"linkLookupFailed"`
	ScheduledDelayMS       int64                        `json:"scheduledDelayMs"`
	NextAttemptAt          any                          `json:"nextAttemptAt"`
}

type integrationWorldSummaryView struct {
	State          poller.GameDataState          `json:"state"`
	CapturedAt     any                           `json:"capturedAt"`
	LastAttemptAt  any                           `json:"lastAttemptAt"`
	FPS            float64                       `json:"fps"`
	FPSAvg         float64                       `json:"fpsAvg"`
	Counts         gameDataCountsView            `json:"counts"`
	Activity       poller.GameDataActivityCounts `json:"activity"`
	LinkedBasePals int                           `json:"linkedBasePals"`
}

type integrationLiveWorkerView struct {
	InstanceID  string   `json:"instanceId"`
	CharacterID string   `json:"characterId"`
	DisplayName string   `json:"displayName"`
	IsBoss      bool     `json:"isBoss"`
	Level       int      `json:"level"`
	HPPercent   *float64 `json:"hpPercent"`
	Active      *bool    `json:"active"`
	Activity    string   `json:"activity"`
	BaseID      string   `json:"baseId"`
	OwnerUID    string   `json:"ownerUid,omitempty"`
	OwnerName   string   `json:"ownerName,omitempty"`
	OwnerSource string   `json:"ownerSource,omitempty"`
}

type integrationLiveWorkersView struct {
	State      poller.GameDataState        `json:"state"`
	CapturedAt any                         `json:"capturedAt"`
	Workers    []integrationLiveWorkerView `json:"workers"`
}

func (s *Server) worldSnapshot(w http.ResponseWriter, _ *http.Request) {
	cached := s.poll.GameData()
	actors := make([]sessionLiveActorView, 0, len(cached.Actors))
	for _, actor := range cached.Actors {
		actors = append(actors, sessionLiveActorView{
			Kind: actor.Kind, CharacterID: actor.CharacterID, IsBoss: actor.IsBoss,
			Name: actor.Name, TrainerName: actor.TrainerName, GuildName: actor.GuildName,
			Level: actor.Level, HPPercent: actor.HPPercent, Active: actor.Active, Activity: actor.Activity,
			InstanceID: actor.InstanceID, BaseID: actor.BaseID, OwnerUID: actor.OwnerUID,
			OwnerName: actor.OwnerName, OwnerSource: actor.OwnerSource, Linked: actor.Linked,
			Location: gameDataLocationView{X: actor.X, Y: actor.Y, Z: actor.Z},
		})
	}
	writeJSON(w, http.StatusOK, sessionWorldSnapshotView{
		State: cached.State, CapturedAt: nullableTime(cached.CapturedAt), LastAttemptAt: nullableTime(cached.LastAttemptAt),
		SourceTime: cached.SourceTime, FPS: cached.FPS, FPSAvg: cached.FPSAvg,
		Counts: newGameDataCountsView(cached.Counts), Activity: cached.Activity, Actors: actors, Truncated: cached.Truncated,
		Diagnostics: gameDataDiagnosticsView{
			LastRequestDurationMS:  cached.Diagnostics.LastRequestDuration.Milliseconds(),
			LastAcceptedActorCount: cached.Diagnostics.LastAcceptedActorCount,
			LastErrorCategory:      cached.Diagnostics.LastErrorCategory,
			LinkedBasePals:         cached.Diagnostics.LinkedBasePals,
			UnresolvedBasePals:     cached.Diagnostics.UnresolvedBasePals,
			LinkLookupFailed:       cached.Diagnostics.LinkLookupFailed,
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
		Activity: cached.Activity, LinkedBasePals: cached.LinkedBasePals,
	}})
}

// integrationWorldWorkers exposes exact save-linked base workers from the shared cache. It
// deliberately omits coordinates, guild/trainer names, runtime actor IDs, and upstream action
// strings. Unresolved actors are excluded rather than associated heuristically.
func (s *Server) integrationWorldWorkers(w http.ResponseWriter, r *http.Request) {
	cached := s.poll.GameData()
	workers := make([]integrationLiveWorkerView, 0, cached.Diagnostics.LinkedBasePals)
	for _, actor := range cached.Actors {
		if actor.Kind != "BaseCampPal" || !actor.Linked || actor.BaseID == "" || actor.InstanceID == "" {
			continue
		}
		workers = append(workers, integrationLiveWorkerView{
			InstanceID: actor.InstanceID, CharacterID: actor.CharacterID, DisplayName: actor.Name,
			IsBoss: actor.IsBoss, Level: actor.Level, HPPercent: actor.HPPercent, Active: actor.Active,
			Activity: actor.Activity, BaseID: actor.BaseID, OwnerUID: actor.OwnerUID,
			OwnerName: actor.OwnerName, OwnerSource: actor.OwnerSource,
		})
	}
	s.writeIntegration(w, r, integrationEnvelope{Data: integrationLiveWorkersView{
		State: cached.State, CapturedAt: nullableTime(cached.CapturedAt), Workers: workers,
	}})
}

func (s *Server) worldActivityHistory(w http.ResponseWriter, r *http.Request) {
	window := r.URL.Query().Get("window")
	duration := time.Hour
	bucket := time.Minute
	switch window {
	case "", "1h":
	case "24h":
		duration = 24 * time.Hour
		bucket = 5 * time.Minute
	case "7d":
		duration = 7 * 24 * time.Hour
		bucket = 30 * time.Minute
	default:
		writeError(w, http.StatusBadRequest, "invalid_window", "window must be 1h, 24h, or 7d")
		return
	}
	rows, err := s.store.GameDataActivityHistory(r.Context(), time.Now().Add(-duration), bucket)
	if err != nil {
		internal(w, err)
		return
	}
	if window == "" {
		window = "1h"
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"window": window, "bucketSec": int64(bucket / time.Second), "samples": rows,
	})
}

func newGameDataCountsView(counts poller.GameDataCounts) gameDataCountsView {
	return gameDataCountsView{
		Players: counts.Players, PartyPals: counts.PartyPals, BasePals: counts.BasePals,
		WildPals: counts.WildPals, NPCs: counts.NPCs, PalBoxes: counts.PalBoxes, Unknown: counts.Unknown,
	}
}
