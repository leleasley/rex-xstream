package store

import (
	"context"
	"crypto/rand"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/leleasley/rex-xstream/internal/database"
	"github.com/leleasley/rex-xstream/internal/models"
	"github.com/leleasley/rex-xstream/internal/security"
)

func newStore(t *testing.T) *Store {
	t.Helper()
	key := make([]byte, 32)
	_, _ = rand.Read(key)
	cipher, err := security.NewCipher(key)
	if err != nil {
		t.Fatal(err)
	}
	db, err := database.Open(context.Background(), filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	return New(db, cipher)
}

func TestPlaylistCredentialsAreEncryptedAndMasked(t *testing.T) {
	st := newStore(t)
	ctx := context.Background()
	playlist, err := st.CreatePlaylist(ctx, PlaylistInput{Name: "Home", BaseURL: "http://example.com:80", Username: "alice", Password: "super-secret"})
	if err != nil {
		t.Fatal(err)
	}
	if playlist.Name != "Home" || !playlist.HasCredentials {
		t.Fatalf("playlist: %+v", playlist)
	}
	var username, password string
	if err := st.DB().QueryRow(`SELECT username_enc, password_enc FROM playlists WHERE id = ?`, playlist.ID).Scan(&username, &password); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(username, "alice") || strings.Contains(password, "super-secret") {
		t.Fatal("credentials were stored in plaintext")
	}
	auth, err := st.PlaylistAuth(ctx, playlist.ID)
	if err != nil {
		t.Fatal(err)
	}
	if auth.Username != "alice" || auth.Password != "super-secret" {
		t.Fatalf("decrypted credentials: %+v", auth)
	}
	libs, err := st.ListLibraries(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(libs) != 3 {
		t.Fatalf("got %d libraries, want 3", len(libs))
	}
}

func TestRenameKeepsStableDirectories(t *testing.T) {
	st := newStore(t)
	ctx := context.Background()
	playlist, err := st.CreatePlaylist(ctx, PlaylistInput{Name: "Old", BaseURL: "http://example.com", Username: "u", Password: "p"})
	if err != nil {
		t.Fatal(err)
	}
	updated, err := st.UpdatePlaylist(ctx, playlist.ID, PlaylistInput{Name: "New"})
	if err != nil {
		t.Fatal(err)
	}
	if updated.Name != "New" {
		t.Fatalf("name: %s", updated.Name)
	}
	libs, _ := st.ListLibraries(ctx)
	directories := map[string]string{}
	for _, lib := range libs {
		directories[lib.Kind] = lib.Directory
	}
	if directories["movies"] != movieDirectory(playlist.ID) || directories["series"] != seriesDirectory(playlist.ID) {
		t.Fatalf("directories changed: %+v", directories)
	}
	for _, lib := range libs {
		if lib.PlaylistID == playlist.ID && lib.Kind == "movies" && lib.Title != "New Movies" {
			t.Fatalf("title: %s", lib.Title)
		}
	}
}

func TestMultiplePlaylistsAreIndependent(t *testing.T) {
	st := newStore(t)
	ctx := context.Background()
	first, _ := st.CreatePlaylist(ctx, PlaylistInput{Name: "A", BaseURL: "http://a.example.com", Username: "u", Password: "p"})
	second, _ := st.CreatePlaylist(ctx, PlaylistInput{Name: "B", BaseURL: "http://b.example.com", Username: "u", Password: "p"})
	if first.ID == second.ID {
		t.Fatal("ids collided")
	}
	if err := st.SyncChannels(ctx, first.ID, []models.Channel{{ProviderID: "1", Name: "A1", Active: true}}); err != nil {
		t.Fatal(err)
	}
	if err := st.SyncChannels(ctx, second.ID, []models.Channel{{ProviderID: "2", Name: "B1", Active: true}}); err != nil {
		t.Fatal(err)
	}
	one, total, err := st.ListChannels(ctx, first.ID, "", 10, 0)
	if err != nil || total != 1 || one[0].Name != "A1" {
		t.Fatalf("first playlist: %v %d %v", one, total, err)
	}
}

func TestUpsertPreventsDuplicates(t *testing.T) {
	st := newStore(t)
	ctx := context.Background()
	playlist, _ := st.CreatePlaylist(ctx, PlaylistInput{Name: "A", BaseURL: "http://a.example.com", Username: "u", Password: "p"})
	movies := []models.Movie{{ProviderID: "10", Title: "Film", StreamURL: "http://provider/movie/10.mp4", Active: true}}
	if err := st.SyncMovies(ctx, playlist.ID, movies); err != nil {
		t.Fatal(err)
	}
	if err := st.SyncMovies(ctx, playlist.ID, movies); err != nil {
		t.Fatal(err)
	}
	var count int
	if err := st.DB().QueryRow(`SELECT COUNT(1) FROM movies WHERE playlist_id = ?`, playlist.ID).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 1 {
		t.Fatalf("got %d rows, want 1", count)
	}
}

func TestStreamURLIsEncryptedAndReturned(t *testing.T) {
	st := newStore(t)
	ctx := context.Background()
	playlist, _ := st.CreatePlaylist(ctx, PlaylistInput{Name: "A", BaseURL: "http://a.example.com", Username: "u", Password: "p"})
	if err := st.SyncMovies(ctx, playlist.ID, []models.Movie{{ProviderID: "1", Title: "F", StreamURL: "http://provider/movie/1.mp4", Container: "mp4", Active: true}}); err != nil {
		t.Fatal(err)
	}
	var stored string
	if err := st.DB().QueryRow(`SELECT stream_url FROM movies WHERE playlist_id = ?`, playlist.ID).Scan(&stored); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(stored, "provider/movie") {
		t.Fatal("stream URL was stored in plaintext")
	}
	var id int64
	_ = st.DB().QueryRow(`SELECT id FROM movies WHERE playlist_id = ?`, playlist.ID).Scan(&id)
	url, owner, err := st.StreamFor(ctx, "movie", playlist.ID, id)
	if err != nil || url != "http://provider/movie/1.mp4" || owner != playlist.ID {
		t.Fatalf("stream: %q %d %v", url, owner, err)
	}
	path, err := st.StreamPath(ctx, "movie", playlist.ID, id)
	if err != nil || path != "p1-movies/1.mp4" {
		t.Fatalf("path: %q %v", path, err)
	}
}

func TestProbeAndPushState(t *testing.T) {
	st := newStore(t)
	ctx := context.Background()
	playlist, _ := st.CreatePlaylist(ctx, PlaylistInput{Name: "A", BaseURL: "http://a.example.com", Username: "u", Password: "p"})
	if err := st.SyncChannels(ctx, playlist.ID, []models.Channel{{ProviderID: "1", Name: "One", StreamURL: "http://provider/live/1.m3u8", Active: true}}); err != nil {
		t.Fatal(err)
	}
	pending, err := st.PendingProbes(ctx, playlist.ID, 10)
	if err != nil || len(pending) != 1 || !pending[0].Live {
		t.Fatalf("pending: %+v %v", pending, err)
	}
	if err := st.SaveProbe(ctx, "channel", pending[0].ID, `{"streams":[{"codec_type":"video"}]}`, ""); err != nil {
		t.Fatal(err)
	}
	dirty, err := st.DirtyChannels(ctx, playlist.ID, 10)
	if err != nil || len(dirty) != 1 {
		t.Fatalf("dirty: %+v %v", dirty, err)
	}
	if err := st.MarkPushed(ctx, "channel", []int64{dirty[0].ID}, []string{"live:p1:1"}); err != nil {
		t.Fatal(err)
	}
	dirty, _ = st.DirtyChannels(ctx, playlist.ID, 10)
	if len(dirty) != 0 {
		t.Fatal("item should not be dirty after push")
	}
}

func TestGuideWindow(t *testing.T) {
	st := newStore(t)
	ctx := context.Background()
	playlist, _ := st.CreatePlaylist(ctx, PlaylistInput{Name: "A", BaseURL: "http://a.example.com", Username: "u", Password: "p"})
	items := []models.Programme{
		{ChannelEPGID: "bbc", Start: timeAt(1000), Stop: timeAt(2000), Title: "One"},
		{ChannelEPGID: "bbc", Start: timeAt(3000), Stop: timeAt(4000), Title: "Two"},
	}
	if err := st.ReplaceProgrammes(ctx, playlist.ID, items); err != nil {
		t.Fatal(err)
	}
	guide, err := st.Guide(ctx, playlist.ID, []string{"bbc"}, 0, 2_500_000)
	if err != nil {
		t.Fatal(err)
	}
	if len(guide["bbc"]) != 1 || guide["bbc"][0].Title != "One" {
		t.Fatalf("guide: %+v", guide)
	}
}

func TestDeleteCascadesContent(t *testing.T) {
	st := newStore(t)
	ctx := context.Background()
	playlist, _ := st.CreatePlaylist(ctx, PlaylistInput{Name: "A", BaseURL: "http://a.example.com", Username: "u", Password: "p"})
	if err := st.SyncChannels(ctx, playlist.ID, []models.Channel{{ProviderID: "1", Name: "One", Active: true}}); err != nil {
		t.Fatal(err)
	}
	if err := st.DeletePlaylist(ctx, playlist.ID); err != nil {
		t.Fatal(err)
	}
	var count int
	_ = st.DB().QueryRow(`SELECT COUNT(1) FROM channels WHERE playlist_id = ?`, playlist.ID).Scan(&count)
	if count != 0 {
		t.Fatal("channels survived playlist deletion")
	}
	libs, _ := st.ListLibraries(ctx)
	for _, lib := range libs {
		if lib.PlaylistID == playlist.ID {
			t.Fatal("playlist library survived deletion")
		}
	}
}

func TestNormalizeBaseURL(t *testing.T) {
	cases := map[string]string{
		"http://example.com:80/": "http://example.com:80",
		"example.com":            "http://example.com",
	}
	for input, want := range cases {
		got, err := NormalizeBaseURL(input)
		if err != nil || got != want {
			t.Fatalf("%q -> %q (%v), want %q", input, got, err, want)
		}
	}
	for _, input := range []string{"", "ftp://example.com", "http://user:pass@example.com", "http://example.com/?x=1"} {
		if _, err := NormalizeBaseURL(input); err == nil {
			t.Fatalf("%q should be rejected", input)
		}
	}
}

func timeAt(seconds int64) (t time.Time) {
	return time.Unix(seconds, 0).UTC()
}
