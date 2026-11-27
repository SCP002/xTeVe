package src

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gorilla/websocket"
)

// Only pages of the same host may open the WebSocket of the web UI.
func TestWebSocketOrigin(t *testing.T) {
	var srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var conn, err = wsUpgrader.Upgrade(w, r, nil)
		if err != nil {
			http.Error(w, "Could not open websocket connection", http.StatusBadRequest)
			return
		}
		_ = conn.Close()
	}))
	defer srv.Close()

	var url = "ws" + strings.TrimPrefix(srv.URL, "http") + "/data/"
	for _, c := range []struct {
		origin string
		ok     bool
	}{
		{"", true},                                 // not a browser
		{srv.URL, true},                            // the web UI itself
		{strings.ToUpper(srv.URL), true},           // host names are case-insensitive
		{"http://evil.example", false},             // another web page
		{"http://evil.example/?" + srv.URL, false}, // only the host counts
	} {
		var header = http.Header{}
		if c.origin != "" {
			header.Set("Origin", c.origin)
		}
		var conn, resp, err = websocket.DefaultDialer.Dial(url, header)
		if conn != nil {
			_ = conn.Close()
		}
		if resp != nil {
			_ = resp.Body.Close()
		}
		if (err == nil) != c.ok {
			t.Errorf("origin %q: err %v, want ok = %v", c.origin, err, c.ok)
		}
	}
}
