// Package playlist builds the list of streams from the playlists (M3U files and HDHomeRun tuners) and the filters.
//
// The list is published as an immutable snapshot (Store): it is built completely and then replaces the previous one,
// so readers never see a list that is half built. A published snapshot and its streams must not be changed.
package playlist

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"sort"
	"sync/atomic"

	"xteve/src/internal/errcode"
	"xteve/src/internal/logger"
	m3u "xteve/src/internal/m3u-parser"
)

// Stream is a stream of a playlist: the values of its M3U entry ("name", "url", "group-title", "tvg-id", ...) and
// the keys added by xTeVe ("_file.m3u.id", "_values", "_uuid.key", ...).
type Stream = map[string]string

// Group is a group of streams (group-title) for the filter menu.
type Group struct {
	Text  string // "News (12)"
	Value string // "News"
}

// Snapshot is the list of streams.
type Snapshot struct {
	All      []Stream
	Active   []Stream // accepted by the filters, or all streams without filters
	Inactive []Stream

	// Groups of the M3U streams, sorted by name
	Groups []Group

	// "name [group]" of the streams, sorted, for the web UI
	PreviewActive   []string
	PreviewInactive []string

	Filters []Filter
}

// GroupTexts returns the texts of the groups for the web UI.
func (s *Snapshot) GroupTexts() []string {
	var texts = make([]string, len(s.Groups))
	for i, g := range s.Groups {
		texts[i] = g.Text
	}
	return texts
}

// GroupValues returns the values of the groups for the web UI.
func (s *Snapshot) GroupValues() []string {
	var values = make([]string, len(s.Groups))
	for i, g := range s.Groups {
		values[i] = g.Value
	}
	return values
}

// Source is a downloaded playlist.
type Source struct {
	ID   string // provider ID, e.g. "M..."
	Type string // "m3u" or "hdhr"
	Name string
	Path string // the local file
}

// Compatibility is the share of streams of a playlist with an attribute, in percent, and the number of streams.
type Compatibility = map[string]int

// Options are the settings that the list depends on.
type Options struct {
	Filters               map[int64]any
	DisallowURLDuplicates bool
}

// Build parses the sources and builds the list of streams. It also returns the compatibility of each source by its
// ID. A source that cannot be parsed is skipped.
func Build(sources []Source, opts Options) (*Snapshot, map[string]Compatibility, error) {
	var filters, err = Rules(opts.Filters)
	if err != nil {
		return nil, nil, err
	}

	// The previews are sent to the web UI, which expects lists rather than null
	var snapshot = &Snapshot{Filters: filters, PreviewActive: []string{}, PreviewInactive: []string{}}
	var compatibilities = make(map[string]Compatibility)
	var urls = make(map[string]bool)
	var groups = make(map[string]int)

	for _, source := range sources {
		var streams, err = parseSource(source)
		if err != nil {
			// The playlist is skipped, the others are still used
			errcode.Report(fmt.Errorf("%s: %w", source.Name, err), 1005)
			continue
		}

		var groupTitle, tvgID, uuid int

		for _, s := range streams {
			s["_file.m3u.path"] = source.Path
			s["_file.m3u.name"] = source.Name
			s["_file.m3u.id"] = source.ID

			if opts.DisallowURLDuplicates {
				if urls[s["url"]] {
					logger.Info(fmt.Sprintf("Streams:Found duplicated URL %v, ignoring the channel %v", s["url"],
						s["name"]))
					continue
				}
				urls[s["url"]] = true
			}

			// Calculate Compatibility
			if len(s["_uuid.key"]) > 0 {
				uuid++
			}
			if group := s["group-title"]; len(group) > 0 {
				groups[group]++
				groupTitle++
			}
			if len(s["tvg-id"]) > 0 {
				tvgID++
			}

			snapshot.All = append(snapshot.All, s)

			var preview string
			if name, ok := s["name"]; ok {
				preview = fmt.Sprintf("%s [%s]", name, s["group-title"])
			}

			if Accept(s, filters) {
				snapshot.PreviewActive = append(snapshot.PreviewActive, preview)
				snapshot.Active = append(snapshot.Active, s)
			} else {
				snapshot.PreviewInactive = append(snapshot.PreviewInactive, preview)
				snapshot.Inactive = append(snapshot.Inactive, s)
			}
		}

		compatibilities[source.ID] = Compatibility{
			"tvg.id":      percent(tvgID, len(streams)),
			"group.title": percent(groupTitle, len(streams)),
			"stream.id":   percent(uuid, len(streams)),
			"streams":     len(streams),
		}
	}

	for group, count := range groups {
		snapshot.Groups = append(snapshot.Groups, Group{Text: fmt.Sprintf("%s (%d)", group, count), Value: group})
	}
	sort.Slice(snapshot.Groups, func(i, j int) bool { return snapshot.Groups[i].Value < snapshot.Groups[j].Value })

	// Without filters all streams are active
	if len(snapshot.Active) == 0 && len(opts.Filters) == 0 {
		snapshot.Active = snapshot.All
		snapshot.Inactive = nil

		snapshot.PreviewActive = snapshot.PreviewInactive
		snapshot.PreviewInactive = []string{}
	}

	sort.Strings(snapshot.PreviewActive)
	sort.Strings(snapshot.PreviewInactive)

	return snapshot, compatibilities, nil
}

func percent(n, total int) int {
	if n == 0 || total == 0 {
		return 0
	}
	return n * 100 / total
}

// parseSource reads and parses a downloaded playlist.
func parseSource(source Source) ([]Stream, error) {
	var content, err = os.ReadFile(source.Path)
	if err != nil {
		return nil, err
	}

	switch source.Type {
	case "m3u":
		var channels, err = m3u.MakeInterfaceFromM3U(content)
		if err != nil {
			return nil, err
		}
		var streams = make([]Stream, 0, len(channels))
		for _, c := range channels {
			if s, ok := c.(map[string]string); ok {
				streams = append(streams, s)
			}
		}
		return streams, nil

	case "hdhr":
		return parseHDHR(content, source.Name, source.ID)
	}

	return nil, fmt.Errorf("unknown playlist type: %s", source.Type)
}

// parseHDHR parses the lineup.json of an HDHomeRun tuner.
func parseHDHR(content []byte, playlistName, id string) ([]Stream, error) {
	var lineup []struct {
		GuideName   string
		GuideNumber string
		URL         string
	}
	if err := json.Unmarshal(content, &lineup); err != nil {
		return nil, err
	}

	var streams = make([]Stream, 0, len(lineup))
	for _, c := range lineup {
		if len(c.GuideName) == 0 || len(c.URL) == 0 {
			continue
		}
		streams = append(streams, Stream{
			"group-title": playlistName,
			"name":        c.GuideName,
			"tvg-id":      c.GuideName,
			"url":         c.URL,
			"ID-" + id:    c.GuideNumber,
			"_uuid.key":   "ID-" + id,
			"_values":     playlistName + " " + c.GuideName,
		})
	}
	if len(streams) == 0 && len(lineup) > 0 {
		return nil, errors.New("the lineup has no channels with a name and URL")
	}
	return streams, nil
}

// Store holds the current list of streams.
type Store struct {
	current atomic.Pointer[Snapshot]
}

// Get returns the current list. It is empty before the first Set and must not be changed.
func (st *Store) Get() *Snapshot {
	if s := st.current.Load(); s != nil {
		return s
	}
	return &Snapshot{PreviewActive: []string{}, PreviewInactive: []string{}}
}

// Set publishes a new list.
func (st *Store) Set(s *Snapshot) { st.current.Store(s) }
