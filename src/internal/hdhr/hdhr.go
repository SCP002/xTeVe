// Package hdhr emulates an HDHomeRun tuner for Plex and Emby: discover.json, device.xml, lineup_status.json and
// lineup.json, with the PMS guide numbers of the channels and the streaming URLs handed out in the lineup and the
// M3U file. The stores of guide numbers and URLs are safe for use by multiple goroutines.
package hdhr

import (
	"bytes"
	"crypto/md5"
	"encoding/hex"
	"encoding/json"
	"encoding/xml"
	"errors"
	"fmt"
	"os"
	"sort"
	"strconv"
	"strings"
	"sync"

	"xteve/src/internal/errcode"
)

// Device is the information that the emulated tuner reports.
type Device struct {
	BaseURL      string // "http://host:port"
	DeviceID     string
	AppName      string // e.g. "xteve"
	Name         string // e.g. "xTeVe"
	Version      string
	TunerCount   int
	ScanProgress bool
}

// DiscoverJSON returns discover.json.
func DiscoverJSON(d Device) ([]byte, error) {
	return json.MarshalIndent(Discover{
		BaseURL:         d.BaseURL,
		DeviceAuth:      d.AppName,
		DeviceID:        d.DeviceID,
		FirmwareName:    "bin_" + d.Version,
		FirmwareVersion: d.Version,
		FriendlyName:    d.Name,
		LineupURL:       d.BaseURL + "/lineup.json",
		Manufacturer:    "Golang",
		ModelNumber:     d.Version,
		TunerCount:      d.TunerCount,
	}, "", "  ")
}

// DeviceXML returns device.xml.
func DeviceXML(d Device) ([]byte, error) {
	var capability Capability

	capability.Xmlns = "urn:schemas-upnp-org:device-1-0"
	capability.URLBase = d.BaseURL
	capability.SpecVersion.Major = 1
	capability.SpecVersion.Minor = 0
	capability.Device.DeviceType = "urn:schemas-upnp-org:device:MediaServer:1"
	capability.Device.FriendlyName = d.Name
	capability.Device.Manufacturer = "Silicondust"
	capability.Device.ModelName = "HDTC-2US"
	capability.Device.ModelNumber = "HDTC-2US"
	capability.Device.SerialNumber = ""
	capability.Device.UDN = "uuid:" + d.DeviceID

	output, err := xml.MarshalIndent(capability, " ", "  ")
	if err != nil {
		return nil, errcode.With(err, 1003)
	}

	var buffer bytes.Buffer
	buffer.WriteString(xml.Header)
	buffer.Write(output)
	return buffer.Bytes(), nil
}

// LineupStatusJSON returns lineup_status.json.
func LineupStatusJSON(scanning bool) ([]byte, error) {
	var status = LineupStatus{ScanPossible: 0, Source: "Cable", SourceList: []string{"Cable"}}
	if scanning {
		status.ScanInProgress = 1
	}
	return json.MarshalIndent(status, "", "  ")
}

// LineupJSON returns lineup.json with the streams sorted by guide number. It is a list also without streams.
func LineupJSON(streams []LineupStream) ([]byte, error) {
	var lineup = make(Lineup, len(streams))
	copy(lineup, streams)
	sort.SliceStable(
		lineup,
		func(i, j int) bool { return GuideNumberLess(lineup[i].GuideNumber, lineup[j].GuideNumber) },
	)
	return json.MarshalIndent(lineup, "", "  ")
}

// GuideNumberLess orders guide numbers numerically, other ones after them in text order.
func GuideNumberLess(a, b string) bool {
	numA, errA := strconv.ParseFloat(a, 64)
	numB, errB := strconv.ParseFloat(b, 64)

	switch {
	case errA == nil && errB == nil:
		return numA < numB
	case errA == nil:
		return true
	case errB == nil:
		return false
	default:
		return a < b
	}
}

// PMSNumbers keeps the guide numbers of the channels in PMS mode (pms.json), by channel name.
type PMSNumbers struct {
	file string

	mu      sync.Mutex
	numbers map[string]string
}

// NewPMSNumbers returns the guide numbers stored in file.
func NewPMSNumbers(file string) *PMSNumbers { return &PMSNumbers{file: file} }

