package epg

import "encoding/xml"

// XMLTV : XMLTV File
type XMLTV struct {
	Generator string   `xml:"generator-info-name,attr"`
	Source    string   `xml:"source-info-name,attr"`
	XMLName   xml.Name `xml:"tv"`

	Channel []*Channel `xml:"channel"`
	Program []*Program `xml:"programme"`
}

// Channel : Channels
type Channel struct {
	ID           string        `xml:"id,attr"`
	DisplayNames []DisplayName `xml:"display-name"`
	Icon         *Icon         `xml:"icon"`
	URL          []URL         `xml:"url"`
}

// IconSrc returns the source of the channel icon, "" if it has none.
func (c *Channel) IconSrc() string {
	if c.Icon == nil {
		return ""
	}
	return c.Icon.Src
}

// DisplayName : Channel Name
type DisplayName struct {
	Lang  string `xml:"lang,attr,omitempty"`
	Value string `xml:",chardata"`
}

// Icon : Station Logo
type Icon struct {
	Src string `xml:"src,attr"`
}

// Program is a programme. The fields follow the order of the XMLTV DTD
// (https://github.com/XMLTV/xmltv/blob/master/xmltv.dtd), so that written programmes are valid; empty elements
// and attributes are left out.
type Program struct {
	Start     string `xml:"start,attr"`
	Stop      string `xml:"stop,attr,omitempty"`
	PDCStart  string `xml:"pdc-start,attr,omitempty"`
	VPSStart  string `xml:"vps-start,attr,omitempty"`
	Showview  string `xml:"showview,attr,omitempty"`
	Videoplus string `xml:"videoplus,attr,omitempty"`
	Channel   string `xml:"channel,attr"`
	Clumpidx  string `xml:"clumpidx,attr,omitempty"`

	Title           []*Title         `xml:"title"`
	SubTitle        []*SubTitle      `xml:"sub-title"`
	Desc            []*Desc          `xml:"desc"`
	Credits         *Credits         `xml:"credits"`
	Date            string           `xml:"date,omitempty"`
	Category        []*Category      `xml:"category"`
	Keyword         []*Text          `xml:"keyword"`
	Language        *Text            `xml:"language"`
	OrigLanguage    *Text            `xml:"orig-language"`
	Length          *Length          `xml:"length"`
	Poster          []Poster         `xml:"icon"`
	URL             []URL            `xml:"url"`
	Country         []*Country       `xml:"country"`
	EpisodeNum      []*EpisodeNum    `xml:"episode-num"`
	Video           *Video           `xml:"video"`
	Audio           *Audio           `xml:"audio"`
	PreviouslyShown *PreviouslyShown `xml:"previously-shown"`
	Premiere        *Text            `xml:"premiere"`
	LastChance      *Text            `xml:"last-chance"`
	New             *New             `xml:"new"`
	Subtitles       []Subtitles      `xml:"subtitles"`
	Rating          []Rating         `xml:"rating"`
	StarRating      []StarRating     `xml:"star-rating"`
	Review          []Review         `xml:"review"`
	Image           []Image          `xml:"image"`

	// Year is not in the XMLTV DTD (iptvx.one), it is read into Date and not written
	Year string `xml:"year,omitempty"`
}

// Text is a text with an optional language.
type Text struct {
	Lang  string `xml:"lang,attr,omitempty"`
	Value string `xml:",chardata"`
}

// Title : Program Title
type Title = Text

// SubTitle : Brief Description
type SubTitle = Text

// Desc : Program Description
type Desc = Text

// Category : Categories
type Category = Text

// Country : Countries
type Country = Text

// Language : Languages
type Language = Text

// Length is the length of a programme.
type Length struct {
	Units string `xml:"units,attr"`
	Value string `xml:",chardata"`
}

// URL is a link to more information.
type URL struct {
	System string `xml:"system,attr,omitempty"`
	Value  string `xml:",chardata"`
}

// Rating : Rating
type Rating struct {
	System string `xml:"system,attr,omitempty"`
	Value  string `xml:"value"`
	Icon   []Icon `xml:"icon"`
}

// StarRating : Rating / Reviews
type StarRating struct {
	System string `xml:"system,attr,omitempty"`
	Value  string `xml:"value"`
	Icon   []Icon `xml:"icon"`
}

// EpisodeNum : Episode Numbering
type EpisodeNum struct {
	System string `xml:"system,attr,omitempty"`
	Value  string `xml:",chardata"`
}

// Poster : Program Poster / Cover
type Poster struct {
	Src    string `xml:"src,attr"`
	Width  string `xml:"width,attr,omitempty"`
	Height string `xml:"height,attr,omitempty"`
}

// Credits are the people of a programme.
type Credits struct {
	Director    []Person `xml:"director"`
	Actor       []Person `xml:"actor"`
	Writer      []Person `xml:"writer"`
	Adapter     []Person `xml:"adapter"`
	Producer    []Person `xml:"producer"`
	Composer    []Person `xml:"composer"`
	Editor      []Person `xml:"editor"`
	Presenter   []Person `xml:"presenter"`
	Commentator []Person `xml:"commentator"`
	Guest       []Person `xml:"guest"`
}

// Person is a person in the credits.
type Person struct {
	Role  string `xml:"role,attr,omitempty"`
	Guest string `xml:"guest,attr,omitempty"`
	Value string `xml:",chardata"`
}

// Director, Actor, Writer, Presenter and Producer are the names of the older versions.
type (
	Director  = Person
	Actor     = Person
	Writer    = Person
	Presenter = Person
	Producer  = Person
)

// Video : Video Metadata
type Video struct {
	Present string `xml:"present,omitempty"`
	Colour  string `xml:"colour,omitempty"`
	Aspect  string `xml:"aspect,omitempty"`
	Quality string `xml:"quality,omitempty"`
}

// Audio is the sound of a programme.
type Audio struct {
	Present string `xml:"present,omitempty"`
	Stereo  string `xml:"stereo,omitempty"`
}

// PreviouslyShown : Repetition or first Broadcast
type PreviouslyShown struct {
	Start   string `xml:"start,attr,omitempty"`
	Channel string `xml:"channel,attr,omitempty"`
}

// New : Declare the Broadcast as new
type New struct{}

// Subtitles are the subtitles of a programme.
type Subtitles struct {
	Type     string `xml:"type,attr,omitempty"`
	Language *Text  `xml:"language"`
}

// Review is a review of a programme.
type Review struct {
	Type     string `xml:"type,attr"`
	Source   string `xml:"source,attr,omitempty"`
	Reviewer string `xml:"reviewer,attr,omitempty"`
	Lang     string `xml:"lang,attr,omitempty"`
	Value    string `xml:",chardata"`
}

// Image is an image of a programme.
type Image struct {
	Type   string `xml:"type,attr,omitempty"`
	Size   string `xml:"size,attr,omitempty"`
	Orient string `xml:"orient,attr,omitempty"`
	System string `xml:"system,attr,omitempty"`
	Value  string `xml:",chardata"`
}
