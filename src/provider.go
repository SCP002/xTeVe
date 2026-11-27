package src

import (
	"errors"
	"fmt"
	"io"
	"maps"
	"net/http"
	"net/url"
	"slices"
	"strings"
	"time"

	"xteve/src/internal/config"
	m3u "xteve/src/internal/m3u-parser"
)

// getProviderData downloads and saves the provider files.
//
// fileType: Which File Type should be updated (m3u, hdhr, xmltv) | fileID: Update a specific File (Provider ID)
//
// Errors of all providers are returned together, each prefixed with the provider name;
// a provider that fails keeps its previous file if there is one.
func getProviderData(fileType, fileID string) (err error) {
	var errs []error

	var fileExtension, serverFileName string
	var body = make([]byte, 0)
	var newProvider bool

	// saveDateFromProvider checks and saves a downloaded file and completes data, the copy of the provider entry
	var saveDateFromProvider = func(
		data map[string]any, fileSource, serverFileName, id string, body []byte,
	) (err error) {
		// Default keys for the Provider Data
		var keys = []string{
			"name", "description", "type", "file." + System.AppName, "file.source", "tuner", "last.update",
			"compatibility", "counter.error", "counter.download", "provider.availability",
		}

		for _, key := range keys {
			if _, ok := data[key]; !ok {
				switch key {
				case "name":
					data[key] = serverFileName

				case "description":
					data[key] = ""

				case "type":
					data[key] = fileType

				case "file." + System.AppName:
					data[key] = id + fileExtension

				case "file.source":
					data[key] = fileSource

				case "last.update":
					data[key] = time.Now().Format("2006-01-02 15:04:05")

				case "tuner":
					if fileType == "m3u" || fileType == "hdhr" {
						if _, ok := data[key].(float64); !ok {
							data[key] = 1
						}
					}

				case "compatibility":
					data[key] = make(map[string]any)

				case "counter.download":
					data[key] = 0.0

				case "counter.error":
					data[key] = 0.0

				case "provider.availability":
					data[key] = 100
				}
			}
		}

		if _, ok := data["id.provider"]; !ok {
			data["id.provider"] = id
		}

		// Extract File
		body, err = extractGZIP(body, fileSource)
		if err != nil {
			return
		}

		// Check Data
		showInfo("Check File:" + fileSource)
		switch fileType {
		case "m3u":
			_, err = m3u.MakeInterfaceFromM3U(body)

		case "hdhr":
			_, err = jsonToInterface(string(body))

		case "xmltv":
			err = checkXMLCompatibility(id, body)
		}

		if err != nil {
			return
		}

		var filePath = System.Folder.Data + data["file."+System.AppName].(string)

		err = writeByteToFile(filePath, body)

		if err == nil {
			data["last.update"] = time.Now().Format("2006-01-02 15:04:05")
			data["counter.download"] = toFloat(data["counter.download"]) + 1
		}

		return
	}

	switch fileType {
	case "m3u":
		fileExtension = ".m3u"

	case "hdhr":
		fileExtension = ".json"

	case "xmltv":
		fileExtension = ".xml"
	}

	// The downloads work on copies of the provider entries, without holding the settings, and the results are
	// merged into the current settings afterwards
	var files = providerFilesOf(config.Get(), fileType)

	for _, dataID := range slices.Sorted(maps.Keys(files)) {
		var data = jsonToMap(mapToJSON(files[dataID]))
		var fileSource, _ = data["file.source"].(string)
		newProvider = false

		if _, ok := data["new"]; ok {
			newProvider = true
			delete(data, "new")
		}

		// If an ID is available and does not match the one from the Database, the Update is skipped (goto)
		if len(fileID) > 0 && !newProvider {
			if dataID != fileID {
				goto Done
			}
		}

		switch fileType {
		case "hdhr":
			// Load from the HDHomeRun Tuner
			showInfo("Tuner:" + fileSource)
			var tunerURL = "http://" + fileSource + "/lineup.json"
			serverFileName, body, err = downloadFileFromServer(tunerURL)

		default:
			if strings.Contains(fileSource, "http://") || strings.Contains(fileSource, "https://") {
				// Load from the Remote Server
				showInfo("Download:" + fileSource)
				serverFileName, body, err = downloadFileFromServer(fileSource)
			} else {
				// Load a local File
				showInfo("Open:" + fileSource)

				err = checkFile(fileSource)
				if err == nil {
					body, err = readByteFromFile(fileSource)
					serverFileName = getFilenameFromPath(fileSource)
				}
			}
		}

		if err == nil {
			err = saveDateFromProvider(data, fileSource, serverFileName, dataID, body)
			if err == nil {
				showInfo("Save File:" + fileSource + " [ID: " + dataID + "]")
			}
		}

		if err != nil {
			var downloadErr = fmt.Errorf("%s: %w", getProviderName(data, dataID), err)

			if !newProvider {
				errs = append(errs, downloadErr)

				// Check whether there is an older File
				var file = System.Folder.Data + dataID + fileExtension

				if checkFile(file) == nil && len(fileID) == 0 {
					showWarning(1011)
				}

				// Increase Error Counter by 1
				data["counter.error"] = toFloat(data["counter.error"]) + 1
				data["counter.download"] = toFloat(data["counter.download"]) + 1
			} else {
				return downloadErr
			}
		}

		// Calculate the Margin of Error
		if !newProvider {
			if toFloat(data["counter.error"]) == 0 {
				data["provider.availability"] = 100
			} else {
				data["provider.availability"] = int(
					toFloat(data["counter.error"])*100/toFloat(data["counter.download"])*-1 + 100,
				)
			}
		}

		if fileType == "xmltv" {
			xepgDB.ForgetXMLTV(System.Folder.Data + dataID + fileExtension)
		}

		if err := mergeProviderFile(fileType, dataID, data); err != nil {
			errs = append(errs, err)
		}

	Done:
	}

	return errors.Join(errs...)
}

