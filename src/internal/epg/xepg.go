package epg

import (
	"cmp"
	"crypto/md5"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"maps"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"slices"
	"sort"
	"strconv"
	"strings"
	"sync/atomic"
	"time"
	"xteve/src/internal/config"
	"xteve/src/internal/errcode"
	"xteve/src/internal/logger"

	"github.com/samber/lo"
)

// Create Mapping Menu for the XMLTV Files
func (b *build) createXEPGMapping() {
	b.files = b.m.cfg.XMLTVFiles()
	b.mapping = make(Mapping)

	for i, file := range slices.Backward(b.files) {
		var fileID = strings.TrimSuffix(filepath.Base(file), path.Ext(filepath.Base(file)))
		logger.Info("XEPG:" + "Parse XMLTV file: " + b.m.xmltvName(fileID))

		var channels, err = b.m.xmltvChannels(file)
		if err != nil {
			// Safe while iterating backwards: only already visited files move
			b.files = append(b.files[:i], b.files[i+1:]...)
			errcode.Report(fmt.Errorf("%s: %w", b.m.xmltvName(fileID), err), 0)
			continue
		}
		b.mapping[filepath.Base(file)] = channels
	}

	if len(b.files) == 0 && !b.m.cfg.System.ConfigurationWizard.Load() {
		errcode.Warn(1007)
	}

	// Create selection for the Dummy
	var dummy = make(map[string]MappingChannel)
	for _, minutes := range []string{"30", "60", "90", "120", "180", "240", "360"} {
		var id = minutes + "_Minutes"
		dummy[id] = MappingChannel{ID: id, DisplayNames: []DisplayName{{Value: minutes + " Minutes"}}}
	}
	b.mapping[dummyFile] = dummy
}

