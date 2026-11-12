package sync

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/leleasley/rex-xstream/internal/config"
	"github.com/leleasley/rex-xstream/internal/database"
	"github.com/leleasley/rex-xstream/internal/models"
	rexclient "github.com/leleasley/rex-xstream/internal/rex"
	"github.com/leleasley/rex-xstream/internal/security"
	"github.com/leleasley/rex-xstream/internal/store"
	"github.com/leleasley/rex-xstream/internal/xc"
)

type fakeProber struct {
	mu    sync.Mutex
	calls []string
}

func (f *fakeProber) Probe(_ context.Context, url string, live bool) (json.RawMessage, error) {
	f.mu.Lock()
	f.calls = append(f.calls, url)
	f.mu.Unlock()
	if live {
		return json.RawMessage(`{"format":{"format_name":"hls"},"streams":[{"codec_type":"video","codec_name":"h264"}]}`), nil
	}
	return json.RawMessage(`{"format":{"format_name":"matroska"},"streams":[{"codec_type":"video","codec_name":"h264"}]}`), nil
}

func xcServer(t *testing.T) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("username") != "user" || r.URL.Query().Get("password") != "pass" {
			http.Error(w, "bad credentials", http.StatusForbidden)
			return
		}
		if r.URL.Path == "/xmltv.php" {
			start := time.Now().UTC().Add(-time.Hour).Format("20060102150405 -0700")
			stop := time.Now().UTC().Add(time.Hour).Format("20060102150405 -0700")
			fmt.Fprintf(w, `<?xml version="1.0"?><tv><channel id="news"><display-name>News</display-name></channel><programme start="%s" stop="%s" channel="news"><title>Bulletin</title></programme></tv>`, start, stop)
			return
		}
		switch r.URL.Query().Get("action") {
		case "":
			fmt.Fprint(w, `{"user_info":{"username":"user","auth":1,"status":"Active","exp_date":"1893456000","max_connections":"2"},"server_info":{"server_software":"Xtream UI","timezone":"UTC"}}`)
		case "get_live_categories":
			fmt.Fprint(w, `[{"category_id":"1","category_name":"News"}]`)
		case "get_vod_categories":
			fmt.Fprint(w, `[{"category_id":"2","category_name":"Films"}]`)
		case "get_series_categories":
			fmt.Fprint(w, `[{"category_id":"3","category_name":"Shows"}]`)
		case "get_live_streams":
			fmt.Fprint(w, `[{"num":"1","name":"News One","stream_id":"10","epg_channel_id":"news","category_id":"1","tv_archive":1},{"num":"2","name":"News Two","stream_id":"11","epg_channel_id":"news","category_id":"1"}]`)
		case "get_vod_streams":
			fmt.Fprint(w, `[{"name":"Film One (2024)","stream_id":"20","tmdb":"123","container_extension":"mp4","category_id":"2"},{"name":"Film Two","stream_id":"21","container_extension":"mkv","category_id":"2"}]`)
		case "get_series":
			fmt.Fprint(w, `[{"name":"Show One - 2025","series_id":"30","tmdb":"456","cover":"https://img/show.jpg","category_id":"3"}]`)
		case "get_series_info":
			fmt.Fprint(w, `{"seasons":[{"season_number":1,"name":"Season 1"}],"episodes":{"1":[{"id":"ep1","episode_num":1,"title":"Pilot","container_extension":"mkv","season":1,"info":{"tmdb_id":999}},{"id":"ep2","episode_num":2,"title":"Second","container_extension":"mkv","season":1,"info":{}}]}}`)
		default:
			http.Error(w, "unknown", http.StatusBadRequest)
		}
	}))
}

