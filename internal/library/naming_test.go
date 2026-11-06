package library

import (
	"strings"
	"testing"
)

func TestDirectoryNamesAreStable(t *testing.T) {
	if MovieDirectory(7) != "p7-movies" {
		t.Fatalf("movie directory changed: %s", MovieDirectory(7))
	}
	if SeriesDirectory(7) != "p7-shows" {
		t.Fatalf("series directory changed: %s", SeriesDirectory(7))
	}
	if LiveDirectory != "live" {
		t.Fatalf("live directory changed: %s", LiveDirectory)
	}
}

func TestTitlesFollowPlaylistName(t *testing.T) {
	if MovieTitle("Sky UK") != "Sky UK Movies" {
		t.Fatal("movie title")
	}
	if SeriesTitle("Sky UK") != "Sky UK Series" {
		t.Fatal("series title")
	}
	if LiveTitle() != "Live TV" {
		t.Fatal("live title")
	}
}

func TestPathsUseStableIDs(t *testing.T) {
	movie := MoviePath(3, 42, "mkv")
	if movie != "p3-movies/42.mkv" {
		t.Fatalf("movie path: %s", movie)
	}
	episode := EpisodePath(3, 99, "mp4")
	if episode != "p3-shows/99.mp4" {
		t.Fatalf("episode path: %s", episode)
	}
	live := LivePath(12)
	if live != "live/12.m3u8" {
		t.Fatalf("live path: %s", live)
	}
}

func TestExtensionFallback(t *testing.T) {
	if !strings.HasSuffix(MoviePath(1, 2, "exe"), ".mp4") {
		t.Fatal("unexpected extension should fall back")
	}
	if !strings.HasSuffix(EpisodePath(1, 2, ""), ".mkv") {
		t.Fatal("empty extension should fall back")
	}
}
