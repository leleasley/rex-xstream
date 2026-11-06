package api

import (
	"context"
	"crypto/rand"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/leleasley/rex-xstream/internal/config"
	"github.com/leleasley/rex-xstream/internal/database"
	"github.com/leleasley/rex-xstream/internal/models"
	"github.com/leleasley/rex-xstream/internal/security"
	"github.com/leleasley/rex-xstream/internal/store"
	xsync "github.com/leleasley/rex-xstream/internal/sync"
)

func testServer(t *testing.T) (*httptest.Server, *store.Store, *config.Config) {
	t.Helper()
	key := make([]byte, 32)
	_, _ = rand.Read(key)
	cipher, _ := security.NewCipher(key)
	db, err := database.Open(context.Background(), filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	st := store.New(db, cipher)
	cfg := &config.Config{
		PublicURL: "http://addon:9981", APIToken: "token-0123456789", RexEnabled: false,
		SyncOnCreate: false, RateLimitPerMinute: 1000, RexSourceUser: "rex", RexSourcePassword: "stream-pass",
		RequestTimeout: 5 * time.Second, ProbeWorkers: 1, SyncConcurrency: 1, RexPushBatch: 10,
	}
	syncer := xsync.New(st, cfg, nil, nil, testLogger())
	srv := httptest.NewServer(New(cfg, st, syncer, testLogger()).Handler())
	t.Cleanup(srv.Close)
	return srv, st, cfg
}

func testLogger() *slog.Logger { return slog.New(slog.NewTextHandler(io.Discard, nil)) }

func request(t *testing.T, method, url, token string, body any) *http.Response {
	t.Helper()
	var reader io.Reader
	if body != nil {
		raw, _ := json.Marshal(body)
		reader = strings.NewReader(string(raw))
	}
	req, err := http.NewRequest(method, url, reader)
	if err != nil {
		t.Fatal(err)
	}
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	return resp
}

func TestHealthIsPublicAndAdminNeedsToken(t *testing.T) {
	srv, _, _ := testServer(t)
	resp := request(t, http.MethodGet, srv.URL+"/api/v1/health", "", nil)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("health: %d", resp.StatusCode)
	}
	resp.Body.Close()
	resp = request(t, http.MethodGet, srv.URL+"/api/v1/playlists", "", nil)
	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("unauthorized: %d", resp.StatusCode)
	}
	resp.Body.Close()
}

func TestPlaylistLifecycleAndRenaming(t *testing.T) {
	srv, _, _ := testServer(t)
	body := map[string]any{"name": "Sky UK", "base_url": "http://example.com:80", "username": "user", "password": "pass"}
	resp := request(t, http.MethodPost, srv.URL+"/api/v1/playlists", "token-0123456789", body)
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("create: %d", resp.StatusCode)
	}
	var created models.Playlist
	_ = json.NewDecoder(resp.Body).Decode(&created)
	resp.Body.Close()
	if created.ID == 0 {
		t.Fatal("no playlist id")
	}
	resp = request(t, http.MethodPatch, fmt.Sprintf("%s/api/v1/playlists/%d", srv.URL, created.ID), "token-0123456789", map[string]any{"name": "Sky UK 2"})
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("patch: %d", resp.StatusCode)
	}
	var updated models.Playlist
	_ = json.NewDecoder(resp.Body).Decode(&updated)
	resp.Body.Close()
	if updated.Name != "Sky UK 2" {
		t.Fatalf("name: %s", updated.Name)
	}
	resp = request(t, http.MethodGet, srv.URL+"/api/v1/libraries", "token-0123456789", nil)
	var libraries struct {
		Items []models.Library `json:"items"`
	}
	_ = json.NewDecoder(resp.Body).Decode(&libraries)
	resp.Body.Close()
	found := false
	for _, lib := range libraries.Items {
		if lib.PlaylistID == created.ID && lib.Kind == "movies" && lib.Title == "Sky UK 2 Movies" {
			found = true
		}
	}
	if !found {
		t.Fatalf("renamed library not found: %+v", libraries.Items)
	}
	resp = request(t, http.MethodDelete, fmt.Sprintf("%s/api/v1/playlists/%d", srv.URL, created.ID), "token-0123456789", nil)
	if resp.StatusCode != http.StatusNoContent {
		t.Fatalf("delete: %d", resp.StatusCode)
	}
	resp.Body.Close()
}

