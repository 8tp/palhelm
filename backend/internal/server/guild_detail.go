package server

import (
	"database/sql"
	"errors"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
)

func (s *Server) guildDetail(w http.ResponseWriter, r *http.Request) {
	rawID := chi.URLParam(r, "id")
	if !integrationUIDPattern.MatchString(rawID) {
		writeError(w, http.StatusNotFound, "not_found", "Guild not found.")
		return
	}
	result, err := s.store.GuildDetail(r.Context(), rawID, time.Now())
	if errors.Is(err, sql.ErrNoRows) {
		writeError(w, http.StatusNotFound, "not_found", "Guild not found.")
		return
	}
	if err != nil {
		internal(w, err)
		return
	}
	online := s.poll.Online()
	for index := range result.Members {
		result.Members[index].Online = online[result.Members[index].UID]
	}
	writeJSON(w, http.StatusOK, result)
}
