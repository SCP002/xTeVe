package src

import (
	"fmt"
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/koron/go-ssdp"
)

// SSDP : SSPD / DLNA Server
func SSDP() (err error) {
	if !Settings.SSDP || System.Flag.Info {
		return
	}

	showInfo(fmt.Sprintf("SSDP / DLNA:%t", Settings.SSDP))

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, os.Interrupt, syscall.SIGTERM)

	// Debug SSDP
	if System.Flag.Debug == 3 {
		ssdp.Logger = log.New(os.Stderr, "[SSDP] ", log.LstdFlags)
	}

	ad, err := ssdp.Advertise(
		"upnp:rootdevice", // send as "ST"
		fmt.Sprintf("uuid:%s::upnp:rootdevice", System.DeviceID), // send as "USN"
		fmt.Sprintf("%s/device.xml", System.URLBase),             // send as "LOCATION"
		System.AppName, // send as "SERVER"
		1800)           // send as "maxAge" in "CACHE-CONTROL"

	if err != nil {
		return
	}

	go func(adv *ssdp.Advertiser) {
		aliveTick := time.Tick(300 * time.Second)

	loop:
		for {
			select {
			case <-aliveTick:
				err = adv.Alive()
				if err != nil {
					ShowError(err, 0)
					stopAdvertising(adv)
					break loop
				}

			case <-quit:
				stopAdvertising(adv)
				os.Exit(0)
				break loop
			}
		}
	}(ad)

	return
}

// stopAdvertising notifies the network that the device is going away and closes the advertiser.
func stopAdvertising(adv *ssdp.Advertiser) {
	if err := adv.Bye(); err != nil {
		ShowError(err, 0)
	}
	if err := adv.Close(); err != nil {
		ShowError(err, 0)
	}
}
