package src

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"xteve/src/internal/authentication"
	"xteve/src/internal/config"
	"xteve/src/internal/logger"

	"github.com/gorilla/websocket"
	"github.com/samber/lo"
)

// webAlerts channel to send to client
var webAlerts = make(chan string, 3)
var restartWebserver = make(chan bool, 1)

// StartWebserver : Start the Webserver
// wsUpgrader upgrades web UI connections to WebSocket.
//
// Origin is not checked and handshake errors are reported by the caller,
// matching the behavior of the deprecated websocket.Upgrade function.
var wsUpgrader = websocket.Upgrader{
	ReadBufferSize:  1024,
	WriteBufferSize: 1024,
	CheckOrigin:     func(r *http.Request) bool { return true },
	Error:           func(w http.ResponseWriter, r *http.Request, status int, reason error) {},
}

func StartWebserver() (err error) {
	http.HandleFunc("/", Index)
	http.HandleFunc("/stream/", Stream)
	http.HandleFunc("/xmltv/", xTeVe)
	http.HandleFunc("/m3u/", xTeVe)
	http.HandleFunc("/data/", WS)
	http.HandleFunc("/web/", Web)
	http.HandleFunc("/download/", Download)
	http.HandleFunc("/api/", API)
	http.HandleFunc("/images/", Images)
	http.HandleFunc("/data_images/", DataImages)

	for {
		showInfo("Web server:" + "Starting")

		showInfo("DVR IP:" + config.Get().HostIP + ":" + config.Get().Port)

		var ips = len(System.IPAddressesV4) + len(System.IPAddressesV6) - 1
		switch ips {
		case 0:
			showHighlight(
				fmt.Sprintf("Web Interface:%s://%s:%s/web/", System.Protocol(), config.Get().HostIP, config.Get().Port),
			)

		case 1:
			showHighlight(
				fmt.Sprintf(
					"Web Interface:%s://%s:%s/web/ | xTeVe is also available via the other %d IP.",
					System.Protocol(),
					config.Get().HostIP,
					config.Get().Port,
					ips,
				),
			)

		default:
			showHighlight(
				fmt.Sprintf(
					"Web Interface:%s://%s:%s/web/ | xTeVe is also available via the other %d IP's.",
					System.Protocol(),
					config.Get().HostIP,
					config.Get().Port,
					len(System.IPAddressesV4)+len(System.IPAddressesV6)-1,
				),
			)
		}

		var port = config.Get().Port
		server := http.Server{Addr: ":" + port}

		// A server that cannot run stops xTeVe instead of leaving it without a web server
		var serverErr = make(chan error, 1)

		go func() {
			var err error

			if config.Get().TLSMode {
				if !allFilesExist(System.File.ServerCertPrivKey, System.File.ServerCert) {
					if err = genCertFiles(); err != nil {
						ShowError(err, 7000)
					}
				}

				System.SetTLS(true)
				err = server.ListenAndServeTLS(System.File.ServerCert, System.File.ServerCertPrivKey)
				if err != nil && err != http.ErrServerClosed {
					// Fall back to HTTP, the URLs handed out then use http as well
					ShowError(err, 1017)
					System.SetTLS(false)
					err = server.ListenAndServe()
				}
			} else {
				System.SetTLS(false)
				err = server.ListenAndServe()
			}

			if err != nil && err != http.ErrServerClosed {
				serverErr <- err
			}
		}()

		select {
		case err = <-serverErr:
			return withCode(err, 1001)
		case <-restartWebserver:
		}

		showInfo("Web server:" + "Restarting")

		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		err = server.Shutdown(ctx)
		cancel()
		if err != nil {
			return withCode(err, 1016)
		}

		showInfo("Web server:" + "Stopped")
	}
}

