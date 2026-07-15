package poller

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/8tp/palhelm/internal/palworld"
)

type fakeGameDataSource struct {
	mu        sync.Mutex
	snapshots []palworld.GameDataSnapshot
	errors    []error
	calls     int
}

func (f *fakeGameDataSource) GameData(context.Context) (palworld.GameDataSnapshot, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	i := f.calls
	f.calls++
	var snapshot palworld.GameDataSnapshot
	if i < len(f.snapshots) {
		snapshot = f.snapshots[i]
	}
	if i < len(f.errors) {
		return snapshot, f.errors[i]
	}
	return snapshot, nil
}

func TestGameDataCacheReadyDeepCopyAndStaleLastGood(t *testing.T) {
	s, _, _ := testService(t, palworld.NewClient("", "", ""))
	s.ConfigureGameData(true, 30*time.Second)
	source := &fakeGameDataSource{
		snapshots: []palworld.GameDataSnapshot{{Time: "server-local", FPS: 55, ActorData: []palworld.GameDataActor{{Type: "Character", UnitType: "Player", NickName: "Hunter", IsActive: "true"}}}},
		errors:    []error{nil, errors.New("temporary failure")},
	}
	s.gameDataSource = source
	if err := s.pollGameData(context.Background()); err != nil {
		t.Fatal(err)
	}
	first := s.GameData()
	if first.State != GameDataReady || first.CapturedAt.IsZero() || len(first.Actors) != 1 || first.Counts.Players != 1 {
		t.Fatalf("ready cache = %#v", first)
	}
	first.Actors[0].Name = "mutated"
	*first.Actors[0].Active = false
	second := s.GameData().Actors[0]
	if second.Name != "Hunter" || second.Active == nil || !*second.Active {
		t.Fatalf("cache projection was mutable through returned actor: %#v", second)
	}
	if err := s.pollGameData(context.Background()); err == nil {
		t.Fatal("expected transient error")
	}
	stale := s.GameData()
	if stale.State != GameDataStale || stale.Actors[0].Name != "Hunter" || stale.CapturedAt != first.CapturedAt {
		t.Fatalf("stale cache did not retain last good: %#v", stale)
	}
	if stale.Diagnostics.LastAcceptedActorCount != 1 || stale.Diagnostics.LastErrorCategory != GameDataErrorUnknown {
		t.Fatalf("stale diagnostics = %#v", stale.Diagnostics)
	}
}

func TestGameDataCacheClassifiesTerminalFailuresWithoutGlobalHealth(t *testing.T) {
	for _, tc := range []struct {
		name  string
		kind  palworld.ErrorKind
		state GameDataState
	}{{"unsupported", palworld.ErrorUnsupported, GameDataUnsupported}, {"unauthorized", palworld.ErrorUnauthorized, GameDataUnauthorized}} {
		t.Run(tc.name, func(t *testing.T) {
			s, _, _ := testService(t, palworld.NewClient("", "", ""))
			s.ConfigureGameData(true, 30*time.Second)
			s.health.setREST("ok")
			s.gameDataSource = &fakeGameDataSource{
				snapshots: []palworld.GameDataSnapshot{{ActorData: []palworld.GameDataActor{{Type: "Character", UnitType: "Player", NickName: "must-clear"}}}},
				errors:    []error{nil, &palworld.APIError{Kind: tc.kind, Status: http.StatusNotFound, Err: errors.New(tc.name)}},
			}
			if err := s.pollGameData(context.Background()); err != nil {
				t.Fatal(err)
			}
			if err := s.pollGameData(context.Background()); err == nil {
				t.Fatal("expected terminal error")
			}
			got := s.GameData()
			if got.State != tc.state || len(got.Actors) != 0 || got.Counts.total() != 0 {
				t.Fatalf("terminal cache = %#v, want state %s and no retained data", got, tc.state)
			}
			if got.Diagnostics.LastAcceptedActorCount != 1 || string(got.Diagnostics.LastErrorCategory) != tc.name {
				t.Fatalf("terminal diagnostics = %#v", got.Diagnostics)
			}
			rest, _, _, _ := s.health.Snapshot()
			if rest != "ok" {
				t.Fatalf("optional endpoint poisoned REST health: %q", rest)
			}
		})
	}
}

func TestGameDataDisabledByDefault(t *testing.T) {
	s, _, _ := testService(t, palworld.NewClient("", "", ""))
	if got := s.GameData(); got.State != GameDataDisabled || !got.CapturedAt.IsZero() {
		t.Fatalf("default cache = %#v", got)
	}
}

