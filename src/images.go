package src

import (
	b64 "encoding/base64"
	"fmt"
	"path/filepath"
	"slices"
	"strings"
)

// logoExtensions are the image types that can be uploaded as logos. SVG is not among them: it is served from the
// address of the web UI and could run scripts there.
var logoExtensions = []string{".png", ".jpg", ".jpeg", ".gif", ".webp"}

func uploadLogo(input, filename string) (logoURL string, err error) {
	// Only the name of the file counts: a path like "../settings.json" must not leave the folder of the logos
	filename = filepath.Base(filepath.Clean("/" + strings.ReplaceAll(filename, "\\", "/")))
	if filename == "/" || filename == "." || !slices.Contains(logoExtensions, strings.ToLower(filepath.Ext(filename))) {
		return "", fmt.Errorf("logo %q: only %s files can be uploaded", filename, strings.Join(logoExtensions, ", "))
	}

	b64data := input[strings.IndexByte(input, ',')+1:]

	// Convert Base64 into bytes and save
	sDec, err := b64.StdEncoding.DecodeString(b64data)
	if err != nil {
		return
	}

	var file = fmt.Sprintf("%s%s", System.Folder.ImagesUpload, filename)

	err = writeByteToFile(file, sDec)
	if err != nil {
		return
	}

	logoURL = fmt.Sprintf("%s://%s/data_images/%s", System.Protocol(), System.Domain(), filename)

	return
}
