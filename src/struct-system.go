package src

import (
	"xteve/src/internal/config"
	"xteve/src/internal/epg"
	"xteve/src/internal/logger"
	"xteve/src/internal/playlist"
	"xteve/src/internal/system"
)

// SystemStruct : Contains all System Information
type SystemStruct = system.Info

// GitStruct : Update information from GitHub
type GitStruct struct {
	Filename string `json:"filename"`
	Version  string `json:"version"`
}

// DataStruct : All Data is stored here. (Lineup, XMLTV)
type DataStruct struct {
	// Caches of the HDHomeRun emulation: PMS IDs of the channels and the streaming URLs handed out
	Cache struct {
		PMS           map[string]string
		StreamingURLS map[string]StreamInfo
	}
}

// Filter : Used for the Filter Rules
type Filter = playlist.Filter

// FilterStruct : Filter Structure
type FilterStruct = playlist.FilterSettings

// StreamingURLS : Information on all Streaming URL's
type StreamingURLS struct {
	Streams map[string]StreamInfo `json:"channels"`
}

// StreamInfo : Information about the Channel for the Streaming URL
type StreamInfo struct {
	ChannelNumber string `json:"channelNumber"`
	Name          string `json:"name"`
	PlaylistID    string `json:"playlistID"`
	URL           string `json:"url"`
	URLid         string `json:"urlID"`
}

// Notification : Notifications in the Web Interface
type Notification = logger.Notification

// SettingsStruct : Content of settings.json
type SettingsStruct = config.Settings

// LanguageUI : Language for the WebUI
type LanguageUI struct {
	Login struct {
		Failed string
	}
}

// XEPGChannelStruct : XEPG Structure
type XEPGChannelStruct = epg.XEPGChannel

// M3UChannelStructXEPG : M3U Structure for XEPG
type M3UChannelStructXEPG = epg.M3UChannel
