package src

import "time"

// Playlist : Contains all Playlist Information that the Buffer needs
type Playlist struct {
	Folder       string
	PlaylistID   string
	PlaylistName string
	Tuner        int

	Clients map[int]ThisClient
	Streams map[int]ThisStream
}

// ThisClient : Client Information
type ThisClient struct {
	Connection int
}

// ThisStream : Contains Information about the Playlist Stream to be played
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

	DynamicStream map[int]DynamicStream

	// Local Temp Files
	OldSegments []string
}

// Segment : URL Segments (HLS / M3U8)
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

// DynamicStream : Stream Information with dynamic Bandwidth
type DynamicStream struct {
	AverageBandwidth int
	Bandwidth        int
	Framerate        float64
	Resolution       string
	URL              string
}

// ClientConnection : Client Connections
type ClientConnection struct {
	Connection int
	Error      error
}

// BandwidthCalculation : Bandwidth Calculation for the Stream
type BandwidthCalculation struct {
	Size     int
	Start    time.Time
	Stop     time.Time
	TimeDiff float64
}
