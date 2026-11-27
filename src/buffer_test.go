package src

import (
	"testing"

	"github.com/avfs/avfs/vfs/memfs"
)

// newTestStream returns a stream for parseM3U8 whose buffer folder does not
// exist, so that waiting for new segments stops after the first step.
func newTestStream(t *testing.T, body string) *ThisStream {
	t.Helper()

	bufferVFS = memfs.New()

	return &ThisStream{
		Body:               body,
		Folder:             "/missing",
		M3U8URL:            "http://example.com/live/index.m3u8",
		URLStreamingServer: "http://example.com",
	}
}

const liveM3U8 = "#EXTM3U\r\n" +
	"#EXT-X-VERSION:3\r\n" +
	"#EXT-X-MEDIA-SEQUENCE:100\r\n" +
	"#EXTINF:0.1,\r\n" +
	"seg100.ts\r\n" +
	"#EXTINF:0.1,\r\n" +
	"/live/seg101.ts\r\n" +
	"#EXTINF:0.1,\r\n" +
	"http://cdn.example.com/seg102.ts\r\n"

func TestParseM3U8FirstRequest(t *testing.T) {
	var stream = newTestStream(t, liveM3U8)

	if err := parseM3U8(stream); err != nil {
		t.Fatal(err)
	}

	// A live stream starts at the second to last segment.
	if len(stream.Segment) != 1 {
		t.Fatalf("segments = %+v, want 1", stream.Segment)
	}
	var s = stream.Segment[0]
	if s.Sequence != 101 || s.URL != "http://example.com/live/seg101.ts" || s.Duration != 0.1 {
		t.Errorf("segment = %+v, want sequence 101 with server relative URL", s)
	}
	if stream.LastSequence != 101 || stream.Sequence != 100 {
		t.Errorf("LastSequence = %d, Sequence = %d, want 101, 100", stream.LastSequence, stream.Sequence)
	}
	if stream.DynamicBandwidth {
		t.Error("DynamicBandwidth = true for a media playlist")
	}
}

func TestParseM3U8NextSegment(t *testing.T) {
	tests := []struct {
		lastSequence int64
		wantURL      string
	}{
		{lastSequence: 99, wantURL: "http://example.com/live/seg100.ts"},
		{lastSequence: 100, wantURL: "http://example.com/live/seg101.ts"},
		{lastSequence: 101, wantURL: "http://cdn.example.com/seg102.ts"},
	}
	for _, tt := range tests {
		var stream = newTestStream(t, liveM3U8)
		stream.Status = true
		stream.LastSequence = tt.lastSequence

		if err := parseM3U8(stream); err != nil {
			t.Fatal(err)
		}
		if len(stream.Segment) != 1 || stream.Segment[0].URL != tt.wantURL {
			t.Errorf("after sequence %d: segments = %+v, want %s", tt.lastSequence, stream.Segment, tt.wantURL)
			continue
		}
		if stream.LastSequence != tt.lastSequence+1 {
			t.Errorf("after sequence %d: LastSequence = %d", tt.lastSequence, stream.LastSequence)
		}
	}
}

func TestParseM3U8NoNewSegment(t *testing.T) {
	var stream = newTestStream(t, liveM3U8)
	stream.Status = true
	stream.LastSequence = 102

	if err := parseM3U8(stream); err != nil {
		t.Fatal(err)
	}
	if len(stream.Segment) != 0 {
		t.Errorf("segments = %+v, want none", stream.Segment)
	}
	if stream.LastSequence != 102 {
		t.Errorf("LastSequence = %d, want 102", stream.LastSequence)
	}
}

func TestParseM3U8VOD(t *testing.T) {
	var stream = newTestStream(t, "#EXTM3U\n"+
		"#EXT-X-PLAYLIST-TYPE:VOD\n"+
		"#EXTINF:10.0,\n"+
		"a.ts\n"+
		"#EXTINF:10.0,\n"+
		"b.ts\n"+
		"#EXTINF:10.0,\n"+
		"c.ts\n"+
		"#EXT-X-ENDLIST\n")

	if err := parseM3U8(stream); err != nil {
		t.Fatal(err)
	}
	if len(stream.Segment) != 1 || stream.Segment[0].URL != "http://example.com/live/a.ts" {
		t.Fatalf("segments = %+v, want the first one", stream.Segment)
	}
	if stream.Segment[0].Sequence != 0 || stream.Segment[0].Duration != 10 {
		t.Errorf("segment = %+v", stream.Segment[0])
	}
}

func TestParseM3U8RelativeURLWithQuery(t *testing.T) {
	var stream = newTestStream(t, "#EXTM3U\n#EXTINF:1,\na.ts?token=1\n#EXTINF:1,\nb.ts?token=1\n")
	stream.M3U8URL = "http://example.com/live/index.m3u8?token=1"

	if err := parseM3U8(stream); err != nil {
		t.Fatal(err)
	}
	if len(stream.Segment) != 1 || stream.Segment[0].URL != "http://example.com/live/a.ts?token=1" {
		t.Errorf("segments = %+v", stream.Segment)
	}
}

