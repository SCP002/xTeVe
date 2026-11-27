package buffer

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"xteve/src/internal/config"
	"xteve/src/internal/logger"
)

// thirdPartyBuffer streams s through FFmpeg or VLC: the process reads the stream URL and writes MPEG-TS to its
// standard output, which is written into segment files until the last client leaves.
func (m *Manager) thirdPartyBuffer(s *stream) {
	defer close(s.done)
	defer m.removeFolder(s)

	var settings = config.Get()
	var bufferSize = settings.BufferSize * 1024
	var bufferType = strings.ToUpper(settings.Buffer)
	var path, options string

	switch settings.Buffer {
	case "ffmpeg":
		path, options = settings.FFmpegPath, settings.FFmpegOptions
	case "vlc":
		path, options = settings.VLCPath, settings.VLCOptions
	default:
		return
	}

	if err := s.vfs.MkdirAll(s.folder, 0755); err != nil {
		m.cfg.Error(err, 0)
		s.fail(err)
		return
	}

	if _, err := os.Stat(path); err != nil {
		m.cfg.Error(err, 0)
		s.fail(err)
		return
	}

	logger.Info(fmt.Sprintf("%s path:%s", bufferType, path))
	logger.Info("Streaming URL:" + s.url)

	var cmd = exec.Command(path, thirdPartyArgs(bufferType, options, s.url, settings.UserAgent)...)
	logger.Debug(1, fmt.Sprintf("%s:%s %s", bufferType, path, cmd.Args[1:]))

	// Byte-Data from the Process
	stdOut, err := cmd.StdoutPipe()
	if err != nil {
		m.cfg.Error(err, 0)
		s.fail(err)
		return
	}

	// Log-Data from the Process
	logOut, err := cmd.StderrPipe()
	if err != nil {
		m.cfg.Error(err, 0)
		s.fail(err)
		return
	}

	logger.Info(bufferType + ":Processing data")

	if err := cmd.Start(); err != nil {
		m.cfg.Error(err, 0)
		s.fail(err)
		return
	}

	// Every exit path below must stop the process; stop() is safe to call more than once
	var stop = sync.OnceFunc(func() { stopProcess(cmd) })
	defer stop()

	var finished = make(chan struct{})
	defer close(finished)

	// The process is stopped when the last client left, also while it sends no data; and when it sends no data
	// within 20 seconds after the start
	var timedOut atomic.Bool
	go func() {
		var timeout = time.NewTimer(20 * time.Second)
		defer timeout.Stop()

		select {
		case <-s.stop:
			stop()
		case <-timeout.C:
			if !s.ready.Load() {
				timedOut.Store(true)
				stop()
			}
			select {
			case <-s.stop:
				stop()
			case <-finished:
			}
		case <-finished:
		}
	}()

	go func() {
		// Show the log of the process, as debug messages once data arrives
		var scanner = bufio.NewScanner(logOut)
		for scanner.Scan() {
			var line = fmt.Sprintf("%s log:%s", bufferType, strings.TrimSpace(scanner.Text()))
			if s.ready.Load() {
				logger.Debug(1, line)
			} else {
				logger.Info(line)
			}
		}

		// The pipe is closed when the process is stopped, that is the normal end
		if err := scanner.Err(); err != nil && !errors.Is(err, os.ErrClosed) {
			logger.Debug(1, fmt.Sprintf("%s log:%s", bufferType, err))
		}
	}()

	logger.Info("Streaming Status:Receive data from " + bufferType)

	var tmpSegment = 1
	var tmpFile = fmt.Sprintf("%s%d.ts", s.folder, tmpSegment)

	f, err := s.vfs.Create(tmpFile)
	if err != nil {
		m.cfg.Error(err, 0)
		s.fail(err)
		return
	}

	var buffer = make([]byte, 1024*4)
	var reader = bufio.NewReader(stdOut)
	var fileSize = 0

	for {
		n, err := reader.Read(buffer)
		if err != nil {
			if err != io.EOF && !s.stopped() {
				m.cfg.Error(err, 0)
			}
			break
		}

		fileSize += n

		if _, err := f.Write(buffer[:n]); err != nil {
			closeFile(f)
			m.cfg.Error(err, 0)
			s.fail(err)
			return
		}

		if fileSize >= bufferSize/2 {
			if tmpSegment == 1 {
				logger.Info(fmt.Sprintf("Streaming Status:Buffering data from %s", bufferType))
			}

			// The segment is complete only if it was closed without errors
			if err := f.Close(); err != nil {
				m.cfg.Error(err, 0)
				s.fail(err)
				return
			}
			s.ready.Store(true)

			tmpSegment++
			tmpFile = fmt.Sprintf("%s%d.ts", s.folder, tmpSegment)
			fileSize = 0

			f, err = s.vfs.Create(tmpFile)
			if err != nil {
				m.cfg.Error(err, 0)
				s.fail(err)
				return
			}
		}
	}

	stop()
	closeFile(f)

	if s.stopped() {
		return
	}

	if timedOut.Load() {
		err = errors.New("timeout")
		m.cfg.Error(err, 4006)
	} else {
		err = errors.New(bufferType + " error")
		m.cfg.Error(err, 1204)
	}
	s.fail(err)
}

// thirdPartyArgs returns the arguments for FFmpeg or VLC: the options with [URL] replaced and the user agent added.
func thirdPartyArgs(bufferType, options, url, userAgent string) []string {
	var args []string

	for i, a := range strings.Split(options, " ") {
		switch bufferType {
		case "FFMPEG":
			if i == 0 && len(userAgent) != 0 {
				args = []string{"-user_agent", userAgent}
			}
			args = append(args, strings.ReplaceAll(a, "[URL]", url))

		case "VLC":
			args = append(args, a)
			if a == "[URL]" {
				args[len(args)-1] = url
				if len(userAgent) != 0 {
					args = append(args, fmt.Sprintf(":http-user-agent=%s", userAgent))
				}
			}
		}
	}

	return args
}

// stopProcess kills the process and waits for it, so that it does not remain as a zombie.
func stopProcess(cmd *exec.Cmd) {
	if err := cmd.Process.Kill(); err != nil && !errors.Is(err, os.ErrProcessDone) {
		logger.Error(err.Error())
	}

	// A killed process always exits with an *exec.ExitError, so only other errors are worth reporting
	if err := cmd.Wait(); err != nil {
		if _, ok := errors.AsType[*exec.ExitError](err); !ok {
			logger.Error(err.Error())
		}
	}
}
