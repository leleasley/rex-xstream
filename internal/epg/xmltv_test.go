package epg

import (
	"bytes"
	"compress/gzip"
	"strings"
	"testing"
	"time"
)

const guide = `<?xml version="1.0" encoding="UTF-8"?>
<tv>
  <channel id="BBC1.uk"><display-name>BBC One</display-name></channel>
  <programme start="20261106180000 +0000" stop="20261106190000 +0000" channel="BBC1.uk">
    <title>Evening News</title><desc>The headlines.</desc><category>News</category>
    <icon src="https://example.com/news.jpg"/>
  </programme>
  <programme start="20261106190000 +0000" stop="20261106200000 +0000" channel="BBC1.uk">
    <title>Drama</title><episode-num system="xmltv_ns">0.4.</episode-num>
  </programme>
  <programme start="20261106180000 +0000" stop="20261106190000 +0000" channel="OTHER">
    <title>Other</title>
  </programme>
</tv>`

func TestParsePlainXMLTV(t *testing.T) {
	from := time.Date(2026, 11, 6, 17, 0, 0, 0, time.UTC)
	to := time.Date(2026, 11, 6, 21, 0, 0, 0, time.UTC)
	items, err := Parse(strings.NewReader(guide), from, to)
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 3 {
		t.Fatalf("got %d programmes, want 3", len(items))
	}
	first := items[0]
	if first.ChannelEPGID != "BBC1.uk" || first.Title != "Evening News" {
		t.Fatalf("unexpected first programme: %+v", first)
	}
	if first.Category != "News" || first.Icon == "" {
		t.Fatalf("category or icon missing: %+v", first)
	}
	if items[1].Season != 1 || items[1].Episode != 5 {
		t.Fatalf("xmltv_ns conversion failed: %+v", items[1])
	}
}

func TestParseGzipAndWindow(t *testing.T) {
	var body bytes.Buffer
	writer := gzip.NewWriter(&body)
	_, _ = writer.Write([]byte(guide))
	_ = writer.Close()
	from := time.Date(2026, 11, 6, 19, 30, 0, 0, time.UTC)
	to := time.Date(2026, 11, 6, 21, 0, 0, 0, time.UTC)
	items, err := Parse(bytes.NewReader(body.Bytes()), from, to)
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 1 || items[0].Title != "Drama" {
		t.Fatalf("window filtering failed: %+v", items)
	}
}

func TestMalformedTimesAreSkipped(t *testing.T) {
	bad := `<tv><programme start="not-a-time" stop="nope" channel="x"><title>Bad</title></programme></tv>`
	items, err := Parse(strings.NewReader(bad), time.Now().Add(-time.Hour), time.Now().Add(time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 0 {
		t.Fatalf("malformed entry was kept: %+v", items)
	}
}