func TestGameDataExpiresExactActorsServerSide(t *testing.T) {
	s, _, _ := testService(t, palworld.NewClient("", "", ""))
	s.ConfigureGameData(true, 30*time.Second)
	s.gameDataSource = &fakeGameDataSource{snapshots: []palworld.GameDataSnapshot{{ActorData: []palworld.GameDataActor{{Type: "Character", UnitType: "Player", NickName: "Hunter"}}}}}
	if err := s.pollGameData(context.Background()); err != nil {
		t.Fatal(err)
	}
	s.mu.Lock()
	s.gameDataCapturedAt = time.Now().UTC().Add(-s.gameDataStaleAfter - time.Second)
	s.mu.Unlock()
	got := s.GameData()
	if got.State != GameDataStale || len(got.Actors) != 0 {
		t.Fatalf("expired cache = %#v, want stale metadata without actors", got)
	}
}

func TestGameDataCollapsedSnapshotRequiresConfirmation(t *testing.T) {
	large := make([]palworld.GameDataActor, 20)
	for i := range large {
		large[i] = palworld.GameDataActor{Type: "Character", UnitType: "WildPal"}
	}
	small := palworld.GameDataSnapshot{ActorData: []palworld.GameDataActor{{Type: "Character", UnitType: "Player", NickName: "Hunter"}}}
	s, _, _ := testService(t, palworld.NewClient("", "", ""))
	s.ConfigureGameData(true, 30*time.Second)
	s.gameDataSource = &fakeGameDataSource{snapshots: []palworld.GameDataSnapshot{{ActorData: large}, small, small}}
	if err := s.pollGameData(context.Background()); err != nil {
		t.Fatal(err)
	}
	if err := s.pollGameData(context.Background()); !errors.Is(err, ErrGameDataCollapsed) {
		t.Fatalf("first collapse error = %v", err)
	}
	if got := s.GameData(); got.Counts.WildPals != 20 || got.State != GameDataStale {
		t.Fatalf("first collapse replaced last-good data: %#v", got)
	}
	if got := s.GameData().Diagnostics; got.LastAcceptedActorCount != 20 || got.LastErrorCategory != GameDataErrorCollapsed {
		t.Fatalf("collapsed diagnostics = %#v", got)
	}
	if err := s.pollGameData(context.Background()); err != nil {
		t.Fatal(err)
	}
	if got := s.GameData(); got.Counts.Players != 1 || got.Counts.WildPals != 0 || got.State != GameDataReady {
		t.Fatalf("confirmed collapse not accepted: %#v", got)
	}
	if got := s.GameData().Diagnostics; got.LastAcceptedActorCount != 1 || got.LastErrorCategory != GameDataErrorNone {
		t.Fatalf("confirmed diagnostics = %#v", got)
	}
}

func TestGameDataDiagnosticsDurationAndScheduleAreDeterministic(t *testing.T) {
	s, _, _ := testService(t, palworld.NewClient("", "", ""))
	s.ConfigureGameData(true, 30*time.Second)
	started := time.Date(2026, 7, 15, 12, 0, 0, 0, time.UTC)
	completed := started.Add(1250 * time.Millisecond)
	clockCalls := 0
	s.gameDataNow = func() time.Time {
		clockCalls++
		if clockCalls == 1 {
			return started
		}
		return completed
	}
	s.gameDataSource = &fakeGameDataSource{snapshots: []palworld.GameDataSnapshot{{ActorData: []palworld.GameDataActor{
		{Type: "Character", UnitType: "Player"},
		{Type: "Character", UnitType: "WildPal"},
	}}}}
	if err := s.pollGameData(context.Background()); err != nil {
		t.Fatal(err)
	}
	diagnostics := s.GameData().Diagnostics
	if diagnostics.LastRequestDuration != 1250*time.Millisecond || diagnostics.LastAcceptedActorCount != 2 || diagnostics.LastErrorCategory != GameDataErrorNone {
		t.Fatalf("accepted diagnostics = %#v", diagnostics)
	}

	s.gameDataNow = func() time.Time { return completed }
	s.setGameDataSchedule(45 * time.Second)
	diagnostics = s.GameData().Diagnostics
	if diagnostics.ScheduledDelay != 45*time.Second || !diagnostics.NextAttemptAt.Equal(completed.Add(45*time.Second)) {
		t.Fatalf("scheduled diagnostics = %#v", diagnostics)
	}
	s.setGameDataSchedule(0)
	diagnostics = s.GameData().Diagnostics
	if diagnostics.ScheduledDelay != 0 || !diagnostics.NextAttemptAt.IsZero() {
		t.Fatalf("cleared schedule diagnostics = %#v", diagnostics)
	}
}

