package src

import (
	"encoding/json"
	"errors"
	"fmt"
	"maps"
	"os"
	"path"
	"slices"
	"strconv"
	"strings"
	"time"

	"xteve/src/internal/authentication"
	"xteve/src/internal/config"
	"xteve/src/internal/playlist"
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

		// A rebuild uses the new image setting and caches the images if they are cached now
		if cacheImages && !reloadData && config.Get().EpgSource == "XEPG" {
			go xepgDB.Rebuild()
		}

		if createXEPGFiles {
			go xepgDB.WriteFiles()
		}
	}

	return
}

// Save Provider Data (WebUI)
func saveFiles(request RequestStruct, fileType string) (err error) {
	var newData map[string]config.ProviderUpdate
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

	for dataID, update := range newData {
		var reloadData = false
		var isNew = dataID == "-"
		if isNew {
			dataID = indicator + randomString(19)
		}

		err = config.Update(func(s *SettingsStruct) error {
			var files = providerFiles(s, fileType)

			if isNew {
				// New Provider File, getProviderData downloads it and resets New
				var provider = config.Provider{Type: fileType, New: true}
				update.Apply(&provider)
				files[dataID] = provider
				return nil
			}

			// Existing Provider File
			var provider, ok = files[dataID]
			if !ok {
				return fmt.Errorf("unknown provider: %s", dataID)
			}
			update.Apply(&provider)
			files[dataID] = provider
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

		if update.Delete {
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
	var updateData map[string]config.ProviderUpdate

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
	var defaultFilter FilterStruct

	defaultFilter.Active = true
	defaultFilter.CaseSensitive = false
	defaultFilter.PreserveMapping = true
	defaultFilter.StartingChannel = strconv.FormatFloat(config.Get().MappingFirstChannel, 'f', -1, 64) // 1000

	err = config.Update(func(s *SettingsStruct) error {
		if s.Filter == nil {
			s.Filter = make(map[int64]config.FilterSettings)
		}

		var createNewID = func() (id int64) {
			for {
				if _, ok := s.Filter[id]; !ok {
					return id
				}
				id++
			}
		}

		for dataID, update := range request.Filter {
			var filter, exists = s.Filter[dataID]
			if dataID == -1 {
				// New Filter
				dataID = createNewID()
				filter, exists = defaultFilter, true
			}

			// Clear Filters
			if update.Delete {
				delete(s.Filter, dataID)
				continue
			}

			// Update filters, nothing is saved if one of them is invalid
			if update.Filter != nil && len(*update.Filter) == 0 {
				return errors.New(getErrMsg(1014))
			}

			if exists {
				update.Apply(&filter)
				s.Filter[dataID] = filter
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
	xepgDB.SaveMapping(request.EpgMapping)

	return
}

// UserForm is a user as the web UI sends it.
type UserForm struct {
	authentication.UserData
	Password string `json:"password,omitempty"`
	Confirm  string `json:"confirm,omitempty"`
	Delete   bool   `json:"delete,omitempty"`
}

// Save User Data (WebUI)
func saveUserData(request RequestStruct) (err error) {
	var users map[string]UserForm
	if err = json.Unmarshal(request.UserData, &users); err != nil {
		return
	}

	for userID, form := range users {
		if len(form.Username) > 0 {
			if err = authentication.ChangeCredentials(userID, form.Username, form.Password); err != nil {
				return
			}
		}

		if request.DeleteUser {
			return authentication.RemoveUser(userID)
		}

		if form.Delete {
			if err = authentication.RemoveUser(userID); err != nil {
				return
			}
			continue
		}

		if err = authentication.WriteUserData(userID, form.UserData); err != nil {
			return
		}
	}

	return
}

// Create New User (WebUI)
func saveNewUser(request RequestStruct) (err error) {
	var form UserForm
	if err = json.Unmarshal(request.UserData, &form); err != nil {
		return
	}

	userID, err := authentication.CreateNewUser(form.Username, form.Password)
	if err != nil {
		return
	}

	return authentication.WriteUserData(userID, form.UserData)
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
			var provider = config.Provider{Type: key, New: true, FileSource: value.(string)}
			var indicator string

			switch key {
			case "m3u":
				provider.Name = "M3U"
				indicator = "M"

			case "xmltv":
				provider.Name = "XMLTV"
				indicator = "X"
			}

			var dataID = indicator + randomString(19)

			err = config.Update(func(s *SettingsStruct) error {
				providerFiles(s, key)[dataID] = provider
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
			}
		}

		if err != nil {
			return
		}
	}

	return
}

// streamsDB is the current list of streams.
var streamsDB playlist.Store

// Create a Database for the DVR System
func buildDatabaseDVR() (err error) {
	var endScan = System.BeginScan()
	defer endScan()

	var settings = config.Get()
	var sources []playlist.Source
	for _, fileType := range []string{"m3u", "hdhr"} {
		for _, file := range getLocalProviderFiles(fileType) {
			var id = strings.TrimSuffix(getFilenameFromPath(file), path.Ext(getFilenameFromPath(file)))
			sources = append(sources, playlist.Source{
				ID:   id,
				Type: fileType,
				Name: providerName(id, fileType),
				Path: file,
			})
		}
	}

	snapshot, compatibilities, err := playlist.Build(sources, playlist.Options{
		Filters:               settings.Filter,
		DisallowURLDuplicates: settings.DisallowURLDuplicates,
	})
	if err != nil {
		return
	}
	streamsDB.Set(snapshot)

	for _, source := range sources {
		if compatibility, ok := compatibilities[source.ID]; ok {
			setProviderCompatibility(source.ID, source.Type, compatibility)
		}
	}

	if len(snapshot.Active) > System.PlexChannelLimit {
		showWarning(2000)
	}

	showInfo(fmt.Sprintf("All streams:%d", len(snapshot.All)))
	showInfo(fmt.Sprintf("Active streams:%d", len(snapshot.Active)))
	showInfo(fmt.Sprintf("Filter:%d", len(snapshot.Filters)))

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

	// In a fixed order, so that new channels get the same numbers whatever the order of the map
	for _, dataID := range slices.Sorted(maps.Keys(providerFilesOf(config.Get(), fileType))) {
		localFiles = append(localFiles, System.Folder.Data+dataID+fileExtension)
	}

	return
}

// Update Provider Statistics Compatibility
func setProviderCompatibility(id, fileType string, compatibility map[string]int) {
	var err = config.Update(func(s *SettingsStruct) error {
		var files = providerFiles(s, fileType)
		if provider, ok := files[id]; ok {
			provider.Compatibility = compatibility
			files[id] = provider
		}
		return nil
	})
	if err != nil {
		ShowError(err, 0)
	}
}
