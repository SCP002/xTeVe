package buffer

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"xteve/src/internal/config"

	"github.com/avfs/avfs/vfs/memfs"
)

// tsChunk is MPEG-TS data: packets with the sync byte 0x47 every 188 bytes.
var tsChunk = func() []byte {
	var b = make([]byte, 188*100)
	for i := 0; i < len(b); i += 188 {
		b[i] = 0x47
	}
	return b
}()

// testSource is a streaming server for the buffer tests.
type testSource struct {
	*httptest.Server
	active atomic.Int32 // open connections to /ts
	hits   sync.Map     // path -> *atomic.Int32
}

func (s *testSource) hit(path string) int32 {
	var v, _ = s.hits.LoadOrStore(path, &atomic.Int32{})
	return v.(*atomic.Int32).Load()
}

func newTestSource(t *testing.T) *testSource {
	t.Helper()
	var src = &testSource{}

	var writeTS = func(w http.ResponseWriter, r *http.Request, limit int) {
		w.Header().Set("Content-Type", "video/mp2t")
		var written int
		for limit == 0 || written < limit {
			if _, err := w.Write(tsChunk); err != nil {
				return
			}
			written += len(tsChunk)
			w.(http.Flusher).Flush()
			select {
			case <-r.Context().Done():
				return
			case <-time.After(5 * time.Millisecond):
			}
		}
	}

	var mux = http.NewServeMux()
	mux.HandleFunc("/ts/", func(w http.ResponseWriter, r *http.Request) {
		src.active.Add(1)
		defer src.active.Add(-1)
		writeTS(w, r, 0)
	})
	mux.HandleFunc("/slow", func(w http.ResponseWriter, r *http.Request) {
		// Like AceStream: the first data after seconds
		select {
		case <-r.Context().Done():
			return
		case <-time.After(5 * time.Second):
		}
		writeTS(w, r, 0)
	})
	mux.HandleFunc("/drop", func(w http.ResponseWriter, r *http.Request) {
		writeTS(w, r, 300*1024)
	})
	mux.HandleFunc("/redirect", func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "/ts/redirected", http.StatusFound)
	})
	mux.HandleFunc("/hls/index.m3u8", func(w http.ResponseWriter, r *http.Request) {
		// A live playlist: a new segment every 200 ms
		var seq = time.Now().UnixMilli() / 200
		w.Header().Set("Content-Type", "application/vnd.apple.mpegurl")
		var playlist = fmt.Sprintf("#EXTM3U\n#EXT-X-VERSION:3\n#EXT-X-TARGETDURATION:1\n#EXT-X-MEDIA-SEQUENCE:%d\n",
			seq-3)
		for i := seq - 3; i <= seq; i++ {
			playlist += fmt.Sprintf("#EXTINF:0.2,\nseg%d.ts\n", i)
		}
		if _, err := io.WriteString(w, playlist); err != nil {
			return
		}
	})
	mux.HandleFunc("/hls/", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "video/mp2t")
		for range 3 {
			if _, err := w.Write(tsChunk); err != nil {
				return
			}
		}
	})

	// Adaptive HLS: a master playlist with a low and a high variant, the high one is slow
	mux.HandleFunc("/abr/master.m3u8", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/vnd.apple.mpegurl")
		_, _ = io.WriteString(w, "#EXTM3U\n"+
			"#EXT-X-STREAM-INF:BANDWIDTH=1000000\nlow/index.m3u8\n"+
			"#EXT-X-STREAM-INF:BANDWIDTH=8000000\nhigh/index.m3u8\n")
	})
	for _, variant := range []string{"low", "high"} {
		mux.HandleFunc("/abr/"+variant+"/index.m3u8", func(w http.ResponseWriter, r *http.Request) {
			var seq = time.Now().UnixMilli() / 500
			w.Header().Set("Content-Type", "application/vnd.apple.mpegurl")
			var playlist = fmt.Sprintf("#EXTM3U\n#EXT-X-VERSION:3\n#EXT-X-TARGETDURATION:1\n"+
				"#EXT-X-MEDIA-SEQUENCE:%d\n", seq-1)
			for i := seq - 1; i <= seq; i++ {
				playlist += fmt.Sprintf("#EXTINF:0.5,\nseg%d.ts\n", i)
			}
			_, _ = io.WriteString(w, playlist)
		})
		mux.HandleFunc("/abr/"+variant+"/", func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "video/mp2t")
			// 3 chunks of 18.8 KB: the high variant comes with about 2 Mbit/s
			for range 3 {
				if _, err := w.Write(tsChunk); err != nil {
					return
				}
				w.(http.Flusher).Flush()
				if variant == "high" {
					time.Sleep(100 * time.Millisecond)
				}
			}
		})
	}

	src.Server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var v, _ = src.hits.LoadOrStore(r.URL.Path, &atomic.Int32{})
		v.(*atomic.Int32).Add(1)
		mux.ServeHTTP(w, r)
	}))
	t.Cleanup(src.Close)
	return src
}

