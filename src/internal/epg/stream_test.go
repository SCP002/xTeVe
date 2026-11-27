package epg

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
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
		func(c *Channel) error { channels = append(channels, c.ID+":"+c.Icon.Src); return nil },
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
