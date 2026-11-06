package rex

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

type Client struct {
	base       string
	sourceName string
	user       string
	pass       string
	token      string
	batch      int
	http       *http.Client
}

type Directory struct {
	Name   string `json:"name"`
	Movies string `json:"movies,omitempty"`
	Shows  string `json:"shows,omitempty"`
	Music  string `json:"music,omitempty"`
}

type Item struct {
	ID          string          `json:"id"`
	Path        string          `json:"path"`
	Kind        string          `json:"kind,omitempty"`
	Name        string          `json:"name,omitempty"`
	Release     string          `json:"release,omitempty"`
	Season      *int            `json:"season,omitempty"`
	Episode     *int            `json:"episode,omitempty"`
	EpisodeEnd  *int            `json:"episode_end,omitempty"`
	TMDBEpisode int             `json:"tmdb_episode,omitempty"`
	IDs         map[string]any  `json:"ids,omitempty"`
	MediaInfo   json.RawMessage `json:"media_info"`
	Display     *Display        `json:"display,omitempty"`
	Channel     *Channel        `json:"channel,omitempty"`
	Subtitles   []Subtitle      `json:"subtitles,omitempty"`
	Collections []string        `json:"collections,omitempty"`
}

type Display struct {
	Title   string       `json:"title,omitempty"`
	Summary string       `json:"summary,omitempty"`
	Date    string       `json:"date,omitempty"`
	Thumb   string       `json:"thumb,omitempty"`
	Art     string       `json:"art,omitempty"`
	Show    *DisplayShow `json:"show,omitempty"`
}

type DisplayShow struct {
	Title   string `json:"title,omitempty"`
	Summary string `json:"summary,omitempty"`
	Poster  string `json:"poster,omitempty"`
	Art     string `json:"art,omitempty"`
}

type Channel struct {
	Number   string       `json:"number"`
	Title    string       `json:"title,omitempty"`
	CallSign string       `json:"call_sign,omitempty"`
	Guide    []GuideEntry `json:"guide,omitempty"`
}

type GuideEntry struct {
	Start           string   `json:"start"`
	End             string   `json:"end"`
	Title           string   `json:"title"`
	Summary         string   `json:"summary,omitempty"`
	Categories      []string `json:"categories,omitempty"`
	Thumb           string   `json:"thumb,omitempty"`
	Type            string   `json:"type,omitempty"`
	Show            string   `json:"show,omitempty"`
	Season          int      `json:"season,omitempty"`
	Episode         int      `json:"episode,omitempty"`
	PreviouslyShown bool     `json:"previously_shown,omitempty"`
}

type Subtitle struct {
	Path     string `json:"path"`
	Language string `json:"language,omitempty"`
	Size     int64  `json:"size,omitempty"`
}

type Result struct {
	Created   int         `json:"created"`
	Updated   int         `json:"updated"`
	Held      int         `json:"held"`
	Unchanged int         `json:"unchanged"`
	Rejected  []Rejection `json:"rejected"`
	Wants     []string    `json:"wants"`
	Deleted   int         `json:"deleted"`
	Trashed   int         `json:"trashed"`
	Released  int         `json:"released"`
	Unknown   int         `json:"unknown"`
}

type Rejection struct {
	ID     string `json:"id"`
	Reason string `json:"reason"`
}

// SetToken enables library renames through the Plex API. It is optional: with
// no token, only the source registration titles change.
func (c *Client) SetToken(token string) { c.token = token }

type Rename struct {
	OldTitle string
	NewTitle string
	Type     string
}

