package imgcache

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
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
		if got := c.Image.GetURL(src); got != src {
			t.Errorf("GetURL(%s) = %s before caching, want the original", src, got)
		}
	}

	if err := c.Image.Caching(); err != nil {
		t.Fatal(err)
	}

	for _, src := range []string{logo, withQuery} {
		var got = c.Image.GetURL(src)
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
	if got := off.Image.GetURL(logo); got != logo {
		t.Errorf("without caching: GetURL = %s, want the original", got)
	}
}
