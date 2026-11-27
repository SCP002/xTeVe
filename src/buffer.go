package src

/*
  Render Tuner Stream-Limit image as Video [ffmpeg]
  -loop 1 -i stream-limit.jpg -c:v libx264 -t 1 -pix_fmt yuv420p -vf scale=1920:1080  stream-limit.ts
*/

import (
	"bufio"
	"bytes"
	"errors"
	"fmt"
	"io"
	"math"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/avfs/avfs/vfs/memfs"
	"github.com/avfs/avfs/vfs/osfs"
	"github.com/samber/lo"
)

func createStreamID(stream map[int]ThisStream) (streamID int) {
	var debug string

	streamID = 0
	for i := 0; i <= len(stream); i++ {
		if _, ok := stream[i]; !ok {
			streamID = i
			break
		}
	}

	debug = fmt.Sprintf("Streaming Status:Stream ID = %d", streamID)
	showDebug(debug, 1)

	return
}

func bufferingStream(playlistID, streamingURL, channelName string, w http.ResponseWriter, r *http.Request) {
	time.Sleep(time.Duration(Settings.BufferTimeout) * time.Millisecond)

	var playlist Playlist
	var client ThisClient
	var stream ThisStream
	var streaming = false
	var streamID int
	var debug string
	var timeOut = 0
	var newStream = true

	w.Header().Set("Connection", "close")

	// Check whether the Playlist is already in use
	if p, ok := BufferInformation.Load(playlistID); !ok {
		var playlistType string
		// Playlist is not yet used, create Default Values for the Playlist
		playlist.Folder = System.Folder.Temp + playlistID + string(os.PathSeparator)
		playlist.PlaylistID = playlistID
		playlist.Streams = make(map[int]ThisStream)
		playlist.Clients = make(map[int]ThisClient)

		err := checkVFSFolder(playlist.Folder, bufferVFS)
		if err != nil {
			ShowError(err, 000)
			httpStatusError(w, r, 404)
			return
		}

		switch playlist.PlaylistID[0:1] {
		case "M":
			playlistType = "m3u"

		case "H":
			playlistType = "hdhr"
		}

		playlist.Tuner = getTuner(playlistID, playlistType)

		playlist.PlaylistName = getProviderParameter(playlist.PlaylistID, playlistType, "name")

		// Create Default Values for the Stream
		streamID = createStreamID(playlist.Streams)

		client.Connection = 1
		stream.URL = streamingURL
		stream.ChannelName = channelName
		stream.Status = false

		playlist.Streams[streamID] = stream
		playlist.Clients[streamID] = client

		BufferInformation.Store(playlistID, playlist)
	} else {
		// Playlist is already being used for streaming
		// Check if the URL is already being streamed by another Client

		playlist = p.(Playlist)

		for id := range playlist.Streams {
			stream = playlist.Streams[id]
			client = playlist.Clients[id]

			if streamingURL == stream.URL {
				streamID = id
				newStream = false
				client.Connection++

				//playlist.Streams[streamID] = stream
				playlist.Clients[streamID] = client

				BufferInformation.Store(playlistID, playlist)

				debug = fmt.Sprintf(
					"Restream Status:Playlist: %s - Channel: %s - Connections: %d",
					playlist.PlaylistName,
					stream.ChannelName,
					client.Connection,
				)

				showDebug(debug, 1)

				if c, ok := BufferClients.Load(playlistID + stream.MD5); ok {
					var clients = c.(ClientConnection)
					clients.Connection = clients.Connection + 1
					showInfo(
						fmt.Sprintf(
							"Streaming Status:Channel: %s (Clients: %d)",
							stream.ChannelName,
							clients.Connection,
						),
					)

					BufferClients.Store(playlistID+stream.MD5, clients)
				}

				break
			}
		}

		// New Stream for an already active Playlist
		if newStream {
			// Check whether the Playlist allows another Stream (Tuner)
			if len(playlist.Streams) >= playlist.Tuner {
				showInfo(
					fmt.Sprintf(
						"Streaming Status:Playlist: %s - No new connections available. Tuner = %d",
						playlist.PlaylistName,
						playlist.Tuner,
					),
				)

				if value, ok := webUI["html/video/stream-limit.ts"]; ok {
					var content = GetHTMLString(value.(string))

					w.Header().Set("Content-type", "video/mpeg")
					w.WriteHeader(200)

					for i := 1; i < 60; i++ {
						if !writeResponse(w, []byte(content)) {
							break
						}
						time.Sleep(time.Duration(500) * time.Millisecond)
					}

					return
				}

				return
			}

			// Playlist allows another Stream (The Tuner limit has not yet been reached)
			// Create Default Values for the Stream
			stream = ThisStream{}
			client = ThisClient{}

			streamID = createStreamID(playlist.Streams)

			client.Connection = 1
			stream.URL = streamingURL
			stream.ChannelName = channelName
			stream.Status = false

			playlist.Streams[streamID] = stream
			playlist.Clients[streamID] = client

			BufferInformation.Store(playlistID, playlist)
		}
	}

	// Check whether the Stream is already being played by another Client
	if !playlist.Streams[streamID].Status && newStream {
		// New buffer is required
		stream = playlist.Streams[streamID]
		stream.MD5 = getMD5(streamingURL)
		stream.Folder = playlist.Folder + stream.MD5 + string(os.PathSeparator)
		stream.PlaylistID = playlistID
		stream.PlaylistName = playlist.PlaylistName

		playlist.Streams[streamID] = stream
		BufferInformation.Store(playlistID, playlist)

		switch Settings.Buffer {
		case "xteve":
			go connectToStreamingServer(streamID, playlistID)
		case "ffmpeg", "vlc":
			go thirdPartyBuffer(streamID, playlistID)

		default:
			break
		}

		showInfo(
			fmt.Sprintf(
				"Streaming Status:Playlist: %s - Tuner: %d / %d",
				playlist.PlaylistName,
				len(playlist.Streams),
				playlist.Tuner,
			),
		)

		var clients ClientConnection
		clients.Connection = 1
		BufferClients.Store(playlistID+stream.MD5, clients)
	}

	w.WriteHeader(200)

	for { // Loop 1: Wait until the first Segment has been downloaded by the Buffer
		if p, ok := BufferInformation.Load(playlistID); ok {
			var playlist = p.(Playlist)

			if stream, ok := playlist.Streams[streamID]; ok {
				if !stream.Status {
					timeOut++

					time.Sleep(time.Duration(100) * time.Millisecond)

					if c, ok := BufferClients.Load(playlistID + stream.MD5); ok {
						var clients = c.(ClientConnection)

						if clients.Error != nil || timeOut > 200 {
							killClientConnection(streamID, stream.PlaylistID, false)
							return
						}
					}

					continue
				}

				var oldSegments []string

				for { // Loop 2: Temporary files are available, Data can be sent to the Client
					// Monitor HTTP Client connection

					select {
					case <-r.Context().Done():
						killClientConnection(streamID, playlistID, false)
						return

					default:
						if c, ok := BufferClients.Load(playlistID + stream.MD5); ok {
							var clients = c.(ClientConnection)
							if clients.Error != nil {
								ShowError(clients.Error, 0)
								killClientConnection(streamID, playlistID, false)
								return
							}
						} else {
							return
						}
					}

					if _, err := bufferVFS.Stat(stream.Folder); fsIsNotExistErr(err) {
						killClientConnection(streamID, playlistID, false)
						return
					}

					var tmpFiles = getBufTmpFiles(&stream)

					for _, f := range tmpFiles {
						if _, err := bufferVFS.Stat(stream.Folder); fsIsNotExistErr(err) {
							killClientConnection(streamID, playlistID, false)
							return
						}

						oldSegments = append(oldSegments, f)

						var fileName = stream.Folder + f

						file, err := bufferVFS.Open(fileName)
						if err == nil {
							var buffer []byte
							l, err := file.Stat()
							if err == nil {
								buffer = make([]byte, int(l.Size()))
								_, err = io.ReadFull(file, buffer)
							}
							closeOrLog(file)

							if err == nil {
								debug = fmt.Sprintf("Buffer Status:Send to client (%s)", fileName)
								showDebug(debug, 2)

								if !streaming {
									contentType := http.DetectContentType(buffer)
									w.Header().Set("Content-type", contentType)
									w.Header().Set("Content-Length", "0")
									w.Header().Set("Connection", "close")
								}

								if _, err := w.Write(buffer); err != nil {
									killClientConnection(streamID, playlistID, false)
									return
								}

								streaming = true
							}

							var n = lo.IndexOf(oldSegments, f)

							if n > 20 {
								var fileToRemove = stream.Folder + oldSegments[0]
								if err = bufferVFS.RemoveAll(getPlatformFile(fileToRemove)); err != nil {
									ShowError(err, 4007)
								}
								oldSegments = append(oldSegments[:0], oldSegments[0+1:]...)
							}
						}
					}

					if len(tmpFiles) == 0 {
						time.Sleep(time.Duration(100) * time.Millisecond)
					}
				} // End of Loop 2
			} else {
				// Stream not available
				killClientConnection(streamID, stream.PlaylistID, false)
				showInfo(
					fmt.Sprintf(
						"Streaming Status:Playlist: %s - Tuner: %d / %d",
						playlist.PlaylistName,
						len(playlist.Streams),
						playlist.Tuner,
					),
				)
				return
			}
		} // End of Buffer Information
	} // End of Loop 1
}

