// Package poller contains resilient background synchronization loops.
package poller

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/8tp/palhelm/internal/palworld"
	"github.com/8tp/palhelm/internal/sav"
	"github.com/8tp/palhelm/internal/store"
)

// Publisher receives live SSE messages from pollers.
type Publisher interface{ Publish(event string, value any) }

// Health is the shared status of external data sources.
type Health struct {
	mu         sync.RWMutex
	REST, RCON string
	SaveState  string
	LastSyncAt time.Time
}

// Snapshot returns a race-free health snapshot.
func (h *Health) Snapshot() (string, string, string, time.Time) {
	h.mu.RLock()
	defer h.mu.RUnlock()
	return h.REST, h.RCON, h.SaveState, h.LastSyncAt
}
func (h *Health) setREST(v string) { h.mu.Lock(); h.REST = v; h.mu.Unlock() }
func (h *Health) setSave(v string, at time.Time) {
	h.mu.Lock()
	h.SaveState = v
	if !at.IsZero() {
		h.LastSyncAt = at
	}
	h.mu.Unlock()
}

// SetRCON records RCON status after a console attempt.
func (h *Health) SetRCON(v string) { h.mu.Lock(); h.RCON = v; h.mu.Unlock() }

// CurrentMetrics is the richer in-memory metric response.
type CurrentMetrics struct {
	FPS         float64 `json:"fps"`
	FPSAvg      float64 `json:"fpsAvg"`
	FrameTimeMS float64 `json:"frameTimeMs"`
	Players     int     `json:"players"`
	MaxPlayers  int     `json:"maxPlayers"`
	Day         int64   `json:"day"`
	UptimeSec   int64   `json:"uptimeSec"`
	BaseCamps   int     `json:"baseCamps"`
}

// Service owns the core pollers, the optional game-data poller, and their current snapshots.
type Service struct {
	client                                  *palworld.Client
	gameDataSource                          GameDataSource
	store                                   *store.Store
	pub                                     Publisher
	health                                  *Health
	log                                     *slog.Logger
	metricsEvery, playersEvery, saveEvery   time.Duration
	saveDir                                 string
	mu                                      sync.RWMutex
	current                                 CurrentMetrics
	ring                                    []float64
	online                                  map[string]palworld.Player
	parsing                                 atomic.Bool
	worldGUID                               atomic.Value
	lastREST                                atomic.Int32
	lastInfo                                palworld.Info
	infoReachable                           bool
	gameDataEnabled                         bool
	gameDataEvery                           time.Duration
	gameDataStaleAfter                      time.Duration
	gameDataState                           GameDataState
	gameDataCapturedAt, gameDataLastAttempt time.Time
	gameDataSourceTime                      string
	gameDataFPS, gameDataFPSAvg             float64
	gameDataCounts                          GameDataCounts
	gameDataActivity                        GameDataActivityCounts
	gameDataActors                          []LiveWorldActor
	gameDataTruncated                       bool
	gameDataCollapsePending                 bool
	gameDataInFlight                        atomic.Bool
	gameDataLastRequestDuration             time.Duration
	gameDataLastAcceptedActorCount          int
	gameDataLinkedBasePals                  int
	gameDataUnresolvedBasePals              int
	gameDataLinkLookupFailed                bool
	gameDataLastErrorCategory               GameDataErrorCategory
	gameDataScheduledDelay                  time.Duration
	gameDataNextAttemptAt                   time.Time
	gameDataNow                             func() time.Time
}

// New constructs the poller service.
func New(c *palworld.Client, s *store.Store, p Publisher, h *Health, metricsEvery, playersEvery, saveEvery time.Duration, saveDir string, log *slog.Logger) *Service {
	if log == nil {
		log = slog.Default()
	}
	if h.REST == "" {
		h.REST = "error"
	}
	if h.RCON == "" {
		h.RCON = "error"
	}
	if h.SaveState == "" {
		h.SaveState = "unavailable"
	}
	return &Service{client: c, gameDataSource: c, store: s, pub: p, health: h, log: log, metricsEvery: metricsEvery, playersEvery: playersEvery, saveEvery: saveEvery, saveDir: saveDir, online: make(map[string]palworld.Player), gameDataState: GameDataDisabled, gameDataLastErrorCategory: GameDataErrorNone, gameDataNow: time.Now}
}

