package api

import (
	"net/http"
	"strconv"
	"strings"
)

func (s *Server) handleLiveCategories(w http.ResponseWriter, r *http.Request) {
	s.categories(w, r, "live")
}
func (s *Server) handleMovieCategories(w http.ResponseWriter, r *http.Request) {
	s.categories(w, r, "movies")
}
func (s *Server) handleSeriesCategories(w http.ResponseWriter, r *http.Request) {
	s.categories(w, r, "series")
}

func (s *Server) categories(w http.ResponseWriter, r *http.Request, kind string) {
	id, ok := pathID(r, "id")
	if !ok {
		writeError(w, http.StatusBadRequest, "invalid_id", "playlist id is not valid")
		return
	}
	items, err := s.store.ListCategories(r.Context(), id, kind)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "database_error", "could not read categories")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": items, "total": len(items)})
}

func (s *Server) handleLiveChannels(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(r, "id")
	if !ok {
		writeError(w, http.StatusBadRequest, "invalid_id", "playlist id is not valid")
		return
	}
	limit := queryInt(r, "limit", 100, 1000)
	offset := queryInt(r, "offset", 0, 1000000)
	items, total, err := s.store.ListChannels(r.Context(), id, r.URL.Query().Get("category"), limit, offset)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "database_error", "could not read channels")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": items, "total": total, "limit": limit, "offset": offset})
}

func (s *Server) handleMovies(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(r, "id")
	if !ok {
		writeError(w, http.StatusBadRequest, "invalid_id", "playlist id is not valid")
		return
	}
	limit := queryInt(r, "limit", 100, 1000)
	offset := queryInt(r, "offset", 0, 1000000)
	items, total, err := s.store.ListMovies(r.Context(), id, r.URL.Query().Get("category"), limit, offset)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "database_error", "could not read movies")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": items, "total": total, "limit": limit, "offset": offset})
}

func (s *Server) handleSeries(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(r, "id")
	if !ok {
		writeError(w, http.StatusBadRequest, "invalid_id", "playlist id is not valid")
		return
	}
	limit := queryInt(r, "limit", 100, 1000)
	offset := queryInt(r, "offset", 0, 1000000)
	items, total, err := s.store.ListSeries(r.Context(), id, r.URL.Query().Get("category"), limit, offset)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "database_error", "could not read series")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": items, "total": total, "limit": limit, "offset": offset})
}

func (s *Server) handleSeriesDetail(w http.ResponseWriter, r *http.Request) {
	playlistID, seriesID, ok := s.seriesIDs(r)
	if !ok {
		writeError(w, http.StatusBadRequest, "invalid_id", "playlist or series id is not valid")
		return
	}
	series, err := s.store.GetSeries(r.Context(), playlistID, seriesID)
	if err != nil {
		writeError(w, http.StatusNotFound, "not_found", "series not found")
		return
	}
	seasons, err := s.store.Seasons(r.Context(), seriesID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "database_error", "could not read seasons")
		return
	}
	episodes, err := s.store.Episodes(r.Context(), seriesID, -1)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "database_error", "could not read episodes")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"series": series, "seasons": seasons, "episodes": episodes})
}

func (s *Server) handleSeasons(w http.ResponseWriter, r *http.Request) {
	_, seriesID, ok := s.seriesIDs(r)
	if !ok {
		writeError(w, http.StatusBadRequest, "invalid_id", "series id is not valid")
		return
	}
	seasons, err := s.store.Seasons(r.Context(), seriesID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "database_error", "could not read seasons")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": seasons, "total": len(seasons)})
}

func (s *Server) handleEpisodes(w http.ResponseWriter, r *http.Request) {
	_, seriesID, ok := s.seriesIDs(r)
	if !ok {
		writeError(w, http.StatusBadRequest, "invalid_id", "series id is not valid")
		return
	}
	season := -1
	if raw := r.URL.Query().Get("season"); raw != "" {
		if parsed, err := strconv.Atoi(raw); err == nil && parsed >= 0 {
			season = parsed
		}
	}
	episodes, err := s.store.Episodes(r.Context(), seriesID, season)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "database_error", "could not read episodes")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": episodes, "total": len(episodes)})
}

func (s *Server) seriesIDs(r *http.Request) (int64, int64, bool) {
	playlistID, ok := pathID(r, "id")
	if !ok {
		return 0, 0, false
	}
	seriesID, ok := pathID(r, "sid")
	if !ok {
		return 0, 0, false
	}
	return playlistID, seriesID, true
}

func (s *Server) handleEPG(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(r, "id")
	if !ok {
		writeError(w, http.StatusBadRequest, "invalid_id", "playlist id is not valid")
		return
	}
	channel := strings.TrimSpace(r.URL.Query().Get("channel"))
	if channel == "" {
		writeError(w, http.StatusBadRequest, "missing_channel", "channel is required")
		return
	}
	limit := queryInt(r, "limit", 100, 1000)
	items, err := s.store.Programmes(r.Context(), id, channel, limit)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "database_error", "could not read guide")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"channel": channel, "items": items, "total": len(items)})
}

func (s *Server) handleLibraries(w http.ResponseWriter, r *http.Request) {
	items, err := s.store.ListLibraries(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, "database_error", "could not read libraries")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": items, "total": len(items)})
}

func (s *Server) handleLibrary(w http.ResponseWriter, r *http.Request) {
	directory := strings.TrimSpace(r.PathValue("directory"))
	if directory == "" {
		writeError(w, http.StatusBadRequest, "invalid_directory", "library directory is required")
		return
	}
	payload, err := s.store.LibraryItems(r.Context(), directory)
	if err != nil {
		writeError(w, http.StatusNotFound, "not_found", "library not found")
		return
	}
	writeJSON(w, http.StatusOK, payload)
}

func (s *Server) handleResolveStream(w http.ResponseWriter, r *http.Request) {
	var request struct {
		PlaylistID int64  `json:"playlist_id"`
		Kind       string `json:"kind"`
		ID         int64  `json:"id"`
	}
	if !decodeBody(w, r, &request) {
		return
	}
	path, err := s.store.StreamPath(r.Context(), request.Kind, request.PlaylistID, request.ID)
	if err != nil {
		writeError(w, http.StatusNotFound, "not_found", "stream not found")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"kind": request.Kind,
		"id":   request.ID,
		"path": path,
		"url":  strings.TrimRight(s.cfg.PublicURL, "/") + "/" + path,
	})
}