// Index : Web Server /
func Index(w http.ResponseWriter, r *http.Request) {
	var err error
	var response []byte
	var path = r.URL.Path
	var debug string

	setGlobalDomain(r.Host)

	debug = fmt.Sprintf("Web Server Request:Path: %s", path)
	showDebug(debug, 2)

	switch path {
	case "/discover.json":
		response, err = getDiscover()
		w.Header().Set("Content-Type", "application/json")

	case "/lineup_status.json":
		response, err = getLineupStatus()
		w.Header().Set("Content-Type", "application/json")

	case "/lineup.json":
		if config.Get().AuthenticationPMS {
			_, err := basicAuth(r, "authentication.pms")
			if err != nil {
				ShowError(err, 000)
				httpStatusError(w, 403)
				return
			}
		}

		response, err = getLineup()
		w.Header().Set("Content-Type", "application/json")

	case "/device.xml", "/capability":
		response, err = getCapability()
		w.Header().Set("Content-Type", "application/xml")

	default:
		response, err = getCapability()
		w.Header().Set("Content-Type", "application/xml")
	}

	if err == nil {
		w.WriteHeader(200)
		writeResponse(w, response)
		return
	}

	ShowError(err, 0)
	httpStatusError(w, 500)
}

// Stream : Web Server /stream/
func Stream(w http.ResponseWriter, r *http.Request) {
	var path = strings.Replace(r.RequestURI, "/stream/", "", 1)

	streamInfo, err := getStreamInfo(path)
	if err != nil {
		ShowError(err, 1203)
		httpStatusError(w, 404)
		return
	}

	// If an UDPxy host is set, and the stream URL is multicast (i.e. starts with 'udp://@'),
	// then streamInfo.URL needs to be rewritten to point to UDPxy.
	if config.Get().UDPxy != "" && strings.HasPrefix(streamInfo.URL, "udp://@") {
		streamInfo.URL = fmt.Sprintf(
			"http://%s/udp/%s/",
			config.Get().UDPxy,
			strings.TrimPrefix(streamInfo.URL, "udp://@"),
		)
	}

	switch config.Get().Buffer {
	case "-":
		showInfo(fmt.Sprintf("Buffer:false [%s]", config.Get().Buffer))

	case "xteve":
		if strings.Contains(streamInfo.URL, "rtsp://") || strings.Contains(streamInfo.URL, "rtp://") {
			err = errors.New("RTSP and RTP streams are not supported")
			ShowError(err, 2004)

			showInfo("Streaming URL:" + streamInfo.URL)
			http.Redirect(w, r, streamInfo.URL, http.StatusFound)

			showInfo("Streaming Info:URL was passed to the client")
			return
		}

		showInfo(fmt.Sprintf("Buffer:true [%s]", config.Get().Buffer))

	default:
		showInfo(fmt.Sprintf("Buffer:true [%s]", config.Get().Buffer))
	}

	if config.Get().Buffer != "-" {
		showInfo(fmt.Sprintf("Buffer Size:%d KB", config.Get().BufferSize))
	}

	showInfo(fmt.Sprintf("Channel Name:%s", streamInfo.Name))
	showInfo(fmt.Sprintf("Client User-Agent:%s", r.Header.Get("User-Agent")))

	// Check whether the Buffer should be used
	switch config.Get().Buffer {
	case "-":
		showInfo("Streaming URL:" + streamInfo.URL)
		http.Redirect(w, r, streamInfo.URL, http.StatusFound)

		showInfo("Streaming Info:URL was passed to the client.")
		showInfo("Streaming Info:xTeVe is no longer involved, the client connects directly to the streaming server.")

	default:
		buffers.Serve(w, r, streamInfo.PlaylistID, streamInfo.URL, streamInfo.Name)
	}
}

