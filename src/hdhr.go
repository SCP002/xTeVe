package src

import (
	"bytes"
	"encoding/json"
	"encoding/xml"
	"fmt"
	"sort"
	"strconv"
	"xteve/src/internal/config"
)

func getCapability() (xmlContent []byte, err error) {
	var capability Capability
	var buffer bytes.Buffer

	capability.Xmlns = "urn:schemas-upnp-org:device-1-0"
	capability.URLBase = System.Protocol() + "://" + System.Domain()

	capability.SpecVersion.Major = 1
	capability.SpecVersion.Minor = 0

	capability.Device.DeviceType = "urn:schemas-upnp-org:device:MediaServer:1"
	capability.Device.FriendlyName = System.Name
	capability.Device.Manufacturer = "Silicondust"
	capability.Device.ModelName = "HDTC-2US"
	capability.Device.ModelNumber = "HDTC-2US"
	capability.Device.SerialNumber = ""
	capability.Device.UDN = "uuid:" + deviceID()

	output, err := xml.MarshalIndent(capability, " ", "  ")
	if err != nil {
		return nil, withCode(err, 1003)
	}

	buffer.Write([]byte(xml.Header))
	buffer.Write([]byte(output))
	xmlContent = buffer.Bytes()

	return
}

func getDiscover() (jsonContent []byte, err error) {
	var discover Discover

	discover.BaseURL = System.Protocol() + "://" + System.Domain()
	discover.DeviceAuth = System.AppName
	discover.DeviceID = deviceID()
	discover.FirmwareName = "bin_" + System.Version
	discover.FirmwareVersion = System.Version
	discover.FriendlyName = System.Name

	discover.LineupURL = fmt.Sprintf("%s://%s/lineup.json", System.Protocol(), System.Domain())
	discover.Manufacturer = "Golang"
	discover.ModelNumber = System.Version
	discover.TunerCount = config.Get().Tuner

	jsonContent, err = json.MarshalIndent(discover, "", "  ")

	return
}

func getLineupStatus() (jsonContent []byte, err error) {
	var lineupStatus LineupStatus

	if System.Scanning() {
		lineupStatus.ScanInProgress = 1
	}
	lineupStatus.ScanPossible = 0
	lineupStatus.Source = "Cable"
	lineupStatus.SourceList = []string{"Cable"}

	jsonContent, err = json.MarshalIndent(lineupStatus, "", "  ")

	return
}

func getLineup() (jsonContent []byte, err error) {
	var lineup Lineup

	switch config.Get().EpgSource {
	case "PMS":
		// PMS IDs are stored by channel name. Channels with the same name get the keys "Name (2)", "Name (3)" and
		// so on, so that each of them has its own ID and the first one keeps the ID stored for the plain name.
		var pmsKeys = make(map[string]bool)

		for i, dsa := range streamsDB.Get().Active {
			var m3uChannel M3UChannelStructXEPG

			err = json.Unmarshal([]byte(mapToJSON(dsa)), &m3uChannel)
			if err != nil {
				return
			}

			var stream LineupStream
			stream.GuideName = m3uChannel.Name
			switch len(m3uChannel.UUIDValue) {
			case 0:
				// The generated number is kept if no stored PMS ID is available
				stream.GuideNumber = fmt.Sprintf("%d", i+1000)
				var pmsKey = stream.GuideName
				for n := 2; pmsKeys[pmsKey]; n++ {
					pmsKey = fmt.Sprintf("%s (%d)", stream.GuideName, n)
				}
				pmsKeys[pmsKey] = true

				if guideNumber, err := getGuideNumberPMS(pmsKey); err != nil {
					ShowError(err, 0)
				} else {
					stream.GuideNumber = guideNumber
				}

			default:
				stream.GuideNumber = m3uChannel.UUIDValue
			}

			// A channel without a streaming URL is left out of the lineup
			if streamURL, err := createStreamingURL(
				"DVR",
				m3uChannel.FileM3UID,
				stream.GuideNumber,
				m3uChannel.Name,
				m3uChannel.URL,
			); err != nil {
				ShowError(err, 1202)
			} else {
				stream.URL = streamURL
				lineup = append(lineup, stream)
			}
		}

	case "XEPG":
		for _, dxc := range xepgDB.Get().Channels {
			var xepgChannel XEPGChannelStruct
			err = json.Unmarshal([]byte(mapToJSON(dxc)), &xepgChannel)
			if err != nil {
				return
			}

			if xepgChannel.XActive {
				var stream LineupStream
				stream.GuideName = xepgChannel.XName
				stream.GuideNumber = xepgChannel.XChannelID
				// A channel without a streaming URL is left out of the lineup
				if streamURL, err := createStreamingURL(
					"DVR",
					xepgChannel.FileM3UID,
					xepgChannel.XChannelID,
					xepgChannel.XName,
					xepgChannel.URL,
				); err != nil {
					ShowError(err, 1202)
				} else {
					stream.URL = streamURL
					lineup = append(lineup, stream)
				}
			}
		}
	}

	// Sort the lineup
	// Have to use type assertions (https://golang.org/ref/spec#Type_assertions) to cast generic interface{} into
	// LineupStream
	sort.SliceStable(lineup, func(i, j int) bool {
		return guideNumberLess(lineup[i].(LineupStream).GuideNumber, lineup[j].(LineupStream).GuideNumber)
	})

	jsonContent, err = json.MarshalIndent(lineup, "", "  ")

	if pmsChanged {
		if saveErr := saveMapToJSONFile(System.File.PMS, Data.Cache.PMS); saveErr != nil {
			ShowError(saveErr, 0)
		}
		pmsChanged = false
	}
	Data.Cache.PMS = nil

	if saveErr := saveMapToJSONFile(System.File.URLS, Data.Cache.StreamingURLS); saveErr != nil && err == nil {
		err = saveErr
	}

	return
}

// pmsChanged reports that getGuideNumberPMS added IDs to Data.Cache.PMS that are not saved yet.
var pmsChanged bool

// getGuideNumberPMS returns the stored PMS ID of a channel, or a new one. New IDs are saved by getLineup.
func getGuideNumberPMS(channelName string) (pmsID string, err error) {
	if len(Data.Cache.PMS) == 0 {
		Data.Cache.PMS = make(map[string]string)

		pms, err := loadJSONFileToMap(System.File.PMS)

		if err != nil {
			return "", err
		}

		for key, value := range pms {
			Data.Cache.PMS[key] = value.(string)
		}
	}

	if value, ok := Data.Cache.PMS[channelName]; ok {
		pmsID = value
	} else {
		// The first free ID: "id-0", "id-1" and so on
		var used = make(map[string]bool, len(Data.Cache.PMS))
		for _, v := range Data.Cache.PMS {
			used[v] = true
		}
		for i := 0; ; i++ {
			if pmsID = fmt.Sprintf("id-%d", i); !used[pmsID] {
				break
			}
		}
		Data.Cache.PMS[channelName] = pmsID
		pmsChanged = true
	}

	return
}

// guideNumberLess orders guide numbers numerically.
//
// A guide number can come from the "tvg-chno" M3U attribute and does not have to be a number;
// such guide numbers go after the numeric ones, in alphabetical order.
func guideNumberLess(a, b string) bool {
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
