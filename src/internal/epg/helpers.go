package epg

import (
	"bytes"
	"compress/gzip"
	"encoding/json"
	"os"
	"path/filepath"
)

// XEPGChannel is a channel of the XEPG database (xepg.json).
type XEPGChannel struct {
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

// M3UChannel is a stream of a playlist as the XEPG database reads it.
type M3UChannel struct {
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

// Channels are the channels of the XEPG database by XEPG ID ("x-ID.12").
type Channels map[string]XEPGChannel

// MappingChannel is an XMLTV channel in the mapping menu of the web UI.
type MappingChannel struct {
	ID           string        `json:"id"`
	DisplayNames []DisplayName `json:"display-names"`
	Icon         string        `json:"icon"`
}

// Mapping is the mapping menu of the web UI: XMLTV file name -> XMLTV channel ID -> channel.
type Mapping map[string]map[string]MappingChannel

// dummyFile is the XMLTV "file" of the dummy programs.
const dummyFile = "xTeVe Dummy"

// loadChannels reads xepg.json.
func loadChannels(file string) (Channels, error) {
	var channels = make(Channels)
	var content, err = os.ReadFile(filepath.Clean(file))
	if err != nil {
		return nil, err
	}
	if err := json.Unmarshal(content, &channels); err != nil {
		return nil, err
	}
	return channels, nil
}

// XEPGChannelStruct and M3UChannelStructXEPG are the names used by the code moved from src.
type (
	XEPGChannelStruct    = XEPGChannel
	M3UChannelStructXEPG = M3UChannel
)

func mapToJSON(v any) string {
	var content, err = json.MarshalIndent(v, "", "  ")
	if err != nil {
		return "{}"
	}
	return string(content)
}

func saveMapToJSONFile(file string, v any) error {
	var content, err = json.MarshalIndent(v, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Clean(file), content, 0644)
}

func readByteFromFile(file string) ([]byte, error) { return os.ReadFile(filepath.Clean(file)) }

func writeByteToFile(file string, data []byte) error {
	return os.WriteFile(filepath.Clean(file), data, 0644)
}

// compressGZIP writes data compressed to file.
func compressGZIP(data *[]byte, file string) error {
	var buf bytes.Buffer
	var w = gzip.NewWriter(&buf)
	if _, err := w.Write(*data); err != nil {
		return err
	}
	if err := w.Close(); err != nil {
		return err
	}
	return os.WriteFile(filepath.Clean(file), buf.Bytes(), 0644)
}