// Create / update XEPG Database
func (b *build) createXEPGDatabase() (err error) {
	var allChannelNumbers = make([]float64, 0)
	b.activeStreams = make([]string, 0)
	b.channels, err = loadChannels(b.m.cfg.System.File.XEPG)
	if err != nil {
		return errcode.With(err, 1004)
	}

	var createNewID = func() (xepg string) {
		var firstID = 0

	newXEPGID:

		if _, ok := b.channels["x-ID."+strconv.FormatInt(int64(firstID), 10)]; ok {
			firstID++
			goto newXEPGID
		}

		xepg = "x-ID." + strconv.FormatInt(int64(firstID), 10)
		return
	}

	// getFreeChannelNumber returns the first free number starting from the
	// first valid one of startingChannels, or from the default first channel
	var getFreeChannelNumber = func(startingChannels ...string) (xChannelID string) {
		var firstFreeNumber = config.Get().MappingFirstChannel
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

	logger.Info("XEPG:" + "Update database")

	// Delete Channel with missing Channel Numbers.
	for id, xepgChannel := range b.channels {
		if len(xepgChannel.XChannelID) == 0 {
			delete(b.channels, id)
		}

		if xChannelID, err := strconv.ParseFloat(xepgChannel.XChannelID, 64); err == nil {
			allChannelNumbers = append(allChannelNumbers, xChannelID)
		}
	}

	// Database channels in a fixed order, so that matching does not depend on
	// the iteration order of the map
	var dbChannels = make([]XEPGChannelStruct, 0, len(b.channels))
	for _, channel := range b.channels {
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

	// The rename rules of the channels, compiled once; an invalid rule is reported once and not used
	var renameRx, groupRx = make(map[string]*regexp.Regexp), make(map[string]*regexp.Regexp)
	for _, c := range dbChannels {
		if len(c.UpdateChannelNameRegex) == 0 {
			continue
		}
		var nameRx, err = regexp.Compile(c.UpdateChannelNameRegex)
		if err != nil {
			errcode.Report(fmt.Errorf("%s: %w", c.XName, err), 1018)
			continue
		}
		if len(c.UpdateChannelNameByGroupRegex) > 0 {
			var rx, err = regexp.Compile(c.UpdateChannelNameByGroupRegex)
			if err != nil {
				errcode.Report(fmt.Errorf("%s: %w", c.XName, err), 1018)
				continue
			}
			groupRx[c.XEPG] = rx
		}
		renameRx[c.XEPG] = nameRx
	}

	var m3uChannels = make([]M3UChannelStructXEPG, len(b.streams))
	for i, dsa := range b.streams {
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

		b.activeStreams = append(b.activeStreams, m3uChannel.Name+m3uChannel.FileM3UID)

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

					// A stream renamed by the provider belongs to the channel if it matches the rename rule of the
					// channel (and the group of the channel matches its group rule)
					var nameRx = renameRx[dxc.XEPG]
					if nameRx == nil {
						continue
					}
					// Guard against the situation when both channels have UUIDValue, they are different, but names are
					// the same
					if dxc.Name == m3uChannel.Name {
						continue
					}
					if !nameRx.MatchString(m3uChannel.Name) {
						continue
					}
					if rx := groupRx[dxc.XEPG]; rx != nil && !rx.MatchString(dxc.XGroupTitle) {
						// Found the channel name to update but it has wrong group
						continue
					}
					logger.Info("XEPG:" + fmt.Sprintf("Renaming the channel '%v' to '%v'", dxc.Name, m3uChannel.Name))
					channelExists = true
					// dxc.Name will be assigned later in channelExists switch. A name set in the web UI is kept.
					if dxc.XName == dxc.Name {
						dxc.XName = m3uChannel.Name
					}
					currentXEPGID = dxc.XEPG
					b.channels[currentXEPGID] = dxc
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
			var xepgChannel = b.channels[currentXEPGID]

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

			b.channels[currentXEPGID] = xepgChannel

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

			b.channels[xepg] = newChannel
		}
	}
	logger.Info("XEPG:" + "Save DB file")
	err = saveMapToJSONFile(b.m.cfg.System.File.XEPG, b.channels)
	if err != nil {
		return
	}

	return
}

// findXMLTVChannel looks for the XMLTV channel of a stream in b.mapping: first by tvg-id in all files,
// then by comparing the stream name with every display name, ignoring case and spaces. Files are searched in name
// order, the dummy last, so the result does not depend on the iteration order of the map.
func (b *build) findXMLTVChannel(tvgID, name string) (file string, channel MappingChannel, ok bool) {
	var files = slices.Sorted(maps.Keys(b.mapping))
	// The dummy last
	slices.SortStableFunc(files, func(x, y string) int {
		switch {
		case x == dummyFile && y != dummyFile:
			return 1
		case x != dummyFile && y == dummyFile:
			return -1
		}
		return 0
	})

	if len(tvgID) > 0 {
		for _, f := range files {
			if c, found := b.mapping[f][tvgID]; found {
				return f, c, true
			}
		}
	}

	var nameSolid = strings.ReplaceAll(name, " ", "")

	for _, f := range files {
		var channels = b.mapping[f]
		for _, id := range slices.Sorted(maps.Keys(channels)) {
			for _, n := range channels[id].DisplayNames {
				if strings.EqualFold(strings.ReplaceAll(n.Value, " ", ""), nameSolid) {
					return f, channels[id], true
				}
			}
		}
	}

	return "", MappingChannel{}, false
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
func (b *build) mapChannels() (err error) {
	logger.Info("XEPG:" + "Map channels")

	for xepg, xepgChannel := range b.channels {
		// Automatic mapping for new Channels. Is only executed if the Channel is deactivated and no XMLTV file and no
		// XMLTV Channel is assigned.
		if !xepgChannel.XActive {
			// Values can be "-", therefore len <= 1
			// If either XmltvFile (XMLTV file / EPG source) or XMapping (XMLTV Channel / EPG program) is "-" or null,
			// then look for a matching EPG program.
			if len(xepgChannel.XmltvFile) <= 1 || len(xepgChannel.XMapping) <= 1 {
				var tvgID = xepgChannel.TvgID

				// Set default for new Channel
				if config.Get().DefaultMissingEPG != "-" {
					xepgChannel.XmltvFile = dummyFile
					xepgChannel.XMapping = config.Get().DefaultMissingEPG
				} else {
					xepgChannel.XmltvFile = "-"
					xepgChannel.XMapping = "-"
				}

				b.channels[xepg] = xepgChannel

				if file, channel, ok := b.findXMLTVChannel(tvgID, xepgChannel.Name); ok {
					xepgChannel.XmltvFile = file
					xepgChannel.XMapping = channel.ID

					// If there is a Logo in the XMLTV file, this will be used. If not, then the Logo from the M3U
					// file
					if len(channel.Icon) > 0 {
						xepgChannel.TvgLogo = channel.Icon
					}

					b.channels[xepg] = xepgChannel
				}
			}
		}

		if config.Get().EnableMappedChannels && (xepgChannel.XmltvFile != "-" || xepgChannel.XMapping != "-") {
			xepgChannel.XActive = true
		}

		// Check whether the assigned XMLTV Files and Channels still exist.
		if xepgChannel.XActive {
			var mapping = xepgChannel.XMapping
			var file = xepgChannel.XmltvFile

			if file != dummyFile {
				if channels, ok := b.mapping[file]; ok {
					if channel, ok := channels[mapping]; ok {
						// Update Channel Logo
						if xepgChannel.XUpdateChannelIcon && len(channel.Icon) > 0 {
							xepgChannel.TvgLogo = channel.Icon
						}
					} else {
						errcode.Report(fmt.Errorf("missing EPG data: %s", xepgChannel.Name), 0)
						errcode.Warn(2302)
						xepgChannel.XActive = false
					}
				} else {
					var fileID = strings.TrimSuffix(filepath.Base(file), path.Ext(filepath.Base(file)))

					errcode.Report(fmt.Errorf("missing XMLTV file: %s", b.m.xmltvName(fileID)), 0)
					errcode.Warn(2301)
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

			b.channels[xepg] = xepgChannel
		}
	}

	err = saveMapToJSONFile(b.m.cfg.System.File.XEPG, b.channels)
	if err != nil {
		return
	}

	return
}

// createXMLTVFile writes xteve.xml and xteve.xml.gz: the active channels, then their programmes. Each XMLTV file is
// read once, element by element, and each programme of an active channel is written at once, so that neither the
// XMLTV files nor xteve.xml are held in memory.
func (b *build) createXMLTVFile() (err error) {
	if len(b.files) == 0 && len(b.streams) == 0 {
		return
	}

	var system = b.m.cfg.System
	logger.Info("XEPG:" + fmt.Sprintf("Create XMLTV file (%s)", system.File.XML))

	// The active channels by channel number, and which of them get the programmes of which XMLTV channel
	var active []XEPGChannel
	for _, c := range b.channels {
		if c.XActive {
			active = append(active, c)
		}
	}
	slices.SortFunc(active, func(x, y XEPGChannel) int {
		return cmp.Or(compareChannelNumbers(x.XChannelID, y.XChannelID), cmp.Compare(x.XEPG, y.XEPG))
	})

	var wanted = make(map[string]map[string][]*programmeTarget) // file -> XMLTV channel ID -> channels
	var dummies []XEPGChannel
	for _, c := range active {
		if c.XmltvFile == dummyFile {
			dummies = append(dummies, c)
			continue
		}
		var file = system.Folder.Data + c.XmltvFile
		if wanted[file] == nil {
			wanted[file] = make(map[string][]*programmeTarget)
		}
		wanted[file][c.XMapping] = append(wanted[file][c.XMapping], newProgrammeTarget(c))
	}

	w, err := newXMLTVWriter(system.File.XML, system.Compressed.GZxml, system.Name,
		fmt.Sprintf("%s - %s", system.Name, system.Version))
	if err != nil {
		return err
	}

	for _, c := range active {
		var channel = Channel{ID: c.XChannelID, DisplayNames: []DisplayName{{Value: c.XName}}}
		if len(c.TvgLogo) > 0 {
			channel.Icon = &Icon{Src: ImageURL(b.images, c.TvgLogo)}
		}
		if err = w.channel(&channel); err != nil {
			w.abort()
			return err
		}
	}

	// The dummy programmes go through programFor like the others, which sets the channel number
	for _, c := range dummies {
		var t = newProgrammeTarget(c)
		for _, p := range b.createDummyProgram(c).Program {
			if err = w.programme(b.programFor(t, p)); err != nil {
				w.abort()
				return err
			}
		}
	}

	// A channel without program data is still listed; failures are reported once at the end
	var failedChannels int
	var firstChannelErr error

	for _, file := range slices.Sorted(maps.Keys(wanted)) {
		var targets = wanted[file]

		// Only the programmes of the active channels are decoded; a programme that can not be decoded is skipped
		var skipped int
		var readErr = func() error {
			var f, err = os.Open(filepath.Clean(file))
			if err != nil {
				return err
			}
			defer func() { _ = f.Close() }()

			return scanXMLTV(f, func(name, channel string, element []byte) error {
				if name != "programme" || len(targets[channel]) == 0 {
					return nil
				}
				var p Program
				if err := decodeElement(element, &p); err != nil {
					skipped++
					return nil
				}
				normalizeProgramme(&p)
				for _, t := range targets[channel] {
					if err := w.programme(b.programFor(t, &p)); err != nil {
						return &writeError{err}
					}
				}
				return nil
			})
		}()
		if skipped > 0 {
			errcode.Report(fmt.Errorf("%s: %d programmes could not be read and are left out", filepath.Base(file),
				skipped), 0)
		}

		if writeErr, ok := errors.AsType[*writeError](readErr); ok {
			w.abort()
			return writeErr.err
		}
		if readErr != nil {
			for _, channels := range targets {
				failedChannels += len(channels)
			}
			if firstChannelErr == nil {
				firstChannelErr = fmt.Errorf("%s: %w", filepath.Base(file), readErr)
			}
		}
	}

	if failedChannels > 0 {
		errcode.Report(
			fmt.Errorf("XMLTV: no program data for %d channels, first error: %w", failedChannels, firstChannelErr),
			0,
		)
	}

	logger.Info("XEPG:" + fmt.Sprintf("Compress XMLTV file (%s)", system.Compressed.GZxml))
	return w.close()
}

// writeError is an error of writing xteve.xml, as opposed to an error of reading an XMLTV file.
type writeError struct{ err error }

func (e *writeError) Error() string { return e.err.Error() }

// compareChannelNumbers orders channel numbers numerically, other ones after them in text order.
func compareChannelNumbers(a, b string) int {
	var x, errX = strconv.ParseFloat(a, 64)
	var y, errY = strconv.ParseFloat(b, 64)
	switch {
	case errX == nil && errY == nil:
		return cmp.Compare(x, y)
	case errX == nil:
		return -1
	case errY == nil:
		return 1
	}
	return cmp.Compare(a, b)
}

// programmeTarget is an active channel that gets the programmes of an XMLTV channel.
type programmeTarget struct {
	channel XEPGChannel
	// timeshift in hours, from the channel settings or the "tvg-shift" M3U attribute
	timeshift         int
	timeshiftReported atomic.Bool
}

func newProgrammeTarget(c XEPGChannel) *programmeTarget {
	var t = &programmeTarget{channel: c}
	if len(c.XTimeshift) > 0 {
		var err error
		if t.timeshift, err = strconv.Atoi(c.XTimeshift); err != nil {
			errcode.Report(
				fmt.Errorf("channel %s: timeshift %q is not a whole number of hours and is ignored", c.XName,
					c.XTimeshift),
				0,
			)
			t.timeshift = 0
		}
	}
	return t
}

// programFor returns the programme of an XMLTV file as the channel shows it. Everything of the source programme is
// kept, the channel, the times (timeshift), the categories, the images, the episode numbers and the video quality
// are adjusted.
func (b *build) programFor(t *programmeTarget, xmltvProgram *Program) *Program {
	var xepgChannel = t.channel
	var program = *xmltvProgram

	// Channel ID
	program.Channel = xepgChannel.XChannelID

	if t.timeshift != 0 {
		start, startErr := shiftXMLTVTime(xmltvProgram.Start, t.timeshift)
		stop, stopErr := shiftXMLTVTime(xmltvProgram.Stop, t.timeshift)
		if shiftErr := errors.Join(startErr, stopErr); shiftErr != nil {
			// The program keeps its original time; report the problem once per channel
			if t.timeshiftReported.CompareAndSwap(false, true) {
				errcode.Report(fmt.Errorf("channel %s: timeshift is not applied: %w", xepgChannel.XName, shiftErr), 0)
			}
		} else {
			program.Start, program.Stop = start, stop
		}
	}

	b.getCategory(&program, xmltvProgram, xepgChannel)
	b.getPoster(&program, xmltvProgram, xepgChannel)
	b.getEpisodeNum(&program, xmltvProgram, xepgChannel)
	b.getVideo(&program, xmltvProgram, xepgChannel)

	return &program
}

// Create Dummy Data (createXMLTVFile)
func (b *build) createDummyProgram(xepgChannel XEPGChannelStruct) (dummyXMLTV XMLTV) {
	var imgc = b.images
	var currentTime = time.Now()
	var dateArray = strings.Fields(currentTime.String())
	var offset = " " + dateArray[2]
	var startTime = time.Date(currentTime.Year(), currentTime.Month(), currentTime.Day(), 0, 0, 0, 0, time.UTC)

	logger.Info("Create Dummy Guide:" + "Time offset" + offset + " - " + xepgChannel.XName)

	var dl = strings.Split(xepgChannel.XMapping, "_")
	dummyLength, err := strconv.Atoi(dl[0])
	if err != nil {
		errcode.Report(err, 000)
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

			if config.Get().XepgReplaceMissingImages && len(xepgChannel.TvgLogo) > 0 {
				poster.Src = ImageURL(imgc, xepgChannel.TvgLogo)
				epg.Poster = append(epg.Poster, poster)
			}

			if xepgChannel.XCategory != "Movie" {
				epg.EpisodeNum = append(
					epg.EpisodeNum,
					&EpisodeNum{Value: epgStartTime.Format("2006-01-02 15:04:05"), System: "original-air-date"},
				)
			}

			epg.New = &New{}

			dummyXMLTV.Program = append(dummyXMLTV.Program, &epg)
			epgStartTime = epgStopTime
		}
	}

	return
}

// Expand Categories (createXMLTVFile)
func (b *build) getCategory(program *Program, xmltvProgram *Program, xepgChannel XEPGChannelStruct) {
	program.Category = nil
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

// getPoster sets the images of a programme, through the image cache. A programme without images gets the channel
// logo if XepgReplaceMissingImages is set.
func (b *build) getPoster(program *Program, xmltvProgram *Program, xepgChannel XEPGChannelStruct) {
	program.Poster = nil
	for _, poster := range xmltvProgram.Poster {
		if len(poster.Src) == 0 {
			continue
		}
		poster.Src = ImageURL(b.images, poster.Src)
		program.Poster = append(program.Poster, poster)
	}

	if len(program.Poster) == 0 && config.Get().XepgReplaceMissingImages && len(xepgChannel.TvgLogo) > 0 {
		program.Poster = append(program.Poster, Poster{Src: ImageURL(b.images, xepgChannel.TvgLogo)})
	}
}

// Apply Episode system, if none is available and a Category has been set in the mapping, an Episode is created
func (b *build) getEpisodeNum(program *Program, xmltvProgram *Program, xepgChannel XEPGChannelStruct) {
	program.EpisodeNum = slices.Clone(xmltvProgram.EpisodeNum)

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
				errcode.Report(err, 0)
			}
		}
	}
}

// Create Video Parameters (createXMLTVFile)
func (b *build) getVideo(program *Program, xmltvProgram *Program, xepgChannel XEPGChannelStruct) {
	var video Video
	if xmltvProgram.Video != nil {
		video = *xmltvProgram.Video
	}

	if len(video.Quality) == 0 {
		var name = strings.ToUpper(xepgChannel.XName)
		if strings.Contains(name, " HD") || strings.Contains(name, " FHD") {
			video.Quality = "HDTV"
		}
		if strings.Contains(name, " UHD") || strings.Contains(name, " 4K") {
			video.Quality = "UHDTV"
		}
	}

	program.Video = nil
	if video != (Video{}) {
		program.Video = &video
	}
}

// Clean up the XEPG Database
func (b *build) cleanupXEPG() {
	var settings = config.Get()
	var sources = make(map[string]bool, len(settings.Files.M3U)+len(settings.Files.HDHR))
	for source := range settings.Files.M3U {
		sources[source] = true
	}
	for source := range settings.Files.HDHR {
		sources[source] = true
	}

	var active = make(map[string]bool, len(b.activeStreams))
	for _, s := range b.activeStreams {
		active[s] = true
	}

	logger.Info("XEPG:" + "Cleanup database")
	b.count = 0

	for id, xepgChannel := range b.channels {
		if !active[xepgChannel.Name+xepgChannel.FileM3UID] {
			delete(b.channels, id)
			continue
		}
		if !sources[xepgChannel.FileM3UID] {
			delete(b.channels, id)
			continue
		}
		if xepgChannel.XActive {
			b.count++
		}
	}

	err := saveMapToJSONFile(b.m.cfg.System.File.XEPG, b.channels)
	if err != nil {
		errcode.Report(err, 000)
		return
	}

	logger.Info("XEPG Channels:" + fmt.Sprintf("%d", b.count))

	if len(b.streams) > 0 && b.count == 0 {
		errcode.Warn(2005)
	}
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