func TestContentEndpoints(t *testing.T) {
	srv, st, _ := testServer(t)
	ctx := context.Background()
	playlist, _ := st.CreatePlaylist(ctx, store.PlaylistInput{Name: "A", BaseURL: "http://example.com", Username: "u", Password: "p"})
	if err := st.SyncChannels(ctx, playlist.ID, []models.Channel{{ProviderID: "1", Name: "News", Active: true}}); err != nil {
		t.Fatal(err)
	}
	if err := st.SyncMovies(ctx, playlist.ID, []models.Movie{{ProviderID: "2", Title: "Film", Active: true}}); err != nil {
		t.Fatal(err)
	}
	base := fmt.Sprintf("%s/api/v1/playlists/%d", srv.URL, playlist.ID)
	for path, want := range map[string]string{
		"/live/channels": "News",
		"/movies":        "Film",
	} {
		resp := request(t, http.MethodGet, base+path, "token-0123456789", nil)
		if resp.StatusCode != http.StatusOK {
			t.Fatalf("%s: %d", path, resp.StatusCode)
		}
		var body struct {
			Items []map[string]any `json:"items"`
		}
		_ = json.NewDecoder(resp.Body).Decode(&body)
		resp.Body.Close()
		if len(body.Items) != 1 || !strings.Contains(fmt.Sprint(body.Items[0]), want) {
			t.Fatalf("%s: %+v", path, body.Items)
		}
	}
}

func TestStreamProxyAndLiveRedirect(t *testing.T) {
	provider := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/live.m3u8" {
			w.Header().Set("Content-Type", "application/vnd.apple.mpegurl")
			fmt.Fprint(w, "#EXTM3U")
			return
		}
		w.Header().Set("Content-Type", "video/mp4")
		w.Header().Set("Accept-Ranges", "bytes")
		w.Header().Set("Content-Range", "bytes 0-3/10")
		w.WriteHeader(http.StatusPartialContent)
		_, _ = w.Write([]byte("data"))
	}))
	defer provider.Close()
	srv, st, _ := testServer(t)
	ctx := context.Background()
	playlist, _ := st.CreatePlaylist(ctx, store.PlaylistInput{Name: "A", BaseURL: provider.URL, Username: "u", Password: "p"})
	if err := st.SyncMovies(ctx, playlist.ID, []models.Movie{{ProviderID: "2", Title: "Film", StreamURL: provider.URL + "/movie.mp4", Container: "mp4", Active: true}}); err != nil {
		t.Fatal(err)
	}
	if err := st.SyncChannels(ctx, playlist.ID, []models.Channel{{ProviderID: "1", Name: "Live", StreamURL: provider.URL + "/live.m3u8", Active: true}}); err != nil {
		t.Fatal(err)
	}
	var movieID, channelID int64
	_ = st.DB().QueryRow(`SELECT id FROM movies WHERE playlist_id = ?`, playlist.ID).Scan(&movieID)
	_ = st.DB().QueryRow(`SELECT id FROM channels WHERE playlist_id = ?`, playlist.ID).Scan(&channelID)
	req, _ := http.NewRequest(http.MethodGet, fmt.Sprintf("%s/p%d-movies/%d.mp4", srv.URL, playlist.ID, movieID), nil)
	req.SetBasicAuth("rex", "stream-pass")
	req.Header.Set("Range", "bytes=0-3")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != http.StatusPartialContent {
		t.Fatalf("proxy status: %d", resp.StatusCode)
	}
	raw, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	if string(raw) != "data" {
		t.Fatalf("body: %q", raw)
	}
	// The live channel answers with a redirect so HLS segments stay at the provider.
	client := &http.Client{CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	req, _ = http.NewRequest(http.MethodGet, fmt.Sprintf("%s/live/%d.m3u8", srv.URL, channelID), nil)
	req.SetBasicAuth("rex", "stream-pass")
	resp, err = client.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusFound {
		t.Fatalf("live status: %d", resp.StatusCode)
	}
}

func TestStreamRequiresAuthAndRejectsTraversal(t *testing.T) {
	srv, _, _ := testServer(t)
	for _, path := range []string{"/p1-movies/1.mp4", "/live/1.m3u8"} {
		resp := request(t, http.MethodGet, srv.URL+path, "", nil)
		if resp.StatusCode != http.StatusUnauthorized {
			t.Fatalf("%s unauthenticated: %d", path, resp.StatusCode)
		}
		resp.Body.Close()
	}
	resp := request(t, http.MethodGet, srv.URL+"/../etc/passwd", "", nil)
	if resp.StatusCode == http.StatusOK {
		t.Fatal("path traversal answered 200")
	}
	resp.Body.Close()
}
