package src

import (
	"encoding/json"
	"errors"
	"fmt"
	"maps"
	"os"
	"path"
	"sort"
	"strconv"
	"strings"
	"time"

	"xteve/src/internal/authentication"
	"xteve/src/internal/config"
	"xteve/src/internal/imgcache"
)

// Change Settings (WebUI)
func updateServerSettings(request RequestStruct) (settings SettingsStruct, err error) {
	var oldSettings = jsonToMap(mapToJSON(config.Get()))
	var newSettings = jsonToMap(mapToJSON(request.Settings))
	// The keys that change, applied to the current settings below
	var changed = make(map[string]any)
	var reloadData = false
	var cacheImages = false
	var createXEPGFiles = false
	var debug string

	for key, value := range newSettings {
		if _, ok := oldSettings[key]; ok {
			switch key {
			case "tuner":
				showWarning(2105)

			case "epgSource":
				reloadData = true

			case "update":
				// Remove spaces from the Values and check the formatting of the Time (0000 - 2359)
				var newUpdateTimes = make([]string, 0)

				for _, v := range value.([]any) {
					v = strings.ReplaceAll(v.(string), " ", "")

					if _, err := time.Parse("1504", v.(string)); err != nil {
						return *config.Get(), withCode(err, 1012)
					}

					newUpdateTimes = append(newUpdateTimes, v.(string))
				}

				value = newUpdateTimes

			case "cache.images":
				cacheImages = true

			case "xepg.replace.missing.images":
				createXEPGFiles = true

			case "backup.path":
				value = strings.TrimRight(value.(string), string(os.PathSeparator)) + string(os.PathSeparator)
				err = checkFolder(value.(string))
				if err == nil {
					err = checkFilePermission(value.(string))
					if err != nil {
						return
					}
				}

				if err != nil {
					return
				}

			case "temp.path":
				value = getValidTempDir(value.(string))

			case "ffmpeg.path", "vlc.path":
				var path = value.(string)
				if len(path) > 0 {
					err = checkFile(path)
					if err != nil {
						return
					}
				}

			case "scheme.m3u", "scheme.xml":
				createXEPGFiles = true

			case "defaultMissingEPG":
				// If DefaultMissingEPG was set, rebuild DVR and XEPG database
				if newSettings["defaultMissingEPG"] != "-" && oldSettings["defaultMissingEPG"] == "-" {
					reloadData = true
				}

			case "enableMappedChannels":
				// If EnableMappedChannels was turned on, rebuild DVR and XEPG database
				if newSettings["enableMappedChannels"] == true && oldSettings["enableMappedChannels"] == false {
					reloadData = true
				}
			}

			changed[key] = value

			switch fmt.Sprintf("%T", value) {
			case "bool":
				debug = fmt.Sprintf("Save Setting:Key: %s | Value: %t (%T)", key, value, value)

			case "string":
				debug = fmt.Sprintf("Save Setting:Key: %s | Value: %s (%T)", key, value, value)

			case "[]interface {}":
				debug = fmt.Sprintf("Save Setting:Key: %s | Value: %v (%T)", key, value, value)

			case "float64":
				debug = fmt.Sprintf("Save Setting:Key: %s | Value: %d (%T)", key, int(value.(float64)), value)

			default:
				debug = fmt.Sprintf("%T", value)
			}

			showDebug(debug, 1)
		}
	}

	// Update Settings
	err = config.Update(func(s *SettingsStruct) error {
		if err := json.Unmarshal([]byte(mapToJSON(changed)), s); err != nil {
			return err
		}

		if !s.AuthenticationWEB {
			s.AuthenticationAPI = false
			s.AuthenticationM3U = false
			s.AuthenticationPMS = false
			s.AuthenticationWEB = false
			s.AuthenticationXML = false
		}

		// Check Buffer Settings
		if len(s.FFmpegOptions) == 0 {
			s.FFmpegOptions = System.FFmpeg.DefaultOptions
		}

		if len(s.VLCOptions) == 0 {
			s.VLCOptions = System.VLC.DefaultOptions
		}

		switch s.Buffer {
		case "ffmpeg":
			if len(s.FFmpegPath) == 0 {
				return errors.New(getErrMsg(2020))
			}

		case "vlc":
			if len(s.VLCPath) == 0 {
				return errors.New(getErrMsg(2021))
			}
		}

		return nil
	})
	if err == nil {
		settings = *config.Get()

		if reloadData {
			err = buildDatabaseDVR()
			if err != nil {
				return
			}

			buildXEPG(false)
		}

		if cacheImages {
			if config.Get().EpgSource == "XEPG" && !System.ImageCachingInProgress.Load() {
				Data.Cache.Images, err = imgcache.New(
					System.Folder.ImagesCache,
					fmt.Sprintf("%s://%s/images/", System.Protocol(), System.Domain()),
					config.Get().CacheImages,
				)
				if err != nil {
					ShowError(err, 0)
				}

				switch config.Get().CacheImages {
				case false:
					if err := createXMLTVFile(); err != nil {
						ShowError(err, 0)
					}
					createM3UFile()

				case true:
					go func() {
						if err := createXMLTVFile(); err != nil {
							ShowError(err, 0)
						}
						createM3UFile()

						System.ImageCachingInProgress.Store(true)
						showInfo("Image Caching:Images are cached")

						if err := Data.Cache.Images.Image.Caching(); err != nil {
							ShowError(err, 0)
						}
						showInfo("Image Caching:Done")

						System.ImageCachingInProgress.Store(false)

						buildXEPG(false)
					}()
				}
			}
		}

		if createXEPGFiles {
			go func() {
				if err := createXMLTVFile(); err != nil {
					ShowError(err, 0)
				}
				createM3UFile()
			}()
		}
	}

	return
}

