package epg

import (
	"bufio"
	"compress/gzip"
	"encoding/xml"
	"fmt"
	"io"
	"strconv"
	"strings"
	"time"

	"github.com/leleasley/rex-xstream/internal/models"
)

// Parse reads an XMLTV document and returns the programmes inside a window.
// It streams the document, so a 34 MB guide does not become a 34 MB string.
func Parse(reader io.Reader, from, to time.Time) ([]models.Programme, error) {
	buffered := bufio.NewReaderSize(reader, 64*1024)
	head, _ := buffered.Peek(2)
	var source io.Reader = buffered
	if len(head) == 2 && head[0] == 0x1f && head[1] == 0x8b {
		gz, err := gzip.NewReader(buffered)
		if err != nil {
			return nil, fmt.Errorf("guide gzip: %w", err)
		}
		defer gz.Close()
		source = gz
	}
	decoder := xml.NewDecoder(source)
	decoder.Strict = false
	var out []models.Programme
	for {
		token, err := decoder.Token()
		if err == io.EOF {
			break
		}
		if err != nil {
			return out, fmt.Errorf("guide XML: %w", err)
		}
		start, ok := token.(xml.StartElement)
		if !ok || start.Name.Local != "programme" {
			continue
		}
		var raw programme
		if err := decoder.DecodeElement(&raw, &start); err != nil {
			continue
		}
		startAt, err := parseXMLTVTime(raw.Start)
		if err != nil {
			continue
		}
		stopAt, err := parseXMLTVTime(raw.Stop)
		if err != nil || !stopAt.After(startAt) {
			continue
		}
		if stopAt.Before(from) || startAt.After(to) {
			continue
		}
		programme := models.Programme{
			ChannelEPGID: strings.TrimSpace(raw.Channel),
			Start:        startAt,
			Stop:         stopAt,
			Title:        strings.TrimSpace(raw.Title),
			Description:  strings.TrimSpace(raw.Description),
			Category:     strings.TrimSpace(raw.Category),
			Icon:         strings.TrimSpace(raw.Icon.Src),
		}
		if programme.Title == "" {
			continue
		}
		if season, episode, ok := parseXMLTVNumbers(raw.EpisodeNum); ok {
			programme.Season = season
			programme.Episode = episode
		}
		out = append(out, programme)
	}
	return out, nil
}

type programme struct {
	Channel     string `xml:"channel,attr"`
	Start       string `xml:"start,attr"`
	Stop        string `xml:"stop,attr"`
	Title       string `xml:"title"`
	Description string `xml:"desc"`
	Category    string `xml:"category"`
	Icon        icon   `xml:"icon"`
	EpisodeNum  string `xml:"episode-num"`
}

type icon struct {
	Src string `xml:"src,attr"`
}

func parseXMLTVTime(value string) (time.Time, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return time.Time{}, fmt.Errorf("empty time")
	}
	layouts := []string{
		"20060102150405 -0700",
		"20060102150405 -07:00",
		"20060102150405",
		"2006-01-02T15:04:05Z07:00",
		"2006-01-02 15:04:05 -0700",
	}
	for _, layout := range layouts {
		if parsed, err := time.Parse(layout, value); err == nil {
			return parsed.UTC(), nil
		}
	}
	return time.Time{}, fmt.Errorf("unsupported XMLTV time %q", value)
}

// parseXMLTVNumbers reads the xmltv_ns form "0.4." (season 0, episode 4,
// zero based) and converts it to the one-based numbers Plex uses.
func parseXMLTVNumbers(value string) (int, int, bool) {
	parts := strings.Split(strings.TrimSpace(value), ".")
	if len(parts) < 2 {
		return 0, 0, false
	}
	season, err1 := strconv.Atoi(strings.TrimSpace(parts[0]))
	episode, err2 := strconv.Atoi(strings.TrimSpace(parts[1]))
	if err1 != nil || err2 != nil {
		return 0, 0, false
	}
	return season + 1, episode + 1, true
}
