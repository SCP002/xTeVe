package buffer

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"math"
	"net/http"
	"path"
	"slices"
	"strings"
	"time"

	"xteve/src/internal/config"
	"xteve/src/internal/logger"
)

// download is the built-in xTeVe buffer: it downloads the stream of s (MPEG-TS, or the segments of an HLS
// playlist) into segment files until the last client leaves.
func (m *Manager) download(s *stream) {
	defer close(s.done)
	defer m.removeFolder(s)

	if err := s.vfs.MkdirAll(s.folder, 0755); err != nil {
		m.cfg.Error(err, 0)
		s.fail(err)
		return
	}
	logger.Debug(1, fmt.Sprintf("Create virtual filesystem Folder:%s", s.folder))

	var networkBandwidth = config.Get().M3U8AdaptiveBandwidthMBPS * 1e+6
	var bufferSize = config.Get().BufferSize

	// st is the state of the download, only used here
	var st = ThisStream{URL: s.url, Folder: s.folder, stop: s.stop}
	var tmpSegment = 1
	var timeOut = 0
	var bandwidth bandwidthCalculation

	// defaultSegment starts again with the stream URL (or where it redirected to)
	var defaultSegment = func() {
		var segment Segment
		if len(st.Location) > 0 {
			segment.URL = st.Location
		} else {
			segment.URL = st.URL
		}

		st.Segment = []Segment{segment}
		st.HLS = false
		st.Sequence = 0
		st.Wait = 0
		st.NetworkBandwidth = networkBandwidth

		timeOut++
	}

	for !s.stopped() {
		defaultSegment()
		if timeOut >= 10 {
			return
		}

		if !st.Status {
			if strings.Contains(st.URL, ".m3u8") {
				logger.Info("Streaming Type:" + "[HLS / M3U8]")
			} else {
				logger.Info("Streaming Type:" + "[TS]")
			}
			logger.Info("Streaming URL:" + st.URL)
		}

		st.TimeStart = time.Now()
		bandwidth.Start = st.TimeStart
		bandwidth.Size = 0

		// Download the segments, an empty list starts again with the stream URL
		for len(st.Segment) > 0 && !s.stopped() {
			var segment = st.Segment[0]
			var currentURL = strings.Trim(segment.URL, "\r\n")
			if len(currentURL) == 0 {
				break
			}

			resp, req, err := m.get(s, &st, currentURL)
			if err != nil {
				return
			}
			currentURL = req.URL.String()

			var contentType = resp.Header.Get("Content-Type")

			if resp.StatusCode != http.StatusOK {
				logger.Info("Content Type:" + contentType)
				logger.Info(fmt.Sprintf("Streaming Status:HTTP Response Status [%d] %s", resp.StatusCode,
					http.StatusText(resp.StatusCode)))
				logger.Info("Error with this URL:" + currentURL)

				var err = errors.New(http.StatusText(resp.StatusCode))
				m.cfg.Error(err, 4004)
				s.fail(err)
				closeBody(resp)
				return
			}

			// Read out information about the streaming server
			if !st.Status {
				if len(st.URLStreamingServer) == 0 {
					var u = req.URL

					st.URLScheme = u.Scheme
					st.URLHost = req.Host
					st.URLPath = u.Path
					st.URLFile = path.Base(u.Path)

					st.URLRedirect = fmt.Sprintf("%s://%s%s", st.URLScheme, st.URLHost, st.URLPath)
					st.URLStreamingServer = fmt.Sprintf("%s://%s", st.URLScheme, st.URLHost)
				}

				logger.Debug(1, fmt.Sprintf("Server URL:%s", st.URLStreamingServer))
				logger.Debug(1, fmt.Sprintf("Temp Folder:%s", st.Folder))
				logger.Info(fmt.Sprintf("Streaming Status:HTTP Response Status [%d] %s", resp.StatusCode,
					http.StatusText(resp.StatusCode)))
				logger.Info("Content Type:" + contentType)
			} else {
				logger.Debug(2, fmt.Sprintf("Content Type:%s", contentType))
			}

			contentType = streamType(resp, contentType)

			switch {
			// M3U8 Playlist
			case slices.Contains(m3u8ContentTypes, contentType):
				body, err := io.ReadAll(resp.Body)
				if err != nil {
					m.cfg.Error(err, 0)
					s.fail(err)
				}

				st.Body = string(body)
				st.HLS = true
				st.M3U8URL = currentURL

				if err = parseM3U8(&st); err != nil {
					m.cfg.Error(err, 4050)
					s.fail(err)
				}

			// Video Stream (TS)
			case slices.Contains(tsContentTypes, contentType):
				if !m.writeSegments(s, &st, resp, bufferSize, &tmpSegment, &bandwidth, &networkBandwidth) {
					closeBody(resp)
					return
				}
				timeOut = 0

			// Unknown Format
			default:
				logger.Info("Content Type:" + resp.Header.Get("Content-Type"))
				var err = errors.New("streaming error")
				m.cfg.Error(err, 4003)
				s.fail(err)
				closeBody(resp)
				return
			}

			// Calculate the waiting time for the Download of the next Segment
			if st.HLS && segment.Duration > 0 {
				st.TimeEnd = time.Now()
				st.TimeDiff = st.TimeEnd.Sub(st.TimeStart).Seconds()

				var sleep = max((segment.Duration-st.TimeDiff)-(segment.Duration*0.25), 0)

				logger.Debug(1, fmt.Sprintf(
					"HLS Status:Download time: %f s | Segment duration: %f s | Sleep: %f s Sequence: %d",
					st.TimeDiff, segment.Duration, sleep, segment.Sequence,
				))

				if sleep > 0 && !s.sleep(time.Duration(math.Ceil(sleep*10))*100*time.Millisecond) {
					closeBody(resp)
					return
				}
			}

			st.Segment = st.Segment[1:]
			closeBody(resp)
		}
	}
}