// bufferTest sets up a manager with the built-in xTeVe buffer and a playlist "Mtest" with tuners, and returns a
// server that streams a channel: GET /?url=<source URL>. After the test it waits until all downloads have ended.
func bufferTest(t *testing.T, tuners int) *httptest.Server {
	t.Helper()

	var old = config.Get()
	config.Default().SetSaver(func(*config.Settings) error { return nil })
	t.Cleanup(func() {
		if err := config.Default().Set(old); err != nil {
			t.Error(err)
		}
	})
	var err = config.Update(func(s *config.Settings) error {
		s.Buffer = "xteve"
		s.BufferSize = 64
		s.BufferTimeout = 0
		s.UserAgent = "xteve-test"
		s.M3U8AdaptiveBandwidthMBPS = 10
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}

	var vfs = memfs.New()
	var m = New(Config{
		VFS:              vfs,
		TempFolder:       func() string { return "/xteve-test/" },
		Playlist:         func(string) (string, int) { return "Test", tuners },
		StreamLimitVideo: func() []byte { return tsChunk },
		Error:            func(err error, code int) { t.Logf("error %d: %v", code, err) },
	})

	var srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		m.Serve(w, r, "Mtest", r.URL.Query().Get("url"), "Channel")
	}))
	t.Cleanup(func() {
		srv.Close()
		if !waitFor(10*time.Second, func() bool { return len(m.Streams()) == 0 }) {
			t.Errorf("streams left after the test: %v", m.Streams())
		}
		// Every download removed its folder
		if !waitFor(5*time.Second, func() bool {
			var files, err = vfs.ReadDir("/xteve-test/Mtest")
			return (err != nil && isNotExist(err)) || (err == nil && len(files) == 0)
		}) {
			var files, _ = vfs.ReadDir("/xteve-test/Mtest")
			t.Errorf("folders left after the test: %d", len(files))
		}
	})
	return srv
}

type streamResult struct {
	status      int
	contentType string
	data        []byte
	err         error
}

// readStream reads from the channel URL until n bytes arrived, the stream ended or the timeout passed.
func readStream(
	ctx context.Context, srv *httptest.Server, sourceURL string, n int, timeout time.Duration,
) streamResult {
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, srv.URL+"/?url="+sourceURL, nil)
	if err != nil {
		return streamResult{err: err}
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return streamResult{err: err}
	}
	defer func() { _ = resp.Body.Close() }()

	var data = make([]byte, 0, n)
	var buf = make([]byte, 32*1024)
	for len(data) < n {
		var k, err = resp.Body.Read(buf)
		data = append(data, buf[:k]...)
		if err != nil {
			break
		}
	}
	return streamResult{status: resp.StatusCode, contentType: resp.Header.Get("Content-Type"), data: data}
}

func isTS(data []byte) bool {
	if len(data) < 188*2 {
		return false
	}
	for i := 0; i+188 <= len(data); i += 188 {
		if data[i] != 0x47 {
			return false
		}
	}
	return true
}

// waitFor waits until cond is true or the timeout passed.
func waitFor(timeout time.Duration, cond func() bool) bool {
	var end = time.Now().Add(timeout)
	for time.Now().Before(end) {
		if cond() {
			return true
		}
		time.Sleep(20 * time.Millisecond)
	}
	return cond()
}

