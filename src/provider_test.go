package src

import (
	"bytes"
	"compress/gzip"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
)

func gzipped(t *testing.T, data string) []byte {
	t.Helper()
	var buf bytes.Buffer
	var w = gzip.NewWriter(&buf)
	if _, err := w.Write([]byte(data)); err != nil {
		t.Fatal(err)
	}
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

func TestFetchProvider(t *testing.T) {
	const playlist = "#EXTM3U\n#EXTINF:-1,One\nhttp://example.com/1\n"
	var dir = t.TempDir()
	var gz = filepath.Join(dir, "playlist.m3u.gz")
	if err := os.WriteFile(gz, gzipped(t, playlist), 0644); err != nil {
		t.Fatal(err)
	}

	var srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Disposition", `attachment; filename="provider list.m3u"`)
		_, _ = w.Write(gzipped(t, playlist))
	}))
	defer srv.Close()

	for name, source := range map[string]string{"local gzip": gz, "HTTP gzip": srv.URL + "/get?token=1"} {
		var dst = filepath.Join(dir, "out.m3u")
		var filename, err = fetchProvider(source, "m3u", dst)
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		if content, _ := os.ReadFile(dst); string(content) != playlist {
			t.Errorf("%s: content = %q, want the unpacked playlist", name, content)
		}
		if err := checkProviderFile("M1", "m3u", dst); err != nil {
			t.Errorf("%s: check: %v", name, err)
		}
		if name == "HTTP gzip" && filename != "provider list.m3u" {
			t.Errorf("file name = %q, want the one of the Content-Disposition header", filename)
		}
	}

	var notFound = httptest.NewServer(http.NotFoundHandler())
	defer notFound.Close()
	if _, err := fetchProvider(notFound.URL, "m3u", filepath.Join(dir, "missing.m3u")); err == nil {
		t.Error("no error for 404")
	}
}