const masterM3U8 = "#EXTM3U\n" +
	"#EXT-X-STREAM-INF:BANDWIDTH=3000000,RESOLUTION=1280x720\n" +
	"720.m3u8\n" +
	"#EXT-X-STREAM-INF:AVERAGE-BANDWIDTH=900000,BANDWIDTH=1000000,RESOLUTION=640x360\n" +
	"360.m3u8\n" +
	"#EXT-X-STREAM-INF:BANDWIDTH=2000000,RESOLUTION=960x540\n" +
	"http://cdn.example.com/540.m3u8\n" +
	"#EXT-X-STREAM-INF:BANDWIDTH=5000000,RESOLUTION=1920x1080\n" +
	"/live/1080.m3u8\n"

func TestParseM3U8MasterPlaylist(t *testing.T) {
	var stream = newTestStream(t, masterM3U8)

	if err := parseM3U8(stream); err != nil {
		t.Fatal(err)
	}
	if !stream.DynamicBandwidth {
		t.Error("DynamicBandwidth = false for a master playlist")
	}

	var want = map[int]string{
		1000000: "http://example.com/live/360.m3u8",
		2000000: "http://cdn.example.com/540.m3u8",
		3000000: "http://example.com/live/720.m3u8",
		5000000: "http://example.com/live/1080.m3u8",
	}
	if len(stream.DynamicStream) != len(want) {
		t.Errorf("DynamicStream = %+v, want %d variants", stream.DynamicStream, len(want))
	}
	for bandwidth, u := range want {
		if got := stream.DynamicStream[bandwidth]; got.Bandwidth != bandwidth || got.URL != u {
			t.Errorf("DynamicStream[%d] = %+v, want URL %s", bandwidth, got, u)
		}
	}
}

func TestParseM3U8SwitchBandwidth(t *testing.T) {
	tests := []struct {
		network int
		want    string
	}{
		{network: 0, want: "360"},        // unknown network speed: lowest
		{network: 500000, want: "360"},   // slower than every variant: lowest
		{network: 1000000, want: "360"},  // exactly the lowest
		{network: 2500000, want: "540"},  // between two variants
		{network: 3500000, want: "720"},  // between two variants
		{network: 9000000, want: "1080"}, // faster than every variant: highest
	}
	for _, tt := range tests {
		var stream = newTestStream(t, masterM3U8)
		stream.NetworkBandwidth = tt.network

		if err := parseM3U8(stream); err != nil {
			t.Fatal(err)
		}
		if len(stream.Segment) != 1 {
			t.Errorf("network %d: segments = %+v, want 1", tt.network, stream.Segment)
			continue
		}
		var s = stream.Segment[0]
		var want = "http://example.com/live/" + tt.want + ".m3u8"
		if tt.want == "540" {
			want = "http://cdn.example.com/540.m3u8"
		}
		if s.URL != want {
			t.Errorf("network %d: URL = %s, want %s", tt.network, s.URL, want)
		}
		if s.Duration != 0 {
			t.Errorf("network %d: Duration = %v, want 0", tt.network, s.Duration)
		}
	}
}

func TestParseM3U8Errors(t *testing.T) {
	tests := map[string]struct {
		body string
		code int
	}{
		"not a playlist":   {body: "<html></html>", code: 4051},
		"invalid duration": {body: "#EXTM3U\n#EXTINF:abc,\na.ts\n", code: 1050},
	}
	for name, tt := range tests {
		var err = parseM3U8(newTestStream(t, tt.body))
		if err == nil {
			t.Errorf("%s: no error", name)
			continue
		}
		if tt.code == 1050 && errorCode(err) != tt.code {
			t.Errorf("%s: code = %d, want %d", name, errorCode(err), tt.code)
		}
		if tt.code == 4051 && err.Error() != getErrMsg(4051) {
			t.Errorf("%s: error = %q, want %q", name, err, getErrMsg(4051))
		}
	}
}

func TestSniffStreamType(t *testing.T) {
	var ts = make([]byte, 189)
	ts[0], ts[188] = 0x47, 0x47
	var notTS = make([]byte, 189)
	notTS[0] = 0x47

	tests := map[string]struct {
		start []byte
		want  string
	}{
		"playlist":          {start: []byte("#EXTM3U\n#EXT-X-VERSION:3\n"), want: "application/vnd.apple.mpegurl"},
		"playlist with BOM": {start: []byte("\uFEFF#EXTM3U\n"), want: "application/vnd.apple.mpegurl"},
		"leading newline":   {start: []byte("\r\n#EXTM3U\n"), want: "application/vnd.apple.mpegurl"},
		"MPEG-TS":           {start: ts, want: "video/mp2t"},
		"one sync byte":     {start: notTS, want: ""},
		"short":             {start: []byte{0x47}, want: ""},
		"HTML":              {start: []byte("<html><body>Not found</body></html>"), want: ""},
	}
	for name, tt := range tests {
		if got := sniffStreamType(tt.start); got != tt.want {
			t.Errorf("%s: sniffStreamType = %q, want %q", name, got, tt.want)
		}
	}
}
