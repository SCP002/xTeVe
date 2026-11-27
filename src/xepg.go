package src

import (
	"encoding/json"
	"encoding/xml"
	"errors"
	"fmt"
	"os"
	"path"
	"regexp"
	"runtime"
	"slices"
	"sort"

	"crypto/md5"
	"encoding/hex"
	"strconv"
	"strings"
	"time"

	"xteve/src/internal/imgcache"

	"github.com/samber/lo"
)

// Check provider XMLTV File
func checkXMLCompatibility(id string, body []byte) (err error) {
	var xmltv XMLTV
	var compatibility = make(map[string]int)

	err = xml.Unmarshal(body, &xmltv)
	if err != nil {
		return
	}

	compatibility["xmltv.channels"] = len(xmltv.Channel)
	compatibility["xmltv.programs"] = len(xmltv.Program)

	setProviderCompatibility(id, "xmltv", compatibility)

	return
}

// Create XEPG Data
func buildXEPG(background bool) {
	// Only one rebuild at a time
	if !System.ScanInProgress.CompareAndSwap(false, true) {
		return
	}

	var err error

	Data.Cache.Images, err = imgcache.New(
		System.Folder.ImagesCache,
		fmt.Sprintf("%s://%s/images/", System.Protocol(), System.Domain()),
		Settings.CacheImages,
	)
	if err != nil {
		ShowError(err, 0)
	}

	if Settings.EpgSource == "XEPG" {
		switch background {
		case true:
			go func() {
				createXEPGMapping()
				if err := createXEPGDatabase(); err != nil {
					ShowError(err, 0)
				}
				if err := mapping(); err != nil {
					ShowError(err, 0)
				}
				cleanupXEPG()
				if err := createXMLTVFile(); err != nil {
					ShowError(err, 0)
				}
				createM3UFile()

				showInfo("XEPG:" + "Ready to use")

				if Settings.CacheImages && System.ImageCachingInProgress.CompareAndSwap(false, true) {
					go func() {
						showInfo(fmt.Sprintf("Image Caching:Images are cached (%d)", len(Data.Cache.Images.Queue)))

						if err := Data.Cache.Images.Image.Caching(); err != nil {
							ShowError(err, 0)
						}
						if err := Data.Cache.Images.Image.Remove(); err != nil {
							ShowError(err, 0)
						}
						showInfo("Image Caching:Done")

						if err := createXMLTVFile(); err != nil {
							ShowError(err, 0)
						}
						createM3UFile()

						System.ImageCachingInProgress.Store(false)
					}()
				}

				System.ScanInProgress.Store(false)

				if Settings.ClearXMLTVCache {
					clearXMLTVCache()
				}
			}()

		case false:
			createXEPGMapping()
			if err := createXEPGDatabase(); err != nil {
				ShowError(err, 0)
			}
			if err := mapping(); err != nil {
				ShowError(err, 0)
			}
			cleanupXEPG()

			go func() {
				if err := createXMLTVFile(); err != nil {
					ShowError(err, 0)
				}
				createM3UFile()

				if Settings.CacheImages && System.ImageCachingInProgress.CompareAndSwap(false, true) {
					go func() {
						showInfo(fmt.Sprintf("Image Caching:Images are cached (%d)", len(Data.Cache.Images.Queue)))

						if err := Data.Cache.Images.Image.Caching(); err != nil {
							ShowError(err, 0)
						}
						if err := Data.Cache.Images.Image.Remove(); err != nil {
							ShowError(err, 0)
						}
						showInfo("Image Caching:Done")

						if err := createXMLTVFile(); err != nil {
							ShowError(err, 0)
						}
						createM3UFile()

						System.ImageCachingInProgress.Store(false)
					}()
				}

				showInfo("XEPG:" + "Ready to use")

				System.ScanInProgress.Store(false)

				if Settings.ClearXMLTVCache {
					clearXMLTVCache()
				}
			}()
		}
	} else {
		if _, err := getLineup(); err != nil {
			ShowError(err, 0)
		}
		System.ScanInProgress.Store(false)
	}
}

