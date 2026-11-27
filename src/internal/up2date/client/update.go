package up2date

import (
	"archive/zip"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"syscall"
)

// DoUpdate : Update binary
func DoUpdate(fileType, filenameBIN string) (err error) {
	var url string
	switch fileType {
	case "bin":
		url = Updater.Response.UpdateBIN
	case "zip":
		url = Updater.Response.UpdateZIP
	}

	switch runtime.GOOS {
	case "windows":
		filenameBIN = filenameBIN + ".exe"
	}

	if len(url) > 0 {
		log.Println("["+strings.ToUpper(fileType)+"]", "New version ("+Updater.Name+"):", Updater.Response.Version)

		// Download new binary
		resp, err := http.Get(url)
		if err != nil {
			return err
		}
		defer closeOrLog(resp.Body)
		log.Println("["+strings.ToUpper(fileType)+"]", "Download new version...")

		if resp.StatusCode != http.StatusOK {
			log.Println("["+strings.ToUpper(fileType)+"]", "Download new version...OK")
			return fmt.Errorf("bad status: %s", resp.Status)
		}

		// Change binary filename to .filename
		binary, err := executablePath()
		if err != nil {
			return err
		}
		var filename = getFilenameFromPath(binary)
		var path = getPlatformPath(binary)
		var oldBinary = path + "_old_" + filename
		var newBinary = binary

		// ZIP
		var tmpFolder = path + "tmp"
		var tmpFile = tmpFolder + string(os.PathSeparator) + filenameBIN

		//fmt.Println(binary, path+"."+filename)
		if err = os.Rename(newBinary, oldBinary); err != nil {
			return err
		}

		// Save the new binary with the old file name
		out, err := os.Create(binary)
		if err != nil {
			restorOldBinary(oldBinary, newBinary)
			return err
		}

		// Write the body to file
		_, err = io.Copy(out, resp.Body)
		if closeErr := out.Close(); err == nil {
			err = closeErr
		}
		if err != nil {
			restorOldBinary(oldBinary, newBinary)
			return err
		}

		// Update as a ZIP file
		if fileType == "zip" {
			log.Println("["+strings.ToUpper(fileType)+"]", "Update file:", filenameBIN)
			log.Println("["+strings.ToUpper(fileType)+"]", "Unzip ZIP file...")
			err = extractZIP(binary, tmpFolder)

			binary = newBinary

			if err != nil {
				log.Println("["+strings.ToUpper(fileType)+"]", "Unzip ZIP file...ERROR")

				restorOldBinary(oldBinary, newBinary)

				return err
			} else {
				log.Println("["+strings.ToUpper(fileType)+"]", "Unzip ZIP file...OK")
				log.Println("["+strings.ToUpper(fileType)+"]", "Copy binary file...")

				err = copyFile(tmpFile, binary)
				if err == nil {
					log.Println("["+strings.ToUpper(fileType)+"]", "Copy binary file...OK")
				} else {
					log.Println("["+strings.ToUpper(fileType)+"]", "Copy binary file...ERROR")
					restorOldBinary(oldBinary, newBinary)

					return err
				}

				removeAllOrLog(tmpFolder)
			}
		}

		// Set the permission
		if err = os.Chmod(binary, 0755); err != nil {
			restorOldBinary(oldBinary, newBinary)
			return err
		}

		log.Println("["+strings.ToUpper(fileType)+"]", "Update Successful")

		// Restart binary (Windows)
		if runtime.GOOS == "windows" {
			bin, err := os.Executable()

			if err != nil {
				restorOldBinary(oldBinary, newBinary)
				return err
			}

			proc, err := start(bin)
			if err != nil {
				restorOldBinary(oldBinary, newBinary)
				return err
			}

			// The new process is running, so the current one has to exit to release the old binary
			if err := proc.Release(); err != nil {
				log.Println("[UPDATE]", err)
			}
			removeAllOrLog(oldBinary)
			os.Exit(0)
		} else {
			// Restart binary (Linux and UNIX)
			removeAllOrLog(oldBinary)
			err = syscall.Exec(binary, os.Args, os.Environ())
			if err != nil {
				restorOldBinary(oldBinary, newBinary)
				log.Fatal(err)
				return err
			}
		}
	}

	return
}

func start(args ...string) (p *os.Process, err error) {
	if args[0], err = exec.LookPath(args[0]); err == nil {
		//fmt.Println(args[0])
		var procAttr os.ProcAttr
		procAttr.Files = []*os.File{os.Stdin, os.Stdout, os.Stderr}
		p, err := os.StartProcess(args[0], args, &procAttr)

		if err == nil {
			return p, nil
		}
	}

	return nil, err
}

func restorOldBinary(oldBinary, newBinary string) {
	removeAllOrLog(newBinary)
	if err := os.Rename(oldBinary, newBinary); err != nil {
		log.Println("[UPDATE] Restore old binary:", err)
	}
}

func getFilenameFromPath(path string) string {
	file := filepath.Base(path)

	return file
}

func getPlatformPath(path string) string {
	var newPath = filepath.Dir(path) + string(os.PathSeparator)

	return newPath
}

func copyFile(src, dst string) (err error) {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer closeOrLog(in)

	out, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer closeKeepErr(out, &err)

	_, err = io.Copy(out, in)
	return err
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
	path := filepath.Join(target, file.Name)

	// Reject entries like "../../etc/passwd" that would be written outside of the target folder
	if !strings.HasPrefix(path, filepath.Clean(target)+string(os.PathSeparator)) {
		return fmt.Errorf("invalid file path in archive: %s", file.Name)
	}

	if file.FileInfo().IsDir() {
		return os.MkdirAll(path, file.Mode())
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

// closeOrLog closes c and logs a failure. Use it for resources that were only read from.
func closeOrLog(c io.Closer) {
	if err := c.Close(); err != nil {
		log.Println("[UPDATE]", err)
	}
}

// closeKeepErr closes c and stores the Close error in *err, unless *err already
// holds an earlier error. Use it with defer for files that were written to.
func closeKeepErr(c io.Closer, err *error) {
	if closeErr := c.Close(); closeErr != nil && *err == nil {
		*err = closeErr
	}
}

// removeAllOrLog removes path and everything it contains and logs a failure.
func removeAllOrLog(path string) {
	if err := os.RemoveAll(path); err != nil {
		log.Println("[UPDATE]", err)
	}
}

// executablePath returns the absolute path of the running binary with symlinks resolved.
func executablePath() (string, error) {
	path, err := os.Executable()
	if err != nil {
		return "", err
	}
	return filepath.EvalSymlinks(path)
}
