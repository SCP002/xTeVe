package epg

import (
	"encoding/xml"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"xteve/src/internal/config"

	"golang.org/x/text/encoding/charmap"
)

func TestDecodeXMLTV(t *testing.T) {
	var file = `<?xml version="1.0"?><!DOCTYPE tv SYSTEM "xmltv.dtd">
<tv generator-info-name="x">
  <channel id="a"><display-name>A</display-name><icon src="http://a.png"/></channel>
  <unknown><deep>skipped</deep></unknown>
  <programme channel="a" start="1" stop="2"><title>One</title></programme>
  <channel id="b"><display-name>B</display-name></channel>
  <programme channel="b" start="3" stop="4"><title>Two</title></programme>
</tv>`

	var channels, programmes []string
	var err = decodeXMLTV(strings.NewReader(file),
		func(c *Channel) error { channels = append(channels, c.ID+":"+c.IconSrc()); return nil },
		func(p *Program) error { programmes = append(programmes, p.Channel+":"+p.Title[0].Value); return nil },
	)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Join(channels, ",") != "a:http://a.png,b:" || strings.Join(programmes, ",") != "a:One,b:Two" {
		t.Errorf("channels %v, programmes %v", channels, programmes)
	}

	// Only channels, programmes are skipped
	channels = nil
	if err := decodeXMLTV(strings.NewReader(file), func(c *Channel) error {
		channels = append(channels, c.ID)
		return nil
	}, nil); err != nil || len(channels) != 2 {
		t.Errorf("channels %v, err %v", channels, err)
	}

	for name, invalid := range map[string]string{
		"html":      "<html><body>Not found</body></html>",
		"empty":     "",
		"truncated": `<tv><channel id="a"><display-name>A</display`,
	} {
		if err := decodeXMLTV(strings.NewReader(invalid), nil, nil); err == nil {
			t.Errorf("%s: no error", name)
		}
	}
}

func TestXMLTVWriter(t *testing.T) {
	var dir = t.TempDir()
	var file, gz = filepath.Join(dir, "xteve.xml"), filepath.Join(dir, "xteve.xml.gz")
	if err := os.WriteFile(file, []byte("old"), 0644); err != nil {
		t.Fatal(err)
	}

	// An aborted file keeps the old one
	w, err := newXMLTVWriter(file, gz, "xTeVe", "xTeVe - test")
	if err != nil {
		t.Fatal(err)
	}
	w.abort()
	if content, _ := os.ReadFile(file); string(content) != "old" {
		t.Errorf("xteve.xml = %q after abort, want the old file", content)
	}

	w, err = newXMLTVWriter(file, gz, "xTeVe", "xTeVe - test")
	if err != nil {
		t.Fatal(err)
	}
	if err := w.channel(&Channel{ID: "1", DisplayNames: []DisplayName{{Value: "One"}}}); err != nil {
		t.Fatal(err)
	}
	if err := w.programme(&Program{Channel: "1", Start: "s", Stop: "e", Title: []*Title{{Value: "News"}}}); err != nil {
		t.Fatal(err)
	}
	if err := w.close(); err != nil {
		t.Fatal(err)
	}

	channels, programmes, err := Count(file)
	if err != nil || channels != 1 || programmes != 1 {
		t.Errorf("written file: %d channels, %d programmes, err %v", channels, programmes, err)
	}
	if files, _ := filepath.Glob(filepath.Join(dir, "*.tmp")); len(files) != 0 {
		t.Errorf("temporary files left: %v", files)
	}
}

// EPG sources do not always follow XML or the XMLTV DTD; what can be read is read.
func TestDecodeXMLTVLenient(t *testing.T) {
	var cp1251 = func(s string) string {
		var b, err = charmap.Windows1251.NewEncoder().String(s)
		if err != nil {
			t.Fatal(err)
		}
		return b
	}

	tests := map[string]struct {
		file       string
		channels   string
		programmes string
	}{
		"windows-1251": {
			file: `<?xml version="1.0" encoding="windows-1251"?>` + cp1251(
				`<tv><channel id="1"><display-name>Первый</display-name></channel>`+
					`<programme channel="1" start="1"><title>Новости</title></programme></tv>`),
			channels: "1:Первый", programmes: "1:Новости",
		},
		"BOM, entities, CDATA, comments": {
			file: "\uFEFF" + `<?xml version="1.0"?><!-- generated --><tv>` +
				`<channel id="a&amp;b"><display-name>A&nbsp;B</display-name></channel>` +
				`<programme channel='a&amp;b' start="1"><title><![CDATA[News <live>]]></title><!-- x --></programme>` +
				`</tv>`,
			channels: "a&b:A B", programmes: "a&b:News <live>",
		},
		"self-closing, unknown elements, year": {
			file: `<tv><foo><bar/></foo><channel id="1"><display-name>One</display-name><extra/></channel>` +
				`<programme channel="1" start="1"/>` +
				`<programme channel="1" start="2"><title>Film</title><year>2021</year><rank>5</rank>` +
				`<icon src="a.jpg"/><icon src="b.jpg" /></programme></tv>`,
			channels: "1:One", programmes: "1:,1:Film/2021/a.jpg b.jpg",
		},
	}

	for name, tt := range tests {
		var channels, programmes []string
		var err = decodeXMLTV(strings.NewReader(tt.file),
			func(c *Channel) error { channels = append(channels, c.ID+":"+c.DisplayNames[0].Value); return nil },
			func(p *Program) error {
				var s = p.Channel + ":"
				if len(p.Title) > 0 {
					s += p.Title[0].Value
				}
				if p.Date != "" {
					s += "/" + p.Date
				}
				if len(p.Poster) > 0 {
					var srcs []string
					for _, poster := range p.Poster {
						srcs = append(srcs, poster.Src)
					}
					s += "/" + strings.Join(srcs, " ")
				}
				programmes = append(programmes, s)
				return nil
			},
		)
		if err != nil {
			t.Errorf("%s: %v", name, err)
			continue
		}
		if strings.Join(channels, ",") != tt.channels || strings.Join(programmes, ",") != tt.programmes {
			t.Errorf("%s: channels %q, programmes %q; want %q, %q", name, channels, programmes, tt.channels,
				tt.programmes)
		}
	}
}

