package src

import (
	"fmt"
	"os"
	"runtime"
	"strings"
	"xteve/src/internal/config"
	"xteve/src/internal/errcode"
	"xteve/src/internal/logger"
)

// System : Contains all System Information
var System SystemStruct

// SystemFiles : All System Files
var SystemFiles = []string{"authentication.json", "pms.json", "settings.json", "xepg.json", "urls.json"}

// Init : System Initialization
func Init() (err error) {
	var debug string

	config.Default().SetSaver(completeAndWriteSettings)

	logger.Default().SetName(System.Name)
	logger.Default().SetDebugLevel(System.Flag.Debug)
	logger.Default().SetQuiet(System.Flag.Info)

	// System Settings
	System.AppName = strings.ToLower(System.Name)
	System.ARCH = runtime.GOARCH
	System.OS = runtime.GOOS
	// Until the web server starts, the protocol follows the setting
	System.SetTLS(config.Get().TLSMode)
	System.PlexChannelLimit = 480
	System.Compatibility = "1.4.4"

	// FFmpeg Default Settings
	System.FFmpeg.DefaultOptions = "-hide_banner -loglevel error -i [URL] -c copy -f mpegts pipe:1"
	System.VLC.DefaultOptions = "-I dummy [URL] --sout #std{mux=ts,access=file,dst=-}"

	// Variables for the Update Process
	System.Update.Git = fmt.Sprintf("https://github.com/%s/%s/blob", System.GitHub.User, System.GitHub.Repo)
	System.Update.Name = "xteve_2"

	// Define folder paths
	if len(System.Folder.Config) == 0 {
		System.Folder.Config = GetUserHomeDirectory() + string(
			os.PathSeparator,
		) + "." + System.AppName + string(
			os.PathSeparator,
		)
	} else {
		System.Folder.Config = strings.TrimRight(
			System.Folder.Config,
			string(os.PathSeparator),
		) + string(
			os.PathSeparator,
		)
	}

	System.Folder.Config = getPlatformPath(System.Folder.Config)

	System.Folder.Backup = System.Folder.Config + "backup" + string(os.PathSeparator)
	System.Folder.Data = System.Folder.Config + "data" + string(os.PathSeparator)
	System.Folder.Cache = System.Folder.Config + "cache" + string(os.PathSeparator)
	System.Folder.Certificates = System.Folder.Config + "certificates" + string(os.PathSeparator)
	System.Folder.ImagesCache = System.Folder.Cache + "images" + string(os.PathSeparator)
	System.Folder.ImagesUpload = System.Folder.Data + "images" + string(os.PathSeparator)
	System.SetTempFolder(getDefaultTempDir())

	// Dev Info
	showDevInfo()

	// Create System Folder
	err = createSystemFolders()
	if err != nil {
		return withCode(err, 1070)
	}

	if len(System.Flag.Restore) > 0 {
		// Settings are restored via CLI. No further Initialization is necessary.
		return
	}

	System.File.ServerCert = getPlatformFile(fmt.Sprintf("%sxteve.crt", System.Folder.Certificates))
	System.File.ServerCertPrivKey = getPlatformFile(fmt.Sprintf("%sxteve.key", System.Folder.Certificates))
	System.File.XML = getPlatformFile(fmt.Sprintf("%s%s.xml", System.Folder.Data, System.AppName))
	System.File.M3U = getPlatformFile(fmt.Sprintf("%s%s.m3u", System.Folder.Data, System.AppName))

	System.Compressed.GZxml = getPlatformFile(fmt.Sprintf("%s%s.xml.gz", System.Folder.Data, System.AppName))

	err = activatedSystemAuthentication()
	if err != nil {
		return
	}

	// Menu for the Web interface
	System.WEB.Menu = []string{"playlist", "filter", "xmltv", "mapping", "users", "settings", "log", "logout"}

	fmt.Println("For help run: " + getPlatformFile(os.Args[0]) + " -h")
	fmt.Println()

	// Check whether xTeVe is running as root
	if os.Geteuid() == 0 {
		showWarning(2110)
	}

	if System.Flag.Debug > 0 {
		debug = fmt.Sprintf("Debug Level:%d", System.Flag.Debug)
		showDebug(debug, 1)
	}

	showInfo("Version:" + System.Version)
	showInfo(fmt.Sprintf("Database Version:%s", System.DBVersion))
	showInfo(fmt.Sprintf("System Folder:%s", getPlatformPath(System.Folder.Config)))

	// Create System Files (If not available)
	err = createSystemFiles()
	if err != nil {
		return withCode(err, 1071)
	}

	initHDHR()

	errcode.SetApp(errcode.App{
		Name:             System.Name,
		Compatibility:    System.Compatibility,
		SettingsFile:     System.File.Settings,
		PlexChannelLimit: System.PlexChannelLimit,
	})

	// Perform conditional Update Changes
	err = conditionalUpdateChanges()
	if err != nil {
		return
	}

	// Load Settings (settings.json)
	showInfo(fmt.Sprintf("Load Settings:%s", System.File.Settings))

	_, err = loadSettings()
	if err != nil {
		return
	}

	err = resolveHostIP()
	if err != nil {
		ShowError(err, 1002)
	}

	showInfo(
		fmt.Sprintf("System IP Addresses:IPv4: %d | IPv6: %d", len(System.IPAddressesV4), len(System.IPAddressesV6)),
	)
	showInfo("Hostname:" + System.Hostname)

	// Check the permissions on all Folders
	if err = checkFilePermission(System.Folder.Config); err != nil {
		return
	}
	if err = checkFilePermission(System.TempFolder()); err != nil {
		return
	}

	showInfo(fmt.Sprintf("Temporary Folder:%s", getPlatformPath(System.TempFolder())))

	err = checkFolder(System.TempFolder())
	if err != nil {
		return
	}

	err = removeChildItems(getPlatformPath(System.TempFolder()))
	if err != nil {
		return
	}

	// Set Branch
	System.Branch = config.Get().Branch

	if System.Dev {
		System.Branch = "Development"
	}

	if len(System.Branch) == 0 {
		System.Branch = "master"
	}

	showInfo(fmt.Sprintf("GitHub:https://github.com/%s", System.GitHub.User))
	showInfo(fmt.Sprintf("Git Branch:%s [%s]", System.Branch, System.GitHub.User))

	// Set Domain Names
	System.SetTLS(config.Get().TLSMode)
	setGlobalDomain(fmt.Sprintf("%s:%s", config.Get().HostIP, config.Get().Port))

	System.URLBase = fmt.Sprintf("%s://%s:%s", System.Protocol(), config.Get().HostIP, config.Get().Port)

	// Create HTML Files, with dev == true the local HTML Files are used
	if System.Dev {
		HTMLInit("webUI", "src", "html"+string(os.PathSeparator), "src"+string(os.PathSeparator)+"webUI.go")
		err = BuildGoFile()
		if err != nil {
			return
		}
	}

	// Start the DLNA Server
	err = SSDP()
	if err != nil {
		return
	}

	// Load HTML Files
	loadHTMLMap()

	return
}