// Save Provider Data (WebUI)
func saveFiles(request RequestStruct, fileType string) (err error) {
	var newData = make(map[string]any)
	var indicator string

	switch fileType {
	case "m3u":
		newData = request.Files.M3U
		indicator = "M"

	case "hdhr":
		newData = request.Files.HDHR
		indicator = "H"

	case "xmltv":
		newData = request.Files.XMLTV
		indicator = "X"
	}

	for dataID, d := range newData {
		var data, ok = d.(map[string]any)
		if !ok {
			return fmt.Errorf("invalid provider data: %v", d)
		}

		var reloadData = false
		var isNew = dataID == "-"
		if isNew {
			dataID = indicator + randomString(19)
		}

		err = config.Update(func(s *SettingsStruct) error {
			var files = providerFiles(s, fileType)

			if isNew {
				// New Provider File, getProviderData downloads it and removes "new"
				var entry = maps.Clone(data)
				entry["new"] = true
				files[dataID] = entry
				return nil
			}

			// Existing Provider File
			var entry, ok = files[dataID].(map[string]any)
			if !ok {
				return fmt.Errorf("unknown provider: %s", dataID)
			}
			maps.Copy(entry, data)
			return nil
		})
		if err != nil {
			return
		}

		if isNew {
			reloadData = true
			if err = getProviderData(fileType, dataID); err != nil {
				removeProviderFile(fileType, dataID)
				return
			}
		}

		if _, ok := data["delete"]; ok {
			deleteLocalProviderFiles(dataID, fileType)
			reloadData = true
		}

		if reloadData {
			err = buildDatabaseDVR()
			if err != nil {
				return err
			}

			buildXEPG(false)
		}
	}

	return
}

// removeProviderFile removes a provider from the settings.
func removeProviderFile(fileType, dataID string) {
	var err = config.Update(func(s *SettingsStruct) error {
		delete(providerFiles(s, fileType), dataID)
		return nil
	})
	if err != nil {
		ShowError(err, 0)
	}
}

