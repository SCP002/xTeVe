package src

import (
	"xteve/src/internal/config"
)

// providerFiles returns the provider files of fileType ("m3u", "hdhr" or "xmltv") in settings that are being
// changed (inside config.Update), creating the map if needed.
func providerFiles(settings *SettingsStruct, fileType string) config.Providers {
	var files *config.Providers

	switch fileType {
	case "m3u":
		files = &settings.Files.M3U
	case "hdhr":
		files = &settings.Files.HDHR
	case "xmltv":
		files = &settings.Files.XMLTV
	default:
		return config.Providers{}
	}

	if *files == nil {
		*files = make(config.Providers)
	}
	return *files
}

// providerFilesOf returns the provider files of fileType in settings for reading only.
func providerFilesOf(settings *SettingsStruct, fileType string) config.Providers {
	switch fileType {
	case "m3u":
		return settings.Files.M3U
	case "hdhr":
		return settings.Files.HDHR
	case "xmltv":
		return settings.Files.XMLTV
	}
	return nil
}

// getProvider returns a provider of the current settings.
func getProvider(id, fileType string) (config.Provider, bool) {
	var p, ok = providerFilesOf(config.Get(), fileType)[id]
	return p, ok
}

// providerName returns the name of a provider, "" if it does not exist.
func providerName(id, fileType string) string {
	var p, _ = getProvider(id, fileType)
	return p.Name
}
