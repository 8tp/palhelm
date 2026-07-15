// Package server exposes Palhelm's authenticated HTTP API.
package server

import (
	"context"
	"database/sql"
	_ "embed"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/8tp/palhelm/internal/backup"
	"github.com/8tp/palhelm/internal/config"
	"github.com/8tp/palhelm/internal/gameconfig"
	"github.com/8tp/palhelm/internal/palworld"
	"github.com/8tp/palhelm/internal/poller"
	"github.com/8tp/palhelm/internal/steamavatar"
	"github.com/8tp/palhelm/internal/store"
	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
)

//go:embed openapi.json
var openapi []byte

// Server owns routing and application services.
type Server struct {
	cfg         config.Config
	store       *store.Store
	pal         *palworld.Client
	rcon        *palworld.RCONClient
	poll        *poller.Service
	health      *poller.Health
	hub         *Hub
	auth        *auth
	shutdown    *orchestrator
	backups     *backup.Engine
	gamecfg     *gameconfig.Editor
	integration *integrationAuth
	avatars     *steamavatar.Resolver
	started     time.Time
	log         *slog.Logger
}

// New creates a fully wired HTTP server and poller service.
func New(cfg config.Config, st *store.Store, log *slog.Logger) (*Server, http.Handler) {
	if log == nil {
		log = slog.Default()
	}
	hub := NewHub()
	pal := palworld.NewClient(cfg.RESTURL, cfg.RESTUser, cfg.PalworldPassword)
	pal.SetGameDataTimeout(cfg.GameDataTimeout)
	health := &poller.Health{REST: "error", RCON: "error", SaveState: "unavailable"}
	p := poller.New(pal, st, hub, health, cfg.MetricsInterval, cfg.PlayersInterval, cfg.SaveSyncInterval, cfg.SaveDir, log)
	p.ConfigureGameData(cfg.GameDataEnabled, cfg.GameDataInterval)
	activeKeys, err := st.ActiveAPIKeys(context.Background())
	if err != nil {
		// A transient DB read failure here must not crash the whole panel: an empty cache
		// fails every bearer request closed (uniform 401) until the next restart, rather
		// than taking down session/admin routes too.
		log.Error("load active integration API keys", "error", err)
	}
	s := &Server{cfg: cfg, store: st, pal: pal, rcon: palworld.NewRCONClient(cfg.RCONAddr, cfg.PalworldPassword), poll: p, health: health, hub: hub, auth: newAuth(cfg.SessionSecret, cfg.AdminPassword, cfg.ViewerPassword, cfg.TrustedProxies...), shutdown: newOrchestrator(pal), integration: newIntegrationAuth(st, activeKeys, cfg.IntegrationRateLimit, log), avatars: steamavatar.New(cfg.SteamWebAPIKey), started: time.Now(), log: log}
	emitBackup := func(message string, meta any) {
		e := store.Event{At: time.Now().UTC(), Kind: "backup", Message: message, Meta: meta}
		_ = st.AddEvent(context.Background(), e)
		hub.Publish("event", e)
	}
	backupDataDir := cfg.DataDir
	if backupDataDir == "" {
		backupDataDir = filepath.Join(os.TempDir(), fmt.Sprintf("palhelm-%p", st))
	}
	s.backups = backup.New(backupDataDir, cfg.SaveDir, st, pal.Save, func(ctx context.Context) bool {
		_, err := pal.Info(ctx)
		return err == nil || !palworld.IsKind(err, palworld.ErrorUnreachable)
	}, emitBackup, log)
	s.backups.SetWorldGUIDResolver(func(ctx context.Context) (string, error) {
		info, err := pal.Info(ctx)
		if palworld.IsKind(err, palworld.ErrorUnreachable) {
			return "", fmt.Errorf("Palworld REST API unavailable: %w", backup.ErrWorldGUIDUnavailable)
		}
		if err != nil {
			return "", err
		}
		if strings.TrimSpace(info.WorldGUID) == "" {
			return "", errors.New("Palworld REST API returned an empty world GUID")
		}
		return info.WorldGUID, nil
	})
	s.backups.SetCachedWorldGUID(p.WorldGUID)
	s.gamecfg = &gameconfig.Editor{ComposeFile: cfg.ComposeFile, Service: cfg.GameService, SaveDir: cfg.SaveDir, DockerControl: cfg.DockerControl, Effective: pal.Settings}
	configWrite := s.gamecfg.Probe()
	if configWrite.Available {
		log.Info("config editor capability detected", "available", true, "composeFile", cfg.ComposeFile)
	} else {
		log.Warn("config editor is read-only", "available", false, "reason", configWrite.Reason, "composeFile", cfg.ComposeFile)
	}
	if err := s.backups.Reconcile(context.Background()); err != nil {
		log.Error("reconcile backups", "error", err)
	}
	return s, s.routes()
}