// Numbers returns the guide numbers of the channels. Channels with the same name get the keys "Name (2)",
// "Name (3)" and so on, so that each of them has its own number and the first one keeps the number stored for the
// plain name. New numbers ("id-0", "id-1", ...) are saved.
func (p *PMSNumbers) Numbers(names []string) ([]string, error) {
	p.mu.Lock()
	defer p.mu.Unlock()

	if p.numbers == nil {
		var numbers = make(map[string]string)
		if content, err := os.ReadFile(p.file); err == nil {
			if err := json.Unmarshal(content, &numbers); err != nil {
				return nil, err
			}
		} else if !errors.Is(err, os.ErrNotExist) {
			return nil, err
		}
		p.numbers = numbers
	}

	var used = make(map[string]bool, len(p.numbers))
	for _, n := range p.numbers {
		used[n] = true
	}

	var keys = make(map[string]bool, len(names))
	var result = make([]string, len(names))
	var changed bool
	var next = 0

	for i, name := range names {
		var key = name
		for n := 2; keys[key]; n++ {
			key = fmt.Sprintf("%s (%d)", name, n)
		}
		keys[key] = true

		if number, ok := p.numbers[key]; ok {
			result[i] = number
			continue
		}

		// The first free number
		for ; used[fmt.Sprintf("id-%d", next)]; next++ {
		}
		var number = fmt.Sprintf("id-%d", next)
		used[number] = true
		p.numbers[key] = number
		result[i] = number
		changed = true
	}

	if changed {
		if err := saveJSON(p.file, p.numbers); err != nil {
			return result, err
		}
	}
	return result, nil
}

// StreamInfo is a streaming URL handed out by xTeVe.
type StreamInfo struct {
	ChannelNumber string `json:"channelNumber"`
	Name          string `json:"name"`
	PlaylistID    string `json:"playlistID"`
	URL           string `json:"url"`
	URLid         string `json:"urlID"`
}

// URLs keeps the streaming URLs handed out (urls.json): /stream/<ID> leads to the stream of a playlist.
type URLs struct {
	file string

	mu   sync.Mutex
	urls map[string]StreamInfo
}

// NewURLs returns the streaming URLs stored in file.
func NewURLs(file string) *URLs { return &URLs{file: file} }

// load reads the file once. u.mu must be held.
func (u *URLs) load() error {
	if u.urls != nil {
		return nil
	}
	var urls = make(map[string]StreamInfo)
	if content, err := os.ReadFile(u.file); err == nil {
		if err := json.Unmarshal(content, &urls); err != nil {
			return err
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		return err
	}
	u.urls = urls
	return nil
}

// Add returns the ID of the stream of a playlist, adding it if needed. The ID stays the same for the same playlist
// and stream URL.
func (u *URLs) Add(playlistID, channelNumber, channelName, url string) (string, error) {
	u.mu.Lock()
	defer u.mu.Unlock()

	if err := u.load(); err != nil {
		return "", err
	}

	var sum = md5.Sum([]byte(playlistID + "-" + url))
	var id = hex.EncodeToString(sum[:])
	if _, ok := u.urls[id]; !ok {
		u.urls[id] = StreamInfo{
			URL:           url,
			Name:          channelName,
			PlaylistID:    playlistID,
			ChannelNumber: channelNumber,
			URLid:         id,
		}
	}
	return id, nil
}

// Get returns the stream of an ID.
func (u *URLs) Get(id string) (StreamInfo, error) {
	u.mu.Lock()
	defer u.mu.Unlock()

	if err := u.load(); err != nil {
		return StreamInfo{}, err
	}
	var s, ok = u.urls[id]
	if !ok {
		return StreamInfo{}, errors.New("streaming error")
	}
	s.URL = strings.Trim(s.URL, "\r\n")
	return s, nil
}

// Save writes the URLs to the file.
func (u *URLs) Save() error {
	u.mu.Lock()
	defer u.mu.Unlock()

	if err := u.load(); err != nil {
		return err
	}
	return saveJSON(u.file, u.urls)
}

// saveJSON writes v to a temporary file and renames it, so that readers never see a half written file.
func saveJSON(file string, v any) error {
	var content, err = json.MarshalIndent(v, "", "  ")
	if err != nil {
		return err
	}
	var tmp = file + ".tmp"
	if err := os.WriteFile(tmp, content, 0644); err != nil {
		return err
	}
	return os.Rename(tmp, file)
}
