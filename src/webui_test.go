package src

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"testing/fstest"

	"xteve/src/internal/authentication"
	"xteve/src/internal/config"
)

// uiTest sets up an empty user database with web authentication and returns a server for /web/api/.
func uiTest(t *testing.T) *httptest.Server {
	t.Helper()
	setSettings(t, func(s *SettingsStruct) {
		s.AuthenticationWEB = true
		s.Files.M3U = config.Providers{"M1": {Name: "Playlist"}}
	})
	if err := authentication.Init(t.TempDir()+string(os.PathSeparator), 60); err != nil {
		t.Fatal(err)
	}
	var srv = httptest.NewServer(http.HandlerFunc(UIAPI))
	t.Cleanup(srv.Close)
	return srv
}

func uiPost(t *testing.T, srv *httptest.Server, path, body string, header map[string]string) (int, string) {
	t.Helper()
	var req, err = http.NewRequestWithContext(t.Context(), http.MethodPost, srv.URL+path, strings.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Content-Type", "application/json")
	for k, v := range header {
		req.Header.Set(k, v)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = resp.Body.Close() }()
	var content, _ = io.ReadAll(resp.Body)
	return resp.StatusCode, string(content)
}

func uiSessionOf(t *testing.T, srv *httptest.Server, cookie string) uiSession {
	t.Helper()
	var req, _ = http.NewRequestWithContext(t.Context(), http.MethodGet, srv.URL+"/web/api/session", nil)
	if cookie != "" {
		req.AddCookie(&http.Cookie{Name: uiTokenCookie, Value: cookie})
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = resp.Body.Close() }()
	var s uiSession
	if err := json.NewDecoder(resp.Body).Decode(&s); err != nil {
		t.Fatal(err)
	}
	return s
}

// The first user is created once, with a matching confirmation; then users log in.
func TestUIFirstUserAndLogin(t *testing.T) {
	var srv = uiTest(t)

	if s := uiSessionOf(t, srv, ""); !s.Authentication || !s.FirstUser || s.LoggedIn {
		t.Errorf("session before the first user = %+v", s)
	}

	for _, c := range []struct {
		body   string
		status int
	}{
		{`{"username":"admin","password":"secret","confirm":"other"}`, http.StatusBadRequest},
		{`{"username":"","password":"secret","confirm":"secret"}`, http.StatusBadRequest},
		{`{"username":"admin","password":"` + strings.Repeat("x", 73) + `","confirm":"` + strings.Repeat("x", 73) +
			`"}`, http.StatusBadRequest},
		{`{"username":"admin","password":"secret","confirm":"secret"}`, http.StatusOK},
		{`{"username":"other","password":"secret","confirm":"secret"}`, http.StatusConflict},
	} {
		if status, body := uiPost(t, srv, "/web/api/first-user", c.body, nil); status != c.status {
			t.Errorf("first user %s: %d %s, want %d", c.body, status, body, c.status)
		}
	}

	if status, _ := uiPost(t, srv, "/web/api/login", `{"username":"admin","password":"wrong"}`, nil); status != 401 {
		t.Errorf("wrong password: %d, want 401", status)
	}
	status, body := uiPost(t, srv, "/web/api/login", `{"username":"admin","password":"secret"}`, nil)
	var token uiToken
	if status != http.StatusOK || json.Unmarshal([]byte(body), &token) != nil || token.Token == "" {
		t.Fatalf("login: %d %s", status, body)
	}
	if s := uiSessionOf(t, srv, token.Token); s.FirstUser || !s.LoggedIn {
		t.Errorf("session after the login = %+v", s)
	}
}

// Other sites can not send requests: browsers send forms without asking, JSON only after asking xTeVe.
func TestUIRequestsFromOtherSites(t *testing.T) {
	var srv = uiTest(t)
	var body = `{"username":"admin","password":"secret","confirm":"secret"}`

	if status, _ := uiPost(
		t,
		srv,
		"/web/api/first-user",
		body,
		map[string]string{"Origin": "http://evil.example"},
	); status != 403 {
		t.Errorf("request of another site: %d, want 403", status)
	}
	if status, _ := uiPost(
		t,
		srv,
		"/web/api/first-user",
		body,
		map[string]string{"Content-Type": "text/plain"},
	); status != 415 {
		t.Errorf("request without JSON: %d, want 415", status)
	}
	if status, _ := uiPost(t, srv, "/web/api/first-user", body, map[string]string{"Origin": srv.URL}); status != 200 {
		t.Errorf("request of the web UI itself: %d, want 200", status)
	}
}

// The built web UI is served with caching for the hashed files; without a build the placeholder explains it.
func TestUIFiles(t *testing.T) {
	var get = func(path string) (*http.Response, string) {
		var w = httptest.NewRecorder()
		UI(w, httptest.NewRequestWithContext(t.Context(), http.MethodGet, path, nil))
		return w.Result(), w.Body.String()
	}

	SetUIFiles(fstest.MapFS{"placeholder.html": {Data: []byte("not built")}})
	t.Cleanup(func() { SetUIFiles(nil) })
	if resp, body := get("/web/"); resp.StatusCode != 503 || body != "not built" {
		t.Errorf("without a build: %d %q", resp.StatusCode, body)
	}

	SetUIFiles(fstest.MapFS{
		"placeholder.html":      {Data: []byte("not built")},
		"app/index.html":        {Data: []byte("<html>app</html>")},
		"app/assets/index-1.js": {Data: []byte("js")},
	})
	if resp, body := get("/web/"); resp.StatusCode != 200 || body != "<html>app</html>" ||
		resp.Header.Get("Cache-Control") != "no-cache" {
		t.Errorf("index: %d %q %s", resp.StatusCode, body, resp.Header.Get("Cache-Control"))
	}
	if resp, body := get("/web/assets/index-1.js"); resp.StatusCode != 200 || body != "js" ||
		!strings.Contains(resp.Header.Get("Cache-Control"), "immutable") {
		t.Errorf("asset: %d %q %s", resp.StatusCode, body, resp.Header.Get("Cache-Control"))
	}
	if resp, _ := get("/web/missing.js"); resp.StatusCode != 404 {
		t.Errorf("missing file: %d", resp.StatusCode)
	}
}
