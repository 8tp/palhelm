package server

import (
	"net/http"
	"time"
)

func (s *Server) activity(w http.ResponseWriter, r *http.Request) {
	windowName := r.URL.Query().Get("window")
	if windowName == "" {
		windowName = "7d"
	}
	var window, bucket time.Duration
	switch windowName {
	case "24h":
		window, bucket = 24*time.Hour, time.Hour
	case "7d":
		window, bucket = 7*24*time.Hour, 6*time.Hour
	case "30d":
		window, bucket = 30*24*time.Hour, 24*time.Hour
	default:
		writeError(w, http.StatusBadRequest, "invalid_window", "window must be 24h, 7d, or 30d")
		return
	}
	result, err := s.store.ServerActivity(r.Context(), time.Now(), window, bucket, windowName, 25)
	if err != nil {
		internal(w, err)
		return
	}
	writeJSON(w, http.StatusOK, result)
}
