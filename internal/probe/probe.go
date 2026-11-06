package probe

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"strings"
	"time"
)

// Prober turns a stream URL into the ffprobe JSON rex expects.
type Prober interface {
	Probe(ctx context.Context, streamURL string, live bool) (json.RawMessage, error)
}

type FFProbe struct {
	Path    string
	Timeout time.Duration
}

func New(path string, timeout time.Duration) *FFProbe {
	if path == "" {
		path = "ffprobe"
	}
	if timeout <= 0 {
		timeout = 30 * time.Second
	}
	return &FFProbe{Path: path, Timeout: timeout}
}

func (p *FFProbe) Probe(ctx context.Context, streamURL string, live bool) (json.RawMessage, error) {
	timeoutCtx, cancel := context.WithTimeout(ctx, p.Timeout)
	defer cancel()
	args := []string{
		"-v", "error",
		"-rw_timeout", "15000000",
		"-print_format", "json",
		"-show_format",
		"-show_streams",
		"-show_chapters",
		"-i", streamURL,
	}
	cmd := exec.CommandContext(timeoutCtx, p.Path, args...)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		if timeoutCtx.Err() == context.DeadlineExceeded {
			return nil, fmt.Errorf("probe timed out after %s", p.Timeout)
		}
		message := sanitize(stderr.String(), streamURL)
		if message == "" {
			message = err.Error()
		}
		return nil, fmt.Errorf("probe failed: %s", truncate(message, 240))
	}
	if stdout.Len() > 16<<20 {
		return nil, fmt.Errorf("probe output is too large")
	}
	var document map[string]any
	if err := json.Unmarshal(stdout.Bytes(), &document); err != nil {
		return nil, fmt.Errorf("probe returned invalid JSON")
	}
	streams, _ := document["streams"].([]any)
	if len(streams) == 0 {
		return nil, fmt.Errorf("probe found no streams")
	}
	format, _ := document["format"].(map[string]any)
	if live {
		name := ""
		if format != nil {
			name, _ = format["format_name"].(string)
		}
		if !strings.Contains(strings.ToLower(name), "hls") {
			return nil, fmt.Errorf("not a live HLS stream")
		}
		// A live probe must have no duration for rex to take it as a channel.
		if format != nil {
			delete(format, "duration")
			delete(format, "size")
		}
		for _, item := range streams {
			if stream, ok := item.(map[string]any); ok {
				delete(stream, "duration")
			}
		}
	}
	clean := stripFilenames(document)
	body, err := json.Marshal(clean)
	if err != nil {
		return nil, err
	}
	return body, nil
}

func stripFilenames(document map[string]any) map[string]any {
	if format, ok := document["format"].(map[string]any); ok {
		delete(format, "filename")
	}
	if streams, ok := document["streams"].([]any); ok {
		for _, item := range streams {
			if stream, ok := item.(map[string]any); ok {
				delete(stream, "filename")
			}
		}
	}
	return document
}

func sanitize(message, streamURL string) string {
	message = strings.ReplaceAll(message, streamURL, "<stream>")
	lines := strings.Split(strings.TrimSpace(message), "\n")
	if len(lines) == 0 {
		return ""
	}
	return strings.TrimSpace(lines[len(lines)-1])
}

func truncate(value string, limit int) string {
	if len(value) <= limit {
		return value
	}
	return value[:limit] + "..."
}