// mergeProviderFile writes the keys of data, a downloaded copy of a provider entry, into the current settings. A
// provider removed in the meantime is not added again, and the compatibility, which setProviderCompatibility keeps
// up to date while the file is checked, is not replaced by the older one of the copy.
func mergeProviderFile(fileType, dataID string, data map[string]any) error {
	return config.Update(func(s *SettingsStruct) error {
		var entry, ok = providerFiles(s, fileType)[dataID].(map[string]any)
		if !ok {
			return nil
		}

		for key, value := range data {
			if _, has := entry[key]; has && key == "compatibility" {
				continue
			}
			entry[key] = value
		}
		delete(entry, "new")

		return nil
	})
}

// toFloat returns a number of a provider entry, which is float64 after JSON and int when set in code.
func toFloat(v any) float64 {
	switch v := v.(type) {
	case float64:
		return v
	case int:
		return float64(v)
	}
	return 0
}

// getProviderName returns the provider name for error messages, or its ID if it has no name yet.
func getProviderName(data map[string]any, dataID string) string {
	if name, ok := data["name"].(string); ok && len(name) > 0 {
		return name
	}
	return dataID
}

func downloadFileFromServer(providerURL string) (filename string, body []byte, err error) {
	_, err = url.ParseRequestURI(providerURL)
	if err != nil {
		return
	}

	req, err := http.NewRequest(http.MethodGet, providerURL, nil)
	if err != nil {
		return
	}
	req.Header.Set("User-Agent", config.Get().UserAgent)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return
	}
	defer closeOrLog(resp.Body)

	if resp.StatusCode != http.StatusOK {
		err = fmt.Errorf("%d: %s %s", resp.StatusCode, http.StatusText(resp.StatusCode), providerURL)
		return
	}

	// Get the File Mame from the Header
	var index = strings.Index(resp.Header.Get("Content-Disposition"), "filename")

	if index > -1 {
		var headerFilename = resp.Header.Get("Content-Disposition")[index:len(resp.Header.Get("Content-Disposition"))]
		var value = strings.Split(headerFilename, `=`)
		var f = strings.ReplaceAll(value[1], `"`, "")

		f = strings.ReplaceAll(f, `;`, "")
		filename = f
		showInfo("Header filename:" + filename)
	} else {
		var cleanFilename = strings.SplitN(getFilenameFromPath(providerURL), "?", 2)
		filename = cleanFilename[0]
	}

	body, err = io.ReadAll(resp.Body)
	if err != nil {
		return
	}

	return
}
