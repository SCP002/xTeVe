package src

import (
	"archive/zip"
	"bytes"
	"compress/gzip"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

func zipFiles(sourceFiles []string, target string) (err error) {
	zipfile, err := os.Create(target)
	if err != nil {
		return err
	}
	defer closeKeepErr(zipfile, &err)

	// Closing the archive writes the ZIP central directory, so it must happen
	// before the file is closed and its error must not be lost.
	archive := zip.NewWriter(zipfile)
	defer closeKeepErr(archive, &err)

	for _, source := range sourceFiles {
		info, err := os.Stat(source)
		if os.IsNotExist(err) {
			continue
		}
		if err != nil {
			return err
		}

		var baseDir string
		if info.IsDir() {
			baseDir = filepath.Base(System.Folder.Data)
		}

		err = filepath.Walk(source, func(path string, info os.FileInfo, err error) error {
			if err != nil {
				return err
			}

			header, err := zip.FileInfoHeader(info)
			if err != nil {
				return err
			}

			// ZIP entry names always use "/", whatever the platform
			if baseDir != "" {
				header.Name = filepath.ToSlash(filepath.Join(strings.TrimPrefix(path, System.Folder.Config)))
			}

			if info.IsDir() {
				header.Name += "/"
			} else {
				header.Method = zip.Deflate
			}

			writer, err := archive.CreateHeader(header)
			if err != nil {
				return err
			}

			if info.IsDir() {
				return nil
			}

			file, err := os.Open(path)
			if err != nil {
				return err
			}
			defer closeOrLog(file)

			_, err = io.Copy(writer, file)

			return err
		})
		if err != nil {
			return err
		}
	}

	return nil
}

func extractZIP(archive, target string) (err error) {
	reader, err := zip.OpenReader(archive)
	if err != nil {
		return err
	}
	defer closeOrLog(reader)

	if err := os.MkdirAll(target, 0755); err != nil {
		return err
	}

	for _, file := range reader.File {
		if err = extractZIPFile(file, target); err != nil {
			return err
		}
	}

	return nil
}

func extractZIPFile(file *zip.File, target string) (err error) {
	// Backups made on Windows by older versions use "\" in entry names
	var name = strings.ReplaceAll(file.Name, `\`, "/")
	path := filepath.Join(target, filepath.FromSlash(name))

	// Reject entries like "../../etc/passwd" that would be written outside of the target folder
	if !strings.HasPrefix(path, filepath.Clean(target)+string(os.PathSeparator)) {
		return fmt.Errorf("invalid file path in archive: %s", file.Name)
	}

	if file.FileInfo().IsDir() || strings.HasSuffix(name, "/") {
		return os.MkdirAll(path, file.Mode())
	}

	// Archives made by other tools do not always contain entries for folders
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return err
	}

	fileReader, err := file.Open()
	if err != nil {
		return err
	}
	defer closeOrLog(fileReader)

	targetFile, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, file.Mode())
	if err != nil {
		return err
	}
	defer closeKeepErr(targetFile, &err)

	_, err = io.Copy(targetFile, fileReader)

	return err
}

func extractGZIP(gzipBody []byte, fileSource string) (body []byte, err error) {
	var b = bytes.NewBuffer(gzipBody)

	var r io.Reader
	r, err = gzip.NewReader(b)
	if err != nil {
		// Not a gzip file
		body = gzipBody
		err = nil
		return
	}

	showInfo("Extract gzip:" + fileSource)

	var resB bytes.Buffer
	_, err = resB.ReadFrom(r)
	if err != nil {
		body = gzipBody
		err = nil
		return
	}

	body = resB.Bytes()
	return
}
