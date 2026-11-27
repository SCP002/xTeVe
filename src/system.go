package src

import (
	"encoding/json"
	"fmt"
	"reflect"
	"time"
	"xteve/src/internal/config"
	"xteve/src/internal/logger"
)

// Show Developer Information
func showDevInfo() {
	if System.Dev {
		fmt.Print("\033[31m")
		fmt.Println("* * * * * D E V   M O D E * * * * *")
		fmt.Println("Version: ", System.Version)
		fmt.Println("* * * * * * * * * * * * * * * * * *")
		fmt.Print("\033[0m")
		fmt.Println()
	}
}

// Create all System Folders
func createSystemFolders() (err error) {
	e := reflect.ValueOf(&System.Folder).Elem()

	for _, field := range e.Fields() {
		var folder = field.Interface().(string)

		err = checkFolder(folder)

		if err != nil {
			return
		}
	}

	return
}

// Create all System Files
func createSystemFiles() (err error) {
	var debug string
	for _, file := range SystemFiles {
		var filename = getPlatformFile(System.Folder.Config + file)

		err = checkFile(filename)
		if err != nil {
			// File does not exist, will be created now
			err = saveMapToJSONFile(filename, make(map[string]any))
			if err != nil {
				return
			}

			debug = fmt.Sprintf("Create File:%s", filename)
			showDebug(debug, 1)
		}

		switch file {
		case "authentication.json":
			System.File.Authentication = filename
		case "pms.json":
			System.File.PMS = filename
		case "settings.json":
			System.File.Settings = filename
		case "xepg.json":
			System.File.XEPG = filename
		case "urls.json":
			System.File.URLS = filename
		}
	}

	return
}

// Load Settings and set Default Values (xTeVe)
func loadSettings() (settings SettingsStruct, err error) {
	settingsMap, err := loadJSONFileToMap(System.File.Settings)
	if err != nil {
		return
	}

	// Set Deafult Values
	var defaults = make(map[string]any)
	var dataMap = make(map[string]any)

	dataMap["xmltv"] = make(map[string]any)
	dataMap["m3u"] = make(map[string]any)
	dataMap["hdhr"] = make(map[string]any)

	defaults["api"] = false
	defaults["authentication.api"] = false
	defaults["authentication.m3u"] = false
	defaults["authentication.pms"] = false
	defaults["authentication.web"] = false
	defaults["authentication.xml"] = false
	defaults["backup.keep"] = 10
	defaults["backup.path"] = System.Folder.Backup
	defaults["buffer.size.kb"] = 1024
	defaults["buffer.timeout"] = 500
	defaults["buffer"] = "-"
	defaults["cache.images"] = false
	defaults["clearXMLTVCache"] = false
	defaults["defaultMissingEPG"] = "-"
	defaults["disallowURLDuplicates"] = false
	defaults["enableMappedChannels"] = false
	defaults["epgSource"] = "PMS"
	defaults["ffmpeg.options"] = System.FFmpeg.DefaultOptions
	defaults["files.update"] = true
	defaults["files"] = dataMap
	defaults["filter"] = make(map[string]any)
	defaults["git.branch"] = System.Branch
	defaults["hostIP"] = "" // Will be set in resolveHostIP()
	defaults["language"] = "en"
	defaults["log.entries.ram"] = 500
	defaults["m3u8.adaptive.bandwidth.mbps"] = 10
	defaults["mapping.first.channel"] = 1000
	defaults["port"] = "34400"
	defaults["ssdp"] = true
	defaults["storeBufferInRAM"] = false
	defaults["temp.path"] = System.TempFolder()
	defaults["tlsMode"] = false
	defaults["tuner"] = 1
	defaults["udpxy"] = ""
	defaults["update"] = []string{"0000"}
	defaults["user.agent"] = System.Name
	defaults["uuid"] = createUUID()
	defaults["version"] = System.DBVersion
	defaults["vlc.options"] = System.VLC.DefaultOptions
	defaults["xepg.replace.missing.images"] = true
	defaults["xteveAutoUpdate"] = true

	// Set Default Values
	for key, value := range defaults {
		if _, ok := settingsMap[key]; !ok {
			settingsMap[key] = value
		}
	}

	err = json.Unmarshal([]byte(mapToJSON(settingsMap)), &settings)
	if err != nil {
		return
	}

	// Adopt the settings from the Flags
	if len(System.Flag.Port) > 0 {
		settings.Port = System.Flag.Port
	}

	if len(System.Flag.Branch) > 0 {
		settings.Branch = System.Flag.Branch
		showInfo(fmt.Sprintf("Git Branch:Switching Git Branch to -> %s", settings.Branch))
	}

	if len(settings.FFmpegPath) == 0 {
		settings.FFmpegPath = searchFileInOS("ffmpeg")
	}

	if len(settings.VLCPath) == 0 {
		settings.VLCPath = searchFileInOS("cvlc")
	}

	// Initialze virutal filesystem for the Buffer
	initBufferVFS(settings.StoreBufferInRAM)

	settings.TempPath = getValidTempDir(settings.TempPath)

	err = saveSettings(settings)

	logger.Default().SetLimit(settings.LogEntriesRAM)

	// Warning if FFmpeg was not found
	if len(config.Get().FFmpegPath) == 0 && config.Get().Buffer == "ffmpeg" {
		showWarning(2020)
	}

	if len(config.Get().VLCPath) == 0 && config.Get().Buffer == "vlc" {
		showWarning(2021)
	}

	return
}