// Update Provider Data manually (WebUI)
func updateFile(request RequestStruct, fileType string) (err error) {
	var updateData = make(map[string]any)

	switch fileType {
	case "m3u":
		updateData = request.Files.M3U

	case "hdhr":
		updateData = request.Files.HDHR

	case "xmltv":
		updateData = request.Files.XMLTV
	}

	for dataID := range updateData {
		if err = getProviderData(fileType, dataID); err != nil {
			return
		}

		if err = buildDatabaseDVR(); err != nil {
			return
		}
		buildXEPG(false)
	}

	return
}

// Delete Provider Data (WebUI)
func deleteLocalProviderFiles(dataID, fileType string) {
	var fileExtension string

	switch fileType {
	case "m3u":
		fileExtension = ".m3u"

	case "hdhr":
		fileExtension = ".json"

	case "xmltv":
		fileExtension = ".xml"
	}

	if _, ok := providerFilesOf(config.Get(), fileType)[dataID]; ok {
		removeProviderFile(fileType, dataID)
		removeAllOrLog(System.Folder.Data + dataID + fileExtension)
	}
}

// Save Filter Settings (WebUI)
func saveFilter(request RequestStruct) (settings SettingsStruct, err error) {
	var filterMap map[int64]any
	var newData map[int64]any
	var defaultFilter FilterStruct

	defaultFilter.Active = true
	defaultFilter.CaseSensitive = false
	defaultFilter.PreserveMapping = true
	defaultFilter.StartingChannel = strconv.FormatFloat(config.Get().MappingFirstChannel, 'f', -1, 64) // 1000

	newData = request.Filter

	err = config.Update(func(s *SettingsStruct) error {
		if s.Filter == nil {
			s.Filter = make(map[int64]any)
		}
		filterMap = s.Filter

		var createNewID = func() (id int64) {
			for {
				if _, ok := filterMap[id]; !ok {
					return id
				}
				id++
			}
		}

		for dataID, data := range newData {
			if dataID == -1 {
				// New Filter
				dataID = createNewID()
				filterMap[dataID] = jsonToMap(mapToJSON(defaultFilter))
			}

			// Clear Filters
			if _, ok := data.(map[string]any)["delete"]; ok {
				delete(filterMap, dataID)
				continue
			}

			// Update filters, nothing is saved if one of them is invalid
			if filter, ok := data.(map[string]any)["filter"].(string); ok && len(filter) == 0 {
				return errors.New(getErrMsg(1014))
			}

			if oldData, ok := filterMap[dataID].(map[string]any); ok {
				maps.Copy(oldData, data.(map[string]any))
			}
		}

		return nil
	})
	if err != nil {
		return
	}

	settings = *config.Get()

	err = buildDatabaseDVR()
	if err != nil {
		return
	}

	buildXEPG(false)

	return
}

// Save XEPG Mapping
func saveXEpgMapping(request RequestStruct) (err error) {
	var tmp = Data.XEPG

	Data.Cache.Images, err = imgcache.New(
		System.Folder.ImagesCache,
		fmt.Sprintf("%s://%s/images/", System.Protocol(), System.Domain()),
		config.Get().CacheImages,
	)
	if err != nil {
		ShowError(err, 0)
	}

	err = json.Unmarshal([]byte(mapToJSON(request.EpgMapping)), &tmp)
	if err != nil {
		return
	}

	err = saveMapToJSONFile(System.File.XEPG, request.EpgMapping)
	if err != nil {
		return err
	}

	Data.XEPG.Channels = request.EpgMapping

	if System.ScanInProgress.CompareAndSwap(false, true) {
		cleanupXEPG()
		System.ScanInProgress.Store(false)
		buildXEPG(true)
	} else {
		// If the Mapping is saved again while the Database is being created, the Database will not be updated again
		// until later.
		go func() {
			// Only one waiting update
			if !System.BackgroundProcess.CompareAndSwap(false, true) {
				return
			}
			defer System.BackgroundProcess.Store(false)

			for !System.ScanInProgress.CompareAndSwap(false, true) {
				time.Sleep(time.Duration(1) * time.Second)
			}

			cleanupXEPG()
			System.ScanInProgress.Store(false)
			buildXEPG(false)
			showInfo("XEPG:" + "Ready to use")
		}()
	}

	return
}

