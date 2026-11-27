package src

import (
	"xteve/src/internal/config"
	"xteve/src/internal/imgcache"
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
	Cache struct {
		Images      *imgcache.Cache
		ImagesCache []string
		ImagesFiles []string
		ImagesURLS  []string
		PMS         map[string]string

		StreamingURLS map[string]StreamInfo
		XMLTV         map[string]XMLTV

		Streams struct {
			Active []string
		}
	}

	XMLTV struct {
		Files   []string
		Mapping map[string]any
	}

	XEPG struct {
		Channels  map[string]any
		XEPGCount int64
	}
}

// Filter : Used for the Filter Rules
type Filter = playlist.Filter

// XEPGChannelStruct : XEPG Structure
type XEPGChannelStruct struct {
	FileM3UID                     string `json:"_file.m3u.id"`
	FileM3UName                   string `json:"_file.m3u.name"`
	FileM3UPath                   string `json:"_file.m3u.path"`
	GroupTitle                    string `json:"group-title"`
	Name                          string `json:"name"`
	TvgID                         string `json:"tvg-id"`
	TvgLogo                       string `json:"tvg-logo"`
	TvgName                       string `json:"tvg-name"`
	TvgShift                      string `json:"tvg-shift"`
	UpdateChannelNameRegex        string `json:"update-channel-name-regex"`
	UpdateChannelNameByGroupRegex string `json:"update-channel-name-by-group-regex"`
	URL                           string `json:"url"`
	UUIDKey                       string `json:"_uuid.key"`
	UUIDValue                     string `json:"_uuid.value,omitempty"`
	Values                        string `json:"_values"`
	XActive                       bool   `json:"x-active"`
	XCategory                     string `json:"x-category"`
	XChannelID                    string `json:"x-channelID"`
	XEPG                          string `json:"x-epg"`
	XGroupTitle                   string `json:"x-group-title"`
	XMapping                      string `json:"x-mapping"`
	XmltvFile                     string `json:"x-xmltv-file"`
	XName                         string `json:"x-name"`
	XUpdateChannelIcon            bool   `json:"x-update-channel-icon"`
	XUpdateChannelName            bool   `json:"x-update-channel-name"`
	XUpdateChannelGroup           bool   `json:"x-update-channel-group"`
	XDescription                  string `json:"x-description"`
	XTimeshift                    string `json:"x-timeshift"`
}

// M3UChannelStructXEPG : M3U Structure for XEPG
type M3UChannelStructXEPG struct {
	FileM3UID       string `json:"_file.m3u.id"`
	FileM3UName     string `json:"_file.m3u.name"`
	FileM3UPath     string `json:"_file.m3u.path"`
	GroupTitle      string `json:"group-title"`
	Name            string `json:"name"`
	TvgID           string `json:"tvg-id"`
	TvgLogo         string `json:"tvg-logo"`
	TvgName         string `json:"tvg-name"`
	TvgShift        string `json:"tvg-shift"`
	URL             string `json:"url"`
	UUIDKey         string `json:"_uuid.key"`
	UUIDValue       string `json:"_uuid.value"`
	Values          string `json:"_values"`
	PreserveMapping string `json:"_preserve-mapping"`
	StartingChannel string `json:"_starting-channel"`
}

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
