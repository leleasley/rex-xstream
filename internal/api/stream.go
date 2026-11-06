package api

import (
	"database/sql"
	"fmt"
	"io"
	"net/http"
	"path"
	"strconv"
	"strings"
)

func (s *Server) handleStream(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		http.NotFound(w, r)
		return
	}
	kind, playlistID, id, err := parseStreamPath(r.URL.Path)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	if !s.validStreamAuth(r) {
		w.Header().Set("WWW-Authenticate", `Basic realm="rex-xstream"`)
		writeError(w, http.StatusUnauthorized, "unauthorized", "stream credentials are required")
		return
	}
	providerURL, owner, err := s.store.StreamFor(r.Context(), kind, playlistID, id)
	if err != nil {
		if err == sql.ErrNoRows {
			http.NotFound(w, r)
			return
		}
		writeError(w, http.StatusInternalServerError, "database_error", "could not resolve stream")
		return
	}
	if playlistID != 0 && owner != playlistID {
		http.NotFound(w, r)
		return
	}
	if providerURL == "" {
		http.NotFound(w, r)
		return
	}
	if kind == "channel" {
		// HLS playlists contain relative segment URLs. A redirect keeps every
		// segment flowing straight from the provider, as rex's IPTV docs describe.
		w.Header().Set("Cache-Control", "no-store")
		http.Redirect(w, r, providerURL, http.StatusFound)
		return
	}
	req, err := http.NewRequestWithContext(r.Context(), r.Method, providerURL, nil)
	if err != nil {
		writeError(w, http.StatusBadGateway, "stream_error", "could not build provider request")
		return
	}
	req.Header.Set("User-Agent", "rex-xstream/1.0")
	if value := r.Header.Get("Range"); value != "" {
		req.Header.Set("Range", value)
	}
	if value := r.Header.Get("Accept"); value != "" {
		req.Header.Set("Accept", value)
	}
	resp, err := s.client.Do(req)
	if err != nil {
		writeError(w, http.StatusBadGateway, "stream_error", "provider stream is unavailable")
		return
	}
	defer resp.Body.Close()
	for _, header := range []string{"Content-Type", "Content-Length", "Content-Range", "Accept-Ranges", "Last-Modified", "ETag"} {
		if value := resp.Header.Get(header); value != "" {
			w.Header().Set(header, value)
		}
	}
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(resp.StatusCode)
	if r.Method == http.MethodHead {
		return
	}
	_, _ = io.Copy(w, resp.Body)
}

func parseStreamPath(urlPath string) (string, int64, int64, error) {
	clean := strings.TrimPrefix(path.Clean("/"+urlPath), "/")
	segments := strings.Split(clean, "/")
	if len(segments) != 2 {
		return "", 0, 0, fmt.Errorf("not a stream path")
	}
	directory, filename := segments[0], segments[1]
	extension := path.Ext(filename)
	idPart := strings.TrimSuffix(filename, extension)
	id, err := strconv.ParseInt(idPart, 10, 64)
	if err != nil || id <= 0 {
		return "", 0, 0, fmt.Errorf("invalid stream id")
	}
	if directory == "live" {
		if extension != ".m3u8" {
			return "", 0, 0, fmt.Errorf("invalid live extension")
		}
		return "channel", 0, id, nil
	}
	var playlistID int64
	if _, err := fmt.Sscanf(directory, "p%d-movies", &playlistID); err == nil {
		return "movie", playlistID, id, nil
	}
	if _, err := fmt.Sscanf(directory, "p%d-shows", &playlistID); err == nil {
		return "episode", playlistID, id, nil
	}
	return "", 0, 0, fmt.Errorf("unknown library")
}