func newTestSyncer(t *testing.T, xcURL, rexURL string, limits config.Config) (*Syncer, *store.Store, *store.PlaylistAuth, *fakeProber) {
	t.Helper()
	ctx := context.Background()
	key := make([]byte, 32)
	_, _ = rand.Read(key)
	cipher, _ := security.NewCipher(key)
	db, err := database.Open(ctx, filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	st := store.New(db, cipher)
	playlist, err := st.CreatePlaylist(ctx, store.PlaylistInput{Name: "Test", BaseURL: xcURL, Username: "user", Password: "pass"})
	if err != nil {
		t.Fatal(err)
	}
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	cfg := &config.Config{
		PublicURL: "http://addon:9981", RexURL: rexURL, RexSourceName: "rex-xstream",
		RexEnabled: true, RexPushBatch: 10, ProbeWorkers: 2, SyncConcurrency: 2,
		ProbeTimeout: time.Second, RequestTimeout: 5 * time.Second, EPGDays: 2,
		AllowPrivateHosts: true,
		SyncMaxLive:       limits.SyncMaxLive, SyncMaxMovies: limits.SyncMaxMovies, SyncMaxSeries: limits.SyncMaxSeries,
		APIToken: "0123456789abcdef", RexSourceUser: "rex", RexSourcePassword: "stream-password",
	}
	rex := rexclient.New(rexURL, cfg.RexSourceName, cfg.RexSourceUser, cfg.RexSourcePassword, cfg.RexPushBatch)
	prober := &fakeProber{}
	syncer := New(st, cfg, rex, prober, logger)
	auth := store.PlaylistAuth{ID: playlist.ID, Name: playlist.Name, BaseURL: xcURL, Username: "user", Password: "pass"}
	return syncer, st, &auth, prober
}

func TestRunSyncsCatalogProbesAndPushes(t *testing.T) {
	xc := xcServer(t)
	defer xc.Close()
	var registered map[string]any
	var pushed []rexclient.Item
	var pushedMu sync.Mutex
	rexServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodPut && strings.HasPrefix(r.URL.Path, "/rex/sources/"):
			_ = json.NewDecoder(r.Body).Decode(&registered)
			fmt.Fprint(w, `{}`)
		case r.Method == http.MethodPost && strings.Contains(r.URL.Path, "/rex/feed/"):
			var payload struct {
				Items []rexclient.Item `json:"items"`
			}
			_ = json.NewDecoder(r.Body).Decode(&payload)
			pushedMu.Lock()
			pushed = append(pushed, payload.Items...)
			pushedMu.Unlock()
			fmt.Fprintf(w, `{"created":%d,"updated":0,"held":0,"unchanged":0}`, len(payload.Items))
		case r.Method == http.MethodDelete:
			fmt.Fprint(w, `{"deleted":0}`)
		default:
			fmt.Fprint(w, `{}`)
		}
	}))
	defer rexServer.Close()
	syncer, st, auth, prober := newTestSyncer(t, xc.URL, rexServer.URL, config.Config{})

	if err := syncer.Run(context.Background(), auth.ID); err != nil {
		t.Fatalf("sync: %v", err)
	}
	run, err := st.LatestSyncRun(context.Background(), auth.ID)
	if err != nil || run == nil || run.Running || run.FinishedAt == nil || run.Error != "" {
		t.Fatalf("sync run: %+v %v", run, err)
	}
	counts, err := st.PlaylistCounts(context.Background(), auth.ID)
	if err != nil {
		t.Fatal(err)
	}
	if counts["channels"] != 2 || counts["movies"] != 2 || counts["series"] != 1 || counts["episodes"] != 2 {
		t.Fatalf("counts: %+v", counts)
	}
	if len(prober.calls) != 6 {
		t.Fatalf("probed %d urls, want 6", len(prober.calls))
	}
	if registered == nil {
		t.Fatal("rex source was not registered")
	}
	directories, _ := registered["directories"].([]any)
	if len(directories) != 3 {
		t.Fatalf("directories: %+v", registered["directories"])
	}
	pushedMu.Lock()
	pushedCount := len(pushed)
	pushedMu.Unlock()
	if pushedCount != 6 {
		t.Fatalf("pushed %d items, want 6", pushedCount)
	}
	programmes, err := st.ProgrammeCount(context.Background(), auth.ID)
	if err != nil || programmes == 0 {
		t.Fatalf("programmes: %d %v", programmes, err)
	}
	pushedMu.Lock()
	for _, item := range pushed {
		if item.Channel != nil && len(item.Channel.Guide) == 0 {
			t.Errorf("live item %s has no guide", item.ID)
		}
	}
	pushedMu.Unlock()
	// A second run must not duplicate rows or re-push unchanged items.
	pushedMu.Lock()
	pushed = nil
	pushedMu.Unlock()
	if err := syncer.Run(context.Background(), auth.ID); err != nil {
		t.Fatalf("second sync: %v", err)
	}
	pushedMu.Lock()
	second := len(pushed)
	pushedMu.Unlock()
	// The guide is re-sent on every sync, so at most the live channels are
	// pushed again. Movies and episodes must not be.
	if second > 2 {
		for _, item := range pushed {
			t.Logf("second push: id=%s path=%s kind=%s", item.ID, item.Path, item.Kind)
		}
		t.Fatalf("second run pushed %d items, want at most the 2 guide-bearing channels", second)
	}
}

