package src

import (
	"fmt"
	"math/rand"
	"time"
	"xteve/src/internal/config"
)

// InitMaintenance : Initialize maintenance process
func InitMaintenance() (err error) {
	System.TimeForAutoUpdate = fmt.Sprintf("0%d%d", randomTime(0, 2), randomTime(10, 59))

	go maintenance()

	return
}

func maintenance() {
	for {
		var t = time.Now()

		// Update the playlist and XMLTV files
		if !System.Scanning() {
			for _, schedule := range config.Get().Update {
				if schedule == t.Format("1504") {
					showInfo("Update:" + schedule)

					// Create a backup
					err := xTeVeAutoBackup()
					if err != nil {
						ShowError(err, 000)
					}

					// Update Playlist and XMLTV Files
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

					// Create database for DVR
					err = buildDatabaseDVR()
					if err != nil {
						ShowError(err, 000)
					}

					if !config.Get().CacheImages && !System.ImageCachingInProgress.Load() {
						if err := removeChildItems(System.Folder.ImagesCache); err != nil {
							ShowError(err, 0)
						}
					}

					// Create XEPG Files
					clearXMLTVCache()
					buildXEPG(false)
				}
			}

			// Update xTeVe (Binary)
			if System.TimeForAutoUpdate == t.Format("1504") {
				if err := BinaryUpdate(); err != nil {
					ShowError(err, 0)
				}
			}
		}

		time.Sleep(60 * time.Second)
	}
}

func randomTime(min, max int) int {
	return rand.Intn(max-min) + min
}