// xTeVe : Web Server /xmltv/ and /m3u/
func xTeVe(w http.ResponseWriter, r *http.Request) {
	var requestType, groupTitle, file, content, contentType string
	var err error
	var path = strings.TrimPrefix(r.URL.Path, "/")
	var groups = []string{}

	setGlobalDomain(r.Host)

	// XMLTV File
	if strings.Contains(path, "xmltv/") {
		requestType = "xml"

		file = System.Folder.Data + getFilenameFromPath(path)

		content, err = readStringFromFile(file)
		if err != nil {
			logUnlessNotExist(err)
			httpStatusError(w, 404)
			return
		}
	}

	// M3U File
	if strings.Contains(path, "m3u/") {
		requestType = "m3u"
		groupTitle = r.URL.Query().Get("group-title")

		if !System.Dev {
			// false: File name is set in the header
			// true: M3U is displayed directly in the browser
			w.Header().Set("Content-Disposition", "attachment; filename="+getFilenameFromPath(path))
		}

		if len(groupTitle) > 0 {
			groups = strings.Split(groupTitle, ",")
		}

		content, err = buildM3U(groups)
		if err != nil {
			ShowError(err, 000)
		}
	}

	// Check Authentication
	err = urlAuth(r, requestType)
	if err != nil {
		ShowError(err, 000)
		httpStatusError(w, 403)
		return
	}

	contentType = http.DetectContentType([]byte(content))
	if strings.Contains(strings.ToLower(contentType), "xml") {
		contentType = "application/xml; charset=utf-8"
	}

	w.Header().Set("Content-Type", contentType)
	writeResponse(w, []byte(content))
}

// Images : Image Cache /images/
func Images(w http.ResponseWriter, r *http.Request) {
	var path = strings.TrimPrefix(r.URL.Path, "/")
	var filePath = System.Folder.ImagesCache + getFilenameFromPath(path)

	content, err := readByteFromFile(filePath)
	if err != nil {
		httpStatusError(w, 404)
		return
	}

	w.Header().Add("Content-Type", getContentType(filePath))
	w.Header().Add("Content-Length", fmt.Sprintf("%d", len(content)))
	w.WriteHeader(200)
	writeResponse(w, content)
}

// DataImages : Image path for Logos / Images that have been uploaded / data_images /
func DataImages(w http.ResponseWriter, r *http.Request) {
	var path = strings.TrimPrefix(r.URL.Path, "/")
	var filePath = System.Folder.ImagesUpload + getFilenameFromPath(path)

	content, err := readByteFromFile(filePath)
	if err != nil {
		httpStatusError(w, 404)
		return
	}

	w.Header().Add("Content-Type", getContentType(filePath))
	w.Header().Add("Content-Length", fmt.Sprintf("%d", len(content)))
	w.WriteHeader(200)
	writeResponse(w, content)
}