// CloseStreams releases lifecycle-bound streaming requests before HTTP shutdown waits, and
// best-effort flushes any coalesced integration-key lastUsedAt writes (spec §2.5) so the
// final minute of bearer activity before a graceful shutdown is not lost.
func (s *Server) CloseStreams() {
	s.hub.Close()
	s.integration.Flush(context.Background())
}

// RunPollers runs all background synchronization loops until context cancellation.
func (s *Server) RunPollers(ctx context.Context) {
	done := make(chan struct{}, 3)
	go func() { s.poll.Run(ctx); done <- struct{}{} }()
	go func() { s.backups.Run(ctx); done <- struct{}{} }()
	go func() { s.rconProbeLoop(ctx); done <- struct{}{} }()
	<-done
	<-done
	<-done
}

// rconProbeLoop keeps health.RCON honest even when the console is idle.
// Console execs also update it, so a failure surfaces within a minute either way.
func (s *Server) rconProbeLoop(ctx context.Context) {
	probe := func() {
		c, cancel := context.WithTimeout(ctx, 5*time.Second)
		defer cancel()
		if _, err := s.rcon.Exec(c, "Info"); err != nil {
			s.health.SetRCON("error")
		} else {
			s.health.SetRCON("ok")
		}
	}
	probe()
	t := time.NewTicker(time.Minute)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			probe()
		}
	}
}

func (s *Server) routes() http.Handler {
	r := chi.NewRouter()
	r.Use(middleware.RequestID, s.securityHeaders, s.recoverer, s.requestLog)
	r.Get("/healthz", func(w http.ResponseWriter, r *http.Request) { writeJSON(w, 200, map[string]string{"status": "ok"}) })
	r.Get("/api/openapi.json", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(openapi)
	})
	r.Post("/api/v1/auth/login", s.login)
	// Mounted before route resolution runs its own middleware chain (spec §1): an
	// unauthenticated probe of any path under here, real or not, gets the uniform 401.
	r.Mount("/api/integration/v1", s.integrationRouter())
	r.Group(func(a chi.Router) {
		a.Use(s.auth.middleware)
		a.Mount("/map-tiles", s.tilesHandler())
		a.Post("/api/v1/auth/logout", s.logout)
		a.Get("/api/v1/auth/session", s.session)
		a.Route("/api/v1", func(api chi.Router) {
			api.Get("/server", s.serverInfo)
			api.Get("/server/health", s.serverHealth)
			api.Get("/metrics/current", s.metricsCurrent)
			api.Get("/metrics/history", s.metricsHistory)
			api.Get("/activity", s.activity)
			api.Get("/players", s.players)
			api.Get("/pals", s.pals)
			api.Get("/players/{uid}", s.player)
			api.Get("/players/{uid}/paldeck", s.playerPaldeck)
			api.Get("/players/{uid}/avatar", s.playerAvatar)
			api.Get("/whitelist", s.whitelist)
			api.Get("/guilds", s.guilds)
			api.Get("/guilds/{id}", s.guildDetail)
			api.Get("/paldeck", s.serverPaldeck)
			api.Get("/world", s.world)
			api.Get("/world/snapshot", s.worldSnapshot)
			api.Get("/world/activity", s.worldActivityHistory)
			api.Get("/map/dataset", s.mapDataset)
			api.Get("/paldeck/icon/{characterId}", s.paldeckIcon)
			api.Get("/paldeck/icon-dataset", s.paldeckIconDataset)
			api.Get("/console/log", s.consoleLog)
			api.Get("/console/saved", s.savedCommands)
			api.Get("/events", s.events)
			api.Get("/events/stream", s.eventStream)
			api.Get("/backups", s.listBackups)
			api.Get("/backups/{id}/download", s.downloadBackup)
			api.Head("/backups/{id}/download", s.downloadBackup)
			api.Get("/backups/{id}/contents", s.backupContents)
			api.Get("/backups/schedule", s.backupSchedule)
			api.Get("/config", s.getConfig)
			api.Group(func(m chi.Router) {
				m.Use(adminOnly)
				m.Get("/config/raw", s.rawConfig)
				m.Post("/server/announce", s.announce)
				m.Post("/server/save", s.save)
				m.Post("/server/shutdown", s.startShutdown)
				m.Post("/server/shutdown/cancel", s.cancelShutdown)
				m.Post("/players/{uid}/kick", s.kick)
				m.Post("/players/{uid}/ban", s.ban)
				m.Post("/players/{uid}/unban", s.unban)
				m.Put("/whitelist", s.putWhitelist)
				m.Post("/world/parse", s.parseWorld)
				m.Post("/console/exec", s.consoleExec)
				m.Post("/console/saved", s.saveCommand)
				m.Delete("/console/saved/{id}", s.deleteCommand)
				m.Post("/backups", s.createBackup)
				m.Post("/backups/{id}/restore/dry-run", s.dryRunRestore)
				m.Post("/backups/{id}/restore", s.restoreBackup)
				m.Delete("/backups/{id}", s.deleteBackup)
				m.Put("/backups/schedule", s.putBackupSchedule)
				m.Put("/config", s.putConfig)
				m.Post("/config/apply", s.applyConfig)
				m.Post("/integration-keys", s.createIntegrationKey)
				m.Get("/integration-keys", s.listIntegrationKeys)
				m.Delete("/integration-keys/{id}", s.revokeIntegrationKey)
			})
		})
	})
	r.Mount("/", spaHandler())
	return r
}

