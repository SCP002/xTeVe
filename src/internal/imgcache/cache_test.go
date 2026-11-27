package imgcache

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestCaching(t *testing.T) {
	var srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "image/png")
		_, _ = w.Write([]byte("png " + r.URL.Path))
	}))
	defer srv.Close()

	var dir = t.TempDir()
	c, err := New(dir, "http://xteve:34400/images/", true)
	if err != nil {
		t.Fatal(err)
	}

	var logo, withQuery = srv.URL + "/logo/1.png", srv.URL + "/logo/2.png?size=big"

	// Before caching the original URLs are used and the images are queued
	for _, src := range []string{logo, withQuery} {
		if got := c.GetURL(src); got != src {
			t.Errorf("GetURL(%s) = %s before caching, want the original", src, got)
		}
	}

	if n, err := c.Download(); err != nil || n != 2 {
		t.Fatalf("Download = %d, %v; want 2 images", n, err)
	}

	for _, src := range []string{logo, withQuery} {
		var got = c.GetURL(src)
		if !strings.HasPrefix(got, "http://xteve:34400/images/") || strings.Contains(got, dir) ||
			!strings.HasSuffix(got, ".png") {
			t.Errorf("GetURL(%s) = %s after caching, want an image URL of xTeVe", src, got)
			continue
		}
		var file = filepath.Join(dir, strings.TrimPrefix(got, "http://xteve:34400/images/"))
		if _, err := os.Stat(file); err != nil {
			t.Errorf("cached file of %s: %v", src, err)
		}
	}

	// Without caching the original URLs are used
	off, err := New(dir, "http://xteve:34400/images/", false)
	if err != nil {
		t.Fatal(err)
	}
	if got := off.GetURL(logo); got != logo {
		t.Errorf("without caching: GetURL = %s, want the original", got)
	}
}

// GetURL answers while images are downloaded, e.g. when the M3U and XMLTV files are written.
func TestGetURLDuringDownload(t *testing.T) {
	var release = make(chan struct{})
	var started = make(chan struct{}, 1)
	var srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		select {
		case started <- struct{}{}:
		default:
		}
		<-release
		_, _ = w.Write([]byte("png"))
	}))
	defer srv.Close()

	c, err := New(t.TempDir(), "http://xteve/images/", true)
	if err != nil {
		t.Fatal(err)
	}
	c.GetURL(srv.URL + "/slow.png")

	var done = make(chan error)
	go func() { _, err := c.Download(); done <- err }()
	<-started

	var answered = make(chan string)
	go func() { answered <- c.GetURL(srv.URL + "/other.png") }()
	select {
	case <-answered:
	case <-time.After(5 * time.Second):
		t.Fatal("GetURL waits for the download")
	}
	close(release)
	if err := <-done; err != nil {
		t.Fatal(err)
	}
	// other.png was queued during the download and is still waiting
	if c.Queued() != 1 {
		t.Errorf("queued = %d, want 1", c.Queued())
	}
}

// A failed image stays queued; Remove keeps the images the build uses and deletes the others.
func TestFailedAndRemove(t *testing.T) {
	var srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains(r.URL.Path, "missing") {
			http.NotFound(w, r)
			return
		}
		_, _ = w.Write([]byte("png"))
	}))
	defer srv.Close()

	var dir = t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "old.png"), []byte("old"), 0644); err != nil {
		t.Fatal(err)
	}
	c, err := New(dir, "http://xteve/images/", true)
	if err != nil {
		t.Fatal(err)
	}
	var ok, missing = srv.URL + "/ok.png", srv.URL + "/missing.png"
	c.GetURL(ok)
	c.GetURL(missing)

	if _, err := c.Download(); err == nil || !strings.Contains(err.Error(), "1 of 2") {
		t.Errorf("Download error = %v, want 1 of 2 failed", err)
	}
	if c.Queued() != 1 || c.GetURL(missing) != missing {
		t.Errorf("queued = %d, the failed image must stay queued", c.Queued())
	}
	if err := c.Remove(); err != nil {
		t.Fatal(err)
	}
	var files, _ = os.ReadDir(dir)
	if len(files) != 1 || files[0].Name() != name(ok) {
		t.Errorf("files after Remove = %v, want only %s", files, name(ok))
	}
}

// An image that another build downloaded meanwhile is not downloaded again.
func TestDownloadedMeanwhile(t *testing.T) {
	var srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("png"))
	}))
	defer srv.Close()

	var dir = t.TempDir()
	var first, _ = New(dir, "http://xteve/images/", true)
	var second, _ = New(dir, "http://xteve/images/", true)
	var logo = srv.URL + "/logo.png"
	first.GetURL(logo)
	second.GetURL(logo)

	if n, err := first.Download(); err != nil || n != 1 {
		t.Fatalf("first Download = %d, %v", n, err)
	}
	if n, err := second.Download(); err != nil || n != 0 {
		t.Errorf("second Download = %d, %v; want nothing downloaded", n, err)
	}
	if got := second.GetURL(logo); got != "http://xteve/images/"+name(logo) {
		t.Errorf("GetURL = %s, want the cached image", got)
	}
}