// WS : Web Sockets /ws/
func WS(w http.ResponseWriter, r *http.Request) {
	var request RequestStruct
	var response ResponseStruct
	response.Status = true

	var newToken string

	conn, err := wsUpgrader.Upgrade(w, r, w.Header())
	if err != nil {
		ShowError(err, 0)
		http.Error(w, "Could not open websocket connection", http.StatusBadRequest)
		return
	}
	defer closeOrLog(conn)

	setGlobalDomain(r.Host)

	for {
		select {
		case response.Alert = <-webAlerts:
			//
		default:
			//
		}

		err = conn.ReadJSON(&request)

		if err != nil {
			return
		}

		if !System.ConfigurationWizard.Load() {
			switch config.Get().AuthenticationWEB {
			// Token Authentication
			case true:
				var token string
				tokens, ok := r.URL.Query()["Token"]

				if !ok || len(tokens[0]) < 1 {
					token = "-"
				} else {
					token = tokens[0]
				}

				newToken, err = tokenAuthentication(token)
				if err != nil {
					response.Status = false
					response.Reload = true
					response.Error = userMessage(err)
					request.Cmd = "-"

					if err = conn.WriteJSON(response); err != nil {
						ShowError(err, 1102)
					}

					return
				}

				response.Token = newToken
				if response.Users, err = authentication.GetAllUserData(); err != nil {
					ShowError(err, 0)
				}
			}
		}

		switch request.Cmd {
		// Read Data
		// Answered with the default response data below
		case "getServerConfig":

		case "updateLog":
			response = setDefaultResponseData(response, false)
			if err = conn.WriteJSON(response); err != nil {
				ShowError(err, 1022)
			}
			return

		// Answered with the default response data below
		case "loadFiles":

		// Save Data
		case "saveSettings":
			var authenticationUpdate = config.Get().AuthenticationWEB
			var previousTLSMode = config.Get().TLSMode
			var previousHostIP = config.Get().HostIP
			var previousStoreBufferInRAM = config.Get().StoreBufferInRAM
			var previousClearXMLTVCache = config.Get().ClearXMLTVCache

			response.Settings, err = updateServerSettings(request)
			if err == nil {
				response.OpenMenu = strconv.Itoa(lo.IndexOf(System.WEB.Menu, "settings"))

				if config.Get().AuthenticationWEB && !authenticationUpdate {
					response.Reload = true
				}

				if config.Get().TLSMode != previousTLSMode {
					showInfo("Web server:" + "Toggling TLS mode")

					reinitialize()

					response.OpenLink = System.URLBase + "/web/"
					restartWebserver <- true
				}

				if config.Get().HostIP != previousHostIP {
					showInfo("Web server:" + fmt.Sprintf("Changing host IP to %s", config.Get().HostIP))

					reinitialize()

					response.OpenLink = System.URLBase + "/web/"
					restartWebserver <- true
				}

				if config.Get().StoreBufferInRAM != previousStoreBufferInRAM {
					initBufferVFS(config.Get().StoreBufferInRAM)
				}

				if config.Get().ClearXMLTVCache && !previousClearXMLTVCache {
					clearXMLTVCache()
				}
			}

		case "saveFilesM3U":
			err = saveFiles(request, "m3u")
			if err == nil {
				response.OpenMenu = strconv.Itoa(lo.IndexOf(System.WEB.Menu, "playlist"))
			}

		case "updateFileM3U":
			err = updateFile(request, "m3u")
			if err == nil {
				response.OpenMenu = strconv.Itoa(lo.IndexOf(System.WEB.Menu, "playlist"))
			}

		case "saveFilesHDHR":
			err = saveFiles(request, "hdhr")
			if err == nil {
				response.OpenMenu = strconv.Itoa(lo.IndexOf(System.WEB.Menu, "playlist"))
			}

		case "updateFileHDHR":
			err = updateFile(request, "hdhr")
			if err == nil {
				response.OpenMenu = strconv.Itoa(lo.IndexOf(System.WEB.Menu, "playlist"))
			}

		case "saveFilesXMLTV":
			err = saveFiles(request, "xmltv")
			if err == nil {
				response.OpenMenu = strconv.Itoa(lo.IndexOf(System.WEB.Menu, "xmltv"))
			}

		case "updateFileXMLTV":
			err = updateFile(request, "xmltv")
			if err == nil {
				response.OpenMenu = strconv.Itoa(lo.IndexOf(System.WEB.Menu, "xmltv"))
			}

		case "saveFilter":
			response.Settings, err = saveFilter(request)
			if err == nil {
				response.OpenMenu = strconv.Itoa(lo.IndexOf(System.WEB.Menu, "filter"))
			}

		case "saveEpgMapping":
			err = saveXEpgMapping(request)

		case "saveUserData":
			err = saveUserData(request)
			if err == nil {
				response.OpenMenu = strconv.Itoa(lo.IndexOf(System.WEB.Menu, "users"))
			}

		case "saveNewUser":
			err = saveNewUser(request)
			if err == nil {
				response.OpenMenu = strconv.Itoa(lo.IndexOf(System.WEB.Menu, "users"))
			}

		case "resetLogs":
			logger.Default().Clear()
			response.OpenMenu = strconv.Itoa(lo.IndexOf(System.WEB.Menu, "log"))

		case "xteveBackup":
			file, errNew := xteveBackup()
			err = errNew
			if err == nil {
				response.OpenLink = fmt.Sprintf("%s://%s/download/%s", System.Protocol(), System.Domain(), file)
			}

		case "xteveRestore":
			logger.Default().Clear()

			if len(request.Base64) > 0 {
				newWebURL, err := xteveRestoreFromWeb(request.Base64)
				if err != nil {
					ShowError(err, 000)
					response.Alert = userMessage(err)
				}

				if err == nil {
					if len(newWebURL) > 0 {
						response.Alert = "Backup was successfully restored.\n" +
							"The port of the xTeVe URL has changed, you have to restart xTeVe.\n" +
							"After a restart, xTeVe can be reached again at the following URL:\n" + newWebURL
					} else {
						response.Alert = "Backup was successfully restored."
						response.Reload = true
					}
					showInfo("xTeVe:" + "Backup successfully restored.")
				}
			}

		case "uploadLogo":
			if len(request.Base64) > 0 {
				response.LogoURL, err = uploadLogo(request.Base64, request.Filename)

				if err == nil {
					if err = conn.WriteJSON(response); err != nil {
						ShowError(err, 1022)
					} else {
						return
					}
				}
			}

		case "saveWizard":
			nextStep, errNew := saveWizard(request)

			err = errNew
			if err == nil {
				if nextStep == 10 {
					System.ConfigurationWizard.Store(false)
					response.Reload = true
				} else {
					response.Wizard = nextStep
				}
			}

		default:
			err = fmt.Errorf("unknown command: %s", request.Cmd)
		}

		// The single place where errors of web UI commands are reported
		if err != nil {
			ShowError(err, 0)
			response.Status = false
			response.Error = userMessage(err)
			response.Settings = *config.Get()
		}

		response = setDefaultResponseData(response, true)
		if System.ConfigurationWizard.Load() {
			response.ConfigurationWizard = System.ConfigurationWizard.Load()
		}

		if err = conn.WriteJSON(response); err != nil {
			ShowError(err, 1022)
		} else {
			break
		}
	}
}

