package xc

import (
	"bytes"
	"compress/gzip"
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func server(t *testing.T, handler http.HandlerFunc) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(handler)
	t.Cleanup(srv.Close)
	return srv
}

func newClient(t *testing.T, base string) *Client {
	t.Helper()
	client, err := New(base, "user name", "p@ss", 5*time.Second, true)
	if err != nil {
		t.Fatal(err)
	}
	return client
}

func TestAccountAndCategories(t *testing.T) {
	srv := server(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("username") != "user name" || r.URL.Query().Get("password") != "p@ss" {
			http.Error(w, "bad credentials", http.StatusForbidden)
			return
		}
		switch r.URL.Query().Get("action") {
		case "":
			fmt.Fprint(w, `{"user_info":{"username":"user name","auth":1,"status":"Active","exp_date":"1893456000","max_connections":"3"},"server_info":{"server_software":"Xtream UI","timezone":"UTC","server_protocol":"http"}}`)
		case "get_live_categories":
			fmt.Fprint(w, `[{"category_id":"1","category_name":"News"},{"category_id":2,"category_name":"Sports"}]`)
		case "get_live_streams":
			fmt.Fprint(w, `[{"num":"1","name":"BBC One","stream_id":10,"stream_icon":"https://img/x.png","epg_channel_id":"BBC1.uk","category_id":"1","tv_archive":1}]`)
		case "get_vod_streams":
			fmt.Fprint(w, `[{"name":"Film (2024)","stream_id":"20","container_extension":"mkv","tmdb":"123","rating":"7.1","category_id":"4"}]`)
		case "get_series":
			fmt.Fprint(w, `[{"name":"Show - 2025","series_id":30,"tmdb":"456","cover":"https://img/s.jpg","last_modified":"abc"}]`)
		case "get_series_info":
			if r.URL.Query().Get("series_id") != "30" {
				http.Error(w, "bad id", http.StatusBadRequest)
				return
			}
			fmt.Fprint(w, `{"seasons":[{"season_number":1,"name":"Season 1"}],"episodes":{"1":[{"id":"ep1","episode_num":1,"title":"Pilot","container_extension":"mkv","season":1,"info":{"tmdb_id":999,"plot":"p","movie_image":"https://img/e.jpg"}}]}}`)
		default:
			http.Error(w, "unknown action", http.StatusBadRequest)
		}
	})
	ctx := context.Background()
	client := newClient(t, srv.URL)
	account, err := client.Account(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if account.ServerInfo.ServerSoftware != "Xtream UI" || account.UserInfo.Status != "Active" {
		t.Fatalf("account: %+v", account)
	}
	if expiry := account.Expiry(); expiry == nil || expiry.Year() != 2030 {
		t.Fatalf("expiry: %v", expiry)
	}
	categories, err := client.LiveCategories(ctx)
	if err != nil || len(categories) != 2 {
		t.Fatalf("categories: %v %v", categories, err)
	}
	streams, err := client.LiveStreams(ctx)
	if err != nil || len(streams) != 1 || streams[0].StreamID.String() != "10" {
		t.Fatalf("streams: %v %v", streams, err)
	}
	vod, err := client.VODStreams(ctx)
	if err != nil || len(vod) != 1 || vod[0].TMDB.String() != "123" {
		t.Fatalf("vod: %v %v", vod, err)
	}
	series, err := client.Series(ctx)
	if err != nil || len(series) != 1 || series[0].SeriesID.String() != "30" {
		t.Fatalf("series: %v %v", series, err)
	}
	info, err := client.SeriesInfo(ctx, "30")
	if err != nil || len(info.Episodes["1"]) != 1 {
		t.Fatalf("series info: %v %v", info, err)
	}
}

func TestFlexibleArtworkArrays(t *testing.T) {
	srv := server(t, func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, `[{"name":"Show","series_id":1,"cover":["https://img/cover.jpg",""],"backdrop_path":["https://img/back.jpg"],"plot":["A plot"],"genre":["Drama"]}]`)
	})
	client := newClient(t, srv.URL)
	items, err := client.Series(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 1 || items[0].Cover.String() != "https://img/cover.jpg" ||
		items[0].BackdropPath.String() != "https://img/back.jpg" || items[0].Genre.String() != "Drama" {
		t.Fatalf("series: %+v", items)
	}
}

func TestInvalidCredentials(t *testing.T) {
	srv := server(t, func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "forbidden", http.StatusForbidden)
	})
	client := newClient(t, srv.URL)
	if _, err := client.Account(context.Background()); !errors.Is(err, ErrAuth) {
		t.Fatalf("got %v, want ErrAuth", err)
	}
}

func TestProviderErrorAndRateLimit(t *testing.T) {
	srv := server(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("action") == "get_series" {
			w.WriteHeader(http.StatusTooManyRequests)
			return
		}
		http.Error(w, "boom", http.StatusInternalServerError)
	})
	client := newClient(t, srv.URL)
	if _, err := client.Series(context.Background()); !errors.Is(err, ErrRateLimit) {
		t.Fatalf("got %v, want ErrRateLimit", err)
	}
	if _, err := client.LiveStreams(context.Background()); err == nil {
		t.Fatal("expected a provider error")
	}
}

func TestErrorEnvelope(t *testing.T) {
	srv := server(t, func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, `{"error":"expired"}`)
	})
	client := newClient(t, srv.URL)
	if _, err := client.LiveCategories(context.Background()); err == nil || !strings.Contains(err.Error(), "expired") {
		t.Fatalf("got %v", err)
	}
}

func TestStreamURLs(t *testing.T) {
	client := newClient(t, "http://example.com:8080")
	if got := client.LiveStreamURL("10"); !strings.HasSuffix(got, "/live/user%20name/p@ss/10.m3u8") {
		t.Fatalf("live url: %s", got)
	}
	if got := client.MovieStreamURL("20", "MKV"); !strings.HasSuffix(got, "/movie/user%20name/p@ss/20.mkv") {
		t.Fatalf("movie url: %s", got)
	}
	if got := client.EpisodeStreamURL("30", ""); !strings.HasSuffix(got, "/series/user%20name/p@ss/30.mkv") {
		t.Fatalf("episode url: %s", got)
	}
	if got := client.MovieStreamURL("1", "exe"); !strings.HasSuffix(got, ".mp4") {
		t.Fatalf("fallback: %s", got)
	}
}

func TestXMLTVGzip(t *testing.T) {
	var body bytes.Buffer
	writer := gzip.NewWriter(&body)
	_, _ = writer.Write([]byte("<tv></tv>"))
	_ = writer.Close()
	srv := server(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Encoding", "gzip")
		_, _ = w.Write(body.Bytes())
	})
	client := newClient(t, srv.URL)
	reader, err := client.XMLTV(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	defer reader.Close()
	raw := make([]byte, 64)
	n, _ := reader.Read(raw)
	if !strings.HasPrefix(string(raw[:n]), "<tv>") {
		t.Fatalf("unexpected guide: %q", string(raw[:n]))
	}
}

func TestPrivateHostRejectedUnlessAllowed(t *testing.T) {
	if _, err := New("http://127.0.0.1:1", "u", "p", time.Second, false); err == nil {
		t.Fatal("private host should be rejected")
	}
	if _, err := New("http://127.0.0.1:1", "u", "p", time.Second, true); err != nil {
		t.Fatalf("allow private: %v", err)
	}
}