// StartSystem : System is starting up
func StartSystem(updateProviderFiles bool) (err error) {
	if System.Scanning() {
		return
	}

	// Output System Information in the Console
	showInfo(fmt.Sprintf("UUID:%s", config.Get().UUID))
	showInfo(fmt.Sprintf("Tuner (Plex / Emby):%d", config.Get().Tuner))
	showInfo(fmt.Sprintf("EPG Source:%s", config.Get().EpgSource))
	showInfo(fmt.Sprintf("Plex Channel Limit:%d", System.PlexChannelLimit))

	// Update Provider Data
	if len(config.Get().Files.M3U) > 0 && config.Get().FilesUpdate || updateProviderFiles {
		err = xTeVeAutoBackup()
		if err != nil {
			ShowError(err, 1090)
		}

		if err := getProviderData("m3u", ""); err != nil {
			ShowError(err, 0)
		}
		if err := getProviderData("hdhr", ""); err != nil {
			ShowError(err, 0)
		}

		if config.Get().EpgSource == "XEPG" {
			if err := getProviderData("xmltv", ""); err != nil {
				ShowError(err, 0)
			}
		}
	}

	err = buildDatabaseDVR()
	if err != nil {
		return
	}

	buildXEPG(false)

	return
}

// reinitialize : Initialize and start up the system, updating provider files
func reinitialize() {
	err := Init()
	if err != nil {
		ShowError(err, 0)
	}

	err = StartSystem(true)
	if err != nil {
		ShowError(err, 0)
	}
}
