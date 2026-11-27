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
// defaultSettings returns the settings used for the keys that settings.json does not have.
func defaultSettings() SettingsStruct {
	var settings = SettingsStruct{
		BackupKeep:               10,
		BackupPath:               System.Folder.Backup,
		BufferSize:               1024,
		BufferTimeout:            500,
		Buffer:                   "-",
		DefaultMissingEPG:        "-",
		EpgSource:                "PMS",
		FFmpegOptions:            System.FFmpeg.DefaultOptions,
		FilesUpdate:              true,
		Filter:                   map[int64]config.FilterSettings{},
		Branch:                   System.Branch,
		Language:                 "en",
		LogEntriesRAM:            500,
		MappingFirstChannel:      1000,
		Port:                     "34400",
		SSDP:                     true,
		TempPath:                 System.TempFolder(),
		Tuner:                    1,
		Update:                   []string{"0000"},
		UserAgent:                System.Name,
		UUID:                     createUUID(),
		Version:                  System.DBVersion,
		VLCOptions:               System.VLC.DefaultOptions,
		XepgReplaceMissingImages: true,
		XteveAutoUpdate:          true,
		// The host IP is set by resolveHostIP
	}
	settings.M3U8AdaptiveBandwidthMBPS = 10
	settings.Files.HDHR = config.Providers{}
	settings.Files.M3U = config.Providers{}
	settings.Files.XMLTV = config.Providers{}
	return settings
}

func loadSettings() (settings SettingsStruct, err error) {
	content, err := readByteFromFile(System.File.Settings)
	if err != nil {
		return
	}

	// The keys of settings.json replace the defaults
	settings = defaultSettings()
	if err = json.Unmarshal(content, &settings); err != nil {
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