// Create Mapping Menu for the XMLTV Files
func createXEPGMapping() {
	Data.XMLTV.Files = getLocalProviderFiles("xmltv")
	Data.XMLTV.Mapping = make(map[string]any)

	var tmpMap = make(map[string]any)

	if len(Data.XMLTV.Files) > 0 {
		for i, file := range slices.Backward(Data.XMLTV.Files) {
			var err error
			var fileID = strings.TrimSuffix(getFilenameFromPath(file), path.Ext(getFilenameFromPath(file)))
			showInfo("XEPG:" + "Parse XMLTV file: " + getProviderParameter(fileID, "xmltv", "name"))

			var xmltv XMLTV

			err = getLocalXMLTV(file, &xmltv)
			if err != nil {
				// Safe while iterating backwards: only already visited files move
				Data.XMLTV.Files = append(Data.XMLTV.Files[:i], Data.XMLTV.Files[i+1:]...)
				ShowError(fmt.Errorf("%s: %w", getProviderParameter(fileID, "xmltv", "name"), err), 0)
			}

			// XML Parsing (Provider File)
			if err == nil {
				// Write Data from the XML File to a temporary Map
				var xmltvMap = make(map[string]any)

				for _, c := range xmltv.Channel {
					var channel = make(map[string]any)

					channel["id"] = c.ID
					channel["display-names"] = c.DisplayNames
					channel["icon"] = c.Icon.Src

					xmltvMap[c.ID] = channel
				}

				tmpMap[getFilenameFromPath(file)] = xmltvMap
				Data.XMLTV.Mapping[getFilenameFromPath(file)] = xmltvMap
			}
		}

		Data.XMLTV.Mapping = tmpMap
	} else {
		if !System.ConfigurationWizard.Load() {
			showWarning(1007)
		}
	}

	// Create selection for the Dummy
	var dummy = make(map[string]any)
	var times = []string{"30", "60", "90", "120", "180", "240", "360"}

	for _, i := range times {
		var dummyChannel = make(map[string]any)
		dummyChannel["display-names"] = []DisplayName{{Value: i + " Minutes"}}
		dummyChannel["id"] = i + "_Minutes"
		dummyChannel["icon"] = ""

		dummy[dummyChannel["id"].(string)] = dummyChannel
	}

	Data.XMLTV.Mapping["xTeVe Dummy"] = dummy
}