func TestServeTS(t *testing.T) {
	var source = newTestSource(t)
	var srv = bufferTest(t, 2)

	var res = readStream(t.Context(), srv, source.URL+"/ts/a", 256*1024, 10*time.Second)
	if res.err != nil || len(res.data) < 256*1024 || !isTS(res.data) {
		t.Fatalf("got %d bytes (TS: %v), err %v", len(res.data), isTS(res.data), res.err)
	}

	// The connection to the source ends after the client left
	if !waitFor(5*time.Second, func() bool { return source.active.Load() == 0 }) {
		t.Errorf("source connections after the client left: %d", source.active.Load())
	}
}

func TestServeSharedStream(t *testing.T) {
	var source = newTestSource(t)
	var srv = bufferTest(t, 2)

	var results = make([]streamResult, 2)
	var wg sync.WaitGroup
	for i := range results {
		wg.Go(func() {
			// The second client joins while the first one still reads
			time.Sleep(time.Duration(i) * 100 * time.Millisecond)
			results[i] = readStream(t.Context(), srv, source.URL+"/ts/shared", (1024-i*512)*1024, 10*time.Second)
		})
	}
	wg.Wait()

	for i, res := range results {
		if res.err != nil || len(res.data) < (1024-i*512)*1024 || !isTS(res.data) {
			t.Errorf("client %d: %d bytes (TS: %v), err %v", i, len(res.data), isTS(res.data), res.err)
		}
	}
	if hits := source.hit("/ts/shared"); hits != 1 {
		t.Errorf("source requests = %d, want 1 for both clients", hits)
	}
}

func TestServeTunerLimit(t *testing.T) {
	var source = newTestSource(t)
	var srv = bufferTest(t, 1)

	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	var first = make(chan streamResult, 1)
	go func() { first <- readStream(ctx, srv, source.URL+"/ts/one", 1<<30, 30*time.Second) }()

	if !waitFor(5*time.Second, func() bool { return source.active.Load() == 1 }) {
		t.Fatal("the first stream did not start")
	}

	// A second channel gets the "stream limit" video
	var second = readStream(t.Context(), srv, source.URL+"/ts/two", 64*1024, 10*time.Second)
	if second.contentType != "video/mpeg" || len(second.data) == 0 || source.hit("/ts/two") != 0 {
		t.Errorf("second channel: content type %q, %d bytes, source requests %d; want the stream limit video",
			second.contentType, len(second.data), source.hit("/ts/two"))
	}

	cancel()
	<-first

	// The tuner is free again
	var third = readStream(t.Context(), srv, source.URL+"/ts/three", 64*1024, 10*time.Second)
	if len(third.data) < 64*1024 || !isTS(third.data) {
		t.Errorf("after the first client left: %d bytes (TS: %v), err %v", len(third.data), isTS(third.data), third.err)
	}
}

func TestServeRedirect(t *testing.T) {
	var source = newTestSource(t)
	var srv = bufferTest(t, 2)

	var res = readStream(t.Context(), srv, source.URL+"/redirect", 128*1024, 10*time.Second)
	if len(res.data) < 128*1024 || !isTS(res.data) || source.hit("/ts/redirected") == 0 {
		t.Errorf("got %d bytes (TS: %v), redirected requests %d", len(res.data), isTS(res.data),
			source.hit("/ts/redirected"))
	}
}

// The source closes the connection after 300 KB, the buffer connects again.
func TestServeReconnect(t *testing.T) {
	var source = newTestSource(t)
	var srv = bufferTest(t, 2)

	var res = readStream(t.Context(), srv, source.URL+"/drop", 700*1024, 20*time.Second)
	if len(res.data) < 700*1024 || source.hit("/drop") < 2 {
		t.Errorf("got %d bytes from %d source requests, want 700 KB from at least 2", len(res.data),
			source.hit("/drop"))
	}
}