func getBufTmpFiles(stream *ThisStream) (tmpFiles []string) {
	var tmpFolder = stream.Folder
	var fileIDs []float64

	if _, err := bufferVFS.Stat(tmpFolder); !fsIsNotExistErr(err) {
		files, err := bufferVFS.ReadDir(getPlatformPath(tmpFolder))
		if err != nil {
			ShowError(err, 000)
			return
		}

		if len(files) > 2 {
			for _, file := range files {
				var fileID = strings.ReplaceAll(file.Name(), ".ts", "")
				var f, err = strconv.ParseFloat(fileID, 64)

				if err == nil {
					fileIDs = append(fileIDs, f)
				}
			}

			sort.Float64s(fileIDs)
			fileIDs = fileIDs[:len(fileIDs)-1]

			for _, file := range fileIDs {
				var fileName = fmt.Sprintf("%d.ts", int64(file))

				if lo.IndexOf(stream.OldSegments, fileName) == -1 {
					tmpFiles = append(tmpFiles, fileName)
					stream.OldSegments = append(stream.OldSegments, fileName)
				}
			}
		}
	}

	return
}

func killClientConnection(streamID int, playlistID string, force bool) {
	Lock.Lock()
	defer Lock.Unlock()

	if p, ok := BufferInformation.Load(playlistID); ok {
		var playlist = p.(Playlist)

		if force {
			delete(playlist.Streams, streamID)
			showInfo(
				fmt.Sprintf(
					"Streaming Status:Playlist: %s - Tuner: %d / %d",
					playlist.PlaylistName,
					len(playlist.Streams),
					playlist.Tuner,
				),
			)
			return
		}

		if stream, ok := playlist.Streams[streamID]; ok {
			if c, ok := BufferClients.Load(playlistID + stream.MD5); ok {
				var clients = c.(ClientConnection)
				clients.Connection = clients.Connection - 1
				BufferClients.Store(playlistID+stream.MD5, clients)

				showInfo("Streaming Status:Client has terminated the connection")
				showInfo(
					fmt.Sprintf("Streaming Status:Channel: %s (Clients: %d)", stream.ChannelName, clients.Connection),
				)

				if clients.Connection <= 0 {
					BufferClients.Delete(playlistID + stream.MD5)
					delete(playlist.Streams, streamID)
					delete(playlist.Clients, streamID)
				}
			}

			BufferInformation.Store(playlistID, playlist)

			if len(playlist.Streams) > 0 {
				showInfo(
					fmt.Sprintf(
						"Streaming Status:Playlist: %s - Tuner: %d / %d",
						playlist.PlaylistName,
						len(playlist.Streams),
						playlist.Tuner,
					),
				)
			}
		}
	}
}