// RenameLibraries updates a library's display title. REX keeps a source's
// registration titles, so a playlist rename needs the Plex section edit.
func (c *Client) RenameLibraries(ctx context.Context, renames []Rename) error {
	if strings.TrimSpace(c.token) == "" || len(renames) == 0 {
		return nil
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.base+"/library/sections", nil)
	if err != nil {
		return err
	}
	req.Header.Set("X-Plex-Token", c.token)
	req.Header.Set("Accept", "application/json")
	resp, err := c.http.Do(req)
	if err != nil {
		return fmt.Errorf("list rex libraries: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("list rex libraries: HTTP %d", resp.StatusCode)
	}
	var payload struct {
		MediaContainer struct {
			Directory []struct {
				Key   string `json:"key"`
				Title string `json:"title"`
				Type  string `json:"type"`
			} `json:"Directory"`
		} `json:"MediaContainer"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		return err
	}
	for _, rename := range renames {
		var key string
		for _, section := range payload.MediaContainer.Directory {
			if section.Title == rename.OldTitle {
				key = section.Key
				break
			}
		}
		if key == "" {
			continue
		}
		values := url.Values{"name": {rename.NewTitle}, "language": {"en-US"}}
		if rename.Type == "show" {
			values.Set("type", "show")
			values.Set("agent", "tv.plex.agents.series")
			values.Set("scanner", "Plex TV Series")
		} else {
			values.Set("type", "movie")
			values.Set("agent", "tv.plex.agents.movie")
			values.Set("scanner", "Plex Movie")
		}
		request, err := http.NewRequestWithContext(ctx, http.MethodPut, c.base+"/library/sections/"+key+"?"+values.Encode(), nil)
		if err != nil {
			return err
		}
		request.Header.Set("X-Plex-Token", c.token)
		response, err := c.http.Do(request)
		if err != nil {
			return fmt.Errorf("rename library: %w", err)
		}
		response.Body.Close()
		if response.StatusCode/100 != 2 {
			return fmt.Errorf("rename library: HTTP %d", response.StatusCode)
		}
	}
	return nil
}

func New(base, sourceName, user, pass string, batch int) *Client {
	if batch < 1 {
		batch = 100
	}
	return &Client{
		base:       strings.TrimRight(base, "/"),
		sourceName: sourceName,
		user:       user,
		pass:       pass,
		batch:      batch,
		http:       &http.Client{Timeout: 60 * time.Second},
	}
}

func (c *Client) Healthy(ctx context.Context) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.base+"/identity", nil)
	if err != nil {
		return err
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("rex answered HTTP %d", resp.StatusCode)
	}
	return nil
}

func (c *Client) Register(ctx context.Context, publicURL string, directories []Directory) error {
	body := map[string]any{
		"url":         strings.TrimRight(publicURL, "/"),
		"user":        c.user,
		"password":    c.pass,
		"trash":       false,
		"directories": directories,
	}
	payload, err := json.Marshal(body)
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPut, c.base+"/rex/sources/"+c.sourceName, bytes.NewReader(payload))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := c.http.Do(req)
	if err != nil {
		return fmt.Errorf("register with rex: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode/100 != 2 {
		return fmt.Errorf("register with rex: HTTP %d: %s", resp.StatusCode, readMessage(resp.Body))
	}
	return nil
}

func (c *Client) Source(ctx context.Context) (map[string]any, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.base+"/rex/sources/"+c.sourceName, nil)
	if err != nil {
		return nil, err
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusNotFound {
		return nil, nil
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("rex source: HTTP %d", resp.StatusCode)
	}
	var out map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return nil, err
	}
	return out, nil
}

func (c *Client) Push(ctx context.Context, items []Item) (Result, error) {
	var total Result
	for start := 0; start < len(items); start += c.batch {
		end := start + c.batch
		if end > len(items) {
			end = len(items)
		}
		result, err := c.pushBatch(ctx, items[start:end])
		if err != nil {
			return total, err
		}
		total.Created += result.Created
		total.Updated += result.Updated
		total.Held += result.Held
		total.Unchanged += result.Unchanged
		total.Rejected = append(total.Rejected, result.Rejected...)
		total.Wants = append(total.Wants, result.Wants...)
	}
	return total, nil
}

func (c *Client) pushBatch(ctx context.Context, items []Item) (Result, error) {
	payload, err := json.Marshal(map[string]any{"items": items})
	if err != nil {
		return Result{}, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.base+"/rex/feed/"+c.sourceName+"/items", bytes.NewReader(payload))
	if err != nil {
		return Result{}, err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := c.http.Do(req)
	if err != nil {
		return Result{}, fmt.Errorf("push to rex: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode/100 != 2 {
		return Result{}, fmt.Errorf("push to rex: HTTP %d: %s", resp.StatusCode, readMessage(resp.Body))
	}
	var result Result
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return Result{}, err
	}
	return result, nil
}

func (c *Client) Delete(ctx context.Context, ids []string) (Result, error) {
	if len(ids) == 0 {
		return Result{}, nil
	}
	var total Result
	for start := 0; start < len(ids); start += c.batch {
		end := start + c.batch
		if end > len(ids) {
			end = len(ids)
		}
		payload, err := json.Marshal(map[string]any{"ids": ids[start:end]})
		if err != nil {
			return total, err
		}
		req, err := http.NewRequestWithContext(ctx, http.MethodDelete, c.base+"/rex/feed/"+c.sourceName+"/items", bytes.NewReader(payload))
		if err != nil {
			return total, err
		}
		req.Header.Set("Content-Type", "application/json")
		resp, err := c.http.Do(req)
		if err != nil {
			return total, fmt.Errorf("delete from rex: %w", err)
		}
		var result Result
		if resp.StatusCode/100 == 2 {
			_ = json.NewDecoder(resp.Body).Decode(&result)
		}
		resp.Body.Close()
		if resp.StatusCode/100 != 2 {
			return total, fmt.Errorf("delete from rex: HTTP %d", resp.StatusCode)
		}
		total.Deleted += result.Deleted
		total.Trashed += result.Trashed
		total.Released += result.Released
		total.Unknown += result.Unknown
	}
	return total, nil
}

func readMessage(body io.Reader) string {
	raw, _ := io.ReadAll(io.LimitReader(body, 512))
	message := strings.TrimSpace(string(raw))
	if message == "" {
		return "no response body"
	}
	return message
}
