package server

import (
	"database/sql"
	"errors"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/palhelm/palhelm/internal/backup"
	"github.com/palhelm/palhelm/internal/gameconfig"
)

func backupID(w http.ResponseWriter, r *http.Request) (int64, bool) {
	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil || id < 1 {
		writeError(w, 400, "invalid_backup_id", "Backup id must be a positive integer.")
		return 0, false
	}
	return id, true
}
func backupErr(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, sql.ErrNoRows), errors.Is(err, os.ErrNotExist):
		writeError(w, 404, "backup_not_found", "The requested backup was not found.")
	case errors.Is(err, backup.ErrRunning):
		writeError(w, 409, "backup_running", err.Error())
	default:
		internal(w, err)
	}
}

func (s *Server) listBackups(w http.ResponseWriter, r *http.Request) {
	v, err := s.backups.List(r.Context())
	if err != nil {
		internal(w, err)
		return
	}
	writeJSON(w, 200, v)
}
func (s *Server) createBackup(w http.ResponseWriter, r *http.Request) {
	v, err := s.backups.Create(r.Context(), "manual")
	if err != nil {
		backupErr(w, err)
		return
	}
	writeJSON(w, 201, v)
}
func (s *Server) downloadBackup(w http.ResponseWriter, r *http.Request) {
	id, ok := backupID(w, r)
	if !ok {
		return
	}
	path, b, err := s.backups.Path(r.Context(), id)
	if err != nil {
		backupErr(w, err)
		return
	}
	f, err := os.Open(path)
	if err != nil {
		backupErr(w, err)
		return
	}
	defer f.Close()
	info, err := f.Stat()
	if err != nil {
		internal(w, err)
		return
	}
	w.Header().Set("Content-Type", "application/gzip")
	w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=%q", filepath.Base(b.File)))
	http.ServeContent(w, r, b.File, info.ModTime(), f)
}
func (s *Server) backupContents(w http.ResponseWriter, r *http.Request) {
	id, ok := backupID(w, r)
	if !ok {
		return
	}
	v, err := s.backups.Contents(r.Context(), id)
	if err != nil {
		backupErr(w, err)
		return
	}
	writeJSON(w, 200, v)
}
func (s *Server) dryRunRestore(w http.ResponseWriter, r *http.Request) {
	id, ok := backupID(w, r)
	if !ok {
		return
	}
	v, err := s.backups.DryRun(r.Context(), id)
	if err != nil {
		backupErr(w, err)
		return
	}
	writeJSON(w, 200, v)
}
func (s *Server) restoreBackup(w http.ResponseWriter, r *http.Request) {
	id, ok := backupID(w, r)
	if !ok {
		return
	}
	var req struct {
		Confirm string `json:"confirm"`
	}
	if !decode(w, r, &req) {
		return
	}
	if req.Confirm != "RESTORE" {
		writeError(w, 400, "confirmation_required", `confirm must be exactly "RESTORE".`)
		return
	}
	v, err := s.backups.Restore(r.Context(), id)
	if err != nil {
		if strings.Contains(err.Error(), "REST is reachable") {
			writeError(w, 409, "server_running", err.Error())
		} else {
			backupErr(w, err)
		}
		return
	}
	writeJSON(w, 200, v)
}
func (s *Server) deleteBackup(w http.ResponseWriter, r *http.Request) {
	id, ok := backupID(w, r)
	if !ok {
		return
	}
	if err := s.backups.Delete(r.Context(), id); err != nil {
		backupErr(w, err)
		return
	}
	s.audit(r, "backup", "backup deleted", map[string]any{"id": id})
	writeJSON(w, 200, map[string]bool{"ok": true})
}
func (s *Server) backupSchedule(w http.ResponseWriter, r *http.Request) {
	v, err := s.backups.GetSchedule(r.Context())
	if err != nil {
		internal(w, err)
		return
	}
	writeJSON(w, 200, v)
}
func (s *Server) putBackupSchedule(w http.ResponseWriter, r *http.Request) {
	var req backup.Schedule
	if !decode(w, r, &req) {
		return
	}
	v, err := s.backups.PutSchedule(r.Context(), req)
	if err != nil {
		writeError(w, 400, "invalid_schedule", err.Error())
		return
	}
	s.audit(r, "backup", "backup schedule updated", v)
	writeJSON(w, 200, v)
}

func (s *Server) getConfig(w http.ResponseWriter, r *http.Request) {
	v, err := s.gamecfg.Get(r.Context())
	if err != nil {
		internal(w, err)
		return
	}
	writeJSON(w, 200, v)
}
func (s *Server) rawConfig(w http.ResponseWriter, r *http.Request) {
	v, err := s.gamecfg.Raw()
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			writeError(w, 404, "config_not_found", "PalWorldSettings.ini was not found.")
		} else {
			internal(w, err)
		}
		return
	}
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.WriteHeader(200)
	_, _ = w.Write(v)
}
func (s *Server) putConfig(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Changes map[string]any `json:"changes"`
		Version string         `json:"version"`
	}
	if !decode(w, r, &req) {
		return
	}
	if len(req.Changes) == 0 {
		writeError(w, 400, "no_changes", "At least one config change is required.")
		return
	}
	if req.Version == "" {
		writeError(w, 400, "version_required", "The compose version from GET /config is required.")
		return
	}
	if err := s.gamecfg.UpdateVersion(req.Changes, req.Version); err != nil {
		if errors.Is(err, gameconfig.ErrReadOnly) {
			writeError(w, 409, "config_read_only", err.Error())
			return
		}
		if errors.Is(err, gameconfig.ErrConflict) {
			writeError(w, 409, "config_conflict", "The compose file changed after it was loaded. Reload configuration and retry.")
			return
		}
		writeError(w, 400, "invalid_config", err.Error())
		return
	}
	s.audit(r, "config", "compose configuration updated", map[string]any{"keys": mapKeys(req.Changes)})
	v, err := s.gamecfg.Get(r.Context())
	if err != nil {
		internal(w, err)
		return
	}
	writeJSON(w, 200, v)
}
func mapKeys(m map[string]any) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}
func (s *Server) applyConfig(w http.ResponseWriter, r *http.Request) {
	manual := "docker compose up -d " + s.cfg.GameService
	writeJSON(w, 501, map[string]any{"error": map[string]any{
		"code":          "docker_apply_disabled",
		"message":       "One-click Docker apply is intentionally disabled; run the manual command from the host directory containing the compose file.",
		"manualCommand": manual,
	}})
}