// get requests url and follows redirects, also relative ones. On an error it fails the stream.
func (m *Manager) get(s *stream, st *ThisStream, url string) (*http.Response, *http.Request, error) {
	var client = &http.Client{
		CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse },
	}

	for range 10 {
		logger.Debug(2, fmt.Sprintf("Connection to:%s", url))

		req, err := http.NewRequest(http.MethodGet, url, nil)
		if err != nil {
			m.cfg.Error(err, 0)
			s.fail(err)
			return nil, nil, err
		}
		req.Header.Set("User-Agent", config.Get().UserAgent)
		req.Header.Set("Connection", "close")
		req.Header.Set("Accept", "*/*")
		debugRequest(req)

		resp, err := client.Do(req)
		if err != nil {
			err = fmt.Errorf("no response from streaming server: %s: %w", url, err)
			m.cfg.Error(err, 0)
			s.fail(err)
			return nil, nil, err
		}

		if resp.StatusCode < 301 || resp.StatusCode > 308 {
			return resp, req, nil
		}

		// Redirect
		debugResponse(resp)
		logger.Debug(2, fmt.Sprintf("Streaming Status:HTTP response status [%d] %s", resp.StatusCode,
			http.StatusText(resp.StatusCode)))
		closeBody(resp)

		var location = strings.Trim(resp.Header.Get("Location"), "\r\n")
		if len(location) == 0 {
			var err = errors.New("streaming server")
			m.cfg.Error(err, 4002)
			s.fail(err)
			return nil, nil, err
		}

		// The location can be relative to the requested URL
		next, err := req.URL.Parse(location)
		if err != nil {
			m.cfg.Error(err, 4002)
			s.fail(err)
			return nil, nil, err
		}

		url = next.String()
		st.Location = url
		logger.Debug(2, fmt.Sprintf("HTTP Redirect:%s", st.Location))
	}

	var err = errors.New("too many redirects")
	m.cfg.Error(err, 4002)
	s.fail(err)
	return nil, nil, err
}

