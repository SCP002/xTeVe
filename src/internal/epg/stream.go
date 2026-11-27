package epg

import (
	"bufio"
	"compress/gzip"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
)

// readXMLTV reads an XMLTV file element by element: channel is called for every channel and programme for every
// programme. A nil function skips its elements without decoding them, so that reading only the channels of a big
// file is fast and needs little memory.
func readXMLTV(file string, channel func(*Channel) error, programme func(*Program) error) error {
	var f, err = os.Open(filepath.Clean(file))
	if err != nil {
		return err
	}
	defer func() { _ = f.Close() }()

	return decodeXMLTV(bufio.NewReaderSize(f, 1<<16), channel, programme)
}

func decodeXMLTV(r io.Reader, channel func(*Channel) error, programme func(*Program) error) error {
	var d = xml.NewDecoder(r)
	var root bool

	for {
		var token, err = d.Token()
		if errors.Is(err, io.EOF) {
			if !root {
				return errors.New("no XMLTV data: the <tv> element is missing")
			}
			return nil
		}
		if err != nil {
			return err
		}

		var start, ok = token.(xml.StartElement)
		if !ok {
			continue
		}

		switch {
		case !root:
			if start.Name.Local != "tv" {
				return fmt.Errorf("no XMLTV data: <%s> instead of <tv>", start.Name.Local)
			}
			root = true

		case start.Name.Local == "channel" && channel != nil:
			var c Channel
			if err := d.DecodeElement(&c, &start); err != nil {
				return err
			}
			if err := channel(&c); err != nil {
				return err
			}

		case start.Name.Local == "programme" && programme != nil:
			var p Program
			if err := d.DecodeElement(&p, &start); err != nil {
				return err
			}
			if err := programme(&p); err != nil {
				return err
			}

		default:
			// Elements inside <tv> that are not needed
			if err := d.Skip(); err != nil {
				return err
			}
		}
	}
}

// Count returns the number of channels and programmes of an XMLTV file, which also checks that the file is valid.
func Count(file string) (channels, programmes int, err error) {
	err = readXMLTV(file,
		func(*Channel) error { channels++; return nil },
		func(*Program) error { programmes++; return nil },
	)
	return
}

// xmltvWriter writes an XMLTV file and its gzip copy at the same time, element by element. The files replace the
// old ones only when they are complete.
type xmltvWriter struct {
	file, gzFile string

	f, gzf *os.File
	gz     *gzip.Writer
	buf    *bufio.Writer
	enc    *xml.Encoder
}

func newXMLTVWriter(file, gzFile, generator, source string) (*xmltvWriter, error) {
	var w = &xmltvWriter{file: file, gzFile: gzFile}
	var err error

	if w.f, err = os.Create(file + ".tmp"); err != nil {
		return nil, err
	}
	if w.gzf, err = os.Create(gzFile + ".tmp"); err != nil {
		w.abort()
		return nil, err
	}
	w.gz = gzip.NewWriter(w.gzf)
	w.buf = bufio.NewWriterSize(io.MultiWriter(w.f, w.gz), 1<<16)

	if _, err = w.buf.WriteString(xml.Header); err != nil {
		w.abort()
		return nil, err
	}

	w.enc = xml.NewEncoder(w.buf)
	w.enc.Indent("  ", "    ")
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
	if err := os.Rename(w.file+".tmp", w.file); err != nil {
		w.remove()
		return err
	}
	return os.Rename(w.gzFile+".tmp", w.gzFile)
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
	_ = os.Remove(w.file + ".tmp")
	_ = os.Remove(w.gzFile + ".tmp")
}
