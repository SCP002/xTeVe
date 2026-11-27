package src

import (
	"context"
	"errors"
	"fmt"

	"xteve/src/internal/config"
	"xteve/src/internal/updater"

	"reflect"
)

// BinaryUpdate updates xTeVe from the GitHub releases and restarts it, if a newer release exists and automatic
// updates are enabled. The "beta" branch also uses pre-releases.
func BinaryUpdate() (err error) {
	if !System.GitHub.Update {
		showWarning(2099)
		return
	}

	if !updater.Updatable(System.Version) {
		showDebug(fmt.Sprintf("Update:Version %s is a local build and is not updated", System.Version), 1)
		return
	}

	var ctx = context.Background()
	var source = updater.Source{
		Owner:      System.GitHub.User,
		Repo:       System.GitHub.Repo,
		Prerelease: System.Branch == "beta",
	}

	release, err := source.Latest(ctx, System.Name)
	if errors.Is(err, updater.ErrNoRelease) {
		showInfo("Update:" + userMessage(err))
		return nil
	}
	if err != nil {
		return withCode(err, 6003)
	}

	if !updater.Newer(release, System.Version) {
		showDebug(fmt.Sprintf("Update:%s is up to date (latest release %s)", System.Version, release.Version), 1)
		return
	}

	showInfo(fmt.Sprintf("Update Available:Version: %s", release.Version))

	if !config.Get().XteveAutoUpdate {
		showWarning(6004)
		return
	}

	// In a container or a package the binary belongs to root: the image or package is updated instead
	if err = source.CanApply(); err != nil {
		showInfo(fmt.Sprintf("Update:The binary can not be replaced (%s), update the image or package instead", err))
		return nil
	}

	showInfo(fmt.Sprintf("Start Update:%s/%s %s", System.GitHub.User, System.GitHub.Repo, release.Version))

	if err = source.Apply(ctx, release); err != nil {
		return withCode(err, 6002)
	}

	showInfo("Update:Restarting with the new version")
	return withCode(updater.Restart(), 6002)
}

func conditionalUpdateChanges() (err error) {
checkVersion:
	settingsMap, err := loadJSONFileToMap(System.File.Settings)
	if err != nil || len(settingsMap) == 0 {
		return
	}

	if settingsVersion, ok := settingsMap["version"].(string); ok {
		if settingsVersion > System.DBVersion {
			showInfo("Settings DB Version:" + settingsVersion)
			showInfo("System DB Version:" + System.DBVersion)
			err = errors.New(getErrMsg(1031))
			return
		}

		// Latest Compatible Version (1.4.4)
		if settingsVersion < System.Compatibility {
			err = errors.New(getErrMsg(1013))
			return
		}

		switch settingsVersion {
		case "1.4.4":
			// Set UUID Value in xepg.json
			err = setValueForUUID()
			if err != nil {
				return
			}

			// New filter (WebUI). Old Filter Settings are converted
			if oldFilter, ok := settingsMap["filter"].([]any); ok {
				var newFilterMap = convertToNewFilter(oldFilter)
				settingsMap["filter"] = newFilterMap

				settingsMap["version"] = "2.0.0"

				err = saveMapToJSONFile(System.File.Settings, settingsMap)
				if err != nil {
					return
				}

				goto checkVersion
			} else {
				err = errors.New(getErrMsg(1030))
				return
			}

		case "2.0.0":
			if oldBuffer, ok := settingsMap["buffer"].(bool); ok {
				var newBuffer string
				switch oldBuffer {
				case true:
					newBuffer = "xteve"
				case false:
					newBuffer = "-"
				}

				settingsMap["buffer"] = newBuffer

				settingsMap["version"] = "2.1.0"

				err = saveMapToJSONFile(System.File.Settings, settingsMap)
				if err != nil {
					return
				}

				goto checkVersion
			} else {
				err = errors.New(getErrMsg(1030))
				return
			}

		case "2.1.0", "2.1.1":
			// Database verison <= 2.1.1 has broken XEPG mapping

			// Clear XEPG mapping
			if err = xepgDB.Reset(); err != nil {
				return err
			}

			// Notify user
			showWarning(2022)
			sendAlert(getErrMsg(2022))

			// Update database version
			settingsMap["version"] = "2.2.0"

			err = saveMapToJSONFile(System.File.Settings, settingsMap)
			if err != nil {
				return
			}

			goto checkVersion

		case "2.2.0", "2.2.1", "2.2.2", "2.2.3", "2.3.0":
			// If there are changes to the Database in a later update, continue here

			break
		}
	} else {
		// settings.json is too old (older than Version 1.4.4)
		err = errors.New(getErrMsg(1013))
	}

	return
}

func convertToNewFilter(oldFilter []any) (newFilterMap map[int]any) {
	newFilterMap = make(map[int]any)

	switch reflect.TypeOf(oldFilter).Kind() {
	case reflect.Slice:
		s := reflect.ValueOf(oldFilter)

		for i := 0; i < s.Len(); i++ {
			var newFilter FilterStruct
			newFilter.Active = true
			newFilter.Name = fmt.Sprintf("Custom filter %d", i+1)
			newFilter.Filter = s.Index(i).Interface().(string)
			newFilter.Type = "custom-filter"
			newFilter.CaseSensitive = false

			newFilterMap[i] = newFilter
		}
	}

	return
}

func setValueForUUID() (err error) {
	xepg, err := loadJSONFileToMap(System.File.XEPG)
	if err != nil {
		return
	}

	for _, c := range xepg {
		var xepgChannel = c.(map[string]any)

		if uuidKey, ok := xepgChannel["_uuid.key"].(string); ok {
			if value, ok := xepgChannel[uuidKey].(string); ok {
				if len(value) > 0 {
					xepgChannel["_uuid.value"] = value
				}
			}
		}
	}

	err = saveMapToJSONFile(System.File.XEPG, xepg)

	return
}
