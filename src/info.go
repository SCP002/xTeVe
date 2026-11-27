package src

import (
	"fmt"
	"strings"
	"xteve/src/internal/config"
)

// ShowSystemInfo : View System Information
func ShowSystemInfo() {
	fmt.Print("Creating the information takes a moment...")
	err := buildDatabaseDVR()
	if err != nil {
		ShowError(err, 0)
		return
	}

	buildXEPG(false)

	fmt.Println("OK")
	fmt.Println()

	fmt.Printf("Version:             %s %s\n", System.Name, System.Version)
	fmt.Printf("Branch:              %s\n", System.Branch)
	fmt.Printf(
		"GitHub:              %s/%s | Git update = %t\n",
		System.GitHub.User,
		System.GitHub.Repo,
		System.GitHub.Update,
	)
	fmt.Printf("Folder (config):     %s\n", System.Folder.Config)

	fmt.Printf("Streams:             %d / %d\n", len(streamsDB.Get().Active), len(streamsDB.Get().All))
	fmt.Printf("Filter:              %d\n", len(streamsDB.Get().Filters))
	fmt.Printf("XEPG Chanels:        %d\n", int(Data.XEPG.XEPGCount))

	fmt.Println()
	fmt.Println("IPv4 Addresses:")

	for i, ip := range System.IPAddressesV4 {
		fmt.Printf("  %-19s%s\n", fmt.Sprintf("%d.", i), ip)
	}

	fmt.Println()
	fmt.Println("IPv6 Addresses:")

	for i, ip := range System.IPAddressesV6 {
		fmt.Printf("  %-19s%s\n", fmt.Sprintf("%d.", i), ip)
	}

	fmt.Println("---")

	fmt.Println("Settings [General]")
	fmt.Printf("xTeVe Update:        %t\n", config.Get().XteveAutoUpdate)
	fmt.Printf("UUID:                %s\n", config.Get().UUID)
	fmt.Printf("Tuner (Plex / Emby): %d\n", config.Get().Tuner)
	fmt.Printf("EPG Source:          %s\n", config.Get().EpgSource)

	fmt.Println("---")

	fmt.Println("Settings [Files]")
	fmt.Printf("Schedule:            %s\n", strings.Join(config.Get().Update, ","))
	fmt.Printf("Files Update:        %t\n", config.Get().FilesUpdate)
	fmt.Printf("Folder (tmp):        %s\n", config.Get().TempPath)
	fmt.Printf("Image Chaching:      %t\n", config.Get().CacheImages)
	fmt.Printf("Replace EPG Image:   %t\n", config.Get().XepgReplaceMissingImages)

	fmt.Println("---")

	fmt.Println("Settings [Streaming]")
	fmt.Printf("Buffer:              %s\n", config.Get().Buffer)
	fmt.Printf("UDPxy:               %s\n", config.Get().UDPxy)
	fmt.Printf("Buffer Size:         %d KB\n", config.Get().BufferSize)
	fmt.Printf("Timeout:             %d ms\n", int(config.Get().BufferTimeout))
	fmt.Printf("User Agent:          %s\n", config.Get().UserAgent)

	fmt.Println("---")

	fmt.Println("Settings [Backup]")
	fmt.Printf("Folder (backup):     %s\n", config.Get().BackupPath)
	fmt.Printf("Backup Keep:         %d\n", config.Get().BackupKeep)
}
