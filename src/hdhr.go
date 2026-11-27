package src

import (
	"encoding/json"
	"fmt"
	"sync/atomic"

	"xteve/src/internal/config"
	"xteve/src/internal/hdhr"
)

// The guide numbers of the PMS mode and the streaming URLs handed out, set by initHDHR when the file paths are
// known.
var (
	pmsNumbers atomic.Pointer[hdhr.PMSNumbers]
	streamURLs atomic.Pointer[hdhr.URLs]
)

// initHDHR opens the stores of the guide numbers and the streaming URLs.
func initHDHR() {
	pmsNumbers.Store(hdhr.NewPMSNumbers(System.File.PMS))
	streamURLs.Store(hdhr.NewURLs(System.File.URLS))
}

// hdhrDevice returns the information that the emulated tuner reports.
func hdhrDevice() hdhr.Device {
	return hdhr.Device{
		BaseURL:    System.Protocol() + "://" + System.Domain(),
		DeviceID:   deviceID(),
		AppName:    System.AppName,
		Name:       System.Name,
		Version:    System.Version,
		TunerCount: config.Get().Tuner,
	}
}

func getCapability() ([]byte, error) { return hdhr.DeviceXML(hdhrDevice()) }

func getDiscover() ([]byte, error) { return hdhr.DiscoverJSON(hdhrDevice()) }

func getLineupStatus() ([]byte, error) { return hdhr.LineupStatusJSON(System.Scanning()) }

// getLineup returns lineup.json: the active streams in PMS mode, the active XEPG channels in XEPG mode.
func getLineup() (jsonContent []byte, err error) {
	var lineup []hdhr.LineupStream

	var add = func(playlistID, number, name, url string) {
		// A channel without a streaming URL is left out of the lineup
		if streamURL, err := createStreamingURL("DVR", playlistID, number, name, url); err != nil {
			ShowError(err, 1202)
		} else {
			lineup = append(lineup, hdhr.LineupStream{GuideName: name, GuideNumber: number, URL: streamURL})
		}
	}

	switch config.Get().EpgSource {
	case "PMS":
		var channels []M3UChannelStructXEPG
		for _, dsa := range streamsDB.Get().Active {
			var m3uChannel M3UChannelStructXEPG
			if err = json.Unmarshal([]byte(mapToJSON(dsa)), &m3uChannel); err != nil {
				return
			}
			channels = append(channels, m3uChannel)
		}

		// Channels without a stream ID of their own get the stored PMS guide number of their name
		var names []string
		for _, c := range channels {
			if len(c.UUIDValue) == 0 {
				names = append(names, c.Name)
			}
		}
		numbers, numbersErr := pmsNumbers.Load().Numbers(names)
		if numbersErr != nil {
			ShowError(numbersErr, 0)
		}

		var next = 0
		for i, c := range channels {
			var number = c.UUIDValue
			if len(number) == 0 {
				// The generated number is kept if no stored PMS number is available
				number = fmt.Sprintf("%d", i+1000)
				if next < len(numbers) && len(numbers[next]) > 0 {
					number = numbers[next]
				}
				next++
			}
			add(c.FileM3UID, number, c.Name, c.URL)
		}

	case "XEPG":
		for _, dxc := range xepgDB.Get().Channels {
			var xepgChannel XEPGChannelStruct
			if err = json.Unmarshal([]byte(mapToJSON(dxc)), &xepgChannel); err != nil {
				return
			}
			if xepgChannel.XActive {
				add(xepgChannel.FileM3UID, xepgChannel.XChannelID, xepgChannel.XName, xepgChannel.URL)
			}
		}
	}

	jsonContent, err = hdhr.LineupJSON(lineup)

	if saveErr := streamURLs.Load().Save(); saveErr != nil && err == nil {
		err = saveErr
	}

	return
}

// guideNumberLess orders guide numbers numerically.
func guideNumberLess(a, b string) bool { return hdhr.GuideNumberLess(a, b) }
