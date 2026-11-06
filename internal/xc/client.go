package xc

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

const DefaultUserAgent = "rex-xstream/1.0"

var (
	ErrAuth      = errors.New("provider rejected the credentials")
	ErrRateLimit = errors.New("provider rate limit")
)

// Error is a provider error that is safe to show to an operator.
type Error struct {
	Status  int
	Message string
}

func (e *Error) Error() string {
	if e.Status > 0 {
		return fmt.Sprintf("provider answered HTTP %d: %s", e.Status, e.Message)
	}
	return "provider error: " + e.Message
}

type Client struct {
	base         string
	user         string
	pass         string
	http         *http.Client
	allowPrivate bool
	ua           string
}

func New(base, user, pass string, timeout time.Duration, allowPrivate bool) (*Client, error) {
	parsed, err := url.Parse(base)
	if err != nil || parsed.Host == "" {
		return nil, fmt.Errorf("invalid provider URL")
	}
	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return nil, fmt.Errorf("provider URL must use http or https")
	}
	if !allowPrivate {
		if err := rejectPrivateHost(parsed.Hostname()); err != nil {
			return nil, err
		}
	}
	if timeout <= 0 {
		timeout = 45 * time.Second
	}
	return &Client{
		base:         strings.TrimRight(base, "/"),
		user:         user,
		pass:         pass,
		allowPrivate: allowPrivate,
		ua:           DefaultUserAgent,
		http: &http.Client{Timeout: timeout, CheckRedirect: func(req *http.Request, via []*http.Request) error {
			if len(via) >= 5 {
				return errors.New("too many redirects")
			}
			return nil
		}},
	}, nil
}

func (c *Client) Account(ctx context.Context) (Account, error) {
	var account Account
	if err := c.getJSON(ctx, "", nil, &account); err != nil {
		return Account{}, err
	}
	if account.UserInfo.Auth.Int() == 0 && strings.EqualFold(account.UserInfo.Status, "") && account.UserInfo.Message != "" {
		return Account{}, fmt.Errorf("%w: %s", ErrAuth, account.UserInfo.Message)
	}
	return account, nil
}

func (c *Client) LiveCategories(ctx context.Context) ([]Category, error) {
	return c.categories(ctx, "get_live_categories")
}

func (c *Client) VODCategories(ctx context.Context) ([]Category, error) {
	return c.categories(ctx, "get_vod_categories")
}

func (c *Client) SeriesCategories(ctx context.Context) ([]Category, error) {
	return c.categories(ctx, "get_series_categories")
}

func (c *Client) categories(ctx context.Context, action string) ([]Category, error) {
	var out []Category
	if err := c.getJSON(ctx, action, nil, &out); err != nil {
		return nil, err
	}
	return out, nil
}

func (c *Client) LiveStreams(ctx context.Context) ([]LiveStream, error) {
	var out []LiveStream
	if err := c.getJSON(ctx, "get_live_streams", nil, &out); err != nil {
		return nil, err
	}
	return out, nil
}

func (c *Client) VODStreams(ctx context.Context) ([]VODStream, error) {
	var out []VODStream
	if err := c.getJSON(ctx, "get_vod_streams", nil, &out); err != nil {
		return nil, err
	}
	return out, nil
}

func (c *Client) Series(ctx context.Context) ([]SeriesItem, error) {
	var out []SeriesItem
	if err := c.getJSON(ctx, "get_series", nil, &out); err != nil {
		return nil, err
	}
	return out, nil
}

func (c *Client) SeriesInfo(ctx context.Context, seriesID string) (SeriesInfo, error) {
	var out SeriesInfo
	if err := c.getJSON(ctx, "get_series_info", url.Values{"series_id": {seriesID}}, &out); err != nil {
		return SeriesInfo{}, err
	}
	return out, nil
}

func (c *Client) VODInfo(ctx context.Context, vodID string) (VODInfo, error) {
	var out VODInfo
	if err := c.getJSON(ctx, "get_vod_info", url.Values{"vod_id": {vodID}}, &out); err != nil {
		return VODInfo{}, err
	}
	return out, nil
}

func (c *Client) getJSON(ctx context.Context, action string, extra url.Values, out any) error {
	body, err := c.get(ctx, action, extra)
	if err != nil {
		return err
	}
	trimmed := strings.TrimSpace(string(body))
	if trimmed == "" {
		return fmt.Errorf("provider returned an empty response")
	}
	if strings.HasPrefix(trimmed, "{") {
		var envelope map[string]json.RawMessage
		if err := json.Unmarshal(body, &envelope); err == nil {
			if raw, ok := envelope["error"]; ok {
				var message string
				_ = json.Unmarshal(raw, &message)
				if message == "" {
					message = string(raw)
				}
				return &Error{Message: message}
			}
			if raw, ok := envelope["user_info"]; ok {
				var probe struct {
					Auth    FlexInt `json:"auth"`
					Status  string  `json:"status"`
					Message string  `json:"message"`
				}
				_ = json.Unmarshal(raw, &probe)
				if probe.Auth.Int() == 0 && probe.Message != "" && !strings.EqualFold(probe.Status, "Active") {
					return fmt.Errorf("%w: %s", ErrAuth, probe.Message)
				}
			}
		}
	}
	if err := json.Unmarshal(body, out); err != nil {
		return fmt.Errorf("provider returned invalid JSON: %w", err)
	}
	return nil
}