func clientConnection(stream ThisStream) (status bool) {
	status = true
	Lock.Lock()
	defer Lock.Unlock()

	if _, ok := BufferClients.Load(stream.PlaylistID + stream.MD5); !ok {
		var debug = fmt.Sprintf("Streaming Status:Remove temporary files (%s)", stream.Folder)
		showDebug(debug, 1)

		status = false

		debug = fmt.Sprintf("Remove tmp folder:%s", stream.Folder)
		showDebug(debug, 1)

		if err := bufferVFS.RemoveAll(stream.Folder); err != nil {
			ShowError(err, 4005)
		}

		if p, ok := BufferInformation.Load(stream.PlaylistID); ok {
			showInfo(
				fmt.Sprintf(
					"Streaming Status:Channel: %s - No client is using this channel anymore. "+
						"Streaming Server connection has ended",
					stream.ChannelName,
				),
			)

			var playlist = p.(Playlist)

			showInfo(
				fmt.Sprintf(
					"Streaming Status:Playlist: %s - Tuner: %d / %d",
					playlist.PlaylistName,
					len(playlist.Streams),
					playlist.Tuner,
				),
			)

			if len(playlist.Streams) <= 0 {
				BufferInformation.Delete(stream.PlaylistID)
			}
		}

		status = false
	}

	return
}