func TestServeMissingSource(t *testing.T) {
	var source = newTestSource(t)
	var srv = bufferTest(t, 1)

	var res = readStream(t.Context(), srv, source.URL+"/missing", 1024, 10*time.Second)
	if res.status != http.StatusBadGateway || len(res.data) == 0 || isTS(res.data) {
		t.Errorf("missing source: status %d, %q; want 502 with the error", res.status, res.data)
	}

	// The tuner is free again
	var next = readStream(t.Context(), srv, source.URL+"/ts/next", 64*1024, 10*time.Second)
	if len(next.data) < 64*1024 {
		t.Errorf("after a missing source: %d bytes, err %v", len(next.data), next.err)
	}
}

func TestServeHLS(t *testing.T) {
	var source = newTestSource(t)
	var srv = bufferTest(t, 2)

	var res = readStream(t.Context(), srv, source.URL+"/hls/index.m3u8", 256*1024, 20*time.Second)
	if len(res.data) < 256*1024 || !isTS(res.data) {
		t.Errorf("got %d bytes (TS: %v), err %v", len(res.data), isTS(res.data), res.err)
	}
	var segments int
	source.hits.Range(func(k, _ any) bool {
		if strings.HasPrefix(k.(string), "/hls/seg") {
			segments++
		}
		return true
	})
	if segments < 3 {
		t.Errorf("segments downloaded = %d, want several", segments)
	}
}

func TestServeManyClients(t *testing.T) {
	var source = newTestSource(t)
	var srv = bufferTest(t, 3)

	var wg sync.WaitGroup
	var failed atomic.Int32
	for i := range 9 {
		wg.Go(func() {
			// Three channels, three clients each, leaving at different times
			var res = readStream(t.Context(), srv, fmt.Sprintf("%s/ts/c%d", source.URL, i%3), (64+i*16)*1024,
				15*time.Second)
			if len(res.data) < (64+i*16)*1024 || !isTS(res.data) {
				failed.Add(1)
			}
		})
	}
	wg.Wait()

	if failed.Load() > 0 {
		t.Errorf("%d of 9 clients did not get their data", failed.Load())
	}
	if !waitFor(5*time.Second, func() bool { return source.active.Load() == 0 }) {
		t.Errorf("source connections after all clients left: %d", source.active.Load())
	}
}

// A slow source gets the headers before its first data, so that the client does not give up.
func TestServeSlowSource(t *testing.T) {
	var source = newTestSource(t)
	var srv = bufferTest(t, 1)

	req, err := http.NewRequestWithContext(t.Context(), http.MethodGet, srv.URL+"/?url="+source.URL+"/slow", nil)
	if err != nil {
		t.Fatal(err)
	}
	var start = time.Now()
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = resp.Body.Close() }()
	var headers = time.Since(start)
	if resp.StatusCode != http.StatusOK || resp.Header.Get("Content-Type") != "video/mpeg" || headers > 4*time.Second {
		t.Errorf("status %d, %s after %v; want 200 video/mpeg before the first data", resp.StatusCode,
			resp.Header.Get("Content-Type"), headers)
	}

	var data = make([]byte, 64*1024)
	if _, err := io.ReadFull(resp.Body, data); err != nil || !isTS(data) {
		t.Errorf("stream after the headers: %v", err)
	}
}

// The variant follows the measured bandwidth: the high variant is too slow, the low one is used.
func TestServeAdaptiveHLS(t *testing.T) {
	var source = newTestSource(t)
	var srv = bufferTest(t, 1)

	var ctx, cancel = context.WithCancel(t.Context())
	defer cancel()
	var res = make(chan streamResult, 1)
	go func() { res <- readStream(ctx, srv, source.URL+"/abr/master.m3u8", 1<<30, 30*time.Second) }()

	var segments = func(variant string) (n int32) {
		source.hits.Range(func(k, v any) bool {
			if strings.HasPrefix(k.(string), "/abr/"+variant+"/seg") {
				n += v.(*atomic.Int32).Load()
			}
			return true
		})
		return n
	}
	// The setting (10 Mbit/s) chooses the high variant first
	if !waitFor(20*time.Second, func() bool { return segments("high") > 0 && segments("low") > 0 }) {
		t.Errorf("segments: high %d, low %d; want a switch to the low variant", segments("high"), segments("low"))
	}
	cancel()
	<-res
}
