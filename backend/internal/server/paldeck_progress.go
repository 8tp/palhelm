package server

import (
	"database/sql"
	"errors"
	"net/http"

	"github.com/go-chi/chi/v5"
)

func (s *Server) serverPaldeck(w http.ResponseWriter, r *http.Request) {
	result, err := s.store.ServerPaldeck(r.Context())
	if err != nil {
		internal(w, err)
		return
	}
	writeJSON(w, http.StatusOK, result)
}

func (s *Server) playerPaldeck(w http.ResponseWriter, r *http.Request) {
	rawUID := chi.URLParam(r, "uid")
	if !integrationUIDPattern.MatchString(rawUID) {
		writeError(w, http.StatusNotFound, "not_found", "Player not found.")
		return
	}
	result, err := s.store.PlayerPaldeck(r.Context(), rawUID)
	if errors.Is(err, sql.ErrNoRows) {
		writeError(w, http.StatusNotFound, "not_found", "Player not found.")
		return
	}
	if err != nil {
		internal(w, err)
		return
	}
	writeJSON(w, http.StatusOK, result)
}
