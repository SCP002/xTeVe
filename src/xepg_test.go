package src

import (
	"encoding/json"
	"fmt"
	"path/filepath"
	"testing"
	"xteve/src/internal/playlist"
)

func TestShiftXMLTVTime(t *testing.T) {
	tests := []struct {
		in      string
		hours   int
		want    string
		wantErr bool
	}{
		{in: "20260101120000 +0100", hours: 2, want: "20260101120000 +0300"},
		{in: "20260101120000 +0100", hours: -3, want: "20260101120000 -0200"},
		{in: "20260101120000 -0500", hours: 0, want: "20260101120000 -0500"},
		{in: "20260101120000", hours: 1, wantErr: true},
		{in: "20260101120000 abc", hours: 1, wantErr: true},
	}
	for _, tt := range tests {
		got, err := shiftXMLTVTime(tt.in, tt.hours)
		if (err != nil) != tt.wantErr {
			t.Errorf("shiftXMLTVTime(%q, %d) error = %v, wantErr %v", tt.in, tt.hours, err, tt.wantErr)
			continue
		}
		if got != tt.want {
			t.Errorf("shiftXMLTVTime(%q, %d) = %q, want %q", tt.in, tt.hours, got, tt.want)
		}
	}
}

// xepgTest sets up the XEPG database file, the active streams and the
// settings used by createXEPGDatabase and mapping, and restores them after the
// test.
func xepgTest(t *testing.T, db []XEPGChannelStruct, streams ...map[string]string) {
	t.Helper()

	var oldFile, oldStreams, oldCache = System.File.XEPG, streamsDB.Get(), Data.Cache.Streams.Active
	var oldChannels, oldMapping = Data.XEPG.Channels, Data.XMLTV.Mapping
	t.Cleanup(func() {
		System.File.XEPG, Data.Cache.Streams.Active = oldFile, oldCache
		streamsDB.Set(oldStreams)
		Data.XEPG.Channels, Data.XMLTV.Mapping = oldChannels, oldMapping
	})

	setSettings(t, func(s *SettingsStruct) {
		s.MappingFirstChannel = 1000
		s.DefaultMissingEPG = "-"
		s.EnableMappedChannels = false
	})

	System.File.XEPG = filepath.Join(t.TempDir(), "xepg.json")
	var channels = make(map[string]any)
	for _, c := range db {
		channels[c.XEPG] = c
	}
	if err := saveMapToJSONFile(System.File.XEPG, channels); err != nil {
		t.Fatal(err)
	}

	streamsDB.Set(&playlist.Snapshot{Active: streams})
	Data.XMLTV.Mapping = make(map[string]any)
}

func m3uStream(name, group string, extra ...string) map[string]string {
	var s = map[string]string{
		"_file.m3u.id": "M1",
		"name":         name,
		"group-title":  group,
		"url":          "http://example.com/" + name + "/" + group,
	}
	for i := 0; i+1 < len(extra); i += 2 {
		s[extra[i]] = extra[i+1]
	}
	return s
}

func dbChannel(id, number, name, group string) XEPGChannelStruct {
	return XEPGChannelStruct{
		XEPG: id, XChannelID: number, FileM3UID: "M1",
		Name: name, XName: name, GroupTitle: group, XGroupTitle: group,
		URL: "http://example.com/old",
	}
}

// xepgChannels returns Data.XEPG.Channels, keyed by the channel name and group.
func xepgChannels(t *testing.T) map[string]XEPGChannelStruct {
	t.Helper()

	var channels = make(map[string]XEPGChannelStruct)
	for _, c := range Data.XEPG.Channels {
		var channel XEPGChannelStruct
		if err := json.Unmarshal([]byte(mapToJSON(c)), &channel); err != nil {
			t.Fatal(err)
		}
		var key = channel.Name + "/" + channel.GroupTitle
		if _, ok := channels[key]; ok {
			key += "/" + channel.XEPG
		}
		channels[key] = channel
	}
	return channels
}