// writeSegments writes the TS stream of resp into segment files of half the buffer size. It reports false if the
// download has to end: the stream was stopped or failed.
func (m *Manager) writeSegments(
	s *stream, st *ThisStream, resp *http.Response, bufferSize int, tmpSegment *int, bandwidth *bandwidthCalculation,
	networkBandwidth *int,
) bool {
	var buffer = make([]byte, 1024*bufferSize*2)
	var tmpFileSize = 1024 * bufferSize

	logger.Debug(3, fmt.Sprintf("Buffer Size:%d KB [SERVER CONNECTION]", len(buffer)/1024))
	logger.Debug(3, fmt.Sprintf("Buffer Size:%d KB [CLIENT CONNECTION]", tmpFileSize/1024))

	var tmpFile = fmt.Sprintf("%s%d.ts", st.Folder, *tmpSegment)

	bufferFile, err := s.vfs.Create(tmpFile)
	if err != nil {
		m.cfg.Error(err, 0)
		s.fail(err)
		return false
	}

	var fileSize int

	for {
		if fileSize == 0 {
			logger.Debug(2, fmt.Sprintf("Buffer Status:Buffering (%s)", tmpFile))
		}

		// Fill the Buffer with data from the Server
		n, err := resp.Body.Read(buffer)
		if err != nil && err != io.EOF {
			m.cfg.Error(err, 0)
			s.fail(err)
			closeFile(bufferFile)
			return false
		}

		if _, err := bufferFile.Write(buffer[:n]); err != nil {
			m.cfg.Error(err, 0)
			s.fail(err)
			closeFile(bufferFile)
			return false
		}

		fileSize += n

		if s.stopped() {
			closeFile(bufferFile)
			return false
		}

		// Save the buffer
		if fileSize >= tmpFileSize/2 || n == 0 {
			bandwidth.Stop = time.Now()
			bandwidth.Size += fileSize
			bandwidth.TimeDiff = bandwidth.Stop.Sub(bandwidth.Start).Seconds()

			// Used for the next request of an HLS playlist
			*networkBandwidth = int(float64(bandwidth.Size) / bandwidth.TimeDiff * 1000)

			logger.Debug(2, fmt.Sprintf("Buffer Status:Done (%s)", tmpFile))

			// The segment is complete only if it was closed without errors
			if err := bufferFile.Close(); err != nil {
				m.cfg.Error(err, 0)
				s.fail(err)
				return false
			}

			st.Status = true
			s.ready.Store(true)

			*tmpSegment++
			tmpFile = fmt.Sprintf("%s%d.ts", st.Folder, *tmpSegment)

			bufferFile, err = s.vfs.Create(tmpFile)
			if err != nil {
				m.cfg.Error(err, 0)
				s.fail(err)
				return false
			}

			fileSize = 0

			if n == 0 {
				closeFile(bufferFile)
				return true
			}
		}
	}
}

// streamType returns the content type of resp; if the server sent none or an unknown one, the first bytes decide.
func streamType(resp *http.Response, contentType string) string {
	if len(contentType) > 0 {
		var ct, _, _ = strings.Cut(contentType, ";")
		contentType = strings.TrimSpace(strings.ToLower(ct))
	}

	if !slices.Contains(m3u8ContentTypes, contentType) && !slices.Contains(tsContentTypes, contentType) {
		var body = bufio.NewReaderSize(resp.Body, 188*2)
		var start, _ = body.Peek(188 + 1)
		resp.Body = readCloser{body, resp.Body}

		if sniffed := sniffStreamType(start); sniffed != "" {
			logger.Debug(1, fmt.Sprintf("Content Type:%q is treated as %s", contentType, sniffed))
			contentType = sniffed
		}
	}

	return contentType
}

// removeFolder removes the segment files of the stream.
func (m *Manager) removeFolder(s *stream) {
	logger.Debug(1, fmt.Sprintf("Remove tmp folder:%s", s.folder))
	if err := s.vfs.RemoveAll(s.folder); err != nil {
		m.cfg.Error(err, 4005)
	}
}

type bandwidthCalculation struct {
	Size     int
	Start    time.Time
	Stop     time.Time
	TimeDiff float64
}

type readCloser struct {
	io.Reader
	io.Closer
}

func closeBody(resp *http.Response) {
	if err := resp.Body.Close(); err != nil {
		logger.Debug(1, fmt.Sprintf("Close:%s", err))
	}
}

func closeFile(f io.Closer) {
	if err := f.Close(); err != nil {
		logger.Error(err.Error())
	}
}

func debugRequest(req *http.Request) {
	const level = 3
	if logger.Default().DebugLevel() < level {
		return
	}

	logger.Debug(level, "Request:Begin HTTP(S) request")
	logger.Debug(level, fmt.Sprintf("Method:%s", req.Method))
	logger.Debug(level, fmt.Sprintf("Proto:%s", req.Proto))
	logger.Debug(level, fmt.Sprintf("URL:%s", req.URL))
	for name, headers := range req.Header {
		for _, h := range headers {
			logger.Debug(level, fmt.Sprintf("Header:%v: %v", strings.ToLower(name), h))
		}
	}
	logger.Debug(level, "Request:End HTTP(S) request")
}

func debugResponse(resp *http.Response) {
	const level = 3
	if logger.Default().DebugLevel() < level {
		return
	}

	logger.Debug(level, "Response:Begin HTTP(S) response")
	logger.Debug(level, fmt.Sprintf("Proto:%s", resp.Proto))
	logger.Debug(level, fmt.Sprintf("Status Code:%d", resp.StatusCode))
	logger.Debug(level, fmt.Sprintf("Status Text:%s", http.StatusText(resp.StatusCode)))
	for name, headers := range resp.Header {
		for _, h := range headers {
			logger.Debug(level, fmt.Sprintf("Header:%v: %v", strings.ToLower(name), h))
		}
	}
	logger.Debug(level, "Response:End HTTP(S) response")
}
