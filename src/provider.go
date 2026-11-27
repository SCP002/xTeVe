package src

import (
	"bufio"
	"cmp"
	"compress/gzip"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"maps"
	"mime"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"xteve/src/internal/config"
	"xteve/src/internal/epg"
	"xteve/src/internal/hdhr"
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

		// The file is written next to the local copy and replaces it only when it is valid
		var file = System.Folder.Data + dataID + fileExtension
		var serverFileName, err = fetchProvider(provider.FileSource, fileType, file+".download")
		if err == nil {
			completeProvider(&provider, dataID, fileType, fileExtension, serverFileName)
			err = checkProviderFile(dataID, fileType, file+".download")
		}
		if err == nil {
			err = os.Rename(file+".download", System.Folder.Data+provider.File)
		}
		if err == nil {
			provider.LastUpdate = time.Now().Format("2006-01-02 15:04:05")
			provider.CounterDownload++
			showInfo("Save File:" + provider.FileSource + " [ID: " + dataID + "]")
		} else {
			removeAllOrLog(file + ".download")
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

// checkProviderFile checks a downloaded file: a valid M3U playlist, HDHomeRun lineup or XMLTV file. The
// compatibility of an XMLTV file is stored at the same time.
func checkProviderFile(id, fileType, file string) error {
	showInfo("Check File:" + file)

	if fileType == "xmltv" {
		var channels, programmes, err = epg.Count(file)
		if err != nil {
			return err
		}
		setProviderCompatibility(id, "xmltv", map[string]int{"xmltv.channels": channels, "xmltv.programs": programmes})
		return nil
	}

	var content, err = os.ReadFile(file)
	if err != nil {
		return err
	}

	switch fileType {
	case "m3u":
		_, err = m3u.Parse(content)

	case "hdhr":
		// The lineup of an HDHomeRun tuner
		var lineup []hdhr.LineupStream
		err = json.Unmarshal(content, &lineup)
	}
	return err
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

// fetchProvider writes the file of a provider to dst: downloaded (HTTP), from an HDHomeRun tuner or copied from a
// local file. A gzip file is unpacked while it is written. It returns the name of the file at the source.
func fetchProvider(source, fileType, dst string) (filename string, err error) {
	var r io.ReadCloser

	switch {
	case fileType == "hdhr":
		// Load from the HDHomeRun Tuner
		showInfo("Tuner:" + source)
		r, filename, err = openURL("http://" + source + "/lineup.json")

	case strings.Contains(source, "http://") || strings.Contains(source, "https://"):
		// Load from the Remote Server
		showInfo("Download:" + source)
		r, filename, err = openURL(source)

	default:
		// Load a local File
		showInfo("Open:" + source)
		r, err = os.Open(filepath.Clean(source))
		filename = getFilenameFromPath(source)
	}
	if err != nil {
		return
	}
	defer closeOrLog(r)

	var in = bufio.NewReaderSize(r, 1<<16)
	var data io.Reader = in

	// Unpack gzip files (magic number 1f 8b) on the way
	if magic, _ := in.Peek(2); len(magic) == 2 && magic[0] == 0x1f && magic[1] == 0x8b {
		showInfo("Extract gzip:" + source)
		var gz *gzip.Reader
		if gz, err = gzip.NewReader(in); err != nil {
			return
		}
		defer closeOrLog(gz)
		data = gz
	}

	out, err := os.Create(dst)
	if err != nil {
		return
	}
	defer closeKeepErr(out, &err)

	_, err = io.Copy(out, data)
	return
}

// openURL requests a URL and returns the body and the name of the file, from the Content-Disposition header or the
// URL.
func openURL(providerURL string) (body io.ReadCloser, filename string, err error) {
	if _, err = url.ParseRequestURI(providerURL); err != nil {
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

	if resp.StatusCode != http.StatusOK {
		closeOrLog(resp.Body)
		err = fmt.Errorf("%d: %s %s", resp.StatusCode, http.StatusText(resp.StatusCode), providerURL)
		return
	}

	// Get the File Name from the Header
	if _, params, err := mime.ParseMediaType(resp.Header.Get("Content-Disposition")); err == nil &&
		len(params["filename"]) > 0 {
		filename = params["filename"]
		showInfo("Header filename:" + filename)
	} else {
		filename, _, _ = strings.Cut(getFilenameFromPath(providerURL), "?")
	}

	return resp.Body, filename, nil
}
