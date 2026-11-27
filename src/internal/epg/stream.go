package epg

import (
	"bufio"
	"bytes"
	"compress/gzip"
	"encoding/xml"
	"errors"
	"fmt"
	"html"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/orisano/gosax"
	"golang.org/x/text/encoding/htmlindex"
)

// readXMLTV reads an XMLTV file element by element: channel is called for every channel and programme for every
// programme. A nil function skips its elements without decoding them, so that reading only the channels of a big
// file is fast.
func readXMLTV(file string, channel func(*Channel) error, programme func(*Program) error) error {
	var f, err = os.Open(filepath.Clean(file))
	if err != nil {
		return err
	}
	defer func() { _ = f.Close() }()

	return decodeXMLTV(f, channel, programme)
}

func decodeXMLTV(r io.Reader, channel func(*Channel) error, programme func(*Program) error) error {
	return scanXMLTV(r, func(name, _ string, element []byte) error {
		switch {
		case name == "channel" && channel != nil:
			var c Channel
			if err := decodeElement(element, &c); err != nil {
				return err
			}
			return channel(&c)

		case name == "programme" && programme != nil:
			var p Program
			if err := decodeElement(element, &p); err != nil {
				return err
			}
			normalizeProgramme(&p)
			return programme(&p)
		}
		return nil
	})
}

// decodeElement decodes one element leniently: HTML entities like &nbsp;, unclosed HTML tags and unknown
// elements are accepted, as EPG sources do not always follow XML and the XMLTV DTD.
func decodeElement(element []byte, v any) error {
	var d = xml.NewDecoder(bytes.NewReader(element))
	d.Strict = false
	d.AutoClose = xml.HTMLAutoClose
	d.Entity = xml.HTMLEntity
	return d.Decode(v)
}

// normalizeProgramme moves what sources write in other ways into the elements of the XMLTV DTD.
func normalizeProgramme(p *Program) {
	// <year> (iptvx.one) is the <date> of the DTD
	if p.Date == "" && p.Year != "" {
		p.Date = p.Year
	}
	p.Year = ""
}

// scanXMLTV reads the elements inside <tv> of an XMLTV document with gosax, many times faster than encoding/xml,
// without decoding them. fn gets the name of each element, the channel attribute of a programme and the bytes of
// the whole element, which are only valid during the call. A document in another encoding than UTF-8 (as declared
// in its XML declaration) is converted to UTF-8.
func scanXMLTV(r io.Reader, fn func(name, channel string, element []byte) error) error {
	r, err := utf8Reader(r)
	if err != nil {
		return err
	}

	var rd = gosax.NewReaderSize(r, 1<<16)
	rd.EmitSelfClosingTag = true

	var depth int
	var root bool
	var element []byte // the element in <tv> that is being collected
	var name, channel string

	for {
		var e, err = rd.Event()
		if err != nil {
			return fmt.Errorf("no XMLTV data: %w", err)
		}

		switch e.Type() {
		case gosax.EventEOF:
			if !root {
				return errors.New("no XMLTV data: the <tv> element is missing")
			}
			if depth > 0 {
				return errors.New("no XMLTV data: the document ends before </tv>")
			}
			return nil

		case gosax.EventStart:
			depth++
			switch {
			case !root:
				if n, _ := gosax.Name(e.Bytes); string(n) != "tv" {
					return fmt.Errorf("no XMLTV data: <%s> instead of <tv>", n)
				}
				root = true
				continue
			case depth == 2:
				var n, _ = gosax.Name(e.Bytes)
				name = string(n)
				channel = ""
				if name == "programme" {
					channel = attribute(e.Bytes, "channel")
				}
				element = append(element[:0], e.Bytes...)
				continue
			}

		case gosax.EventEnd:
			depth--
			switch depth {
			case 0:
				// </tv>, what follows is ignored
				return nil
			case 1:
				if !selfClosing(e) {
					element = append(element, e.Bytes...)
				}
				if err := fn(name, channel, element); err != nil {
					return err
				}
				element = element[:0]
				continue
			}
		}

		// Content of an element in <tv>
		if depth >= 2 && (e.Type() != gosax.EventEnd || !selfClosing(e)) {
			element = append(element, e.Bytes...)
		}
	}
}

// selfClosing reports whether an end event is a self-closing tag. Its bytes repeat the tag of the start event,
// which is already in the element.
func selfClosing(e gosax.Event) bool { return bytes.HasSuffix(e.Bytes, []byte("/>")) }

// xmlDeclaration finds the encoding of an XML declaration.
var xmlDeclaration = regexp.MustCompile(`^\s*<\?xml[^>]*\bencoding\s*=\s*["']([^"']+)["']`)

// utf8Reader returns r as UTF-8: a byte order mark is removed, another encoding is converted.
func utf8Reader(r io.Reader) (io.Reader, error) {
	var br = bufio.NewReaderSize(r, 1<<16)
	var start, _ = br.Peek(512)

	if bytes.HasPrefix(start, []byte("\xef\xbb\xbf")) {
		_, _ = br.Discard(3)
		return br, nil
	}

	var m = xmlDeclaration.FindSubmatch(start)
	if m == nil {
		return br, nil
	}
	var name = strings.ToLower(string(m[1]))
	if name == "utf-8" || name == "utf8" || name == "us-ascii" {
		return br, nil
	}

	var enc, err = htmlindex.Get(name)
	if err != nil {
		return nil, fmt.Errorf("XMLTV encoding %q is not supported", name)
	}
	return enc.NewDecoder().Reader(br), nil
}