func connectToStreamingServer(streamID int, playlistID string) {
	if p, ok := BufferInformation.Load(playlistID); ok {
		var playlist = p.(Playlist)

		var timeOut = 0
		var debug string
		var tmpSegment = 1
		var tmpFolder = playlist.Streams[streamID].Folder
		var m3u8Segments []string
		var bandwidth BandwidthCalculation
		var networkBandwidth = Settings.M3U8AdaptiveBandwidthMBPS * 1e+6
		// Size of the Buffer
		var bufferSize = Settings.BufferSize
		var buffer = make([]byte, 1024*bufferSize*2)

		var defaultSegment = func() {
			var segment Segment

			if len(playlist.Streams[streamID].Location) > 0 {
				segment.URL = playlist.Streams[streamID].Location
			} else {
				segment.URL = playlist.Streams[streamID].URL
			}

			segment.Duration = 0

			var stream = playlist.Streams[streamID]
			stream.Segment = []Segment{}
			stream.Segment = append(stream.Segment, segment)

			stream.HLS = false
			stream.Sequence = 0
			stream.Wait = 0
			stream.NetworkBandwidth = networkBandwidth

			playlist.Streams[streamID] = stream

			timeOut++
		}

		var addErrorToStream = func(err error) {
			var stream = playlist.Streams[streamID]

			if c, ok := BufferClients.Load(playlistID + stream.MD5); ok {
				var clients = c.(ClientConnection)
				clients.Error = err
				BufferClients.Store(playlistID+stream.MD5, clients)
			}
		}

		if err := bufferVFS.RemoveAll(getPlatformPath(tmpFolder)); err != nil {
			ShowError(err, 4005)
		}

		err := checkVFSFolder(tmpFolder, bufferVFS)
		if err != nil {
			ShowError(err, 0)
			addErrorToStream(err)
			return
		}

		// M3U8 Segments
	InitBuffer:
		defaultSegment()

		if len(m3u8Segments) > 30 {
			m3u8Segments = m3u8Segments[15:]
		}
		if timeOut >= 10 {
			return
		}

		var stream = playlist.Streams[streamID]

		if !stream.Status {
			if strings.Contains(stream.URL, ".m3u8") {
				showInfo("Streaming Type:" + "[HLS / M3U8]")
			} else {
				showInfo("Streaming Type:" + "[TS]")
			}

			showInfo("Streaming URL:" + stream.URL)
		}

		var s = 0

		stream.TimeStart = time.Now()
		bandwidth.Start = stream.TimeStart
		bandwidth.Size = 0

		for {
			if !clientConnection(stream) {
				return
			}

			if len(stream.Segment) == 0 || len(stream.URL) == 0 {
				goto InitBuffer
			}

			var segment = stream.Segment[0]

			var currentURL = strings.Trim(segment.URL, "\r\n")

			if len(currentURL) == 0 {
				goto InitBuffer
			}

			debug = fmt.Sprintf("Connection to:%s", currentURL)
			showDebug(debug, 2)

			// Jump for redirect (301 <---> 308)
		Redirect:

			req, err := http.NewRequest("GET", currentURL, nil)
			if err != nil {
				ShowError(err, 0)
				addErrorToStream(err)
				killClientConnection(streamID, playlistID, true)
				clientConnection(stream)
				return
			}
			req.Header.Set("User-Agent", Settings.UserAgent)
			req.Header.Set("Connection", "close")
			req.Header.Set("Accept", "*/*")
			debugRequest(req)

			client := &http.Client{}
			client.CheckRedirect = func(req *http.Request, via []*http.Request) error {
				return errors.New("Redirect")
			}

			resp, err := client.Do(req)

			if resp != nil && err != nil {
				debugResponse(resp)
			}

			if err != nil {
				if resp == nil {
					err = fmt.Errorf("no response from streaming server: %s", currentURL)
					ShowError(err, 0)

					addErrorToStream(err)

					killClientConnection(streamID, playlistID, true)
					clientConnection(stream)

					return
				}

				// Redirect
				if resp.StatusCode >= 301 && resp.StatusCode <= 308 {
					debug = fmt.Sprintf(
						"Streaming Status:HTTP response status [%d] %s",
						resp.StatusCode,
						http.StatusText(resp.StatusCode),
					)
					showDebug(debug, 2)

					currentURL = strings.Trim(resp.Header.Get("Location"), "\r\n")

					stream.Location = currentURL

					if len(currentURL) > 0 {
						debug = fmt.Sprintf("HTTP Redirect:%s", stream.Location)
						showDebug(debug, 2)
						closeOrLog(resp.Body)
						goto Redirect
					} else {
						err = errors.New("streaming server")
						ShowError(err, 4002)
						addErrorToStream(err)

						closeOrLog(resp.Body)

						return
					}
				} else {
					ShowError(err, 0)
					addErrorToStream(err)

					closeOrLog(resp.Body)

					return
				}
			}

			// Check HTTP Status, in case of errors the stream is terminated
			var contentType = resp.Header.Get("Content-Type")
			var httpStatusCode = resp.StatusCode
			var httpStatusInfo = fmt.Sprintf(
				"HTTP Response Status [%d] %s", httpStatusCode, http.StatusText(resp.StatusCode),
			)

			if resp.StatusCode != http.StatusOK {
				showInfo("Content Type:" + contentType)
				showInfo("Streaming Status:" + httpStatusInfo)
				showInfo("Error with this URL:" + currentURL)

				var err = errors.New(http.StatusText(resp.StatusCode))
				ShowError(err, 4004)

				debug = fmt.Sprintf(
					"Streaming Status:Playlist: %s - Tuner: %d / %d",
					playlist.PlaylistName,
					len(playlist.Streams),
					playlist.Tuner,
				)
				showDebug(debug, 1)

				BufferInformation.Store(playlist.PlaylistID, playlist)
				addErrorToStream(err)

				killClientConnection(streamID, playlistID, true)
				clientConnection(stream)
				closeOrLog(resp.Body)

				return
			}

			// Read out information about the streaming server
			if !stream.Status {
				if len(stream.URLStreamingServer) == 0 {
					// req was built from currentURL, so its parsed URL is already available
					var u = req.URL

					stream.URLScheme = u.Scheme
					stream.URLHost = req.Host
					stream.URLPath = u.Path
					stream.URLFile = path.Base(u.Path)

					stream.URLRedirect = fmt.Sprintf("%s://%s%s", stream.URLScheme, stream.URLHost, stream.URLPath)
					stream.URLStreamingServer = fmt.Sprintf("%s://%s", stream.URLScheme, stream.URLHost)
				}

				debug = fmt.Sprintf("Server URL:%s", stream.URLStreamingServer)
				showDebug(debug, 1)

				debug = fmt.Sprintf("Temp Folder:%s", tmpFolder)
				showDebug(debug, 1)

				showInfo(
					"Streaming Status:" + "HTTP Response Status [" + strconv.Itoa(
						resp.StatusCode,
					) + "] " + http.StatusText(
						resp.StatusCode,
					),
				)
				showInfo("Content Type:" + contentType)
			} else {
				debug = fmt.Sprintf("Content Type:%s", contentType)
				showDebug(debug, 2)
			}

			// Clean up Content Type
			if len(contentType) > 0 {
				var ct = strings.SplitN(contentType, ";", 2)
				contentType = strings.ToLower(ct[0])
			}

			switch contentType {
			// M3U8 Playlist
			case "application/x-mpegurl", "application/vnd.apple.mpegurl", "audio/mpegurl", "audio/x-mpegurl":
				body, err := io.ReadAll(resp.Body)
				if err != nil {
					ShowError(err, 0)
					addErrorToStream(err)
				}

				stream.Body = string(body)
				stream.HLS = true
				stream.M3U8URL = currentURL

				err = parseM3U8(&stream)
				if err != nil {
					ShowError(err, 4050)
					addErrorToStream(err)
				}

			// Video Stream (TS)
			case "video/mpeg",
				"video/mp4",
				"video/mp2t",
				"video/m2ts",
				"application/octet-stream",
				"binary/octet-stream",
				"application/mp2t",
				"video/x-matroska":

				var fileSize int

				// Size of the Buffer
				buffer = make([]byte, 1024*bufferSize*2)
				var tmpFileSize = 1024 * bufferSize * 1

				debug = fmt.Sprintf("Buffer Size:%d KB [SERVER CONNECTION]", len(buffer)/1024)
				showDebug(debug, 3)

				debug = fmt.Sprintf("Buffer Size:%d KB [CLIENT CONNECTION]", tmpFileSize/1024)
				showDebug(debug, 3)

				var tmpFile = fmt.Sprintf("%s%d.ts", tmpFolder, tmpSegment)

				if !clientConnection(stream) {
					closeOrLog(resp.Body)
					return
				}

				bufferFile, err := bufferVFS.Create(tmpFile)
				if err != nil {
					ShowError(err, 0)
					addErrorToStream(err)
					closeOrLog(resp.Body)
					return
				}

				for {
					if fileSize == 0 {
						debug = fmt.Sprintf("Buffer Status:Buffering (%s)", tmpFile)
						showDebug(debug, 2)
					}

					timeOut = 0
					// Fill the Buffer with data from the Server
					n, err := resp.Body.Read(buffer)

					if err != nil && err != io.EOF {
						ShowError(err, 0)
						addErrorToStream(err)
						closeOrLog(bufferFile)
						closeOrLog(resp.Body)
						return
					}

					if _, err := bufferFile.Write(buffer[:n]); err != nil {
						ShowError(err, 0)
						addErrorToStream(err)
						closeOrLog(bufferFile)
						closeOrLog(resp.Body)
						return
					}

					fileSize = fileSize + n

					if !clientConnection(stream) {
						closeOrLog(resp.Body)
						closeOrLog(bufferFile)

						if err = bufferVFS.RemoveAll(stream.Folder); err != nil {
							ShowError(err, 4005)
						}
						return
					}

					// Save the buffer to the Hard Disk
					if fileSize >= tmpFileSize/2 || n == 0 {
						Lock.Lock()

						bandwidth.Stop = time.Now()
						bandwidth.Size += fileSize

						bandwidth.TimeDiff = bandwidth.Stop.Sub(bandwidth.Start).Seconds()

						networkBandwidth = int(float64(bandwidth.Size) / bandwidth.TimeDiff * 1000)

						stream.NetworkBandwidth = networkBandwidth
						bandwidth.NetworkBandwidth = stream.NetworkBandwidth

						debug = fmt.Sprintf("Buffer Status:Done (%s)", tmpFile)
						showDebug(debug, 2)

						// The segment is complete only if it was closed without errors
						if err := bufferFile.Close(); err != nil {
							Lock.Unlock()
							ShowError(err, 0)
							addErrorToStream(err)
							closeOrLog(resp.Body)
							return
						}

						stream.Status = true
						playlist.Streams[streamID] = stream
						BufferInformation.Store(playlistID, playlist)
						Lock.Unlock()

						tmpSegment++

						tmpFile = fmt.Sprintf("%s%d.ts", tmpFolder, tmpSegment)

						if !clientConnection(stream) {
							closeOrLog(resp.Body)

							if err = bufferVFS.RemoveAll(stream.Folder); err != nil {
								ShowError(err, 4005)
							}

							return
						}

						bufferFile, err = bufferVFS.Create(tmpFile)
						if err != nil {
							ShowError(err, 0)
							addErrorToStream(err)
							closeOrLog(resp.Body)
							return
						}

						fileSize = 0
						buffer = make([]byte, 1024*bufferSize*2)

						if n == 0 {
							closeOrLog(bufferFile)
							break
						}
					}
				}

				//--

			// Unknown Format
			default:
				showInfo("Content Type:" + resp.Header.Get("Content-Type"))
				err = errors.New("streaming error")
				ShowError(err, 4003)

				addErrorToStream(err)
				closeOrLog(resp.Body)
				return
			}

			s++

			// Calculate the waiting time for the Download of the next Segment
			if stream.HLS {
				var sleep float64

				if segment.Duration > 0 {
					stream.TimeEnd = time.Now()
					stream.TimeDiff = stream.TimeEnd.Sub(stream.TimeStart).Seconds()

					sleep = (segment.Duration - stream.TimeDiff) - (segment.Duration * 0.25)

					if sleep < 0 {
						sleep = 0
					}

					debug = fmt.Sprintf(
						"HLS Status:Download time: %f s | Segment duration: %f s | Sleep: %f s Sequence: %d",
						stream.TimeDiff,
						segment.Duration,
						sleep,
						segment.Sequence,
					)
					showDebug(debug, 1)

					if sleep > 0 {
						// Sleep in 100 ms steps to notice a stopped stream early
						for range int(math.Ceil(sleep * 10)) {
							time.Sleep(time.Duration(100) * time.Millisecond)

							if _, err := bufferVFS.Stat(stream.Folder); fsIsNotExistErr(err) {
								break
							}
						}
					}
				}
			}

			stream.Segment = stream.Segment[1:len(stream.Segment)]

			closeOrLog(resp.Body)
		} // End for loop
	} // End of BufferInformation
}