// Save User Data (WebUI)
func saveUserData(request RequestStruct) (err error) {
	var userData = request.UserData

	var newCredentials = func(userID string, newUserData map[string]any) (err error) {
		var newUsername, newPassword string
		if username, ok := newUserData["username"].(string); ok {
			newUsername = username
		}

		if password, ok := newUserData["password"].(string); ok {
			newPassword = password
		}

		if len(newUsername) > 0 {
			err = authentication.ChangeCredentials(userID, newUsername, newPassword)
		}

		return
	}

	for userID, newUserData := range userData {
		err = newCredentials(userID, newUserData.(map[string]any))
		if err != nil {
			return
		}

		if request.DeleteUser {
			err = authentication.RemoveUser(userID)
			return
		}

		delete(newUserData.(map[string]any), "password")
		delete(newUserData.(map[string]any), "confirm")

		if _, ok := newUserData.(map[string]any)["delete"]; ok {
			if err = authentication.RemoveUser(userID); err != nil {
				return
			}
		} else {
			err = authentication.WriteUserData(userID, newUserData.(map[string]any))
			if err != nil {
				return
			}
		}
	}

	return
}

// Create New User (WebUI)
func saveNewUser(request RequestStruct) (err error) {
	var data = request.UserData
	var username = data["username"].(string)
	var password = data["password"].(string)

	delete(data, "password")
	delete(data, "confirm")

	userID, err := authentication.CreateNewUser(username, password)
	if err != nil {
		return
	}

	err = authentication.WriteUserData(userID, data)
	return
}

// Wizard (WebUI)
func saveWizard(request RequestStruct) (nextStep int, err error) {
	var wizard = jsonToMap(mapToJSON(request.Wizard))

	for key, value := range wizard {
		switch key {
		case "tuner":
			nextStep = 1
			err = config.Update(func(s *SettingsStruct) error {
				s.Tuner = int(value.(float64))
				return nil
			})

		case "epgSource":
			nextStep = 2
			err = config.Update(func(s *SettingsStruct) error {
				s.EpgSource = value.(string)
				return nil
			})

		case "m3u", "xmltv":
			var data = make(map[string]any)
			var indicator string

			data["type"] = key
			data["new"] = true
			data["file.source"] = value.(string)

			switch key {
			case "m3u":
				data["name"] = "M3U"
				indicator = "M"

			case "xmltv":
				data["name"] = "XMLTV"
				indicator = "X"
			}

			var dataID = indicator + randomString(19)

			err = config.Update(func(s *SettingsStruct) error {
				providerFiles(s, key)[dataID] = data
				return nil
			})
			if err != nil {
				return
			}

			err = getProviderData(key, dataID)
			if err != nil {
				removeProviderFile(key, dataID)
				return
			}

			switch key {
			case "m3u":
				nextStep = 3

				err = buildDatabaseDVR()
				if err != nil {
					removeProviderFile(key, dataID)
					return
				}

				if config.Get().EpgSource == "PMS" {
					nextStep = 10
				}

			case "xmltv":
				nextStep = 10

				buildXEPG(false)
				System.ScanInProgress.Store(false)
			}
		}

		if err != nil {
			return
		}
	}

	return
}