// Create / update XEPG Database
func createXEPGDatabase() (err error) {
	var allChannelNumbers = make([]float64, 0)
	Data.Cache.Streams.Active = make([]string, 0)
	Data.XEPG.Channels = make(map[string]any)

	Data.XEPG.Channels, err = loadJSONFileToMap(System.File.XEPG)
	if err != nil {
		return withCode(err, 1004)
	}

	var createNewID = func() (xepg string) {
		var firstID = 0

	newXEPGID:

		if _, ok := Data.XEPG.Channels["x-ID."+strconv.FormatInt(int64(firstID), 10)]; ok {
			firstID++
			goto newXEPGID
		}

		xepg = "x-ID." + strconv.FormatInt(int64(firstID), 10)
		return
	}

	// getFreeChannelNumber returns the first free number starting from the
	// first valid one of startingChannels, or from the default first channel
	var getFreeChannelNumber = func(startingChannels ...string) (xChannelID string) {
		var firstFreeNumber = Settings.MappingFirstChannel
		for _, s := range startingChannels {
			if n, err := strconv.ParseFloat(s, 64); err == nil && n > 0 {
				firstFreeNumber = n
				break
			}
		}

		for {
			if lo.IndexOf(allChannelNumbers, firstFreeNumber) == -1 {
				xChannelID = fmt.Sprintf("%g", firstFreeNumber)
				allChannelNumbers = append(allChannelNumbers, firstFreeNumber)
				return
			}

			firstFreeNumber++
		}
	}

	var generateHashForChannel = func(
		m3uID, name, groupTitle, tvgID, tvgName, uuidKey, uuidValue string,
	) string {
		hash := md5.Sum([]byte(m3uID + name + groupTitle + tvgID + tvgName + uuidKey + uuidValue))
		return hex.EncodeToString(hash[:])
	}

	showInfo("XEPG:" + "Update database")

	// Delete Channel with missing Channel Numbers.
	for id, dxc := range Data.XEPG.Channels {
		var xepgChannel XEPGChannelStruct
		err = json.Unmarshal([]byte(mapToJSON(dxc)), &xepgChannel)
		if err != nil {
			return
		}

		if len(xepgChannel.XChannelID) == 0 {
			delete(Data.XEPG.Channels, id)
		}

		if xChannelID, err := strconv.ParseFloat(xepgChannel.XChannelID, 64); err == nil {
			allChannelNumbers = append(allChannelNumbers, xChannelID)
		}
	}

	// Database channels in a fixed order, so that matching does not depend on
	// the iteration order of the map
	var dbChannels = make([]XEPGChannelStruct, 0, len(Data.XEPG.Channels))
	for _, v := range Data.XEPG.Channels {
		var channel XEPGChannelStruct
		err = json.Unmarshal([]byte(mapToJSON(v)), &channel)
		if err != nil {
			return
		}
		dbChannels = append(dbChannels, channel)
	}
	sort.Slice(dbChannels, func(i, j int) bool { return xepgIDLess(dbChannels[i].XEPG, dbChannels[j].XEPG) })

	// Database channels by their previously downloaded attributes -- filename, group, title, etc. Several channels
	// can have the same attributes, if a playlist contains the same channel more than once.
	var xepgChannelsValuesMap = make(map[string][]XEPGChannelStruct)
	for _, channel := range dbChannels {
		channelHash := generateHashForChannel(
			channel.FileM3UID,
			channel.Name,
			channel.GroupTitle,
			channel.TvgID,
			channel.TvgName,
			channel.UUIDKey,
			channel.UUIDValue,
		)
		xepgChannelsValuesMap[channelHash] = append(xepgChannelsValuesMap[channelHash], channel)
	}

	var m3uChannels = make([]M3UChannelStructXEPG, len(Data.Streams.Active))
	for i, dsa := range Data.Streams.Active {
		err = json.Unmarshal([]byte(mapToJSON(dsa)), &m3uChannels[i])
		if err != nil {
			return
		}
	}

	// Each database channel is used by one stream only. Exact matches go first, so that a full scan for another
	// stream cannot take a channel that matches a stream exactly.
	var claimed = make(map[string]bool)
	var exactMatch = make([]string, len(m3uChannels))
	for i, m3uChannel := range m3uChannels {
		m3uChannelHash := generateHashForChannel(
			m3uChannel.FileM3UID,
			m3uChannel.Name,
			m3uChannel.GroupTitle,
			m3uChannel.TvgID,
			m3uChannel.TvgName,
			m3uChannel.UUIDKey,
			m3uChannel.UUIDValue,
		)
		for _, channel := range xepgChannelsValuesMap[m3uChannelHash] {
			if !claimed[channel.XEPG] {
				claimed[channel.XEPG] = true
				exactMatch[i] = channel.XEPG
				break
			}
		}
	}

	for i, m3uChannel := range m3uChannels {
		var channelExists = false  // Decides whether a Channel should be added to the Database
		var channelHasUUID = false // Checks whether the Channel (Stream) has Unique IDs
		// Current Database ID (XEPG) Used to update the Channel in the Database with the Stream of the M3U
		var currentXEPGID string

		Data.Cache.Streams.Active = append(Data.Cache.Streams.Active, m3uChannel.Name+m3uChannel.FileM3UID)

		// Try to find the channel based on matching all known values. If that fails, then move to full channel scan
		if len(exactMatch[i]) > 0 {
			channelExists = true
			currentXEPGID = exactMatch[i]
			if len(m3uChannel.UUIDValue) > 0 {
				channelHasUUID = true
			}
		} else {
			// Run through the XEPG Database to search for the Channel (full scan)
			for _, dxc := range dbChannels {
				if claimed[dxc.XEPG] {
					continue
				}

				if m3uChannel.FileM3UID == dxc.FileM3UID {
					dxc.FileM3UID = m3uChannel.FileM3UID
					dxc.FileM3UName = m3uChannel.FileM3UName

					// Compare the Stream using a UUID in the M3U with the Channel in the Database
					if len(dxc.UUIDValue) > 0 && len(m3uChannel.UUIDValue) > 0 {
						if dxc.UUIDValue == m3uChannel.UUIDValue && dxc.UUIDKey == m3uChannel.UUIDKey {
							channelExists = true
							channelHasUUID = true
							currentXEPGID = dxc.XEPG
							break
						}
					} else {
						// Compare the Stream to the Channel in the Database using the Channel Name
						if dxc.Name == m3uChannel.Name {
							channelExists = true
							currentXEPGID = dxc.XEPG
							break
						}
					}

					// Rename the Channel if it's update regex matches new channel name
					if len(dxc.UpdateChannelNameRegex) == 0 {
						continue
					}
					// Guard against the situation when both channels have UUIDValue, they are different, but names are
					// the same
					if dxc.Name == m3uChannel.Name {
						continue
					}
					nameRx, err := regexp.Compile(dxc.UpdateChannelNameRegex)
					if err != nil {
						ShowError(err, 1018)
						continue
					}
					if !nameRx.MatchString(m3uChannel.Name) {
						continue
					}
					if len(dxc.UpdateChannelNameByGroupRegex) > 0 {
						groupRx, err := regexp.Compile(dxc.UpdateChannelNameByGroupRegex)
						if err != nil {
							ShowError(err, 1018)
							continue
						}
						if !groupRx.MatchString(dxc.XGroupTitle) {
							// Found the channel name to update but it has wrong group
							continue
						}
					}
					showInfo("XEPG:" + fmt.Sprintf("Renaming the channel '%v' to '%v'", dxc.Name, m3uChannel.Name))
					channelExists = true
					// dxc.Name will be assigned later in channelExists switch
					dxc.XName = m3uChannel.Name
					currentXEPGID = dxc.XEPG
					Data.XEPG.Channels[currentXEPGID] = dxc
					break
				}
			}

			if channelExists {
				claimed[currentXEPGID] = true
			}
		}

		switch channelExists {
		case true:
			// Existing Channel
			var xepgChannel XEPGChannelStruct
			err = json.Unmarshal([]byte(mapToJSON(Data.XEPG.Channels[currentXEPGID])), &xepgChannel)
			if err != nil {
				return
			}

			// Update Streaming URL
			xepgChannel.URL = m3uChannel.URL

			// Update Name, the Name is used to check whether the Channel is still available in a Playlist. Function:
			// cleanupXEPG
			xepgChannel.Name = m3uChannel.Name

			// Update Channel Name, only possible with Channel ID's
			if channelHasUUID {
				if xepgChannel.XUpdateChannelName {
					xepgChannel.XName = m3uChannel.Name
				}
			}

			// Update GroupTitle
			xepgChannel.GroupTitle = m3uChannel.GroupTitle

			if xepgChannel.XUpdateChannelGroup {
				xepgChannel.XGroupTitle = m3uChannel.GroupTitle
			}

			// Update Channel Logo. Will be overwritten again if the Logo is present in the XMLTV file
			if xepgChannel.XUpdateChannelIcon {
				xepgChannel.TvgLogo = m3uChannel.TvgLogo
			}

			Data.XEPG.Channels[currentXEPGID] = xepgChannel

		case false:
			// New Channel
			var xepg = createNewID()
			var xChannelID string
			if m3uChannel.PreserveMapping == "true" {
				xChannelID = getFreeChannelNumber(m3uChannel.UUIDValue, m3uChannel.StartingChannel)
			} else {
				xChannelID = getFreeChannelNumber(m3uChannel.StartingChannel)
			}
			var newChannel XEPGChannelStruct
			newChannel.FileM3UID = m3uChannel.FileM3UID
			newChannel.FileM3UName = m3uChannel.FileM3UName
			newChannel.FileM3UPath = m3uChannel.FileM3UPath
			newChannel.Values = m3uChannel.Values
			newChannel.GroupTitle = m3uChannel.GroupTitle
			newChannel.Name = m3uChannel.Name
			newChannel.TvgID = m3uChannel.TvgID
			newChannel.TvgLogo = m3uChannel.TvgLogo
			newChannel.TvgName = m3uChannel.TvgName
			if m3uChannel.TvgShift == "" {
				newChannel.TvgShift = "0"
			} else {
				newChannel.TvgShift = m3uChannel.TvgShift
			}
			newChannel.URL = m3uChannel.URL
			newChannel.XmltvFile = ""
			newChannel.XMapping = ""

			if len(m3uChannel.UUIDKey) > 0 {
				newChannel.UUIDKey = m3uChannel.UUIDKey
				newChannel.UUIDValue = m3uChannel.UUIDValue
			}

			newChannel.XName = m3uChannel.Name
			newChannel.XGroupTitle = m3uChannel.GroupTitle
			newChannel.XEPG = xepg
			newChannel.XChannelID = xChannelID
			newChannel.XTimeshift = newChannel.TvgShift

			Data.XEPG.Channels[xepg] = newChannel
		}
	}
	showInfo("XEPG:" + "Save DB file")
	err = saveMapToJSONFile(System.File.XEPG, Data.XEPG.Channels)
	if err != nil {
		return
	}

	return
}