// Run launches pollers and blocks until cancellation.
func (s *Service) Run(ctx context.Context) {
	var wg sync.WaitGroup
	loops := []func(context.Context){s.metricsLoop, s.playersLoop, s.saveLoop}
	if s.gameDataEnabled {
		loops = append(loops, s.gameDataLoop)
	}
	for _, fn := range loops {
		wg.Add(1)
		go func(f func(context.Context)) { defer wg.Done(); f(ctx) }(fn)
	}
	wg.Wait()
}
func loop(ctx context.Context, every time.Duration, fn func(context.Context)) {
	fn(ctx)
	t := time.NewTicker(every)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			fn(ctx)
		}
	}
}
func (s *Service) metricsLoop(ctx context.Context) { loop(ctx, s.metricsEvery, s.pollMetrics) }
func (s *Service) playersLoop(ctx context.Context) { loop(ctx, s.playersEvery, s.pollPlayers) }
func (s *Service) saveLoop(ctx context.Context) {
	loop(ctx, s.saveEvery, func(c context.Context) {
		if err := s.ParseNow(c); err != nil && !errors.Is(err, ErrParseBusy) {
			s.log.Error("scheduled world save parse failed", "error", err)
		}
	})
}

func (s *Service) restResult(ctx context.Context, err error) {
	ok := err == nil
	next := int32(1)
	if ok {
		next = 2
	}
	previous := s.lastREST.Swap(next)
	s.health.setREST(map[bool]string{true: "ok", false: "error"}[ok])
	if previous == next {
		return
	}
	msg := "Palworld REST API is reachable"
	if !ok {
		msg = "Palworld REST API is unreachable"
	}
	e := store.Event{At: time.Now().UTC(), Kind: "system", Message: msg, Meta: map[string]any{"rest": map[bool]string{true: "ok", false: "error"}[ok]}}
	_ = s.store.AddEvent(ctx, e)
	s.pub.Publish("event", e)
}
func (s *Service) pollMetrics(ctx context.Context) {
	m, err := s.client.Metrics(ctx)
	s.restResult(ctx, err)
	// Cache the last-successful Info snapshot on the same cadence, independent of the
	// Metrics() outcome above: the integration /server endpoint (spec §4, §S1) reads only
	// this cache, never making its own per-request upstream call.
	info, infoErr := s.client.Info(ctx)
	s.mu.Lock()
	s.infoReachable = infoErr == nil
	if infoErr == nil {
		s.lastInfo = info
	}
	s.mu.Unlock()
	if err != nil {
		return
	}
	now := time.Now().UTC()
	_ = s.store.AddMetric(ctx, store.Metric{At: now, FPS: m.ServerFPS, FrameTimeMS: m.ServerFrameTime, Players: m.CurrentPlayerNum})
	_ = s.store.MaintainMetrics(ctx, now)
	s.mu.Lock()
	s.ring = append(s.ring, m.ServerFPS)
	if len(s.ring) > 60 {
		s.ring = s.ring[len(s.ring)-60:]
	}
	avg := 0.0
	for _, v := range s.ring {
		avg += v
	}
	avg /= float64(len(s.ring))
	s.current = CurrentMetrics{FPS: m.ServerFPS, FPSAvg: avg, FrameTimeMS: m.ServerFrameTime, Players: m.CurrentPlayerNum, MaxPlayers: m.MaxPlayerNum, Day: m.Days, UptimeSec: m.Uptime, BaseCamps: m.BaseCampNum}
	v := s.current
	s.mu.Unlock()
	s.pub.Publish("metrics", v)
}
func (s *Service) pollPlayers(ctx context.Context) {
	players, err := s.client.Players(ctx)
	s.restResult(ctx, err)
	if err != nil {
		return
	}
	s.syncPlayers(ctx, players)
}

func (s *Service) syncPlayers(ctx context.Context, players []palworld.Player) {
	now := time.Now().UTC()
	next := make(map[string]palworld.Player, len(players))
	for _, p := range players {
		candidate := store.NormalizeUID(p.PlayerID)
		if candidate == "" || candidate == "none" {
			continue
		}
		uid := s.store.ResolveUID(ctx, candidate)
		if uid == "" {
			continue
		}
		next[uid] = p
		b, _ := json.Marshal(p)
		x, y := p.LocationX, p.LocationY
		_ = s.store.UpsertLivePlayer(ctx, store.Player{UID: uid, SteamID: p.UserID, Name: p.Name, AccountName: p.AccountName, Level: p.Level, Ping: p.Ping, X: &x, Y: &y, Raw: b}, now)
	}
	joins, leaves := DiffPlayers(s.online, next)
	for _, uid := range joins {
		p := next[uid]
		_ = s.store.StartSession(ctx, uid, now)
		e := store.Event{At: now, Kind: "join", Message: p.Name + " joined", Meta: map[string]any{"uid": uid}}
		_ = s.store.AddEvent(ctx, e)
		s.pub.Publish("event", e)
	}
	for _, uid := range leaves {
		p := s.online[uid]
		_ = s.store.EndSession(ctx, uid, now)
		e := store.Event{At: now, Kind: "leave", Message: p.Name + " left", Meta: map[string]any{"uid": uid}}
		_ = s.store.AddEvent(ctx, e)
		s.pub.Publish("event", e)
	}
	s.mu.Lock()
	s.online = next
	s.mu.Unlock()
	s.pub.Publish("players", map[string]any{"online": len(next), "joins": joins, "leaves": leaves})
}

