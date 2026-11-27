package src

import (
	"fmt"

	"xteve/src/internal/config"
	"xteve/src/internal/epg"
	"xteve/src/internal/playlist"
)

// xepgDB is the XEPG database.
var xepgDB *epg.Manager

// Created in init, because writing the M3U file reads xepgDB
func init() {
	xepgDB = epg.NewManager(epg.Config{
		System:     &System,
		Streams:    func() []playlist.Stream { return streamsDB.Get().Active },
		XMLTVFiles: func() []string { return getLocalProviderFiles("xmltv") },
		XMLTVName:  func(id string) string { return providerName(id, "xmltv") },
		WriteM3U:   createM3UFile,
	})
}

// XMLTV types used in src
type (
	XMLTV       = epg.XMLTV
	DisplayName = epg.DisplayName
)

// Create XEPG Data. In PMS mode only the lineup is built.
func buildXEPG(background bool) {
	if config.Get().EpgSource != "XEPG" {
		var endScan = System.BeginScan()
		defer endScan()
		if _, err := getLineup(); err != nil {
			ShowError(err, 0)
		}
		return
	}

	if background {
		go xepgDB.Rebuild()
	} else {
		xepgDB.Rebuild()
	}
}

// Create M3U File
func createM3UFile() {
	showInfo("XEPG:" + fmt.Sprintf("Create M3U file (%s)", System.File.M3U))
	_, err := buildM3U([]string{})
	if err != nil {
		ShowError(err, 000)
	}

	if err := streamURLs.Load().Save(); err != nil {
		ShowError(err, 0)
	}
}

// clearXMLTVCache forgets the parsed XMLTV files.
func clearXMLTVCache() { xepgDB.ClearXMLTVCache() }