func parseM3U8(stream *ThisStream) (err error) {
	var debug string
	var noNewSegment = false
	var lastSegmentDuration float64
	var segment Segment
	var m3u8Segments []Segment
	var sequence int64

	stream.DynamicBandwidth = false

	debug = fmt.Sprintf(`M3U8 Playlist:`+"\n"+`%s`, stream.Body)
	showDebug(debug, 3)

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
							return withCode(err, 1050)
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
		err = errors.New(getErrMsg(4051))
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
				ShowError(err, 0)
			}
		} else {
			stream.Segment = append(stream.Segment, segment)
		}
	}

	if noNewSegment {
		var sleep = lastSegmentDuration * 0.5

		// Sleep in 100 ms steps to notice a stopped stream early
		for range int(math.Ceil(sleep * 10)) {
			time.Sleep(time.Duration(100) * time.Millisecond)

			if _, err := bufferVFS.Stat(stream.Folder); fsIsNotExistErr(err) {
				break
			}
		}
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

// Buffer with FFMPEG
func thirdPartyBuffer(streamID int, playlistID string) {
	if p, ok := BufferInformation.Load(playlistID); ok {
		var playlist = p.(Playlist)
		var debug, path, options, bufferType string
		var tmpSegment = 1
		var bufferSize = Settings.BufferSize * 1024
		var stream = playlist.Streams[streamID]
		var buf bytes.Buffer
		var fileSize = 0
		var streamStatus = make(chan bool)

		var tmpFolder = playlist.Streams[streamID].Folder
		var url = playlist.Streams[streamID].URL

		stream.Status = false

		bufferType = strings.ToUpper(Settings.Buffer)

		switch Settings.Buffer {
		case "ffmpeg":
			path = Settings.FFmpegPath
			options = Settings.FFmpegOptions

		case "vlc":
			path = Settings.VLCPath
			options = Settings.VLCOptions

		default:
			return
		}

		var addErrorToStream = func(err error) {
			var stream = playlist.Streams[streamID]

			if c, ok := BufferClients.Load(playlistID + stream.MD5); ok {
				var clients = c.(ClientConnection)
				clients.Error = err
				BufferClients.Store(playlistID+stream.MD5, clients)
			}
		}

		if err := bufferVFS.RemoveAll(getPlatformPath(tmpFolder)); err != nil {
			ShowError(err, 4005)
		}

		err := checkVFSFolder(tmpFolder, bufferVFS)
		if err != nil {
			ShowError(err, 0)
			addErrorToStream(err)
			return
		}

		err = checkFile(path)
		if err != nil {
			ShowError(err, 0)
			addErrorToStream(err)
			return
		}

		showInfo(fmt.Sprintf("%s path:%s", bufferType, path))
		showInfo("Streaming URL:" + stream.URL)

		var tmpFile = fmt.Sprintf("%s%d.ts", tmpFolder, tmpSegment)

		f, err := bufferVFS.Create(tmpFile)
		if err != nil {
			ShowError(err, 0)
			addErrorToStream(err)
			return
		}
		if err = f.Close(); err != nil {
			ShowError(err, 0)
			addErrorToStream(err)
			return
		}

		// Set User-Agent
		var args []string

		for i, a := range strings.Split(options, " ") {
			switch bufferType {
			case "FFMPEG":
				a = strings.ReplaceAll(a, "[URL]", url)
				if i == 0 {
					if len(Settings.UserAgent) != 0 {
						args = []string{"-user_agent", Settings.UserAgent}
					}
				}

				args = append(args, a)

			case "VLC":
				if a == "[URL]" {
					a = strings.ReplaceAll(a, "[URL]", url)
					args = append(args, a)

					if len(Settings.UserAgent) != 0 {
						args = append(args, fmt.Sprintf(":http-user-agent=%s", Settings.UserAgent))
					}
				} else {
					args = append(args, a)
				}
			}
		}

		var cmd = exec.Command(path, args...)

		debug = fmt.Sprintf("%s:%s %s", bufferType, path, args)
		showDebug(debug, 1)

		// Byte-Data from the Process
		stdOut, err := cmd.StdoutPipe()
		if err != nil {
			ShowError(err, 0)
			addErrorToStream(err)
			return
		}

		// Log-Data from the Process
		logOut, err := cmd.StderrPipe()
		if err != nil {
			ShowError(err, 0)
			addErrorToStream(err)
			return
		}

		if len(buf.Bytes()) == 0 && !stream.Status {
			showInfo(bufferType + ":Processing data")
		}

		if err := cmd.Start(); err != nil {
			ShowError(err, 0)
			addErrorToStream(err)
			return
		}

		// Every exit path below must stop the process; stop() is safe to call more than once.
		var stop = sync.OnceFunc(func() { stopProcess(cmd) })
		defer stop()

		go func() {
			// Show Log Data from the Process in Debug Mode 1.
			scanner := bufio.NewScanner(logOut)
			scanner.Split(bufio.ScanLines)

			for scanner.Scan() {
				debug = fmt.Sprintf("%s log:%s", bufferType, strings.TrimSpace(scanner.Text()))

				select {
				case <-streamStatus:
					showDebug(debug, 1)
				default:
					showInfo(debug)
				}

				time.Sleep(time.Duration(10) * time.Millisecond)
			}
		}()

		f, err = bufferVFS.OpenFile(tmpFile, os.O_APPEND|os.O_WRONLY, 0600)
		if err != nil {
			ShowError(err, 0)
			addErrorToStream(err)
			return
		}

		buffer := make([]byte, 1024*4)

		reader := bufio.NewReader(stdOut)

		t := make(chan int)

		go func() {
			var timeout = 0
			for {
				time.Sleep(time.Duration(1000) * time.Millisecond)
				timeout++

				select {
				case <-t:
					return
				default:
					t <- timeout
				}
			}
		}()

		for {
			select {
			case timeout := <-t:
				if timeout >= 20 && tmpSegment == 1 {
					stop()
					closeOrLog(f)
					err = errors.New("timeout")
					ShowError(err, 4006)
					addErrorToStream(err)
					return
				}

			default:
			}

			if fileSize == 0 && !stream.Status {
				showInfo("Streaming Status:Receive data from " + bufferType)
			}

			if !clientConnection(stream) {
				stop()
				closeOrLog(f)
				return
			}

			n, err := reader.Read(buffer)
			if err != nil {
				if err != io.EOF {
					ShowError(err, 0)
				}
				break
			}

			fileSize = fileSize + len(buffer[:n])

			if _, err := f.Write(buffer[:n]); err != nil {
				stop()
				closeOrLog(f)
				ShowError(err, 0)
				addErrorToStream(err)
				return
			}

			if fileSize >= bufferSize/2 {
				if tmpSegment == 1 && !stream.Status {
					close(t)
					close(streamStatus)
					showInfo(fmt.Sprintf("Streaming Status:Buffering data from %s", bufferType))
				}

				// The segment is complete only if it was closed without errors
				if err := f.Close(); err != nil {
					stop()
					ShowError(err, 0)
					addErrorToStream(err)
					return
				}
				tmpSegment++

				if !stream.Status {
					Lock.Lock()
					stream.Status = true
					playlist.Streams[streamID] = stream
					BufferInformation.Store(playlistID, playlist)
					Lock.Unlock()
				}

				tmpFile = fmt.Sprintf("%s%d.ts", tmpFolder, tmpSegment)

				fileSize = 0

				f, err = bufferVFS.OpenFile(tmpFile, os.O_CREATE|os.O_TRUNC|os.O_APPEND|os.O_WRONLY, 0600)
				if err != nil {
					stop()
					ShowError(err, 0)
					addErrorToStream(err)
					return
				}
			}
		}

		stop()
		closeOrLog(f)

		err = errors.New(bufferType + " error")
		addErrorToStream(err)
		ShowError(err, 1204)

		time.Sleep(time.Duration(500) * time.Millisecond)
		clientConnection(stream)

		return
	}
}

func getTuner(id, playlistType string) (tuner int) {
	switch Settings.Buffer {
	case "-":
		tuner = Settings.Tuner

	case "xteve", "ffmpeg", "vlc":
		i, err := strconv.Atoi(getProviderParameter(id, playlistType, "tuner"))
		if err == nil {
			tuner = i
		} else {
			ShowError(err, 0)
			tuner = 1
		}
	}

	return
}

func initBufferVFS(virtual bool) {
	if virtual {
		bufferVFS = memfs.New()
	} else {
		bufferVFS = osfs.New()
	}
}

func debugRequest(req *http.Request) {
	var debugLevel = 3

	if System.Flag.Debug < debugLevel {
		return
	}

	var debug string

	debug = "Request:Begin HTTP(S) request"
	showDebug(debug, debugLevel)

	debug = fmt.Sprintf("Method:%s", req.Method)
	showDebug(debug, debugLevel)

	debug = fmt.Sprintf("Proto:%s", req.Proto)
	showDebug(debug, debugLevel)

	debug = fmt.Sprintf("URL:%s", req.URL)
	showDebug(debug, debugLevel)

	for name, headers := range req.Header {
		name = strings.ToLower(name)

		for _, h := range headers {
			debug = fmt.Sprintf("Header:%v: %v", name, h)
			showDebug(debug, debugLevel)
		}
	}

	debug = "Request:End HTTP(S) request"
	showDebug(debug, debugLevel)
}

func debugResponse(resp *http.Response) {
	var debugLevel = 3

	if System.Flag.Debug < debugLevel {
		return
	}

	var debug string

	debug = "Response:Begin HTTP(S) response"
	showDebug(debug, debugLevel)

	debug = fmt.Sprintf("Proto:%s", resp.Proto)
	showDebug(debug, debugLevel)

	debug = fmt.Sprintf("Status Code:%d", resp.StatusCode)
	showDebug(debug, debugLevel)

	debug = fmt.Sprintf("Status Text:%s", http.StatusText(resp.StatusCode))
	showDebug(debug, debugLevel)

	for key, value := range resp.Header {
		switch fmt.Sprintf("%T", value) {
		case "[]string":
			debug = fmt.Sprintf("Header:%v: %s", key, strings.Join(value, " "))

		default:
			debug = fmt.Sprintf("Header:%v: %v", key, value)
		}

		showDebug(debug, debugLevel)
	}

	debug = "Response:End HTTP(S) response"
	showDebug(debug, debugLevel)
}