// findXMLTVChannel looks for the XMLTV channel of a stream in Data.XMLTV.Mapping: first by tvg-id in all files,
// then by comparing the stream name with every display name, ignoring case and spaces. Files are searched in name
// order, the dummy last, so the result does not depend on the iteration order of the map.
func findXMLTVChannel(tvgID, name string) (file string, channel map[string]any, ok bool) {
	var files = make([]string, 0, len(Data.XMLTV.Mapping))
	for f := range Data.XMLTV.Mapping {
		files = append(files, f)
	}
	sort.Slice(files, func(i, j int) bool {
		if (files[i] == "xTeVe Dummy") != (files[j] == "xTeVe Dummy") {
			return files[j] == "xTeVe Dummy"
		}
		return files[i] < files[j]
	})

	var channelsOf = func(file string) map[string]any {
		channels, _ := Data.XMLTV.Mapping[file].(map[string]any)
		return channels
	}

	if len(tvgID) > 0 {
		for _, f := range files {
			if c, isMap := channelsOf(f)[tvgID].(map[string]any); isMap {
				if _, hasID := c["id"].(string); hasID {
					return f, c, true
				}
			}
		}
	}

	var nameSolid = strings.ReplaceAll(name, " ", "")

	for _, f := range files {
		var channels = channelsOf(f)
		var ids = make([]string, 0, len(channels))
		for id := range channels {
			ids = append(ids, id)
		}
		sort.Strings(ids)

		for _, id := range ids {
			c, isMap := channels[id].(map[string]any)
			if !isMap {
				continue
			}
			if _, hasID := c["id"].(string); !hasID {
				continue
			}
			names, _ := c["display-names"].([]DisplayName)
			for _, n := range names {
				if strings.EqualFold(strings.ReplaceAll(n.Value, " ", ""), nameSolid) {
					return f, c, true
				}
			}
		}
	}

	return "", nil, false
}