// Web : Web Server /web/
func Web(w http.ResponseWriter, r *http.Request) {
	var lang = make(map[string]any)
	var err error

	var requestFile = strings.ReplaceAll(r.URL.Path, "/web", "html")
	var content, contentType, file string

	var language LanguageUI

	setGlobalDomain(r.Host)

	if System.Dev {
		lang, err = loadJSONFileToMap(fmt.Sprintf("html/lang/%s.json", config.Get().Language))
		if err != nil {
			ShowError(err, 000)
		}
	} else {
		var languageFile = "html/lang/en.json"

		if value, ok := webUI[languageFile].(string); ok {
			content = GetHTMLString(value)
			lang = jsonToMap(content)
		}
	}

	err = json.Unmarshal([]byte(mapToJSON(lang)), &language)
	if err != nil {
		ShowError(err, 000)
		return
	}

	if getFilenameFromPath(requestFile) == "html" {
		if !System.ScanInProgress.Load() {
			if len(config.Get().Files.M3U) == 0 && len(config.Get().Files.HDHR) == 0 {
				System.ConfigurationWizard.Store(true)
			}
		}

		switch System.ConfigurationWizard.Load() {
		case true:
			file = requestFile + "configuration.html"
			// The first setup runs without authentication
			if config.Get().AuthenticationWEB {
				err = config.Update(func(s *SettingsStruct) error {
					s.AuthenticationWEB = false
					return nil
				})
				if err != nil {
					ShowError(err, 0)
				}
			}

		case false:
			file = requestFile + "index.html"
		}

		if System.ScanInProgress.Load() {
			file = requestFile + "maintenance.html"
		}

		switch config.Get().AuthenticationWEB {
		case true:
			var username, password, confirm string
			switch r.Method {
			case "POST":
				allUsers, err := authentication.GetAllUserData()
				if err != nil {
					ShowError(err, 0)
					httpStatusError(w, 500)
					return
				}

				username = r.FormValue("username")
				password = r.FormValue("password")

				if len(allUsers) == 0 {
					confirm = r.FormValue("confirm")
				}

				// First user is created (Password confirmation is available)
				if len(confirm) > 0 {
					var token, err = createFirstUserForAuthentication(username, password)
					if err != nil {
						httpStatusError(w, 429)
						return
					}
					// Redirect so that the Data is deleted from the Browser.
					authentication.SetCookieToken(w, token)
					http.Redirect(w, r, "/web", http.StatusMovedPermanently)
					return
				}

				// Username and Password available, will now be checked
				if len(username) > 0 && len(password) > 0 {
					var token, err = authentication.UserAuthentication(username, password)
					if err != nil {
						file = requestFile + "login.html"
						lang["authenticationErr"] = language.Login.Failed
						break
					}

					authentication.SetCookieToken(w, token)
					http.Redirect(
						w,
						r,
						"/web",
						http.StatusMovedPermanently,
					) // Redirect so that the Data is deleted from the Browser.
				} else {
					authentication.SetCookieToken(w, "-")
					http.Redirect(
						w,
						r,
						"/web",
						http.StatusMovedPermanently,
					) // Redirect so that the Data is deleted from the Browser.
				}

				return

			case "GET":
				lang["authenticationErr"] = ""
				token, err := authentication.CheckTheValidityOfTheTokenFromHTTPHeader(w, r)

				if err != nil {
					file = requestFile + "login.html"
					break
				}

				err = checkAuthorizationLevel(token, "authentication.web")
				if err != nil {
					file = requestFile + "login.html"
					break
				}
			}

			allUserData, err := authentication.GetAllUserData()
			if err != nil {
				ShowError(err, 000)
				httpStatusError(w, 403)
				return
			}

			if len(allUserData) == 0 && config.Get().AuthenticationWEB {
				file = requestFile + "create-first-user.html"
			}
		}

		requestFile = file
	}

	if value, ok := webUI[requestFile].(string); ok {
		content = GetHTMLString(value)
		contentType = getContentType(requestFile)

		if contentType == "text/plain" {
			w.Header().Set("Content-Disposition", "attachment; filename="+getFilenameFromPath(requestFile))
		}
	} else {
		httpStatusError(w, 404)
		return
	}

	contentType = getContentType(requestFile)

	if System.Dev {
		// Local web server Files are loaded, only for Development
		var err error
		if content, err = readStringFromFile(requestFile); err != nil {
			httpStatusError(w, 404)
			return
		}
	}

	w.Header().Add("Content-Type", contentType)
	w.WriteHeader(200)

	if contentType == "text/html" || contentType == "application/javascript" {
		content = parseTemplate(content, lang)
	}

	writeResponse(w, []byte(content))
}