func TestCreateXEPGDatabaseNewChannels(t *testing.T) {
	xepgTest(t, []XEPGChannelStruct{dbChannel("x-ID.0", "1001", "Old", "News")},
		m3uStream("Old", "News"),
		m3uStream("A", "News"),
		m3uStream("B", "Sport", "_starting-channel", "2000"),
		m3uStream("C", "Movies", "_preserve-mapping", "true", "_uuid.key", "tvg-chno", "_uuid.value", "50"),
		m3uStream("D", "Movies", "_preserve-mapping", "true", "_uuid.key", "tvg-chno", "_uuid.value", "abc",
			"_starting-channel", "3000"),
	)

	if err := createXEPGDatabase(); err != nil {
		t.Fatal(err)
	}

	var channels = xepgChannels(t)
	var want = map[string]string{
		"Old/News": "1001",
		"A/News":   "1000", // first free number
		"B/Sport":  "2000", // starting channel of the filter
		"C/Movies": "50",   // tvg-chno with preserve mapping
		"D/Movies": "3000", // invalid tvg-chno: starting channel of the filter
	}
	if len(channels) != len(want) {
		t.Errorf("channels = %v", channels)
	}
	for key, number := range want {
		if channels[key].XChannelID != number {
			t.Errorf("%s: channel number = %q, want %q", key, channels[key].XChannelID, number)
		}
	}
	if c := channels["A/News"]; c.XName != "A" || c.XGroupTitle != "News" || c.TvgShift != "0" || c.XTimeshift != "0" {
		t.Errorf("new channel = %+v", c)
	}
	if channels["Old/News"].XEPG != "x-ID.0" || channels["A/News"].XEPG == "x-ID.0" {
		t.Errorf("XEPG IDs: old %q, new %q", channels["Old/News"].XEPG, channels["A/News"].XEPG)
	}
}

func TestCreateXEPGDatabaseExistingChannel(t *testing.T) {
	var renamed = dbChannel("x-ID.1", "1001", "BBC", "News")
	renamed.XName = "My BBC"
	var withUUID = dbChannel("x-ID.2", "1002", "CNN", "News")
	withUUID.UUIDKey, withUUID.UUIDValue, withUUID.XUpdateChannelName = "tvg-id", "cnn.us", true
	var updateGroup = dbChannel("x-ID.3", "1003", "ESPN", "Sport")
	updateGroup.XUpdateChannelGroup = true
	var noUpdate = dbChannel("x-ID.4", "1004", "HBO", "Movies")

	xepgTest(t, []XEPGChannelStruct{renamed, withUUID, updateGroup, noUpdate, dbChannel("x-ID.5", "", "Gone", "News")},
		m3uStream("BBC", "News"),
		m3uStream("CNN International", "News", "_uuid.key", "tvg-id", "_uuid.value", "cnn.us"),
		m3uStream("ESPN", "Sports"),
		m3uStream("HBO", "Film"),
	)

	if err := createXEPGDatabase(); err != nil {
		t.Fatal(err)
	}

	var channels = xepgChannels(t)
	if len(channels) != 4 {
		t.Errorf("channels = %v, want 4 (the one without a number is removed)", channels)
	}

	var bbc = channels["BBC/News"]
	if bbc.XEPG != "x-ID.1" || bbc.XChannelID != "1001" || bbc.XName != "My BBC" ||
		bbc.URL != "http://example.com/BBC/News" {
		t.Errorf("BBC = %+v, want the same channel with the new URL and the user's name", bbc)
	}
	var cnn = channels["CNN International/News"]
	if cnn.XEPG != "x-ID.2" || cnn.XName != "CNN International" {
		t.Errorf("CNN = %+v, want it found by UUID and renamed", cnn)
	}
	var espn = channels["ESPN/Sports"]
	if espn.XEPG != "x-ID.3" || espn.XGroupTitle != "Sports" {
		t.Errorf("ESPN = %+v, want it found by name with the new group", espn)
	}
	var hbo = channels["HBO/Film"]
	if hbo.XEPG != "x-ID.4" || hbo.XGroupTitle != "Movies" {
		t.Errorf("HBO = %+v, want it found by name, keeping the user's group", hbo)
	}
}