func (s *Server) securityHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Security-Policy", "default-src 'self'; img-src 'self' data:; style-src 'self' 'unsafe-inline'; script-src 'self'; connect-src 'self'; font-src 'self'; object-src 'none'; base-uri 'self'; frame-ancestors 'none'; form-action 'self'")
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("X-Frame-Options", "DENY")
		w.Header().Set("Referrer-Policy", "no-referrer")
		w.Header().Set("Permissions-Policy", "camera=(), microphone=(), geolocation=()")
		if strings.HasPrefix(r.URL.Path, "/api/v1/auth/") || r.URL.Path == "/api/v1/config" || r.URL.Path == "/api/v1/config/raw" || r.URL.Path == "/api/v1/world/snapshot" || strings.HasPrefix(r.URL.Path, "/api/v1/integration-keys") {
			w.Header().Set("Cache-Control", "no-store")
		}
		next.ServeHTTP(w, r)
	})
}
func (s *Server) recoverer(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			if v := recover(); v != nil {
				s.log.Error("http panic", "panic", v, "path", r.URL.Path)
				writeError(w, 500, "internal_error", "The server could not complete the request.")
			}
		}()
		next.ServeHTTP(w, r)
	})
}
func (s *Server) requestLog(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		ww := middleware.NewWrapResponseWriter(w, r.ProtoMajor)
		next.ServeHTTP(ww, r)
		s.log.Info("http request", "method", r.Method, "path", r.URL.Path, "status", ww.Status(), "duration", time.Since(start))
	})
}