// Programmes are written in the order and with the elements of the XMLTV DTD, whatever the source does.
func TestProgrammeNormalized(t *testing.T) {
	var source = `<tv><programme start="20260101120000 +0000" stop="20260101130000 +0000" channel="x" showview="12">` +
		`<rating system="MPAA"><value>PG</value></rating><category lang="">News</category>` +
		`<live/><new/><year>2021</year><video><quality></quality></video><credits></credits>` +
		`<title lang="ru">Новости</title><keyword>politics</keyword><length units="minutes">60</length>` +
		`<icon src=""/><icon src="http://img/a.jpg" width="100"/><desc>Text</desc>` +
		`<subtitles type="teletext"><language>en</language></subtitles>` +
		`<credits><actor role="Host">Ann</actor><director>Bob</director></credits>` +
		`<review type="text">Good</review><image type="poster">http://img/p.jpg</image></programme></tv>`

	var p *Program
	if err := decodeXMLTV(strings.NewReader(source), nil, func(x *Program) error { p = x; return nil }); err != nil {
		t.Fatal(err)
	}

	var m, _ = managerTest(t)
	setSettings(t, func(s *config.Settings) { s.XepgReplaceMissingImages = true })
	var b = &build{m: m}
	var target = newProgrammeTarget(XEPGChannel{XChannelID: "1001", XName: "News HD", TvgLogo: "http://img/logo.png"})
	encoded, err := xml.Marshal(b.programFor(target, p))
	if err != nil {
		t.Fatal(err)
	}
	var out = string(encoded)

	// The order of the DTD
	var order = []string{"<title", "<desc", "<credits", "<director", "<actor", "<date", "<category", "<keyword",
		"<length", "<icon", "<video", "<new", "<subtitles", "<rating", "<review", "<image"}
	var last = -1
	for _, tag := range order {
		var i = strings.Index(out, tag)
		if i < 0 || i < last {
			t.Errorf("%s missing or out of order in\n%s", tag, out)
		}
		last = i
	}

	for _, unwanted := range []string{"<live", "<year", `lang=""`, `src=""`, "<credits></credits>",
		`height=""`, "<quality></quality>"} {
		if strings.Contains(out, unwanted) {
			t.Errorf("%s in\n%s", unwanted, out)
		}
	}
	for _, wanted := range []string{`channel="1001"`, `showview="12"`, "<date>2021</date>", `role="Host"`,
		"<quality>HDTV</quality>", `src="http://img/a.jpg"`, `width="100"`} {
		if !strings.Contains(out, wanted) {
			t.Errorf("%s missing in\n%s", wanted, out)
		}
	}
	if strings.Count(out, "<icon") != 1 {
		t.Errorf("icons in\n%s, want only the one with a source", out)
	}
}

// A programme without images gets the channel logo; without a logo it gets no icon at all.
func TestProgrammeMissingImage(t *testing.T) {
	var m, _ = managerTest(t)
	setSettings(t, func(s *config.Settings) { s.XepgReplaceMissingImages = true })
	var b = &build{m: m}
	var p = &Program{Start: "1", Channel: "x", Title: []*Title{{Value: "T"}}}

	var withLogo = b.programFor(newProgrammeTarget(XEPGChannel{XChannelID: "1", TvgLogo: "http://logo.png"}), p)
	if len(withLogo.Poster) != 1 || withLogo.Poster[0].Src != "http://logo.png" {
		t.Errorf("posters with channel logo = %+v", withLogo.Poster)
	}
	var withoutLogo = b.programFor(newProgrammeTarget(XEPGChannel{XChannelID: "1"}), p)
	if len(withoutLogo.Poster) != 0 {
		t.Errorf("posters without channel logo = %+v", withoutLogo.Poster)
	}
}

// The dummy programmes belong to the channel number, not to the dummy mapping.
func TestDummyProgrammesChannel(t *testing.T) {
	var m, info = managerTest(t)
	var b = m.newBuild()
	b.files = []string{"x"}
	b.channels = Channels{"x-ID.0": {XEPG: "x-ID.0", XChannelID: "1005", XName: "Dummy", XActive: true,
		XmltvFile: dummyFile, XMapping: "60_Minutes"}}

	if err := b.createXMLTVFile(); err != nil {
		t.Fatal(err)
	}
	var programmes, wrong int
	if err := readXMLTV(info.File.XML, nil, func(p *Program) error {
		programmes++
		if p.Channel != "1005" {
			wrong++
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	if programmes == 0 || wrong > 0 {
		t.Errorf("dummy programmes: %d, %d with another channel than 1005", programmes, wrong)
	}
}