// Create Filter Rules
func createFilterRules() (err error) {
	Data.Filter = nil

	for _, f := range config.Get().Filter {
		var dataFilter Filter
		var filter FilterStruct

		var exclude, include string

		err = json.Unmarshal([]byte(mapToJSON(f)), &filter)
		if err != nil {
			return
		}

		switch filter.Type {
		case "custom-filter":
			dataFilter.CaseSensitive = filter.CaseSensitive
			dataFilter.Rule = filter.Filter
			dataFilter.Type = filter.Type

			Data.Filter = append(Data.Filter, dataFilter)

		case "group-title":
			if len(filter.Include) > 0 {
				include = fmt.Sprintf(" {%s}", filter.Include)
			}

			if len(filter.Exclude) > 0 {
				exclude = fmt.Sprintf(" !{%s}", filter.Exclude)
			}

			dataFilter.CaseSensitive = filter.CaseSensitive
			dataFilter.PreserveMapping = filter.PreserveMapping
			dataFilter.StartingChannel = filter.StartingChannel
			dataFilter.Rule = fmt.Sprintf("%s%s%s", filter.Filter, include, exclude)
			dataFilter.Type = filter.Type

			Data.Filter = append(Data.Filter, dataFilter)
		}
	}

	return
}

// Create a Database for the DVR System
func buildDatabaseDVR() (err error) {
	System.ScanInProgress.Store(true)

	Data.Streams.All = make([]any, 0)
	Data.Streams.Active = make([]any, 0)
	Data.Streams.Inactive = make([]any, 0)
	Data.Playlist.M3U.Groups.Text = []string{}
	Data.Playlist.M3U.Groups.Value = []string{}
	Data.StreamPreviewUI.Active = []string{}
	Data.StreamPreviewUI.Inactive = []string{}

	var availableFileTypes = []string{"m3u", "hdhr"}

	var urlValuesMap = make(map[string]string)
	var tmpGroupsM3U = make(map[string]int64)

	err = createFilterRules()
	if err != nil {
		return
	}

	for _, fileType := range availableFileTypes {
		var playlistFile = getLocalProviderFiles(fileType)

		for _, i := range playlistFile {
			var channels []any
			var groupTitle, tvgID, uuid = 0, 0, 0
			var keys = []string{"group-title", "tvg-id", "uuid"}
			var compatibility = make(map[string]int)

			var id = strings.TrimSuffix(getFilenameFromPath(i), path.Ext(getFilenameFromPath(i)))
			var playlistName = getProviderParameter(id, fileType, "name")

			channels, parseErr := parsePlaylist(i, fileType)
			if parseErr != nil {
				// The playlist is skipped, the others are still used
				ShowError(fmt.Errorf("%s: %w", playlistName, parseErr), 1005)
				continue
			}

			// Analyze Streams
			for _, stream := range channels {
				var s = stream.(map[string]string)
				s["_file.m3u.path"] = i
				s["_file.m3u.name"] = playlistName
				s["_file.m3u.id"] = id

				if config.Get().DisallowURLDuplicates {
					if _, haveURL := urlValuesMap[s["url"]]; haveURL {
						showInfo(
							"Streams:" + fmt.Sprintf(
								"Found duplicated URL %v, ignoring the channel %v",
								s["url"],
								s["name"],
							),
						)
						continue
					} else {
						urlValuesMap[s["url"]] = s["_values"]
					}
				}

				// Calculate Compatibility
				for _, key := range keys {
					switch key {
					case "uuid":
						if value, ok := s["_uuid.key"]; ok {
							if len(value) > 0 {
								uuid++
							}
						}

					case "group-title":
						if value, ok := s[key]; ok {
							if len(value) > 0 {
								tmpGroupsM3U[value]++

								groupTitle++
							}
						}

					case "tvg-id":
						if value, ok := s[key]; ok {
							if len(value) > 0 {
								tvgID++
							}
						}
					}
				}

				Data.Streams.All = append(Data.Streams.All, stream)

				// New Filter from Version 1.3.0
				var preview string
				var status = filterThisStream(stream)

				if name, ok := s["name"]; ok {
					var group string

					if v, ok := s["group-title"]; ok {
						group = v
					}

					preview = fmt.Sprintf("%s [%s]", name, group)
				}

				switch status {
				case true:
					Data.StreamPreviewUI.Active = append(Data.StreamPreviewUI.Active, preview)
					Data.Streams.Active = append(Data.Streams.Active, stream)

				case false:
					Data.StreamPreviewUI.Inactive = append(Data.StreamPreviewUI.Inactive, preview)
					Data.Streams.Inactive = append(Data.Streams.Inactive, stream)
				}
			}

			if tvgID == 0 {
				compatibility["tvg.id"] = 0
			} else {
				compatibility["tvg.id"] = int(tvgID * 100 / len(channels))
			}

			if groupTitle == 0 {
				compatibility["group.title"] = 0
			} else {
				compatibility["group.title"] = int(groupTitle * 100 / len(channels))
			}

			if uuid == 0 {
				compatibility["stream.id"] = 0
			} else {
				compatibility["stream.id"] = int(uuid * 100 / len(channels))
			}

			compatibility["streams"] = len(channels)

			setProviderCompatibility(id, fileType, compatibility)
		}
	}

	for group, count := range tmpGroupsM3U {
		var text = fmt.Sprintf("%s (%d)", group, count)
		var value = group
		Data.Playlist.M3U.Groups.Text = append(Data.Playlist.M3U.Groups.Text, text)
		Data.Playlist.M3U.Groups.Value = append(Data.Playlist.M3U.Groups.Value, value)
	}

	sort.Strings(Data.Playlist.M3U.Groups.Text)
	sort.Strings(Data.Playlist.M3U.Groups.Value)

	if len(Data.Streams.Active) == 0 && len(config.Get().Filter) == 0 {
		Data.Streams.Active = Data.Streams.All
		Data.Streams.Inactive = make([]any, 0)

		Data.StreamPreviewUI.Active = Data.StreamPreviewUI.Inactive
		Data.StreamPreviewUI.Inactive = []string{}
	}

	if len(Data.Streams.Active) > System.PlexChannelLimit {
		showWarning(2000)
	}

	System.ScanInProgress.Store(false)
	showInfo(fmt.Sprintf("All streams:%d", len(Data.Streams.All)))
	showInfo(fmt.Sprintf("Active streams:%d", len(Data.Streams.Active)))
	showInfo(fmt.Sprintf("Filter:%d", len(Data.Filter)))

	sort.Strings(Data.StreamPreviewUI.Active)
	sort.Strings(Data.StreamPreviewUI.Inactive)

	return
}