func (s *Server) login(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Password string `json:"password"`
	}
	if !decode(w, r, &req) {
		return
	}
	if !s.auth.allow(s.auth.clientIP(r)) {
		writeError(w, 429, "rate_limited", "Too many login attempts; try again later.")
		return
	}
	role := ""
	if secureEqual(req.Password, s.cfg.AdminPassword) {
		role = "admin"
	} else if s.cfg.ViewerPassword != "" && secureEqual(req.Password, s.cfg.ViewerPassword) {
		role = "viewer"
	}
	if role == "" {
		writeError(w, 401, "invalid_credentials", "The password is incorrect.")
		return
	}
	expires := time.Now().Add(7 * 24 * time.Hour)
	token, err := s.auth.token(role, expires)
	if err != nil {
		internal(w, err)
		return
	}
	secure := s.cfg.SecureCookies || r.TLS != nil || s.auth.forwardedHTTPS(r)
	http.SetCookie(w, &http.Cookie{Name: sessionCookie, Value: token, Path: "/", Expires: expires, MaxAge: 7 * 24 * 3600, HttpOnly: true, SameSite: http.SameSiteLaxMode, Secure: secure})
	writeJSON(w, 200, map[string]string{"role": role})
}
func (s *Server) logout(w http.ResponseWriter, r *http.Request) {
	secure := s.cfg.SecureCookies || r.TLS != nil || s.auth.forwardedHTTPS(r)
	http.SetCookie(w, &http.Cookie{Name: sessionCookie, Path: "/", MaxAge: -1, HttpOnly: true, SameSite: http.SameSiteLaxMode, Secure: secure})
	writeJSON(w, 200, map[string]bool{"ok": true})
}
func (s *Server) session(w http.ResponseWriter, r *http.Request) {
	p := principalFrom(r)
	writeJSON(w, 200, map[string]string{"role": p.Role, "username": p.Username})
}
func (s *Server) serverInfo(w http.ResponseWriter, r *http.Request) {
	i, err := s.pal.Info(r.Context())
	state := s.shutdown.State()
	if err != nil {
		state = "unreachable"
	}
	writeJSON(w, 200, map[string]any{"name": i.ServerName, "description": i.Description, "version": i.Version, "worldGuid": i.WorldGUID, "state": state, "uptimeSec": i.Uptime, "panelVersion": PanelVersion})
}
func (s *Server) serverHealth(w http.ResponseWriter, r *http.Request) {
	rest, rcon, save, at := s.health.Snapshot()
	writeJSON(w, 200, map[string]any{"rest": rest, "rcon": rcon, "save": map[string]any{"state": save, "lastSyncAt": nullableTime(at)}})
}
func (s *Server) metricsCurrent(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, 200, s.poll.Current())
}
func (s *Server) metricsHistory(w http.ResponseWriter, r *http.Request) {
	window := r.URL.Query().Get("window")
	d := time.Hour
	switch window {
	case "", "1h":
		d = time.Hour
	case "24h":
		d = 24 * time.Hour
	case "7d":
		d = 7 * 24 * time.Hour
	default:
		writeError(w, 400, "invalid_window", "window must be 1h, 24h, or 7d")
		return
	}
	rows, err := s.store.MetricsHistory(r.Context(), time.Now().Add(-d), d > 24*time.Hour)
	if err != nil {
		internal(w, err)
		return
	}
	series := map[string]any{"t": []int64{}, "fps": []float64{}, "frameTimeMs": []float64{}, "players": []int{}}
	for _, m := range rows {
		series["t"] = append(series["t"].([]int64), m.At.Unix())
		series["fps"] = append(series["fps"].([]float64), m.FPS)
		series["frameTimeMs"] = append(series["frameTimeMs"].([]float64), m.FrameTimeMS)
		series["players"] = append(series["players"].([]int), m.Players)
	}
	writeJSON(w, 200, map[string]any{"series": series})
}
func (s *Server) players(w http.ResponseWriter, r *http.Request) {
	p, err := s.store.Players(r.Context(), s.poll.Online())
	if err != nil {
		internal(w, err)
		return
	}
	writeJSON(w, 200, playerViews(p))
}
func (s *Server) player(w http.ResponseWriter, r *http.Request) {
	p, err := s.store.PlayerByUID(r.Context(), chi.URLParam(r, "uid"))
	if errors.Is(err, sql.ErrNoRows) {
		writeError(w, 404, "not_found", "Player not found.")
		return
	}
	if err != nil {
		internal(w, err)
		return
	}
	p.Online = s.poll.Online()[p.UID]
	activity, err := s.store.PlayerActivity(r.Context(), p.UID, time.Now(), 20)
	if err != nil {
		internal(w, err)
		return
	}
	pals, err := s.store.Pals(r.Context(), p.UID)
	if err != nil {
		internal(w, err)
		return
	}
	v := playerView(p)
	// Keep the legacy sessions field bounded and aligned with the richer activity projection.
	// Both describe panel-observed connection intervals, not lifetime Palworld playtime.
	v["sessions"] = activity.RecentSessions
	v["activity"] = activity
	v["pals"] = pals
	writeJSON(w, 200, v)
}

