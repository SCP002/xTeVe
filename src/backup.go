package src

import (
	b64 "encoding/base64"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"
)

func xTeVeAutoBackup() (err error) {

	var archive = "xteve_auto_backup_" + time.Now().Format("20060102_1504") + ".zip"
	var target string
	var sourceFiles = make([]string, 0)
	var oldBackupFiles = make([]string, 0)
	var debug string

	if len(Settings.BackupPath) > 0 {
		System.Folder.Backup = Settings.BackupPath
	}

	showInfo("Backup Path:" + System.Folder.Backup)

	err = checkFolder(System.Folder.Backup)
	if err != nil {
		return withCode(err, 1070)
	}

	// Delete Old Backups
	files, err := os.ReadDir(System.Folder.Backup)

	if err == nil {

		for _, file := range files {

			if filepath.Ext(file.Name()) == ".zip" && strings.Contains(file.Name(), "xteve_auto_backup") {
				oldBackupFiles = append(oldBackupFiles, file.Name())
			}

		}

		// Delete All Backups
		var end int
		switch Settings.BackupKeep {
		case 0:
			end = 0
		default:
			end = Settings.BackupKeep - 1
		}

		for i := 0; i < len(oldBackupFiles)-end; i++ {

			removeAllOrLog(System.Folder.Backup + oldBackupFiles[i])
			debug = fmt.Sprintf("Delete backup file:%s", oldBackupFiles[i])
			showDebug(debug, 1)

		}

		if Settings.BackupKeep == 0 {
			return
		}

	} else {

		return

	}

	// Create a Backup
	if err == nil {

		target = System.Folder.Backup + archive

		for _, i := range SystemFiles {
			sourceFiles = append(sourceFiles, System.Folder.Config+i)
		}

		sourceFiles = append(sourceFiles, System.Folder.ImagesUpload)
		if Settings.TLSMode {
			sourceFiles = append(sourceFiles, System.Folder.Certificates)
		}

		err = zipFiles(sourceFiles, target)

		if err == nil {

			debug = fmt.Sprintf("Create backup file:%s", target)
			showDebug(debug, 1)

			showInfo("Backup file:" + target)

		}

	}

	return
}

func xteveBackup() (archive string, err error) {

	err = checkFolder(System.Folder.Temp)
	if err != nil {
		return
	}

	archive = "xteve_backup_" + time.Now().Format("20060102_1504") + ".zip"

	var target = System.Folder.Temp + archive
	var sourceFiles = make([]string, 0)

	for _, i := range SystemFiles {
		sourceFiles = append(sourceFiles, System.Folder.Config+i)
	}

	sourceFiles = append(sourceFiles, System.Folder.Data)
	if Settings.TLSMode {
		sourceFiles = append(sourceFiles, System.Folder.Certificates)
	}

	err = zipFiles(sourceFiles, target)

	return
}

func xteveRestore(archive string) (newWebURL string, err error) {

	var newPort, oldPort, backupVersion, tmpRestore string

	tmpRestore = System.Folder.Temp + "restore" + string(os.PathSeparator)

	defer removeAllOrLog(tmpRestore)
	defer removeAllOrLog(archive)

	err = checkFolder(tmpRestore)
	if err != nil {
		return
	}

	// Unpack the ZIP Archive in tmp
	err = extractZIP(archive, tmpRestore)
	if err != nil {
		return
	}

	// Load a new Config to check the Port and Version
	newConfig, err := loadJSONFileToMap(tmpRestore + "settings.json")
	if err != nil {
		return
	}

	backupVersion, ok := newConfig["version"].(string)
	if !ok {
		err = errors.New("invalid backup: settings.json has no version")
		return
	}
	if backupVersion < System.Compatibility {
		err = errors.New(getErrMsg(1013))
		return
	}

	if err = removeChildItems(getPlatformPath(System.Folder.Config)); err != nil {
		ShowError(err, 1073)
	}

	// Extract the ZIP Archive into the Config Folder
	err = extractZIP(archive, System.Folder.Config)
	if err != nil {
		return
	}

	// Load a new Config to check the Port and Version
	newConfig, err = loadJSONFileToMap(System.Folder.Config + "settings.json")
	if err != nil {
		return
	}

	newPort, ok = newConfig["port"].(string)
	if !ok {
		err = errors.New("invalid backup: settings.json has no port")
		return
	}
	oldPort = Settings.Port

	if newPort == oldPort {

		if _, err = loadSettings(); err != nil {
			return
		}

		if err = Init(); err != nil {
			return "", err
		}

		return "", StartSystem(true)
	}

	var url = System.URLBase + "/web/"
	newWebURL = strings.Replace(url, ":"+oldPort, ":"+newPort, 1)

	return
}

func xteveRestoreFromWeb(input string) (newWebURL string, err error) {

	// Convert base64 JSON string to base64
	b64data := input[strings.IndexByte(input, ',')+1:]

	// Convert Base64 into bytes and save
	sDec, err := b64.StdEncoding.DecodeString(b64data)

	if err != nil {
		return
	}

	var archive = System.Folder.Temp + "restore.zip"

	err = writeByteToFile(archive, sDec)
	if err != nil {
		return
	}

	newWebURL, err = xteveRestore(archive)

	return
}

// XteveRestoreFromCLI : Recovery from the Command Line
func XteveRestoreFromCLI(archive string) (err error) {

	var confirm string

	println()
	showInfo(fmt.Sprintf("Version:%s Build: %s", System.Version, System.Build))
	showInfo(fmt.Sprintf("Backup File:%s", archive))
	showInfo(fmt.Sprintf("System Folder:%s", getPlatformPath(System.Folder.Config)))
	println()

	fmt.Print("All data will be replaced with those from the backup. Should the files be restored? [yes|no]:")

	// An empty line is also reported as an error; it is handled as invalid input below
	if _, err := fmt.Scanln(&confirm); errors.Is(err, io.EOF) || errors.Is(err, io.ErrUnexpectedEOF) {
		return errors.New("no confirmation received: standard input is closed")
	}

	switch strings.ToLower(confirm) {

	case "yes":
		break

	case "no":
		return

	default:
		fmt.Println("Invalid input")
		return

	}

	if len(System.Folder.Config) > 0 {

		err = checkFilePermission(System.Folder.Config)
		if err != nil {
			return
		}

		_, err = xteveRestore(archive)
		if err != nil {
			return
		}

		showHighlight(fmt.Sprintf("Restor:Backup was successfully restored. %s can now be started normally", System.Name))

	}
	return
}