// xepgIDLess orders XEPG IDs ("x-ID.12") by their number, so that older channels come first.
func xepgIDLess(a, b string) bool {
	na, errA := strconv.Atoi(strings.TrimPrefix(a, "x-ID."))
	nb, errB := strconv.Atoi(strings.TrimPrefix(b, "x-ID."))
	if errA != nil || errB != nil {
		return a < b
	}
	return na < nb
}

// Automatically assign Channels and check the Mapping
func mapping() (err error) {
	showInfo("XEPG:" + "Map channels")

	for xepg, dxc := range Data.XEPG.Channels {
		var xepgChannel XEPGChannelStruct
		err = json.Unmarshal([]byte(mapToJSON(dxc)), &xepgChannel)
		if err != nil {
			return
		}

		// Automatic mapping for new Channels. Is only executed if the Channel is deactivated and no XMLTV file and no
		// XMLTV Channel is assigned.
		if !xepgChannel.XActive {
			// Values can be "-", therefore len <= 1
			// If either XmltvFile (XMLTV file / EPG source) or XMapping (XMLTV Channel / EPG program) is "-" or null,
			// then look for a matching EPG program.
			if len(xepgChannel.XmltvFile) <= 1 || len(xepgChannel.XMapping) <= 1 {
				var tvgID = xepgChannel.TvgID

				// Set default for new Channel
				if Settings.DefaultMissingEPG != "-" {
					xepgChannel.XmltvFile = "xTeVe Dummy"
					xepgChannel.XMapping = Settings.DefaultMissingEPG
				} else {
					xepgChannel.XmltvFile = "-"
					xepgChannel.XMapping = "-"
				}

				Data.XEPG.Channels[xepg] = xepgChannel

				if file, channel, ok := findXMLTVChannel(tvgID, xepgChannel.Name); ok {
					xepgChannel.XmltvFile = file
					xepgChannel.XMapping = channel["id"].(string)

					// If there is a Logo in the XMLTV file, this will be used. If not, then the Logo from the M3U
					// file
					if icon, ok := channel["icon"].(string); ok && len(icon) > 0 {
						xepgChannel.TvgLogo = icon
					}

					Data.XEPG.Channels[xepg] = xepgChannel
				}
			}
		}

		if Settings.EnableMappedChannels && (xepgChannel.XmltvFile != "-" || xepgChannel.XMapping != "-") {
			xepgChannel.XActive = true
		}

		// Check whether the assigned XMLTV Files and Channels still exist.
		if xepgChannel.XActive {
			var mapping = xepgChannel.XMapping
			var file = xepgChannel.XmltvFile

			if file != "xTeVe Dummy" {
				if value, ok := Data.XMLTV.Mapping[file].(map[string]any); ok {
					if channel, ok := value[mapping].(map[string]any); ok {
						// Update Channel Logo
						if logo, ok := channel["icon"].(string); ok {
							if xepgChannel.XUpdateChannelIcon && len(logo) > 0 {
								xepgChannel.TvgLogo = logo
							}
						}
					} else {
						ShowError(fmt.Errorf("missing EPG data: %s", xepgChannel.Name), 0)
						showWarning(2302)
						xepgChannel.XActive = false
					}
				} else {
					var fileID = strings.TrimSuffix(getFilenameFromPath(file), path.Ext(getFilenameFromPath(file)))

					ShowError(fmt.Errorf("missing XMLTV file: %s", getProviderParameter(fileID, "xmltv", "name")), 0)
					showWarning(2301)
					xepgChannel.XActive = false
				}
			}

			if len(xepgChannel.XmltvFile) == 0 {
				xepgChannel.XmltvFile = "-"
				xepgChannel.XActive = false
			}

			if len(xepgChannel.XMapping) == 0 {
				xepgChannel.XMapping = "-"
				xepgChannel.XActive = false
			}

			Data.XEPG.Channels[xepg] = xepgChannel
		}
	}

	err = saveMapToJSONFile(System.File.XEPG, Data.XEPG.Channels)
	if err != nil {
		return
	}

	return
}

