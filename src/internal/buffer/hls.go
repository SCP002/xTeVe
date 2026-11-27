package buffer

import (
	"bytes"
	"errors"
	"fmt"
	"math"
	"net/url"
	"path"
	"sort"
	"strconv"
	"strings"
	"time"

	"xteve/src/internal/errcode"
	"xteve/src/internal/logger"
)

// ThisStream is the state of a download, used only by its downloader.
type ThisStream struct {
	ChannelName      string
	Error            string
	Folder           string
	MD5              string
	NetworkBandwidth int
	PlaylistID       string
	PlaylistName     string
	Status           bool
	URL              string

	Segment []Segment

	// Server information
	Location           string
	URLFile            string
	URLHost            string
	URLPath            string
	URLRedirect        string
	URLScheme          string
	URLStreamingServer string

	// Is only used for HLS / M3U8
	Body             string
	Difference       float64
	Duration         float64
	DynamicBandwidth bool
	FirstSequence    int64
	HLS              bool
	LastSequence     int64
	M3U8URL          string
	NewSegCount      int
	OldSegCount      int
	Sequence         int64
	TimeDiff         float64
	TimeEnd          time.Time
	TimeSegDuration  float64
	TimeStart        time.Time
	Version          int
	Wait             float64

	// stop is closed when the stream stops, see stream.stop
	stop <-chan struct{}

	DynamicStream map[int]DynamicStream

	// Local Temp Files
	OldSegments []string
}
type Segment struct {
	Duration     float64
	Info         bool
	PlaylistType string
	Sequence     int64
	URL          string
	Version      int
	Wait         float64

	StreamInf struct {
		AverageBandwidth int
		Bandwidth        int
		Framerate        float64
		Resolution       string
		SegmentURL       string
	}
}
type DynamicStream struct {
	AverageBandwidth int
	Bandwidth        int
	Framerate        float64
	Resolution       string
	URL              string
}

// Content types of HLS playlists
var m3u8ContentTypes = []string{
	"application/x-mpegurl", "application/mpegurl", "application/vnd.apple.mpegurl", "audio/mpegurl",
	"audio/x-mpegurl",
}

// Content types of streams that are passed through as they are
var tsContentTypes = []string{
	"video/mpeg", "video/mp4", "video/mp2t", "video/m2ts", "application/octet-stream", "binary/octet-stream",
	"application/mp2t", "video/x-matroska",
}

// sniffStreamType returns the content type of a stream judging by its first bytes: an HLS playlist or an MPEG-TS
// stream (sync byte 0x47 every 188 bytes). It returns "" if it is neither.
func sniffStreamType(start []byte) string {
	if bytes.HasPrefix(bytes.TrimLeft(start, "\ufeff \t\r\n"), []byte("#EXTM3U")) {
		return "application/vnd.apple.mpegurl"
	}
	if len(start) > 188 && start[0] == 0x47 && start[188] == 0x47 {
		return "video/mp2t"
	}
	return ""
}