func TestCreateXEPGDatabaseRenameRegex(t *testing.T) {
	var c = dbChannel("x-ID.0", "1001", "BBC One HD", "News")
	c.UpdateChannelNameRegex = "^BBC One"
	c.UpdateChannelNameByGroupRegex = "^News$"
	var other = dbChannel("x-ID.1", "1002", "CNN HD", "Other")
	other.UpdateChannelNameRegex = "^CNN"
	other.UpdateChannelNameByGroupRegex = "^News$"

	xepgTest(t, []XEPGChannelStruct{c, other},
		m3uStream("BBC One FHD", "News"),
		m3uStream("CNN FHD", "News"),
	)

	if err := createXEPGDatabase(); err != nil {
		t.Fatal(err)
	}

	var channels = xepgChannels(t)
	if bbc := channels["BBC One FHD/News"]; bbc.XEPG != "x-ID.0" || bbc.XName != "BBC One FHD" ||
		bbc.XChannelID != "1001" {
		t.Errorf("BBC = %+v, want the old channel renamed", bbc)
	}
	if cnn := channels["CNN FHD/News"]; cnn.XEPG == "x-ID.1" {
		t.Errorf("CNN = %+v, want a new channel: the group regex does not match", cnn)
	}
}

// Two streams must never end up in the same database channel, whatever the
// order of the streams.
func TestCreateXEPGDatabaseSameNameInTwoGroups(t *testing.T) {
	var streams = []map[string]string{m3uStream("BBC", "News"), m3uStream("BBC", "UK")}

	for _, order := range [][]map[string]string{streams, {streams[1], streams[0]}} {
		xepgTest(t, []XEPGChannelStruct{dbChannel("x-ID.0", "1001", "BBC", "News")}, order...)

		if err := createXEPGDatabase(); err != nil {
			t.Fatal(err)
		}

		var channels = xepgChannels(t)
		if len(channels) != 2 {
			t.Errorf("order %v: channels = %v, want 2", order[0]["group-title"], channels)
			continue
		}
		if c := channels["BBC/News"]; c.XEPG != "x-ID.0" || c.URL != "http://example.com/BBC/News" {
			t.Errorf("order %v: BBC/News = %+v, want the existing channel", order[0]["group-title"], c)
		}
		if c := channels["BBC/UK"]; c.XEPG == "x-ID.0" || c.URL != "http://example.com/BBC/UK" {
			t.Errorf("order %v: BBC/UK = %+v, want a new channel", order[0]["group-title"], c)
		}
	}
}

func TestCreateXEPGDatabaseIdenticalStreams(t *testing.T) {
	var a, b = m3uStream("BBC", "News"), m3uStream("BBC", "News")
	b["url"] = "http://example.com/backup"
	xepgTest(t, nil, a, b)

	// The first run creates two channels, the next ones must update both
	for run := 1; run <= 3; run++ {
		a["url"], b["url"] = fmt.Sprintf(
			"http://example.com/main/%d",
			run,
		), fmt.Sprintf(
			"http://example.com/backup/%d",
			run,
		)
		if err := createXEPGDatabase(); err != nil {
			t.Fatal(err)
		}
		var urls = make(map[string]string)
		for _, c := range xepgChannels(t) {
			urls[c.URL] = c.XEPG
		}
		if _, ok := urls[a["url"]]; !ok || len(urls) != 2 {
			t.Errorf("run %d: channels by URL = %v, want both streams", run, urls)
		}
	}
}

// xmltvChannel returns an entry of Data.XMLTV.Mapping as createXEPGMapping builds it.
func xmltvChannel(id, icon string, names ...string) map[string]any {
	var displayNames []DisplayName
	for _, n := range names {
		displayNames = append(displayNames, DisplayName{Value: n})
	}
	return map[string]any{"id": id, "icon": icon, "display-names": displayNames}
}