// API : API request /api/
func API(w http.ResponseWriter, r *http.Request) {
	/*
			API conditions (without Authentication):
			- API must be activated in the Settings

			Example API Request with curl
			Status:
			curl -X POST -H "Content-Type: application/json" -d '{"cmd":"status"}' http://localhost:34400/api/

			- - - - -

			API conditions (with Authentication):
			- API must be activated in the Settings
			- API must be activated in the Authentication Settings
			- User must have API authorization

			A Token is generated after each API request, which is valid once every 60 minutes.
			A new Token is included in every answer

			Example API Request with curl
			Login request:
			curl -X POST -H "Content-Type: application/json" \
				-d '{"cmd":"login","username":"plex","password":"123"}' http://localhost:34400/api/

			Response:
			{
		  	"status": true,
		  	"token": "U0T-NTSaigh-RlbkqERsHvUpgvaaY2dyRGuwIIvv"
			}

			Status Request using a Token:
			curl -X POST -H "Content-Type: application/json" \
				-d '{"cmd":"status","token":"U0T-NTSaigh-RlbkqERsHvUpgvaaY2dyRGuwIIvv"}' http://localhost:34400/api/

			Response:
			{
			  "epg.source": "XEPG",
			  "status": true,
			  "streams.active": 7,
			  "streams.all": 63,
			  "streams.xepg": 2,
			  "token": "mXiG1NE1MrTXDtyh7PxRHK5z8iPI_LzxsQmY-LFn",
			  "url.dvr": "localhost:34400",
			  "url.m3u": "http://localhost:34400/m3u/xteve.m3u",
			  "url.xepg": "http://localhost:34400/xmltv/xteve.xml",
			  "version.api": "1.1.0",
			  "version.xteve": "1.3.0"
			}
	*/

	setGlobalDomain(r.Host)
	var request APIRequestStruct
	var response APIResponseStruct

	var responseAPIError = func(err error) {
		var response APIResponseStruct

		response.Status = false
		response.Error = userMessage(err)
		writeResponse(w, []byte(mapToJSON(response)))
	}

	response.Status = true

	if !config.Get().API {
		httpStatusError(w, 423)
		return
	}

	if r.Method == "GET" {
		httpStatusError(w, 404)
		return
	}

	// The server closes the request body itself
	b, err := io.ReadAll(r.Body)
	if err != nil {
		httpStatusError(w, 400)
		return
	}

	err = json.Unmarshal(b, &request)
	if err != nil {
		httpStatusError(w, 400)
		return
	}

	w.Header().Set("content-type", "application/json")

	if config.Get().AuthenticationAPI {
		var token string
		switch len(request.Token) {
		case 0:
			if request.Cmd == "login" {
				token, err = authentication.UserAuthentication(request.Username, request.Password)
				if err != nil {
					responseAPIError(err)
					return
				}
			} else {
				err = errors.New("login incorrect")
				if err != nil {
					responseAPIError(err)
					return
				}
			}

		default:
			token, err = tokenAuthentication(request.Token)
			if err != nil {
				responseAPIError(err)
				return
			}
		}
		err = checkAuthorizationLevel(token, "authentication.api")
		if err != nil {
			responseAPIError(err)
			return
		}

		response.Token = token
	}

	switch request.Cmd {
	case "login": // Nothing has to be handed over

	case "status":
		response.VersionXteve = System.Version
		response.VersionAPI = System.APIVersion
		response.StreamsActive = int64(len(streamsDB.Get().Active))
		response.StreamsAll = int64(len(streamsDB.Get().All))
		response.StreamsXepg = int64(Data.XEPG.XEPGCount)
		response.EpgSource = config.Get().EpgSource
		response.URLDvr = System.Domain()
		response.URLM3U = System.Protocol() + "://" + System.Domain() + "/m3u/xteve.m3u"
		response.URLXepg = System.Protocol() + "://" + System.Domain() + "/xmltv/xteve.xml"

	case "update.m3u":
		err = getProviderData("m3u", "")
		if err != nil {
			ShowError(err, 0)
			break
		}

		err = buildDatabaseDVR()
		if err != nil {
			break
		}

	case "update.hdhr":
		err = getProviderData("hdhr", "")
		if err != nil {
			ShowError(err, 0)
			break
		}

		err = buildDatabaseDVR()
		if err != nil {
			break
		}

	case "update.xmltv":
		err = getProviderData("xmltv", "")
		if err != nil {
			ShowError(err, 0)
			break
		}

	case "update.xepg":
		buildXEPG(false)

	default:
		err = errors.New(getErrMsg(5000))
	}

	if err != nil {
		responseAPIError(err)
		return
	}

	writeResponse(w, []byte(mapToJSON(response)))
}