// playerAvatar proxies a player's Steam profile avatar same-origin (the panel's
// CSP is img-src 'self'). 404 means "no avatar available" — the frontend falls
// back to the neutral placeholder, the same contract paldeckIcon uses.
func (s *Server) playerAvatar(w http.ResponseWriter, r *http.Request) {
	p, err := s.store.PlayerByUID(r.Context(), chi.URLParam(r, "uid"))
	if errors.Is(err, sql.ErrNoRows) {
		http.NotFound(w, r)
		return
	}
	if err != nil {
		internal(w, err)
		return
	}
	data, contentType, ok := s.avatars.Image(r.Context(), p.SteamID)
	if !ok {
		http.NotFound(w, r)
		return
	}
	w.Header().Set("Content-Type", contentType)
	w.Header().Set("Cache-Control", "private, max-age=86400")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	_, _ = w.Write(data)
}

func playerViews(ps []store.Player) []map[string]any {
	out := make([]map[string]any, 0, len(ps))
	for _, p := range ps {
		out = append(out, playerView(p))
	}
	return out
}
func playerView(p store.Player) map[string]any {
	var loc any
	if p.X != nil && p.Y != nil {
		loc = map[string]float64{"x": *p.X, "y": *p.Y}
	}
	return map[string]any{"uid": p.UID, "steamId": p.SteamID, "name": p.Name, "accountName": p.AccountName, "online": p.Online, "level": p.Level, "guildId": p.GuildID, "guildName": p.GuildName, "ping": p.Ping, "location": loc, "firstSeenAt": nullableTime(p.FirstSeenAt), "lastSeenAt": nullableTime(p.LastSeenAt), "playtimeSec": p.PlaytimeSec, "banned": p.Banned, "whitelisted": p.Whitelisted}
}
func (s *Server) moderate(w http.ResponseWriter, r *http.Request, kind string) {
	p, err := s.store.PlayerByUID(r.Context(), chi.URLParam(r, "uid"))
	if errors.Is(err, sql.ErrNoRows) {
		writeError(w, 404, "not_found", "Player not found.")
		return
	}
	if err != nil {
		internal(w, err)
		return
	}
	if p.SteamID == "" {
		writeError(w, 409, "identity_unavailable", "The player's REST userId is unknown.")
		return
	}
	var req struct {
		Message string `json:"message"`
	}
	if r.ContentLength != 0 && !decode(w, r, &req) {
		return
	}
	switch kind {
	case "kick":
		err = s.pal.Kick(r.Context(), p.SteamID, req.Message)
	case "ban":
		err = s.pal.Ban(r.Context(), p.SteamID, req.Message)
	case "unban":
		err = s.pal.Unban(r.Context(), p.SteamID)
	}
	if err != nil {
		upstream(w, err)
		return
	}
	if kind == "ban" || kind == "unban" {
		v := kind == "ban"
		_ = s.store.SetPlayerFlags(r.Context(), p.UID, &v, nil)
	}
	s.audit(r, "panel", kind+" player", map[string]any{"action": kind, "uid": p.UID, "userId": p.SteamID})
	writeJSON(w, 200, map[string]bool{"ok": true})
}
func (s *Server) kick(w http.ResponseWriter, r *http.Request)  { s.moderate(w, r, "kick") }
func (s *Server) ban(w http.ResponseWriter, r *http.Request)   { s.moderate(w, r, "ban") }
func (s *Server) unban(w http.ResponseWriter, r *http.Request) { s.moderate(w, r, "unban") }
func (s *Server) whitelist(w http.ResponseWriter, r *http.Request) {
	v, err := s.store.Whitelist(r.Context())
	if err != nil {
		internal(w, err)
		return
	}
	writeJSON(w, 200, v)
}
func (s *Server) putWhitelist(w http.ResponseWriter, r *http.Request) {
	var v []map[string]string
	if !decode(w, r, &v) {
		return
	}
	if err := s.store.ReplaceWhitelist(r.Context(), v); err != nil {
		writeError(w, 400, "invalid_request", err.Error())
		return
	}
	s.audit(r, "panel", "replaced whitelist", map[string]any{"count": len(v)})
	writeJSON(w, 200, v)
}
func (s *Server) guilds(w http.ResponseWriter, r *http.Request) {
	v, err := s.store.GuildJSON(r.Context())
	if err != nil {
		internal(w, err)
		return
	}
	writeJSON(w, 200, v)
}
func (s *Server) world(w http.ResponseWriter, r *http.Request) {
	v, err := s.store.WorldState(r.Context())
	if err != nil {
		internal(w, err)
		return
	}
	writeJSON(w, 200, map[string]any{"day": v.Day, "lastParseAt": nullableTime(v.LastParseAt), "parseDurationMs": v.ParseDurationMS, "stats": map[string]int{"players": v.Players, "pals": v.Pals, "guilds": v.Guilds, "skippedProps": v.SkippedProps}, "formatDrift": v.FormatDrift})
}
func (s *Server) parseWorld(w http.ResponseWriter, r *http.Request) {
	if err := s.poll.ParseNow(r.Context()); errors.Is(err, poller.ErrParseBusy) {
		writeError(w, 409, "parse_in_progress", err.Error())
		return
	} else if err != nil {
		internal(w, err)
		return
	}
	s.audit(r, "panel", "parsed world save", nil)
	writeJSON(w, 200, map[string]bool{"ok": true})
}
func (s *Server) announce(w http.ResponseWriter, r *http.Request) {
	var v struct {
		Message string `json:"message"`
	}
	if !decode(w, r, &v) {
		return
	}
	if strings.TrimSpace(v.Message) == "" {
		writeError(w, 400, "invalid_request", "message is required")
		return
	}
	if err := s.pal.Announce(r.Context(), v.Message); err != nil {
		upstream(w, err)
		return
	}
	s.audit(r, "panel", "announced message", map[string]any{"message": v.Message})
	writeJSON(w, 200, map[string]bool{"ok": true})
}
func (s *Server) save(w http.ResponseWriter, r *http.Request) {
	if err := s.pal.Save(r.Context()); err != nil {
		upstream(w, err)
		return
	}
	s.audit(r, "panel", "saved world", nil)
	writeJSON(w, 200, map[string]bool{"ok": true})
}
func (s *Server) startShutdown(w http.ResponseWriter, r *http.Request) {
	var v struct {
		WaitSec   int    `json:"waitSec"`
		Message   string `json:"message"`
		Countdown bool   `json:"countdown"`
	}
	if !decode(w, r, &v) {
		return
	}
	if v.WaitSec < 0 {
		writeError(w, 400, "invalid_request", "waitSec cannot be negative")
		return
	}
	if err := s.shutdown.Start(context.Background(), time.Duration(v.WaitSec)*time.Second, v.Message, v.Countdown); err != nil {
		writeError(w, 409, "shutdown_pending", err.Error())
		return
	}
	s.audit(r, "panel", "scheduled shutdown", v)
	writeJSON(w, 202, map[string]string{"state": s.shutdown.State()})
}
func (s *Server) cancelShutdown(w http.ResponseWriter, r *http.Request) {
	if !s.shutdown.Cancel() {
		writeError(w, 409, "no_countdown", "No cancellable countdown is pending.")
		return
	}
	s.audit(r, "panel", "cancelled shutdown", nil)
	writeJSON(w, 200, map[string]bool{"ok": true})
}
func (s *Server) consoleExec(w http.ResponseWriter, r *http.Request) {
	var v struct {
		Command string `json:"command"`
	}
	if !decode(w, r, &v) {
		return
	}
	if strings.TrimSpace(v.Command) == "" {
		writeError(w, 400, "invalid_request", "command is required")
		return
	}
	out, err := s.rcon.Exec(r.Context(), v.Command)
	s.health.SetRCON(map[bool]string{true: "ok", false: "error"}[err == nil])
	entry := store.ConsoleEntry{At: time.Now().UTC(), User: principalFrom(r).Username, Command: v.Command, Output: out, IsError: err != nil}
	if err != nil {
		entry.Output = err.Error()
	}
	_ = s.store.AddConsole(r.Context(), entry)
	s.audit(r, "panel", "executed console command", map[string]any{"command": v.Command, "error": err != nil})
	if err != nil {
		upstream(w, err)
		return
	}
	writeJSON(w, 200, map[string]string{"output": out})
}
func (s *Server) consoleLog(w http.ResponseWriter, r *http.Request) {
	n, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	v, err := s.store.ConsoleLog(r.Context(), n)
	if err != nil {
		internal(w, err)
		return
	}
	writeJSON(w, 200, v)
}
func (s *Server) savedCommands(w http.ResponseWriter, r *http.Request) {
	v, err := s.store.SavedCommands(r.Context())
	if err != nil {
		internal(w, err)
		return
	}
	writeJSON(w, 200, v)
}
func (s *Server) saveCommand(w http.ResponseWriter, r *http.Request) {
	var v store.SavedCommand
	if !decode(w, r, &v) {
		return
	}
	if v.Name == "" || v.Command == "" {
		writeError(w, 400, "invalid_request", "name and command are required")
		return
	}
	v, err := s.store.SaveCommand(r.Context(), v)
	if err != nil {
		internal(w, err)
		return
	}
	s.audit(r, "panel", "saved console command", map[string]any{"id": v.ID, "name": v.Name})
	writeJSON(w, 201, v)
}
func (s *Server) deleteCommand(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil {
		writeError(w, 400, "invalid_id", "id must be an integer")
		return
	}
	if err = s.store.DeleteCommand(r.Context(), id); err != nil {
		writeError(w, 404, "not_found", "Saved command not found.")
		return
	}
	s.audit(r, "panel", "deleted console command", map[string]any{"id": id})
	w.WriteHeader(204)
}
func (s *Server) events(w http.ResponseWriter, r *http.Request) {
	n, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	v, err := s.store.Events(r.Context(), n, r.URL.Query().Get("kind"))
	if err != nil {
		internal(w, err)
		return
	}
	writeJSON(w, 200, v)
}
func (s *Server) eventStream(w http.ResponseWriter, r *http.Request) {
	f, ok := w.(http.Flusher)
	if !ok {
		writeError(w, 500, "stream_unsupported", "Streaming is unavailable.")
		return
	}
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	ch, unsub := s.hub.subscribe()
	defer unsub()
	heartbeat := time.NewTicker(25 * time.Second)
	defer heartbeat.Stop()
	for {
		select {
		case <-r.Context().Done():
			return
		case <-s.hub.closed():
			return
		case m := <-ch:
			_, _ = fmt.Fprintf(w, "event: %s\ndata: %s\n\n", m.event, m.data)
			f.Flush()
		case <-heartbeat.C:
			_, _ = fmt.Fprint(w, ": heartbeat\n\n")
			f.Flush()
		}
	}
}
func (s *Server) audit(r *http.Request, kind, msg string, meta any) {
	e := store.Event{At: time.Now().UTC(), Kind: kind, Message: msg, Meta: map[string]any{"actor": principalFrom(r).Username, "detail": meta}}
	_ = s.store.AddEvent(r.Context(), e)
	s.hub.Publish("event", e)
}

func decode(w http.ResponseWriter, r *http.Request, v any) bool {
	d := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20))
	d.DisallowUnknownFields()
	if err := d.Decode(v); err != nil {
		writeError(w, 400, "invalid_json", "Request body is invalid: "+err.Error())
		return false
	}
	return true
}
func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}
func writeError(w http.ResponseWriter, status int, code, message string) {
	writeJSON(w, status, map[string]any{"error": map[string]string{"code": code, "message": message}})
}
func internal(w http.ResponseWriter, err error) { writeError(w, 500, "internal_error", err.Error()) }
func upstream(w http.ResponseWriter, err error) {
	status := 502
	code := "palworld_error"
	if palworld.IsKind(err, palworld.ErrorUnauthorized) {
		status = 502
		code = "palworld_unauthorized"
	}
	writeError(w, status, code, err.Error())
}
func nullableTime(t time.Time) any {
	if t.IsZero() {
		return nil
	}
	return t.UTC().Format(time.RFC3339)
}