// Create XMLTV File
func createXMLTVFile() (err error) {
	// Image Cache
	var imgc = Data.Cache.Images

	Data.Cache.ImagesFiles = []string{}
	Data.Cache.ImagesURLS = []string{}
	Data.Cache.ImagesCache = []string{}

	files, err := os.ReadDir(System.Folder.ImagesCache)
	if err == nil {
		for _, file := range files {
			if lo.IndexOf(Data.Cache.ImagesCache, file.Name()) == -1 {
				Data.Cache.ImagesCache = append(Data.Cache.ImagesCache, file.Name())
			}
		}
	}

	if len(Data.XMLTV.Files) == 0 && len(Data.Streams.Active) == 0 {
		Data.XEPG.Channels = make(map[string]any)
		return
	}

	showInfo("XEPG:" + fmt.Sprintf("Create XMLTV file (%s)", System.File.XML))

	var xepgXML XMLTV

	xepgXML.Generator = System.Name

	xepgXML.Source = fmt.Sprintf("%s - %s", System.Name, System.Version)

	var tmpProgram = &XMLTV{}

	// A channel without program data is still listed; failures are reported once at the end
	var failedChannels int
	var firstChannelErr error
	var channelFailed = func(err error) {
		failedChannels++
		if firstChannelErr == nil {
			firstChannelErr = err
		}
	}

	for _, dxc := range Data.XEPG.Channels {
		var xepgChannel XEPGChannelStruct
		err := json.Unmarshal([]byte(mapToJSON(dxc)), &xepgChannel)
		if err != nil {
			channelFailed(err)
		} else {
			if xepgChannel.XActive {
				// Channels
				var channel Channel
				channel.ID = xepgChannel.XChannelID
				channel.Icon = Icon{Src: imgc.Image.GetURL(xepgChannel.TvgLogo)}
				channel.DisplayNames = append(channel.DisplayNames, DisplayName{Value: xepgChannel.XName})

				xepgXML.Channel = append(xepgXML.Channel, &channel)

				// Programs

				*tmpProgram, err = getProgramData(xepgChannel)
				if err != nil {
					channelFailed(fmt.Errorf("%s: %w", xepgChannel.XName, err))
				} else {
					xepgXML.Program = append(xepgXML.Program, tmpProgram.Program...)
				}
			}
		}
	}

	if failedChannels > 0 {
		ShowError(
			fmt.Errorf("XMLTV: no program data for %d channels, first error: %w", failedChannels, firstChannelErr),
			0,
		)
	}

	content, err := xml.MarshalIndent(xepgXML, "  ", "    ")
	if err != nil {
		return
	}
	var xmlOutput = []byte(xml.Header + string(content))
	if err = writeByteToFile(System.File.XML, xmlOutput); err != nil {
		return
	}

	showInfo("XEPG:" + fmt.Sprintf("Compress XMLTV file (%s)", System.Compressed.GZxml))
	err = compressGZIP(&xmlOutput, System.Compressed.GZxml)

	xepgXML = XMLTV{}

	return
}

// Create Program Data (createXMLTVFile)
func getProgramData(xepgChannel XEPGChannelStruct) (xepgXML XMLTV, err error) {
	var xmltvFile = System.Folder.Data + xepgChannel.XmltvFile
	var channelID = xepgChannel.XMapping

	var xmltv XMLTV

	if xmltvFile == System.Folder.Data+"xTeVe Dummy" {
		xmltv = createDummyProgram(xepgChannel)
	} else {
		err = getLocalXMLTV(xmltvFile, &xmltv)
		if err != nil {
			return
		}
	}

	// Timeshift in hours, from the channel settings or the "tvg-shift" M3U attribute
	var timeshift int
	if len(xepgChannel.XTimeshift) > 0 {
		var parseErr error
		if timeshift, parseErr = strconv.Atoi(xepgChannel.XTimeshift); parseErr != nil {
			ShowError(
				fmt.Errorf(
					"channel %s: timeshift %q is not a whole number of hours and is ignored",
					xepgChannel.XName,
					xepgChannel.XTimeshift,
				),
				0,
			)
			timeshift = 0
		}
	}
	var timeshiftReported bool

	for _, xmltvProgram := range xmltv.Program {
		if xmltvProgram.Channel == channelID {
			var program = &Program{}

			// Channel ID
			program.Channel = xepgChannel.XChannelID
			program.Start = xmltvProgram.Start
			program.Stop = xmltvProgram.Stop

			if timeshift != 0 {
				start, startErr := shiftXMLTVTime(xmltvProgram.Start, timeshift)
				stop, stopErr := shiftXMLTVTime(xmltvProgram.Stop, timeshift)
				if shiftErr := errors.Join(startErr, stopErr); shiftErr != nil {
					// The program keeps its original time; report the problem once per channel
					if !timeshiftReported {
						ShowError(
							fmt.Errorf("channel %s: timeshift is not applied: %w", xepgChannel.XName, shiftErr),
							0,
						)
						timeshiftReported = true
					}
				} else {
					program.Start, program.Stop = start, stop
				}
			}

			// Title
			program.Title = xmltvProgram.Title

			// Subtitle
			program.SubTitle = xmltvProgram.SubTitle

			// Description
			program.Desc = xmltvProgram.Desc

			// Category
			getCategory(program, xmltvProgram, xepgChannel)

			// Credits
			program.Credits = xmltvProgram.Credits

			// Rating
			program.Rating = xmltvProgram.Rating

			// StarRating
			program.StarRating = xmltvProgram.StarRating

			// Country
			program.Country = xmltvProgram.Country

			// Program icon
			getPoster(program, xmltvProgram)

			// Language
			program.Language = xmltvProgram.Language

			// Episodes numbers
			getEpisodeNum(program, xmltvProgram, xepgChannel)

			// Video
			getVideo(program, xmltvProgram, xepgChannel)

			// Date
			program.Date = xmltvProgram.Date

			// Previously shown
			program.PreviouslyShown = xmltvProgram.PreviouslyShown

			// New
			program.New = xmltvProgram.New

			// Live
			program.Live = xmltvProgram.Live

			// Premiere
			program.Premiere = xmltvProgram.Premiere

			xepgXML.Program = append(xepgXML.Program, program)
		}
	}

	return
}