// Save Settings (xTeVe)
// saveSettings saves settings and publishes them as the current settings, replacing the current ones.
func saveSettings(settings SettingsStruct) error { return config.Default().Set(&settings) }

// completeAndWriteSettings fills in defaults and writes the settings to settings.json. It is the saver of the
// settings store (config.Store.SetSaver), so it runs for every change of the settings.
func completeAndWriteSettings(settings *SettingsStruct) error {
	if settings.BackupKeep == 0 {
		settings.BackupKeep = 10
	}

	if len(settings.BackupPath) == 0 {
		settings.BackupPath = System.Folder.Backup
	}

	if settings.BufferTimeout < 0 {
		settings.BufferTimeout = 0
	}

	if System.Dev {
		settings.UUID = "2019-01-DEV-xTeVe!"
	}

	System.SetTempFolder(getValidTempDir(settings.TempPath + settings.UUID))

	return writeByteToFile(System.File.Settings, []byte(mapToJSON(settings)))
}

// Enable access via the Domain
func setGlobalDomain(domain string) { System.SetDomain(domain) }

// clientAddresses returns the addresses shown in the web UI for the DVR, the M3U and the XMLTV file.
func clientAddresses() (dvr, m3u, xml string) {
	var domain, protocol = System.Domain(), System.Protocol()

	if config.Get().EpgSource != "XEPG" {
		dvr = domain
		if config.Get().AuthenticationPMS {
			dvr = "username:password@" + domain
		}
		return dvr, getErrMsg(2106), getErrMsg(2106)
	}

	dvr = domain
	m3u = protocol + "://" + domain + "/m3u/xteve.m3u     (Specific groups: [http://...?group-title=foo,bar])"
	xml = protocol + "://" + domain + "/xmltv/xteve.xml"

	if config.Get().AuthenticationPMS {
		dvr = "username:password@" + domain
	}
	if config.Get().AuthenticationM3U {
		m3u = protocol + "://" + domain +
			"/m3u/xteve.m3u?username=xxx&password=yyy<br>(Specific groups: [http://...&group-title=foo,bar])"
	}
	if config.Get().AuthenticationXML {
		xml = protocol + "://" + domain + "/xmltv/xteve.xml?username=xxx&password=yyy"
	}

	return dvr, m3u, xml
}

// Generate UUID
func createUUID() (uuid string) {
	uuid = time.Now().Format("2006-01") + "-" + randomString(4) + "-" + randomString(6)
	return
}

// deviceID returns the unique device ID for Plex. It changes with the number of tuners, so that Plex sees a new
// device.
func deviceID() string {
	if config.Get().Tuner == 1 {
		return config.Get().UUID
	}
	return fmt.Sprintf("%s:%d", config.Get().UUID, config.Get().Tuner)
}

// Convert Provider Streaming URL to xTeVe Streaming URL
func createStreamingURL(_, playlistID, channelNumber, channelName, url string) (string, error) {
	var id, err = streamURLs.Load().Add(playlistID, channelNumber, channelName, url)
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("%s://%s/stream/%s", System.Protocol(), System.Domain(), id), nil
}

// getStreamInfo returns the stream of a streaming URL handed out by xTeVe.
func getStreamInfo(urlID string) (StreamInfo, error) { return streamURLs.Load().Get(urlID) }