func parseM3U8(stream *ThisStream) (err error) {
	var noNewSegment = false
	var lastSegmentDuration float64
	var segment Segment
	var m3u8Segments []Segment
	var sequence int64

	stream.DynamicBandwidth = false

	logger.Debug(3, fmt.Sprintf(`M3U8 Playlist:`+"\n"+`%s`, stream.Body))

	var getBandwidth = func(line string) int {
		var infos = strings.Split(line, ",")

		for _, info := range infos {
			if strings.Contains(info, "BANDWIDTH=") {
				var bandwidth = strings.ReplaceAll(info, "BANDWIDTH=", "")
				n, err := strconv.Atoi(bandwidth)
				if err == nil {
					return n
				}
			}
		}

		return 0
	}

	var parseParameter = func(line string, segment *Segment) (err error) {
		line = strings.Trim(line, "\r\n")

		var parameters = []string{
			"#EXT-X-VERSION:", "#EXT-X-PLAYLIST-TYPE:", "#EXT-X-MEDIA-SEQUENCE:", "#EXT-X-STREAM-INF:", "#EXTINF:",
		}

		for _, parameter := range parameters {
			if strings.Contains(line, parameter) {
				var value = strings.ReplaceAll(line, parameter, "")

				switch parameter {
				case "#EXT-X-VERSION:":
					version, err := strconv.Atoi(value)
					if err == nil {
						segment.Version = version
					}

				case "#EXT-X-PLAYLIST-TYPE:":
					segment.PlaylistType = value

				case "#EXT-X-MEDIA-SEQUENCE:":
					n, err := strconv.ParseInt(value, 10, 64)
					if err == nil {
						stream.Sequence = n
						sequence = n
					}

				case "#EXT-X-STREAM-INF:":
					segment.Info = true
					segment.StreamInf.Bandwidth = getBandwidth(value)

				case "#EXTINF:":
					var d = strings.Split(value, ",")
					if len(d) > 0 {
						value = strings.ReplaceAll(d[0], ",", "")
						duration, err := strconv.ParseFloat(value, 64)
						if err == nil {
							segment.Duration = duration
						} else {
							return errcode.With(err, 1050)
						}
					}
				}
			}
		}

		return
	}

	var parseURL = func(line string, segment *Segment) {
		// Check if the address is a valid URL (http://... or /path/to/stream)
		u, err := url.ParseRequestURI(line)
		if err == nil {
			if len(u.Host) == 0 {
				// Check whether the domain is included in the address
				segment.URL = stream.URLStreamingServer + line
			} else {
				// Domain included in the address
				segment.URL = line
			}
		} else {
			// not URL, but a file path (media/file-01.ts)
			var serverURLPath = strings.ReplaceAll(stream.M3U8URL, path.Base(stream.M3U8URL), line)
			segment.URL = serverURLPath
		}
	}

	if strings.Contains(stream.Body, "#EXTM3U") {
		var lines = strings.Split(strings.ReplaceAll(stream.Body, "\r\n", "\n"), "\n")

		if !stream.DynamicBandwidth {
			stream.DynamicStream = make(map[int]DynamicStream)
		}

		// Parse Parameters
		for _, line := range lines {
			if len(line) > 0 {
				if line[0:1] == "#" {
					err := parseParameter(line, &segment)
					if err != nil {
						return err
					}

					lastSegmentDuration = segment.Duration
				}

				// M3U8 contains several links to additional M3U8 Playlists (Bandwidth option)
				if segment.Info && len(line) > 0 && line[0:1] != "#" {
					var dynamicStream DynamicStream

					segment.Duration = 0
					noNewSegment = false

					stream.DynamicBandwidth = true
					parseURL(line, &segment)

					dynamicStream.Bandwidth = segment.StreamInf.Bandwidth
					dynamicStream.URL = segment.URL

					stream.DynamicStream[dynamicStream.Bandwidth] = dynamicStream
				}

				// Segment with TS Stream
				if segment.Duration > 0 && line[0:1] != "#" {
					parseURL(line, &segment)

					if len(segment.URL) > 0 {
						segment.Sequence = sequence
						m3u8Segments = append(m3u8Segments, segment)
						sequence++
					}
				}
			}
		}
	} else {
		err = errcode.With(errors.New("#EXTM3U header is missing"), 4051)
		return
	}

	if len(m3u8Segments) > 0 {
		noNewSegment = true

		if !stream.Status {
			if len(m3u8Segments) >= 2 {
				m3u8Segments = m3u8Segments[0 : len(m3u8Segments)-1]
			}
		}

		for _, s := range m3u8Segments {
			segment = s

			if !stream.Status {
				noNewSegment = false
				stream.LastSequence = segment.Sequence

				// Stream is of type VOD. The first segment of the M3U8 playlist must be used.
				if strings.ToUpper(segment.PlaylistType) == "VOD" {
					break
				}
			} else {
				if segment.Sequence > stream.LastSequence {
					stream.LastSequence = segment.Sequence
					noNewSegment = false
					break
				}
			}
		}
	}

	if !noNewSegment {
		if stream.DynamicBandwidth {
			if err := switchBandwidth(stream); err != nil {
				logger.Error(err.Error())
			}
		} else {
			stream.Segment = append(stream.Segment, segment)
		}
	}

	if noNewSegment {
		var sleep = lastSegmentDuration * 0.5

		// Wait for new segments, a stopped stream ends the wait early
		var t = time.NewTimer(time.Duration(math.Ceil(sleep*10)) * 100 * time.Millisecond)
		select {
		case <-stream.stop:
		case <-t.C:
		}
		t.Stop()
	}

	return
}

func switchBandwidth(stream *ThisStream) (err error) {
	var bandwidth []int
	var dynamicStream DynamicStream
	var segment Segment

	for key := range stream.DynamicStream {
		bandwidth = append(bandwidth, key)
	}

	sort.Ints(bandwidth)

	if len(bandwidth) == 0 {
		err = errors.New("M3U8 does not contain streaming URLs")
		return
	}

	// Use the highest bandwidth the network can handle, or the lowest one
	// if the network speed is unknown or slower than every variant
	dynamicStream = stream.DynamicStream[bandwidth[0]]

	if stream.NetworkBandwidth > 0 {
		for _, b := range bandwidth {
			if b > stream.NetworkBandwidth {
				break
			}
			dynamicStream = stream.DynamicStream[b]
		}
	}

	segment.StreamInf.Bandwidth = dynamicStream.Bandwidth

	segment.URL = dynamicStream.URL
	segment.Duration = 0
	stream.Segment = append(stream.Segment, segment)

	return
}