// Create Dummy Data (createXMLTVFile)
func createDummyProgram(xepgChannel XEPGChannelStruct) (dummyXMLTV XMLTV) {
	var imgc = Data.Cache.Images
	var currentTime = time.Now()
	var dateArray = strings.Fields(currentTime.String())
	var offset = " " + dateArray[2]
	var startTime = time.Date(currentTime.Year(), currentTime.Month(), currentTime.Day(), 0, 0, 0, 0, time.UTC)

	showInfo("Create Dummy Guide:" + "Time offset" + offset + " - " + xepgChannel.XName)

	var dl = strings.Split(xepgChannel.XMapping, "_")
	dummyLength, err := strconv.Atoi(dl[0])
	if err != nil {
		ShowError(err, 000)
		return
	}

	for d := range 4 {
		var epgStartTime = startTime.Add(time.Hour * time.Duration(d*24))

		for t := dummyLength; t <= 1440; t = t + dummyLength {
			var epgStopTime = epgStartTime.Add(time.Minute * time.Duration(dummyLength))

			var epg Program
			poster := Poster{}

			epg.Channel = xepgChannel.XMapping
			epg.Start = epgStartTime.Format("20060102150405") + offset
			epg.Stop = epgStopTime.Format("20060102150405") + offset
			epg.Title = append(
				epg.Title,
				&Title{
					Value: xepgChannel.XName + " (" + epgStartTime.Weekday().String()[0:2] + ". " + epgStartTime.Format(
						"15:04",
					) + " - " + epgStopTime.Format(
						"15:04",
					) + ")",
					Lang: "en",
				},
			)

			if len(xepgChannel.XDescription) == 0 {
				epg.Desc = append(
					epg.Desc,
					&Desc{
						Value: "xTeVe: (" + strconv.Itoa(
							dummyLength,
						) + " Minutes) " + epgStartTime.Weekday().
							String() +
							" " + epgStartTime.Format(
							"15:04",
						) + " - " + epgStopTime.Format(
							"15:04",
						),
						Lang: "en",
					},
				)
			} else {
				epg.Desc = append(epg.Desc, &Desc{Value: xepgChannel.XDescription, Lang: "en"})
			}

			if Settings.XepgReplaceMissingImages {
				poster.Src = imgc.Image.GetURL(xepgChannel.TvgLogo)
				epg.Poster = append(epg.Poster, poster)
			}

			if xepgChannel.XCategory != "Movie" {
				epg.EpisodeNum = append(
					epg.EpisodeNum,
					&EpisodeNum{Value: epgStartTime.Format("2006-01-02 15:04:05"), System: "original-air-date"},
				)
			}

			epg.New = &New{Value: ""}

			dummyXMLTV.Program = append(dummyXMLTV.Program, &epg)
			epgStartTime = epgStopTime
		}
	}

	return
}

// Expand Categories (createXMLTVFile)
func getCategory(program *Program, xmltvProgram *Program, xepgChannel XEPGChannelStruct) {
	for _, i := range xmltvProgram.Category {
		category := &Category{}
		category.Value = i.Value
		category.Lang = i.Lang
		program.Category = append(program.Category, category)
	}

	if len(xepgChannel.XCategory) > 0 {
		category := &Category{}
		category.Value = xepgChannel.XCategory
		category.Lang = "en"
		program.Category = append(program.Category, category)
	}
}

// Load the Poster Cover Program from the XMLTV File
func getPoster(program *Program, xmltvProgram *Program) {
	var imgc = Data.Cache.Images

	for _, poster := range xmltvProgram.Poster {
		poster.Src = imgc.Image.GetURL(poster.Src)
		program.Poster = append(program.Poster, poster)
	}

	if Settings.XepgReplaceMissingImages {
		if len(xmltvProgram.Poster) == 0 {
			var poster Poster
			poster.Src = imgc.Image.GetURL(poster.Src)
			program.Poster = append(program.Poster, poster)
		}
	}
}