// Download : File Download
func Download(w http.ResponseWriter, r *http.Request) {
	var path = r.URL.Path
	var file = System.TempFolder() + getFilenameFromPath(path)
	w.Header().Set("Content-Disposition", "attachment; filename="+getFilenameFromPath(file))

	content, err := readStringFromFile(file)
	if err != nil {
		logUnlessNotExist(err)
		w.WriteHeader(404)
		return
	}

	removeAllOrLog(System.TempFolder() + getFilenameFromPath(path))
	writeResponse(w, []byte(content))
}

func setDefaultResponseData(response ResponseStruct, data bool) (defaults ResponseStruct) {
	defaults = response

	// Always transfer the following Data to the Client
	defaults.ClientInfo.ARCH = System.ARCH
	defaults.ClientInfo.EpgSource = config.Get().EpgSource
	defaults.ClientInfo.DVR, defaults.ClientInfo.M3U, defaults.ClientInfo.XML = clientAddresses()
	defaults.ClientInfo.OS = System.OS
	defaults.ClientInfo.Streams = fmt.Sprintf("%d / %d", len(streamsDB.Get().Active), len(streamsDB.Get().All))
	defaults.ClientInfo.UUID = config.Get().UUID
	var log = logger.Default().Log()
	defaults.ClientInfo.Errors = log.Errors
	defaults.ClientInfo.Warnings = log.Warnings
	defaults.IPAddressesV4Host = System.IPAddressesV4Host
	defaults.Settings.HostIP = config.Get().HostIP
	defaults.Notification = logger.Default().Notifications()
	defaults.Log = log

	defaults.ClientInfo.Version = System.Version

	if System.Branch != "master" {
		defaults.ClientInfo.Branch = System.Branch
	}

	if data {
		var err error
		if defaults.Users, err = authentication.GetAllUserData(); err != nil {
			ShowError(err, 0)
		}

		if config.Get().EpgSource == "XEPG" {
			defaults.ClientInfo.XEPGCount = Data.XEPG.XEPGCount

			var XEPG = make(map[string]any)

			if len(streamsDB.Get().Active) > 0 {
				XEPG["epgMapping"] = Data.XEPG.Channels
				XEPG["xmltvMap"] = Data.XMLTV.Mapping
			} else {
				XEPG["epgMapping"] = make(map[string]any)
				XEPG["xmltvMap"] = make(map[string]any)
			}

			defaults.XEPG = XEPG
		}

		defaults.Settings = *config.Get()

		var streams = streamsDB.Get()
		defaults.Data.Playlist.M3U.Groups.Text = streams.GroupTexts()
		defaults.Data.Playlist.M3U.Groups.Value = streams.GroupValues()
		defaults.Data.StreamPreviewUI.Active = streams.PreviewActive
		defaults.Data.StreamPreviewUI.Inactive = streams.PreviewInactive
	}

	return
}

