package src

import (
	"fmt"
	"slices"
	"sort"
	"strconv"
	"strings"
	"xteve/src/internal/epg"
)

// Create xTeVe M3U file
func buildM3U(groups []string) (m3u string, err error) {
	var xepg = xepgDB.Get()
	var imgc = xepg.Images
	var m3uChannels = make(map[float64]XEPGChannelStruct)
	var channelNumbers []float64

	for _, xepgChannel := range xepg.Channels {
		if !xepgChannel.XActive {
			continue
		}
		if len(groups) > 0 && !slices.Contains(groups, xepgChannel.XGroupTitle) {
			continue
		}

		var channelNumber, err = strconv.ParseFloat(strings.TrimSpace(xepgChannel.XChannelID), 64)
		if err == nil {
			m3uChannels[channelNumber] = xepgChannel
			channelNumbers = append(channelNumbers, channelNumber)
		}
	}

	// Create M3U Content
	sort.Float64s(channelNumbers)

	var xmltvURL = fmt.Sprintf("%s://%s/xmltv/xteve.xml", System.Protocol(), System.Domain())
	m3u = fmt.Sprintf(`#EXTM3U url-tvg="%s" x-tvg-url="%s"`+"\n", xmltvURL, xmltvURL)

	for _, channelNumber := range channelNumbers {
		var channel = m3uChannels[channelNumber]

		var parameter = fmt.Sprintf(
			`#EXTINF:0 channelID="%s" tvg-chno="%s" tvg-name="%s" tvg-id="%s" tvg-logo="%s" group-title="%s",%s`+"\n",
			channel.XEPG,
			channel.XChannelID,
			channel.XName,
			channel.XChannelID,
			epg.ImageURL(imgc, channel.TvgLogo),
			channel.XGroupTitle,
			channel.XName,
		)
		var stream, err = createStreamingURL("M3U", channel.FileM3UID, channel.XChannelID, channel.XName, channel.URL)
		if err == nil {
			m3u = m3u + parameter + stream + "\n"
		}
	}

	if len(groups) == 0 {
		var filename = System.Folder.Data + "xteve.m3u"
		err = writeByteToFile(filename, []byte(m3u))
	}

	return
}
