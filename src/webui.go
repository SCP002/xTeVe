package src

import (
	"encoding/json"
	"errors"
	"io/fs"
	"mime"
	"net/http"
	"net/url"
	"strings"
	"time"

	"xteve/src/internal/authentication"
	"xteve/src/internal/config"
)

// The web UI (web/ in the repository) is served at /web/. Its data comes over the WebSocket /data/; /web/api/ has
// the login and the first user.

// uiFiles is web/dist: the build of Vite in app/, or only placeholder.html if the web UI was not built.
var uiFiles fs.FS

// SetUIFiles sets where the files of the web UI come from.
func SetUIFiles(files fs.FS) { uiFiles = files }

// uiTokenCookie is the cookie of the login token of the web UI.
const uiTokenCookie = "Token"

// UI serves the web UI.
func UI(w http.ResponseWriter, r *http.Request) {
	if uiFiles == nil {
		httpStatusError(w, http.StatusNotFound)
		return
	}

	var app, err = fs.Sub(uiFiles, "app")
	if err == nil {
		_, err = fs.Stat(app, "index.html")
	}
	if err != nil {
		// Not built: explain how to build it
		var page, err = fs.ReadFile(uiFiles, "placeholder.html")
		if err != nil {
			httpStatusError(w, http.StatusNotFound)
			return
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Header().Set("Cache-Control", "no-cache")
		w.WriteHeader(http.StatusServiceUnavailable)
		writeResponse(w, page)
		return
	}

	// Files of Vite with a hash in the name never change, index.html must be loaded again
	if strings.HasPrefix(r.URL.Path, "/web/assets/") {
		w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
	} else {
		w.Header().Set("Cache-Control", "no-cache")
	}
	http.StripPrefix("/web/", http.FileServerFS(app)).ServeHTTP(w, r)
}

// uiSession is the state the web UI starts with.
type uiSession struct {
	// Authentication: the web UI needs a login
	Authentication bool `json:"authentication"`
	// FirstUser: no user exists yet, the first one has to be created
	FirstUser bool `json:"firstUser"`
	// LoggedIn: the token cookie is valid
	LoggedIn bool `json:"loggedIn"`
	// Wizard: the configuration wizard runs (no playlist yet)
	Wizard bool `json:"wizard"`
}

type uiCredentials struct {
	Username string `json:"username"`
	Password string `json:"password"`
	// Confirm is the repeated password of the first user
	Confirm string `json:"confirm"`
}

type uiError struct {
	Error string `json:"error"`
}

type uiToken struct {
	Token string `json:"token"`
}

// UIAPI answers the JSON requests of the web UI at /web/api/.
func UIAPI(w http.ResponseWriter, r *http.Request) {
	setGlobalDomain(r.Host)

	switch r.URL.Path {
	case "/web/api/session":
		if r.Method != http.MethodGet {
			uiReply(w, http.StatusMethodNotAllowed, uiError{Error: "GET only"})
			return
		}
		uiReply(w, http.StatusOK, currentUISession(r))

	case "/web/api/login":
		var c, ok = uiReadCredentials(w, r)
		if !ok {
			return
		}
		token, err := authentication.UserAuthentication(c.Username, c.Password)
		if err == nil {
			err = checkAuthorizationLevel(token, "authentication.web")
		}
		if err != nil {
			uiReply(w, http.StatusUnauthorized, uiError{Error: "User authentication failed"})
			return
		}
		uiLoggedIn(w, token)

	case "/web/api/first-user":
		var c, ok = uiReadCredentials(w, r)
		if !ok {
			return
		}
		var users, err = authentication.GetAllUserData()
		switch {
		case err != nil:
			ShowError(err, 0)
			uiReply(w, http.StatusInternalServerError, uiError{Error: userMessage(err)})
			return
		case len(users) > 0:
			uiReply(w, http.StatusConflict, uiError{Error: "A user exists already, log in instead"})
			return
		case c.Username == "" || c.Password == "":
			uiReply(w, http.StatusBadRequest, uiError{Error: "Username and password are required"})
			return
		case c.Password != c.Confirm:
			uiReply(w, http.StatusBadRequest, uiError{Error: "The passwords do not match"})
			return
		}
		token, err := createFirstUserForAuthentication(c.Username, c.Password)
		if err != nil {
			uiReply(w, http.StatusBadRequest, uiError{Error: userMessage(err)})
			return
		}
		uiLoggedIn(w, token)

	default:
		uiReply(w, http.StatusNotFound, uiError{Error: "unknown request"})
	}
}

func currentUISession(r *http.Request) uiSession {
	// Without playlists the configuration wizard runs
	if !System.Scanning() && len(config.Get().Files.M3U) == 0 && len(config.Get().Files.HDHR) == 0 {
		System.ConfigurationWizard.Store(true)
	}

	var s = uiSession{Authentication: config.Get().AuthenticationWEB, Wizard: System.ConfigurationWizard.Load()}
	if users, err := authentication.GetAllUserData(); err == nil {
		s.FirstUser = len(users) == 0
	}
	if cookie, err := r.Cookie(uiTokenCookie); err == nil {
		_, err := authentication.GetUserID(cookie.Value)
		s.LoggedIn = err == nil
	}
	return s
}

// uiReadCredentials reads the credentials of a POST request. Only JSON from a page of the same host is accepted:
// another web page can send neither without the browser asking xTeVe first, which it does not allow.
func uiReadCredentials(w http.ResponseWriter, r *http.Request) (uiCredentials, bool) {
	var c uiCredentials
	if r.Method != http.MethodPost {
		uiReply(w, http.StatusMethodNotAllowed, uiError{Error: "POST only"})
		return c, false
	}
	if mediaType, _, _ := mime.ParseMediaType(r.Header.Get("Content-Type")); mediaType != "application/json" {
		uiReply(w, http.StatusUnsupportedMediaType, uiError{Error: "JSON only"})
		return c, false
	}
	if !sameOrigin(r) {
		uiReply(w, http.StatusForbidden, uiError{Error: "request from another site"})
		return c, false
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<16)).Decode(&c); err != nil {
		uiReply(w, http.StatusBadRequest, uiError{Error: "invalid request"})
		return c, false
	}
	return c, true
}

// sameOrigin reports whether a request without an Origin header (not a browser) or from a page of the same host.
func sameOrigin(r *http.Request) bool {
	var origin = r.Header.Get("Origin")
	if origin == "" {
		return true
	}
	var u, err = url.Parse(origin)
	return err == nil && strings.EqualFold(u.Host, r.Host)
}

// uiLoggedIn sets the token cookie of the web UI and answers with the token.
func uiLoggedIn(w http.ResponseWriter, token string) {
	http.SetCookie(w, &http.Cookie{
		Name:     uiTokenCookie,
		Value:    token,
		Path:     "/web/",
		Expires:  time.Now().Add(time.Hour),
		SameSite: http.SameSiteStrictMode,
	})
	uiReply(w, http.StatusOK, uiToken{Token: token})
}

func uiReply(w http.ResponseWriter, status int, v any) {
	var content, err = json.Marshal(v)
	if err != nil {
		ShowError(errors.Join(errors.New("web UI reply"), err), 0)
		status, content = http.StatusInternalServerError, []byte(`{"error":"internal error"}`)
	}
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(status)
	writeResponse(w, content)
}