// attribute returns the value of an attribute of a start tag, "" if it has none.
func attribute(tag []byte, name string) string {
	for i := 0; ; {
		var j = bytes.Index(tag[i:], []byte(name))
		if j < 0 {
			return ""
		}
		i += j
		var k = i + len(name)

		// The name must stand on its own: after a space, followed by "="
		if i == 0 || !isSpace(tag[i-1]) {
			i = k
			continue
		}
		for k < len(tag) && isSpace(tag[k]) {
			k++
		}
		if k >= len(tag) || tag[k] != '=' {
			i = k
			continue
		}
		k++
		for k < len(tag) && isSpace(tag[k]) {
			k++
		}
		if k >= len(tag) || (tag[k] != '"' && tag[k] != '\'') {
			return ""
		}
		var quote = tag[k]
		var end = bytes.IndexByte(tag[k+1:], quote)
		if end < 0 {
			return ""
		}
		var value = tag[k+1 : k+1+end]
		if bytes.IndexByte(value, '&') >= 0 {
			return html.UnescapeString(string(value))
		}
		return string(value)
	}
}

func isSpace(c byte) bool { return c == ' ' || c == '\t' || c == '\n' || c == '\r' }

// Count returns the number of channels and programmes of an XMLTV file and checks that it can be read.
func Count(file string) (channels, programmes int, err error) {
	var f *os.File
	if f, err = os.Open(filepath.Clean(file)); err != nil {
		return
	}
	defer func() { _ = f.Close() }()

	err = scanXMLTV(f, func(name, _ string, element []byte) error {
		switch name {
		case "channel":
			channels++
			// Channels are decoded, so that a file the mapping menu can not read is rejected
			return decodeElement(element, &Channel{})
		case "programme":
			programmes++
		}
		return nil
	})
	return
}

// xmltvWriter writes an XMLTV file and its gzip copy at the same time, element by element. The files replace the
// old ones only when they are complete.
type xmltvWriter struct {
	file, gzFile string
	// temporary files next to them
	tmp, gzTmp string

	f, gzf *os.File
	gz     *gzip.Writer
	buf    *bufio.Writer
	enc    *xml.Encoder
}

func newXMLTVWriter(file, gzFile, generator, source string) (*xmltvWriter, error) {
	var w = &xmltvWriter{file: file, gzFile: gzFile}
	var err error

	if w.f, err = os.CreateTemp(filepath.Dir(file), filepath.Base(file)+".*.tmp"); err != nil {
		return nil, err
	}
	w.tmp = w.f.Name()
	if w.gzf, err = os.CreateTemp(filepath.Dir(gzFile), filepath.Base(gzFile)+".*.tmp"); err != nil {
		w.abort()
		return nil, err
	}
	w.gzTmp = w.gzf.Name()
	w.gz = gzip.NewWriter(w.gzf)
	w.buf = bufio.NewWriterSize(io.MultiWriter(w.f, w.gz), 1<<16)

	if _, err = w.buf.WriteString(xml.Header); err != nil {
		w.abort()
		return nil, err
	}

	w.enc = xml.NewEncoder(w.buf)
	w.enc.Indent(xmltvPrefix, xmltvIndent)
	err = w.enc.EncodeToken(xml.StartElement{
		Name: xml.Name{Local: "tv"},
		Attr: []xml.Attr{
			{Name: xml.Name{Local: "generator-info-name"}, Value: generator},
			{Name: xml.Name{Local: "source-info-name"}, Value: source},
		},
	})
	if err != nil {
		w.abort()
		return nil, err
	}
	return w, nil
}

func (w *xmltvWriter) channel(c *Channel) error {
	return w.enc.EncodeElement(c, xml.StartElement{Name: xml.Name{Local: "channel"}})
}

func (w *xmltvWriter) programme(p *Program) error {
	return w.enc.EncodeElement(p, xml.StartElement{Name: xml.Name{Local: "programme"}})
}

// The indentation of the written file: elements in <tv> are on the second level
const (
	xmltvPrefix = "  "
	xmltvIndent = "    "
)

// close completes both files and replaces the old ones.
func (w *xmltvWriter) close() error {
	var err = errors.Join(
		w.enc.EncodeToken(xml.EndElement{Name: xml.Name{Local: "tv"}}),
		w.enc.Flush(),
		w.buf.Flush(),
		w.gz.Close(),
		w.f.Close(),
		w.gzf.Close(),
	)
	if err != nil {
		w.remove()
		return err
	}
	if err := os.Rename(w.tmp, w.file); err != nil {
		w.remove()
		return err
	}
	return os.Rename(w.gzTmp, w.gzFile)
}

// abort removes the incomplete files.
func (w *xmltvWriter) abort() {
	if w.f != nil {
		_ = w.f.Close()
	}
	if w.gzf != nil {
		_ = w.gzf.Close()
	}
	w.remove()
}

func (w *xmltvWriter) remove() {
	if w.tmp != "" {
		_ = os.Remove(w.tmp)
	}
	if w.gzTmp != "" {
		_ = os.Remove(w.gzTmp)
	}
}