func TestProgrammeFromFallbackGuide(t *testing.T) {
	title := base64.StdEncoding.EncodeToString([]byte("BBC News"))
	description := base64.StdEncoding.EncodeToString([]byte("The headlines"))
	programme, ok := programmeFromListing("BBC1.uk", xc.EPGListing{
		Title: title, Description: description,
		StartTimestamp: "1793988000", StopTimestamp: "1793991600",
	})
	if !ok {
		t.Fatal("listing was rejected")
	}
	if programme.Title != "BBC News" || programme.Description != "The headlines" {
		t.Fatalf("programme: %+v", programme)
	}
	if programme.Stop.Sub(programme.Start).Seconds() != 3600 {
		t.Fatalf("times: %v %v", programme.Start, programme.Stop)
	}
}

func TestSafeImageRejectsLocalAddresses(t *testing.T) {
	if safeImage("http://127.0.0.1/logo.png") != "" {
		t.Fatal("loopback artwork should be rejected")
	}
	if safeImage("file:///etc/passwd") != "" {
		t.Fatal("non-HTTP artwork should be rejected")
	}
	if safeImage("https://image.example.com/logo.png") == "" {
		t.Fatal("public artwork should be kept")
	}
}

func TestSplitYearCleansTitles(t *testing.T) {
	if title, year := splitYear("Nocturne - 2026"); title != "Nocturne" || year != 2026 {
		t.Fatalf("got %q %d", title, year)
	}
	if title, year := splitYear("Film (2024)"); title != "Film" || year != 2024 {
		t.Fatalf("got %q %d", title, year)
	}
}

func TestDuplicateMoviesAreCollapsed(t *testing.T) {
	items := []models.Movie{
		{ProviderID: "1", Title: "Film", Year: 2025, TMDBID: "10", Rating: "5.0"},
		{ProviderID: "2", Title: "Film", Year: 2025, TMDBID: "10", Rating: "8.0"},
		{ProviderID: "3", Title: "Film", Year: 2025, TMDBID: "10", Rating: "6.0"},
		{ProviderID: "4", Title: "Other", Year: 2024, TMDBID: "11"},
	}
	got := dedupeMovies(items)
	if len(got) != 2 {
		t.Fatalf("got %d movies, want 2", len(got))
	}
	if got[0].ProviderID != "2" {
		t.Fatalf("kept %s, want the best rated copy", got[0].ProviderID)
	}
}

func TestLimitsApply(t *testing.T) {
	xc := xcServer(t)
	defer xc.Close()
	rexServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { fmt.Fprint(w, `{}`) }))
	defer rexServer.Close()
	syncer, st, auth, _ := newTestSyncer(t, xc.URL, rexServer.URL, config.Config{SyncMaxLive: 1, SyncMaxMovies: 1, SyncMaxSeries: 1})
	if err := syncer.Run(context.Background(), auth.ID); err != nil {
		t.Fatal(err)
	}
	counts, _ := st.PlaylistCounts(context.Background(), auth.ID)
	if counts["channels"] != 1 || counts["movies"] != 1 || counts["series"] != 1 {
		t.Fatalf("limits: %+v", counts)
	}
}

func TestUnreachableProviderLeavesPlaylistError(t *testing.T) {
	rexServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { fmt.Fprint(w, `{}`) }))
	defer rexServer.Close()
	syncer, st, auth, _ := newTestSyncer(t, "http://127.0.0.1:1", rexServer.URL, config.Config{})
	err := syncer.Run(context.Background(), auth.ID)
	if err == nil {
		t.Fatal("expected a sync error")
	}
	playlist, _ := st.GetPlaylist(context.Background(), auth.ID)
	if playlist.Status != "error" || playlist.LastSyncError == "" {
		t.Fatalf("playlist: %+v", playlist)
	}
}

func TestEPGFallsBackWhenXMLTVUnavailable(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/xmltv.php" {
			http.Error(w, "unavailable", http.StatusNotFound)
			return
		}
		if r.URL.Query().Get("action") == "get_simple_data_table" {
			fmt.Fprintf(w, `{"epg_listings":[{"title":"QnVsbGV0aW4=","start_timestamp":"%d","stop_timestamp":"%d"}]}`, time.Now().Add(-time.Hour).Unix(), time.Now().Add(time.Hour).Unix())
			return
		}
		http.NotFound(w, r)
	}))
	defer server.Close()
	s, st, auth, _ := newTestSyncer(t, server.URL, "", config.Config{})
	if err := st.SyncChannels(context.Background(), auth.ID, []models.Channel{{ProviderID: "10", Name: "News", EPGID: "news"}}); err != nil {
		t.Fatal(err)
	}
	client, err := xc.New(server.URL, "user", "pass", time.Second, true)
	if err != nil {
		t.Fatal(err)
	}
	if err := s.syncEPG(context.Background(), client, auth.ID); err != nil {
		t.Fatal(err)
	}
	count, err := st.ProgrammeCount(context.Background(), auth.ID)
	if err != nil || count != 1 {
		t.Fatalf("programmes = %d, err = %v", count, err)
	}
}