// Load Storage Location of all local Provider Files, always for one File Type (M3U, XMLTV etc.)
func getLocalProviderFiles(fileType string) (localFiles []string) {
	var fileExtension string

	switch fileType {
	case "m3u":
		fileExtension = ".m3u"

	case "hdhr":
		fileExtension = ".json"

	case "xmltv":
		fileExtension = ".xml"
	}

	for dataID := range providerFilesOf(config.Get(), fileType) {
		localFiles = append(localFiles, System.Folder.Data+dataID+fileExtension)
	}

	return
}

// Output Provider Parameters based on the Key
func getProviderParameter(id, fileType, key string) (s string) {
	if data, ok := providerFilesOf(config.Get(), fileType)[id].(map[string]any); ok {
		switch v := data[key].(type) {
		case string:
			s = v
		case float64:
			s = strconv.Itoa(int(v))
		case int:
			s = strconv.Itoa(v)
		}
	}

	return
}

// Update Provider Statistics Compatibility
func setProviderCompatibility(id, fileType string, compatibility map[string]int) {
	var err = config.Update(func(s *SettingsStruct) error {
		if data, ok := providerFiles(s, fileType)[id].(map[string]any); ok {
			data["compatibility"] = compatibility
		}
		return nil
	})
	if err != nil {
		ShowError(err, 0)
	}
}