// DiffPlayers returns sorted joined and departed normalized UIDs.
func DiffPlayers(previous, next map[string]palworld.Player) (joins, leaves []string) {
	for k := range next {
		if _, ok := previous[k]; !ok {
			joins = append(joins, k)
		}
	}
	for k := range previous {
		if _, ok := next[k]; !ok {
			leaves = append(leaves, k)
		}
	}
	sort.Strings(joins)
	sort.Strings(leaves)
	return
}

// Current returns the latest in-memory metric snapshot.
func (s *Service) Current() CurrentMetrics { s.mu.RLock(); defer s.mu.RUnlock(); return s.current }

// Online returns a copy of normalized online identities.
func (s *Service) Online() map[string]bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	v := make(map[string]bool, len(s.online))
	for k := range s.online {
		v[k] = true
	}
	return v
}

// ParseNow synchronously parses the discovered world save; concurrent calls return ErrParseBusy.
func (s *Service) ParseNow(ctx context.Context) error {
	if !s.parsing.CompareAndSwap(false, true) {
		return ErrParseBusy
	}
	defer s.parsing.Store(false)
	level, err := s.discoverLevel(ctx)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) || s.saveDir == "" {
			s.health.setSave("unavailable", time.Time{})
			return nil
		}
		s.health.setSave("error", time.Time{})
		return err
	}
	start := time.Now()
	w, err := sav.ParseLevel(level, sav.Options{})
	if err != nil {
		s.health.setSave("error", time.Time{})
		return err
	}
	at := time.Now().UTC()
	previous, err := s.store.WorldState(ctx)
	if err != nil {
		return err
	}
	nextDrift := len(w.Stats.DecodeFailures) > 0 || w.Stats.SkippedProperties > 0 || w.Stats.SkippedStructs > 0
	if err = s.store.ReplaceWorld(ctx, w, at, time.Since(start)); err != nil {
		return err
	}
	s.emitFormatDriftTransition(ctx, previous.FormatDrift, nextDrift, w.Stats.SkippedProperties+w.Stats.SkippedStructs, at)
	s.health.setSave("ok", at)
	s.pub.Publish("players", map[string]any{"saveSyncAt": at})
	return nil
}

func (s *Service) emitFormatDriftTransition(ctx context.Context, previous, next bool, skippedProps int, at time.Time) {
	if previous == next {
		return
	}
	e := store.Event{At: at, Kind: "system", Message: "world save format drift resolved", Meta: nil}
	if next {
		e.Message = "world save format drift detected"
		e.Meta = map[string]any{"skippedProps": skippedProps}
	}
	_ = s.store.AddEvent(ctx, e)
	s.pub.Publish("event", e)
}

// ErrParseBusy indicates a save parse is already running.
var ErrParseBusy = errors.New("save parse already running")

func (s *Service) discoverLevel(ctx context.Context) (string, error) {
	if s.saveDir == "" {
		return "", os.ErrNotExist
	}
	root := filepath.Join(s.saveDir, "SaveGames", "0")
	guid := ""
	if info, err := s.client.Info(ctx); err == nil {
		guid = normalizeDirGUID(info.WorldGUID)
		s.worldGUID.Store(info.WorldGUID)
	}
	if guid != "" {
		matches, _ := os.ReadDir(root)
		for _, e := range matches {
			if e.IsDir() && normalizeDirGUID(e.Name()) == guid {
				return filepath.Join(root, e.Name(), "Level.sav"), nil
			}
		}
	}
	entries, err := os.ReadDir(root)
	if err != nil {
		return "", err
	}
	type candidate struct {
		path string
		mod  time.Time
	}
	var cs []candidate
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		p := filepath.Join(root, e.Name(), "Level.sav")
		if st, e2 := os.Stat(p); e2 == nil {
			cs = append(cs, candidate{p, st.ModTime()})
		}
	}
	if len(cs) == 0 {
		return "", os.ErrNotExist
	}
	sort.Slice(cs, func(i, j int) bool { return cs[i].mod.After(cs[j].mod) })
	return cs[0].path, nil
}
func normalizeDirGUID(v string) string { return strings.ToLower(strings.ReplaceAll(v, "-", "")) }

// CachedInfo returns the poller's last-fetched server Info and whether that most recent
// fetch attempt succeeded. This is the integration /server endpoint's only data source
// (spec §4, §S1): a stale-but-once-successful snapshot is never served as if it were live
// when the poller currently reports REST unreachable, so callers must zero the fields
// themselves when reachable is false rather than trust Info's contents.
func (s *Service) CachedInfo() (palworld.Info, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.lastInfo, s.infoReachable
}

// WorldGUID returns the most recently observed world GUID.
func (s *Service) WorldGUID() string {
	v := s.worldGUID.Load()
	if v == nil {
		return ""
	}
	return v.(string)
}
