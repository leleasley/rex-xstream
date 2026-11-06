package api

import (
	"net/http"

	"github.com/leleasley/rex-xstream/internal/store"
)

type playlistRequest struct {
	Name     string `json:"name"`
	BaseURL  string `json:"base_url"`
	Username string `json:"username"`
	Password string `json:"password"`
	Enabled  *bool  `json:"enabled"`
}

func (s *Server) handleListPlaylists(w http.ResponseWriter, r *http.Request) {
	playlists, err := s.store.ListPlaylists(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, "database_error", "could not read playlists")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": playlists, "total": len(playlists)})
}

func (s *Server) handleCreatePlaylist(w http.ResponseWriter, r *http.Request) {
	var request playlistRequest
	if !decodeBody(w, r, &request) {
		return
	}
	playlist, err := s.store.CreatePlaylist(r.Context(), store.PlaylistInput{
		Name: request.Name, BaseURL: request.BaseURL, Username: request.Username,
		Password: request.Password, Enabled: request.Enabled,
	})
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_playlist", err.Error())
		return
	}
	if s.cfg.RexEnabled {
		if err := s.syncer.RegisterSource(r.Context()); err != nil {
			s.log.Warn("register source after creating playlist", "error", err)
		}
	}
	if s.cfg.SyncOnCreate {
		s.syncer.Start(r.Context(), playlist.ID)
	}
	writeJSON(w, http.StatusCreated, playlist)
}

func (s *Server) handleGetPlaylist(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(r, "id")
	if !ok {
		writeError(w, http.StatusBadRequest, "invalid_id", "playlist id is not valid")
		return
	}
	playlist, err := s.store.GetPlaylist(r.Context(), id)
	if err != nil {
		writeError(w, http.StatusNotFound, "not_found", "playlist not found")
		return
	}
	writeJSON(w, http.StatusOK, playlist)
}

func (s *Server) handleUpdatePlaylist(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(r, "id")
	if !ok {
		writeError(w, http.StatusBadRequest, "invalid_id", "playlist id is not valid")
		return
	}
	var request playlistRequest
	if !decodeBody(w, r, &request) {
		return
	}
	existing, existingErr := s.store.GetPlaylist(r.Context(), id)
	if existingErr != nil {
		writeError(w, http.StatusNotFound, "not_found", "playlist not found")
		return
	}
	playlist, err := s.store.UpdatePlaylist(r.Context(), id, store.PlaylistInput{
		Name: request.Name, BaseURL: request.BaseURL, Username: request.Username,
		Password: request.Password, Enabled: request.Enabled,
	})
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_playlist", err.Error())
		return
	}
	if s.cfg.RexEnabled {
		if request.Name != "" && request.Name != existing.Name {
			if err := s.syncer.RenameLibraries(r.Context(), existing.Name, playlist.Name); err != nil {
				s.log.Warn("rename rex libraries", "error", err)
			}
		}
		if err := s.syncer.RegisterSource(r.Context()); err != nil {
			s.log.Warn("register source after updating playlist", "error", err)
		}
	}
	writeJSON(w, http.StatusOK, playlist)
}

func (s *Server) handleDeletePlaylist(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(r, "id")
	if !ok {
		writeError(w, http.StatusBadRequest, "invalid_id", "playlist id is not valid")
		return
	}
	if _, err := s.store.GetPlaylist(r.Context(), id); err != nil {
		writeError(w, http.StatusNotFound, "not_found", "playlist not found")
		return
	}
	if err := s.syncer.DeletePlaylistItems(r.Context(), id); err != nil {
		writeError(w, http.StatusBadGateway, "rex_delete_failed", "rex items could not be removed: "+err.Error())
		return
	}
	if err := s.store.DeletePlaylist(r.Context(), id); err != nil {
		writeError(w, http.StatusInternalServerError, "database_error", "could not delete playlist")
		return
	}
	if s.cfg.RexEnabled {
		if err := s.syncer.RegisterSource(r.Context()); err != nil {
			s.log.Warn("register source after deleting playlist", "error", err)
		}
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handleTestPlaylist(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(r, "id")
	if !ok {
		writeError(w, http.StatusBadRequest, "invalid_id", "playlist id is not valid")
		return
	}
	playlist, err := s.syncer.Test(r.Context(), id)
	if err != nil {
		writeError(w, http.StatusBadGateway, "provider_error", err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"connection": "ok", "playlist": playlist})
}

func (s *Server) handleSyncPlaylist(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(r, "id")
	if !ok {
		writeError(w, http.StatusBadRequest, "invalid_id", "playlist id is not valid")
		return
	}
	if _, err := s.store.GetPlaylist(r.Context(), id); err != nil {
		writeError(w, http.StatusNotFound, "not_found", "playlist not found")
		return
	}
	if !s.syncer.Start(r.Context(), id) {
		writeError(w, http.StatusConflict, "sync_running", "a sync is already running for this playlist")
		return
	}
	writeJSON(w, http.StatusAccepted, map[string]any{"started": true, "playlist_id": id})
}

func (s *Server) handlePlaylistStatus(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(r, "id")
	if !ok {
		writeError(w, http.StatusBadRequest, "invalid_id", "playlist id is not valid")
		return
	}
	playlist, err := s.store.GetPlaylist(r.Context(), id)
	if err != nil {
		writeError(w, http.StatusNotFound, "not_found", "playlist not found")
		return
	}
	run, err := s.store.LatestSyncRun(r.Context(), id)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "database_error", "could not read sync status")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"playlist": playlist,
		"running":  s.syncer.Running(id),
		"sync":     run,
	})
}