func httpStatusError(w http.ResponseWriter, httpStatusCode int) {
	http.Error(w, fmt.Sprintf("%s [%d]", http.StatusText(httpStatusCode), httpStatusCode), httpStatusCode)
}

func getContentType(filename string) (contentType string) {
	if strings.HasSuffix(filename, ".html") {
		contentType = "text/html"
	} else if strings.HasSuffix(filename, ".css") {
		contentType = "text/css"
	} else if strings.HasSuffix(filename, ".js") {
		contentType = "application/javascript"
	} else if strings.HasSuffix(filename, ".png") {
		contentType = "image/png"
	} else if strings.HasSuffix(filename, ".jpg") {
		contentType = "image/jpeg"
	} else if strings.HasSuffix(filename, ".gif") {
		contentType = "image/gif"
	} else if strings.HasSuffix(filename, ".svg") {
		contentType = "image/svg+xml"
	} else if strings.HasSuffix(filename, ".mp4") {
		contentType = "video/mp4"
	} else if strings.HasSuffix(filename, ".webm") {
		contentType = "video/webm"
	} else if strings.HasSuffix(filename, ".ogg") {
		contentType = "video/ogg"
	} else if strings.HasSuffix(filename, ".mp3") {
		contentType = "audio/mp3"
	} else if strings.HasSuffix(filename, ".wav") {
		contentType = "audio/wav"
	} else {
		contentType = "text/plain"
	}

	return
}