// Apply Episode system, if none is available and a Category has been set in the mapping, an Episode is created
func getEpisodeNum(program *Program, xmltvProgram *Program, xepgChannel XEPGChannelStruct) {
	program.EpisodeNum = xmltvProgram.EpisodeNum

	if len(xepgChannel.XCategory) > 0 && xepgChannel.XCategory != "Movie" {
		if len(xmltvProgram.EpisodeNum) == 0 {
			var timeLayout = "20060102150405"

			t, err := time.Parse(timeLayout, strings.Split(xmltvProgram.Start, " ")[0])
			if err == nil {
				program.EpisodeNum = append(
					program.EpisodeNum,
					&EpisodeNum{Value: t.Format("2006-01-02 15:04:05"), System: "original-air-date"},
				)
			} else {
				ShowError(err, 0)
			}
		}
	}
}

// Create Video Parameters (createXMLTVFile)
func getVideo(program *Program, xmltvProgram *Program, xepgChannel XEPGChannelStruct) {
	var video Video
	video.Present = xmltvProgram.Video.Present
	video.Colour = xmltvProgram.Video.Colour
	video.Aspect = xmltvProgram.Video.Aspect
	video.Quality = xmltvProgram.Video.Quality

	if len(xmltvProgram.Video.Quality) == 0 {
		if strings.Contains(strings.ToUpper(xepgChannel.XName), " HD") ||
			strings.Contains(strings.ToUpper(xepgChannel.XName), " FHD") {
			video.Quality = "HDTV"
		}

		if strings.Contains(strings.ToUpper(xepgChannel.XName), " UHD") ||
			strings.Contains(strings.ToUpper(xepgChannel.XName), " 4K") {
			video.Quality = "UHDTV"
		}
	}

	program.Video = video
}

// Load Local Provider XMLTV file
func getLocalXMLTV(file string, xmltv *XMLTV) (err error) {
	if _, ok := Data.Cache.XMLTV[file]; !ok {
		// Initialize Cache
		if len(Data.Cache.XMLTV) == 0 {
			Data.Cache.XMLTV = make(map[string]XMLTV)
		}

		// Read XML Data
		content, err := readByteFromFile(file)

		// Local XML File does not exist in the folder: Data
		if err != nil {
			return withCode(fmt.Errorf("local copy of the file can not be read: %w", err), 1004)
		}

		// Parse XML File
		err = xml.Unmarshal(content, &xmltv)
		if err != nil {
			return err
		}

		Data.Cache.XMLTV[file] = *xmltv
	} else {
		*xmltv = Data.Cache.XMLTV[file]
	}

	return
}

// Create M3U File
func createM3UFile() {
	showInfo("XEPG:" + fmt.Sprintf("Create M3U file (%s)", System.File.M3U))
	_, err := buildM3U([]string{})
	if err != nil {
		ShowError(err, 000)
	}

	if err := saveMapToJSONFile(System.File.URLS, Data.Cache.StreamingURLS); err != nil {
		ShowError(err, 0)
	}
}

// Clean up the XEPG Database
func cleanupXEPG() {
	var sourceIDs []string

	for source := range Settings.Files.M3U {
		sourceIDs = append(sourceIDs, source)
	}

	for source := range Settings.Files.HDHR {
		sourceIDs = append(sourceIDs, source)
	}

	showInfo("XEPG:" + "Cleanup database")
	Data.XEPG.XEPGCount = 0

	for id, dxc := range Data.XEPG.Channels {
		var xepgChannel XEPGChannelStruct
		err := json.Unmarshal([]byte(mapToJSON(dxc)), &xepgChannel)
		if err != nil {
			continue
		}
		if lo.IndexOf(Data.Cache.Streams.Active, xepgChannel.Name+xepgChannel.FileM3UID) == -1 {
			delete(Data.XEPG.Channels, id)
			continue
		}
		if lo.IndexOf(sourceIDs, xepgChannel.FileM3UID) == -1 {
			delete(Data.XEPG.Channels, id)
			continue
		}
		if xepgChannel.XActive {
			Data.XEPG.XEPGCount++
		}
	}

	err := saveMapToJSONFile(System.File.XEPG, Data.XEPG.Channels)
	if err != nil {
		ShowError(err, 000)
		return
	}

	showInfo("XEPG Channels:" + fmt.Sprintf("%d", Data.XEPG.XEPGCount))

	if len(Data.Streams.Active) > 0 && Data.XEPG.XEPGCount == 0 {
		showWarning(2005)
	}
}

// clearXMLTVCache empties XMLTV cache and runs a garbage collector
func clearXMLTVCache() {
	Data.Cache.XMLTV = make(map[string]XMLTV)
	runtime.GC()
}

// shiftXMLTVTime shifts the time zone offset of an XMLTV time ("20060102150405 +0100") by hours.
func shiftXMLTVTime(t string, hours int) (string, error) {
	parts := strings.Split(t, " ")
	if len(parts) != 2 {
		return "", fmt.Errorf("XMLTV time without time zone: %q", t)
	}
	tz, err := strconv.Atoi(parts[1])
	if err != nil {
		return "", fmt.Errorf("invalid XMLTV time zone: %q", t)
	}
	return fmt.Sprintf("%s %+05d", parts[0], tz+hours*100), nil
}