func (c *Client) get(ctx context.Context, action string, extra url.Values) ([]byte, error) {
	values := url.Values{"username": {c.user}, "password": {c.pass}}
	if action != "" {
		values.Set("action", action)
	}
	for key, list := range extra {
		for _, value := range list {
			values.Add(key, value)
		}
	}
	endpoint := c.base + "/player_api.php?" + values.Encode()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", c.ua)
	req.Header.Set("Accept", "application/json, text/plain, */*")
	resp, err := c.http.Do(req)
	if err != nil {
		return nil, fmt.Errorf("could not reach provider: %s", cleanNetworkError(err))
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, 128<<20))
	if err != nil {
		return nil, err
	}
	switch resp.StatusCode {
	case http.StatusOK:
		return body, nil
	case http.StatusUnauthorized, http.StatusForbidden:
		return nil, ErrAuth
	case http.StatusTooManyRequests:
		return nil, ErrRateLimit
	default:
		return nil, &Error{Status: resp.StatusCode, Message: strings.TrimSpace(string(body))}
	}
}

// XMLTV returns the provider's XMLTV guide, decompressing it when needed.
func (c *Client) XMLTV(ctx context.Context) (io.ReadCloser, error) {
	values := url.Values{"username": {c.user}, "password": {c.pass}}
	endpoint := c.base + "/xmltv.php?" + values.Encode()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", c.ua)
	resp, err := c.http.Do(req)
	if err != nil {
		return nil, fmt.Errorf("could not reach provider guide: %s", cleanNetworkError(err))
	}
	if resp.StatusCode != http.StatusOK {
		resp.Body.Close()
		if resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden {
			return nil, ErrAuth
		}
		return nil, &Error{Status: resp.StatusCode, Message: "guide request failed"}
	}
	return resp.Body, nil
}

func (c *Client) LiveStreamURL(streamID string) string {
	return c.base + "/live/" + escape(c.user) + "/" + escape(c.pass) + "/" + escape(streamID) + ".m3u8"
}

func (c *Client) MovieStreamURL(streamID, container string) string {
	ext := normalContainer(container, "mp4")
	return c.base + "/movie/" + escape(c.user) + "/" + escape(c.pass) + "/" + escape(streamID) + "." + ext
}

func (c *Client) EpisodeStreamURL(episodeID, container string) string {
	ext := normalContainer(container, "mkv")
	return c.base + "/series/" + escape(c.user) + "/" + escape(c.pass) + "/" + escape(episodeID) + "." + ext
}

func normalContainer(container, fallback string) string {
	value := strings.ToLower(strings.TrimSpace(strings.TrimPrefix(container, ".")))
	switch value {
	case "mp4", "mkv", "avi", "mov", "m4v", "ts", "m3u8", "webm":
		return value
	default:
		return fallback
	}
}

func escape(value string) string {
	return url.PathEscape(value)
}

func rejectPrivateHost(host string) error {
	host = strings.Trim(host, "[]")
	if host == "" {
		return fmt.Errorf("provider URL has no host")
	}
	ips, err := net.LookupIP(host)
	if err != nil {
		return fmt.Errorf("provider host could not be resolved")
	}
	for _, ip := range ips {
		if isForbiddenIP(ip) {
			return fmt.Errorf("provider host resolves to a private or local address; set XC_ALLOW_PRIVATE_HOSTS=true only if you trust it")
		}
	}
	return nil
}

func isForbiddenIP(ip net.IP) bool {
	return ip.IsLoopback() || ip.IsPrivate() || ip.IsLinkLocalUnicast() || ip.IsLinkLocalMulticast() ||
		ip.IsUnspecified() || ip.IsMulticast()
}

func cleanNetworkError(err error) string {
	message := err.Error()
	if index := strings.Index(message, "?"); index >= 0 {
		message = message[:index]
	}
	return message
}

// Expiry parses the account's exp_date. Panels use epoch seconds, sometimes
// as a string.
func (a Account) Expiry() *time.Time {
	raw := strings.TrimSpace(a.UserInfo.ExpDate.String())
	if raw == "" || raw == "0" || raw == "null" {
		return nil
	}
	seconds, err := strconv.ParseInt(raw, 10, 64)
	if err != nil || seconds <= 0 {
		return nil
	}
	t := time.Unix(seconds, 0).UTC()
	return &t
}
