package errcode

import (
	"fmt"
	"os"
	"sync/atomic"
	"unicode"
	"unicode/utf8"

	"xteve/src/internal/logger"
)

// App is the information about xTeVe used in messages.
type App struct {
	Name             string
	Compatibility    string
	SettingsFile     string
	PlexChannelLimit int
}

var appInfo atomic.Pointer[App]

func init() { appInfo.Store(&App{Name: "xTeVe"}) }

// SetApp sets the information about xTeVe used in messages.
func SetApp(app App) { appInfo.Store(&app) }

// Report logs err with the message of its error code: the code attached to err (see With), or else code.
func Report(err error, code int) {
	if c := Of(err); c != 0 {
		code = c
	}
	logger.Error(fmt.Sprintf("%s (%s) - EC: %d", UserMessage(err), Message(code), code))
}

// Warn logs the warning of a code.
func Warn(code int) { logger.Warning(Message(code)) }

// UserMessage turns an error into a message for the web UI: it starts with a capital letter.
func UserMessage(err error) string {
	var msg = err.Error()
	r, size := utf8.DecodeRuneInString(msg)
	if r == utf8.RuneError {
		return msg
	}
	return string(unicode.ToUpper(r)) + msg[size:]
}

// Message returns the message of an xTeVe error code.
func Message(errCode int) (errMsg string) {
	var app = appInfo.Load()

	switch errCode {
	case 0:
		return

	// Errors
	case 1001:
		errMsg = "Web server could not be started."
	case 1002:
		errMsg = "No local IP address found."
	case 1003:
		errMsg = "Invalid xml"
	case 1004:
		errMsg = "File not found"
	case 1005:
		errMsg = "Invalid M3U file, an extended M3U file is required."
	case 1006:
		errMsg = "No playlist!"
	case 1007:
		errMsg = "XEPG requires an XMLTV file."
	case 1010:
		errMsg = "Invalid file compression"
	case 1011:
		errMsg = fmt.Sprintf("Data is corrupt or unavailable, %s now uses an older version of this file", app.Name)
	case 1012:
		errMsg = "Invalid formatting of the time"
	case 1013:
		errMsg = fmt.Sprintf(
			"Invalid settings file (settings.json), file must be at least version %s",
			app.Compatibility,
		)
	case 1014:
		errMsg = "Invalid filter rule"
	case 1015:
		errMsg = fmt.Sprintf("Specified temp folder path is invalid, fallback to %s", os.TempDir())
	case 1016:
		errMsg = "Web server could not be stopped."
	case 1017:
		errMsg = "Web server could not be started in TLS mode, fallback to default."
	case 1018:
		errMsg = "Failed to compile channel name update regex"

	case 1020:
		errMsg = "Data could not be saved, invalid keyword"

	// Database Update
	case 1030:
		errMsg = fmt.Sprintf("Invalid settings file (%s)", app.SettingsFile)
	case 1031:
		errMsg = "Database error. The database version of your settings is not compatible with this version."

	// M3U Parser
	case 1050:
		errMsg = "Invalid duration specification in the M3U8 playlist."

	case 1060:
		errMsg = "Invalid characters found in the tvg parameters, streams with invalid parameters were skipped."

	// Filesystem
	case 1070:
		errMsg = "Folder could not be created."
	case 1071:
		errMsg = "File could not be created"
	case 1072:
		errMsg = "File not found"
	case 1073:
		errMsg = "Can not remove old config folder contents before recover"

	// Backup
	case 1090:
		errMsg = "Automatic backup failed"

	// Websockets
	case 1100:
		errMsg = "WebUI build error"
	case 1101:
		errMsg = "WebUI request error"
	case 1102:
		errMsg = "WebUI response error"

	// PMS Guide Numbers
	case 1200:
		errMsg = "Could not create file"

	// Stream URL Error
	case 1201:
		errMsg = "Plex stream error"
	case 1202:
		errMsg = "Steaming URL could not be found in any playlist"
	case 1203:
		errMsg = "Steaming URL could not be found in any playlist"
	case 1204:
		errMsg = "Streaming was stopped by third party transcoder (FFmpeg / VLC)"

	// Warnings
	case 2000:
		errMsg = fmt.Sprintf("Plex can not handle more than %d streams. Use filter to reduce the number of streams. "+
			"If you do not use Plex, ignore this warning.", app.PlexChannelLimit)
	case 2001:
		// Free slot
		return
	case 2002:
		errMsg = "PMS can not play m3u8 streams"
	case 2003:
		errMsg = "PMS can not play streams over RTSP."
	case 2004:
		errMsg = "Buffer is disabled for this stream."
	case 2005:
		errMsg = "There are no channels mapped, use the mapping menu to assign EPG data to the channels."
	case 2010:
		errMsg = "No valid streaming URL"
	case 2020:
		errMsg = "FFmpeg binary was not found. Check the FFmpeg binary path in the xTeVe settings."
	case 2021:
		errMsg = "VLC binary was not found. Check the VLC path binary in the xTeVe settings."
	case 2022:
		errMsg = "Loaded database had broken XEPG mapping (version <= 2.1.1). It was cleared."

	case 2099:
		errMsg = "Updates have been disabled by the developer"

	// Tuner
	case 2105:
		errMsg = "The number of tuners has changed, you have to delete " + app.Name +
			" in Plex / Emby HDHR and set it up again."
	case 2106:
		errMsg = "This function is only available with XEPG as EPG source"

	case 2110:
		errMsg = "Don't run this as Root!"

	case 2300:
		errMsg = "No channel logo found in the XMLTV or M3U file."
	case 2301:
		errMsg = "XMLTV file no longer available, channel has been deactivated."
	case 2302:
		errMsg = "Channel ID in the XMLTV file has changed. Channel has been deactivated."

	// User Authentication
	case 3000:
		errMsg = "Database for user authentication could not be initialized."
	case 3001:
		errMsg = "The user has no authorization to load the channels."

	// Buffer
	case 4000:
		errMsg = "Connection to streaming source was interrupted."
	case 4001:
		errMsg = "Too many errors connecting to the provider. Streaming is canceled."
	case 4002:
		errMsg = "New URL for the redirect to the streaming server is missing"
	case 4003:
		errMsg = "Server sends an incompatible content-type"
	case 4004:
		errMsg = "This error message comes from the provider"
	case 4005:
		errMsg = "Temporary buffer files could not be deleted"
	case 4006:
		errMsg = "Server connection timeout"
	case 4007:
		errMsg = "Old temporary buffer file could not be deleted"

	// Buffer (M3U8)
	case 4050:
		errMsg = "Invalid M3U8 file"
	case 4051:
		errMsg = "#EXTM3U header is missing"

	// Caching
	case 4100:
		errMsg = "Unknown content type for downloaded image"
	case 4101:
		errMsg = "Invalid URL, original URL is used for this image"

	// API
	case 5000:
		errMsg = "Invalid API command"

	// Updates from the GitHub releases
	case 6002:
		errMsg = "Update failed"
	case 6003:
		errMsg = "Release information not available"
	case 6004:
		errMsg = "A new version is available, automatic updates are disabled in the settings"

	// Certificates
	case 7000:
		errMsg = "Can not generate a certificate"

	default:
		errMsg = fmt.Sprintf("Unknown error / warning (%d)", errCode)
	}

	return errMsg
}
