package src

import (
	"cmp"
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

	var fileExtension string
	switch fileType {
	case "m3u":
		fileExtension = ".m3u"

	case "hdhr":
		fileExtension = ".json"

	case "xmltv":
		fileExtension = ".xml"
	}

	// The downloads work on copies of the providers, without holding the settings, and the results are merged
	// into the current settings afterwards
	var files = providerFilesOf(config.Get(), fileType)

	for _, dataID := range slices.Sorted(maps.Keys(files)) {
		var provider = files[dataID]
		var newProvider = provider.New

		// If an ID is given, only that provider (and new ones) are updated
		if len(fileID) > 0 && !newProvider && dataID != fileID {
			continue
		}

		var serverFileName string
		var body []byte
		var err error

		switch fileType {
		case "hdhr":
			// Load from the HDHomeRun Tuner
			showInfo("Tuner:" + provider.FileSource)
			serverFileName, body, err = downloadFileFromServer("http://" + provider.FileSource + "/lineup.json")

		default:
			if strings.Contains(provider.FileSource, "http://") || strings.Contains(provider.FileSource, "https://") {
				// Load from the Remote Server
				showInfo("Download:" + provider.FileSource)
				serverFileName, body, err = downloadFileFromServer(provider.FileSource)
			} else {
				// Load a local File
				showInfo("Open:" + provider.FileSource)

				err = checkFile(provider.FileSource)
				if err == nil {
					body, err = readByteFromFile(provider.FileSource)
					serverFileName = getFilenameFromPath(provider.FileSource)
				}
			}
		}

		if err == nil {
			completeProvider(&provider, dataID, fileType, fileExtension, serverFileName)
			err = saveProviderFile(&provider, dataID, fileType, body)
			if err == nil {
				showInfo("Save File:" + provider.FileSource + " [ID: " + dataID + "]")
			}
		}

		if err != nil {
			var downloadErr = fmt.Errorf("%s: %w", cmp.Or(provider.Name, dataID), err)

			if newProvider {
				return downloadErr
			}
			errs = append(errs, downloadErr)

			// Check whether there is an older File
			if checkFile(System.Folder.Data+dataID+fileExtension) == nil && len(fileID) == 0 {
				showWarning(1011)
			}

			// Increase Error Counter by 1
			provider.CounterError++
			provider.CounterDownload++
		}

		// Calculate the Margin of Error
		if !newProvider {
			provider.Availability = 100
			if provider.CounterError > 0 {
				provider.Availability = int(provider.CounterError*100/provider.CounterDownload*-1 + 100)
			}
		}

		if fileType == "xmltv" {
			xepgDB.ForgetXMLTV(System.Folder.Data + dataID + fileExtension)
		}

		if err := mergeProviderFile(fileType, dataID, provider); err != nil {
			errs = append(errs, err)
		}
	}

	return errors.Join(errs...)
}

// completeProvider fills in the defaults of a provider before its first download.
func completeProvider(p *config.Provider, id, fileType, fileExtension, serverFileName string) {
	p.ID = cmp.Or(p.ID, id)
	p.Name = cmp.Or(p.Name, serverFileName)
	p.Type = cmp.Or(p.Type, fileType)
	p.File = cmp.Or(p.File, id+fileExtension)
	p.LastUpdate = cmp.Or(p.LastUpdate, time.Now().Format("2006-01-02 15:04:05"))
	if (fileType == "m3u" || fileType == "hdhr") && p.Tuner == 0 {
		p.Tuner = 1
	}
	if p.Compatibility == nil {
		p.Compatibility = map[string]int{}
	}
	if p.New {
		p.Availability = 100
	}
}

// saveProviderFile checks a downloaded file and saves it as the local copy of the provider.
func saveProviderFile(p *config.Provider, id, fileType string, body []byte) (err error) {
	// Extract File
	body, err = extractGZIP(body, p.FileSource)
	if err != nil {
		return
	}

	// Check Data
	showInfo("Check File:" + p.FileSource)
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

	if err = writeByteToFile(System.Folder.Data+p.File, body); err != nil {
		return
	}

	p.LastUpdate = time.Now().Format("2006-01-02 15:04:05")
	p.CounterDownload++
	return nil
}

// mergeProviderFile writes the results of a download into the current provider. A provider removed in the meantime
// is not added again; what the web UI may have changed meanwhile (name, source, tuners) and the compatibility, which
// setProviderCompatibility keeps up to date while the file is checked, stay as they are.
func mergeProviderFile(fileType, dataID string, downloaded config.Provider) error {
	return config.Update(func(s *SettingsStruct) error {
		var files = providerFiles(s, fileType)
		var current, ok = files[dataID]
		if !ok {
			return nil
		}

		current.ID = downloaded.ID
		current.Name = cmp.Or(current.Name, downloaded.Name)
		current.Type = downloaded.Type
		current.File = downloaded.File
		current.LastUpdate = downloaded.LastUpdate
		current.CounterError = downloaded.CounterError
		current.CounterDownload = downloaded.CounterDownload
		current.Availability = downloaded.Availability
		if current.Tuner == 0 {
			current.Tuner = downloaded.Tuner
		}
		if current.Compatibility == nil {
			current.Compatibility = downloaded.Compatibility
		}
		current.New = false

		files[dataID] = current
		return nil
	})
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