func TestClassifyGameDataErrorUsesOnlyBoundedCategories(t *testing.T) {
	for _, tc := range []struct {
		name string
		err  error
		want GameDataErrorCategory
	}{
		{"none", nil, GameDataErrorNone},
		{"collapsed", ErrGameDataCollapsed, GameDataErrorCollapsed},
		{"timeout", context.DeadlineExceeded, GameDataErrorTimeout},
		{"canceled", context.Canceled, GameDataErrorCanceled},
		{"unreachable", &palworld.APIError{Kind: palworld.ErrorUnreachable, Err: errors.New("PRIVATE upstream detail")}, GameDataErrorUnreachable},
		{"unauthorized", &palworld.APIError{Kind: palworld.ErrorUnauthorized, Err: errors.New("PRIVATE upstream detail")}, GameDataErrorUnauthorized},
		{"unsupported", &palworld.APIError{Kind: palworld.ErrorUnsupported, Err: errors.New("PRIVATE upstream detail")}, GameDataErrorUnsupported},
		{"response", &palworld.APIError{Kind: palworld.ErrorResponse, Err: errors.New("PRIVATE upstream detail")}, GameDataErrorResponse},
		{"unknown", errors.New("PRIVATE upstream detail"), GameDataErrorUnknown},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got := classifyGameDataError(tc.err)
			if got != tc.want {
				t.Fatalf("category = %q, want %q", got, tc.want)
			}
			if strings.Contains(string(got), "PRIVATE") {
				t.Fatalf("category leaked raw detail: %q", got)
			}
		})
	}
}

func TestNextGameDataDelayBackoffIsDeterministicAndBounded(t *testing.T) {
	transient := errors.New("transient")
	if got := nextGameDataDelay(30*time.Second, 30*time.Second, nil); got != 30*time.Second {
		t.Fatalf("successful delay = %s", got)
	}
	if got := nextGameDataDelay(time.Minute, 30*time.Second, ErrGameDataCollapsed); got != 30*time.Second {
		t.Fatalf("collapsed delay = %s", got)
	}
	if got := nextGameDataDelay(30*time.Second, 30*time.Second, transient); got != time.Minute {
		t.Fatalf("first failure delay = %s", got)
	}
	if got := nextGameDataDelay(8*time.Minute, 30*time.Second, transient); got != 10*time.Minute {
		t.Fatalf("capped failure delay = %s", got)
	}
	if got := nextGameDataDelay(0, time.Second, nil); got != 15*time.Second {
		t.Fatalf("minimum interval = %s", got)
	}
}

func TestGameDataProjectionIsBoundedAndPrioritizesPlayers(t *testing.T) {
	raw := make([]palworld.GameDataActor, maxCachedLiveActors+10)
	for i := range raw {
		raw[i] = palworld.GameDataActor{Type: "Character", UnitType: "BaseCampPal", Class: "SheepBall"}
	}
	raw[len(raw)-1] = palworld.GameDataActor{Type: "Character", UnitType: "Player", NickName: "Hunter"}
	counts, actors, truncated := projectGameData(raw)
	if !truncated || len(actors) != maxCachedLiveActors || counts.Players != 1 || actors[0].Kind != "Player" || actors[0].Name != "Hunter" {
		t.Fatalf("projection counts=%#v len=%d truncated=%v first=%#v", counts, len(actors), truncated, actors[0])
	}
}

func TestGameDataProjectionRejectsRecognizedUnitOnUnknownActorType(t *testing.T) {
	counts, actors, truncated := projectGameData([]palworld.GameDataActor{{Type: "FutureActor", UnitType: "Player", NickName: "spoof"}})
	if truncated || len(actors) != 0 || counts.Unknown != 1 || counts.Players != 0 {
		t.Fatalf("projection accepted mismatched actor: counts=%#v actors=%#v truncated=%v", counts, actors, truncated)
	}
}

func TestGameDataProjectionNormalizesBossAndActivity(t *testing.T) {
	counts, actors, truncated := projectGameData([]palworld.GameDataActor{{
		Type: "Character", UnitType: "BaseCampPal", Class: "/Game/Pal/BOSS_Mammorest.BOSS_Mammorest_C",
		TrainerNickName: "  Hunter\nAdmin\x00 ", HP: 50, MaxHP: 100, AIAction: "TransportItem", IsActive: "true",
	}})
	if truncated || counts.BasePals != 1 || len(actors) != 1 {
		t.Fatalf("projection = %#v %#v truncated=%v", counts, actors, truncated)
	}
	got := actors[0]
	if got.CharacterID != "Mammorest" || !got.IsBoss || got.TrainerName != "Hunter Admin" || got.Activity != "transporting" || got.HPPercent == nil || *got.HPPercent != 50 {
		t.Fatalf("actor = %#v", got)
	}
}

func TestGameDataSummaryContainsNoActorProjection(t *testing.T) {
	s, _, _ := testService(t, palworld.NewClient("", "", ""))
	s.ConfigureGameData(true, 30*time.Second)
	s.gameDataSource = &fakeGameDataSource{snapshots: []palworld.GameDataSnapshot{{ActorData: []palworld.GameDataActor{{Type: "Character", UnitType: "Player", NickName: "private-name"}}}}}
	if err := s.pollGameData(context.Background()); err != nil {
		t.Fatal(err)
	}
	b, err := json.Marshal(s.GameDataSummary())
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(strings.ToLower(string(b)), "actor") || strings.Contains(string(b), "private-name") {
		t.Fatalf("summary leaked actor data: %s", b)
	}
}
