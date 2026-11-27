package src

// providerFiles returns the provider files of fileType ("m3u", "hdhr" or "xmltv") in settings that are being
// changed (inside config.Update), creating the map if needed.
func providerFiles(settings *SettingsStruct, fileType string) map[string]any {
	var files *map[string]any

	switch fileType {
	case "m3u":
		files = &settings.Files.M3U
	case "hdhr":
		files = &settings.Files.HDHR
	case "xmltv":
		files = &settings.Files.XMLTV
	default:
		return nil
	}

	if *files == nil {
		*files = make(map[string]any)
	}
	return *files
}

// providerFilesOf returns the provider files of fileType in settings for reading only.
func providerFilesOf(settings *SettingsStruct, fileType string) map[string]any {
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
