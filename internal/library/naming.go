package library

import (
	"crypto/sha1"
	"encoding/hex"
	"fmt"
	"regexp"
	"strings"
)

const LiveDirectory = "live"

var unsafe = regexp.MustCompile(`[^a-zA-Z0-9._-]+`)

func MovieDirectory(playlistID int64) string  { return fmt.Sprintf("p%d-movies", playlistID) }
func SeriesDirectory(playlistID int64) string { return fmt.Sprintf("p%d-shows", playlistID) }
func LiveTitle() string                       { return "Live TV" }

func MovieTitle(playlist string) string {
	return strings.TrimSpace(playlist) + " Movies"
}

func SeriesTitle(playlist string) string {
	return strings.TrimSpace(playlist) + " Series"
}

// Slug turns a provider value into a safe path segment and adds a short hash,
// so two different provider ids cannot collide after cleaning.
func Slug(value string) string {
	cleaned := strings.Trim(unsafe.ReplaceAllString(strings.ToLower(value), "-"), "-.")
	if cleaned == "" {
		cleaned = "item"
	}
	if len(cleaned) > 80 {
		cleaned = cleaned[:80]
	}
	sum := sha1.Sum([]byte(value))
	return cleaned + "-" + hex.EncodeToString(sum[:4])
}

func MovieItemID(playlistID int64, providerID string) string {
	return fmt.Sprintf("movie:p%d:%s", playlistID, providerID)
}

func EpisodeItemID(playlistID int64, seriesID, season, episode int) string {
	return fmt.Sprintf("episode:p%d:%d:s%de%d", playlistID, seriesID, season, episode)
}

func LiveItemID(playlistID int64, providerID string) string {
	return fmt.Sprintf("live:p%d:%s", playlistID, providerID)
}

func MoviePath(playlistID, movieID int64, container string) string {
	ext := extension(container, "mp4")
	return fmt.Sprintf("%s/%d.%s", MovieDirectory(playlistID), movieID, ext)
}

func EpisodePath(playlistID, episodeID int64, container string) string {
	ext := extension(container, "mkv")
	return fmt.Sprintf("%s/%d.%s", SeriesDirectory(playlistID), episodeID, ext)
}

func LivePath(channelID int64) string {
	return fmt.Sprintf("%s/%d.m3u8", LiveDirectory, channelID)
}

func extension(value, fallback string) string {
	value = strings.ToLower(strings.TrimPrefix(strings.TrimSpace(value), "."))
	switch value {
	case "mp4", "mkv", "avi", "mov", "m4v", "ts", "m3u8", "webm":
		return value
	default:
		return fallback
	}
}