func TestMapping(t *testing.T) {
	var byID = dbChannel("x-ID.0", "1", "BBC One", "News")
	byID.TvgID = "bbc1.uk"
	var byName = dbChannel("x-ID.1", "2", "cnn  international", "News")
	var noMatch = dbChannel("x-ID.2", "3", "Unknown", "News")
	var mapped = dbChannel("x-ID.3", "4", "Mapped", "News")
	mapped.XActive, mapped.XmltvFile, mapped.XMapping = true, "b.xml", "gone"
	var mappedFileGone = dbChannel("x-ID.4", "5", "Mapped too", "News")
	mappedFileGone.XActive, mappedFileGone.XmltvFile, mappedFileGone.XMapping = true, "gone.xml", "x"
	var updateIcon = dbChannel("x-ID.5", "6", "Icon", "News")
	updateIcon.XActive, updateIcon.XmltvFile, updateIcon.XMapping = true, "b.xml", "icon"
	updateIcon.XUpdateChannelIcon = true

	xepgTest(t, []XEPGChannelStruct{byID, byName, noMatch, mapped, mappedFileGone, updateIcon})
	if err := createXEPGDatabase(); err != nil {
		t.Fatal(err)
	}
	Data.XMLTV.Mapping = map[string]any{
		"a.xml": map[string]any{"bbc1.uk": xmltvChannel("bbc1.uk", "http://img/bbc.png", "BBC 1")},
		"b.xml": map[string]any{
			"cnn":  xmltvChannel("cnn", "", "CNN International", "CNN"),
			"icon": xmltvChannel("icon", "http://img/icon.png", "Icon"),
		},
	}

	if err := mapping(); err != nil {
		t.Fatal(err)
	}

	var channels = xepgChannels(t)
	tests := []struct {
		key, file, mapping string
		active             bool
	}{
		{key: "BBC One/News", file: "a.xml", mapping: "bbc1.uk"},
		{key: "cnn  international/News", file: "b.xml", mapping: "cnn"},
		{key: "Unknown/News", file: "-", mapping: "-"},
		{key: "Mapped/News", file: "b.xml", mapping: "gone"},             // channel missing in the file: deactivated
		{key: "Mapped too/News", file: "gone.xml", mapping: "x"},         // file missing: deactivated
		{key: "Icon/News", file: "b.xml", mapping: "icon", active: true}, // still valid
	}
	for _, tt := range tests {
		var c = channels[tt.key]
		if c.XmltvFile != tt.file || c.XMapping != tt.mapping || c.XActive != tt.active {
			t.Errorf("%s: file %q, mapping %q, active %v; want %q, %q, %v",
				tt.key, c.XmltvFile, c.XMapping, c.XActive, tt.file, tt.mapping, tt.active)
		}
	}
	if logo := channels["BBC One/News"].TvgLogo; logo != "http://img/bbc.png" {
		t.Errorf("BBC logo = %q, want the XMLTV icon", logo)
	}
	if logo := channels["Icon/News"].TvgLogo; logo != "http://img/icon.png" {
		t.Errorf("Icon logo = %q, want the XMLTV icon", logo)
	}
}

func TestMappingDefaults(t *testing.T) {
	xepgTest(
		t,
		[]XEPGChannelStruct{dbChannel("x-ID.0", "1", "Unknown", "News"), dbChannel("x-ID.1", "2", "BBC", "News")},
	)
	if err := createXEPGDatabase(); err != nil {
		t.Fatal(err)
	}
	setSettings(t, func(s *SettingsStruct) {
		s.DefaultMissingEPG = "60_Minutes"
		s.EnableMappedChannels = true
	})
	Data.XMLTV.Mapping = map[string]any{"a.xml": map[string]any{"bbc": xmltvChannel("bbc", "", "BBC")}}

	if err := mapping(); err != nil {
		t.Fatal(err)
	}

	var channels = xepgChannels(t)
	if c := channels["Unknown/News"]; c.XmltvFile != "xTeVe Dummy" || c.XMapping != "60_Minutes" || !c.XActive {
		t.Errorf("Unknown = %+v, want the active default dummy", c)
	}
	if c := channels["BBC/News"]; c.XmltvFile != "a.xml" || c.XMapping != "bbc" || !c.XActive {
		t.Errorf("BBC = %+v, want it mapped and activated", c)
	}
}

// A match by tvg-id in any file must win over a match by name in another file.
func TestMappingPrefersTvgID(t *testing.T) {
	var c = dbChannel("x-ID.0", "1", "BBC", "News")
	c.TvgID = "bbc.uk"
	xepgTest(t, []XEPGChannelStruct{c})
	if err := createXEPGDatabase(); err != nil {
		t.Fatal(err)
	}
	Data.XMLTV.Mapping = map[string]any{
		"a.xml": map[string]any{"other": xmltvChannel("other", "", "BBC")},
		"b.xml": map[string]any{"bbc.uk": xmltvChannel("bbc.uk", "", "BBC One")},
	}

	if err := mapping(); err != nil {
		t.Fatal(err)
	}
	if c := xepgChannels(t)["BBC/News"]; c.XmltvFile != "b.xml" || c.XMapping != "bbc.uk" {
		t.Errorf("BBC: file %q, mapping %q; want b.xml, bbc.uk", c.XmltvFile, c.XMapping)
	}
}
