package config

import (
	"encoding/json"
	"maps"
	"strconv"
)

// Provider is a playlist (M3U), an HDHomeRun tuner or an XMLTV file, with its download statistics.
type Provider struct {
	ID          string `json:"id.provider"`
	Name        string `json:"name"`
	Description string `json:"description"`
	Type        string `json:"type"`
	// File is the local copy in the data folder
	File       string `json:"file.xteve"`
	FileSource string `json:"file.source"`
	// Tuner is the number of streams at a time, not used for XMLTV files
	Tuner      Tuner  `json:"tuner,omitempty"`
	LastUpdate string `json:"last.update"`

	Compatibility   map[string]int `json:"compatibility"`
	CounterError    float64        `json:"counter.error"`
	CounterDownload float64        `json:"counter.download"`
	Availability    int            `json:"provider.availability"`

	// New marks a provider that is added and not downloaded yet
	New bool `json:"new,omitempty"`
}

// Providers are the providers of one type by provider ID.
type Providers map[string]Provider

func (p Providers) clone() Providers {
	if p == nil {
		return nil
	}
	var c = make(Providers, len(p))
	for id, provider := range p {
		provider.Compatibility = maps.Clone(provider.Compatibility)
		c[id] = provider
	}
	return c
}

// ProviderUpdate is a change of a provider from the web UI: only the fields that are set change.
type ProviderUpdate struct {
	Name        *string `json:"name,omitempty"`
	Description *string `json:"description,omitempty"`
	FileSource  *string `json:"file.source,omitempty"`
	Tuner       *Tuner  `json:"tuner,omitempty"`
	// Delete removes the provider
	Delete bool `json:"delete,omitempty"`
}

// Apply changes p with the fields that are set.
func (u ProviderUpdate) Apply(p *Provider) {
	if u.Name != nil {
		p.Name = *u.Name
	}
	if u.Description != nil {
		p.Description = *u.Description
	}
	if u.FileSource != nil {
		p.FileSource = *u.FileSource
	}
	if u.Tuner != nil {
		p.Tuner = *u.Tuner
	}
}

// Tuner is a number of tuners. It is read from a number or, as older versions saved it, from a string.
type Tuner int

// UnmarshalJSON reads a number or a string with a number.
func (t *Tuner) UnmarshalJSON(b []byte) error {
	var n int
	if err := json.Unmarshal(b, &n); err == nil {
		*t = Tuner(n)
		return nil
	}
	var s string
	if err := json.Unmarshal(b, &s); err != nil {
		return err
	}
	if s == "" {
		*t = 0
		return nil
	}
	n, err := strconv.Atoi(s)
	if err != nil {
		return err
	}
	*t = Tuner(n)
	return nil
}

// FilterSettings is a filter as saved in the settings.
type FilterSettings struct {
	CaseSensitive   bool   `json:"caseSensitive"`
	PreserveMapping bool   `json:"preserveMapping"`
	Description     string `json:"description"`
	Exclude         string `json:"exclude"`
	Filter          string `json:"filter"`
	Include         string `json:"include"`
	Name            string `json:"name"`
	Rule            string `json:"rule,omitempty"`
	Type            string `json:"type"`
	StartingChannel string `json:"startingChannel"`
}

// FilterUpdate is a change of a filter from the web UI: only the fields that are set change.
type FilterUpdate struct {
	CaseSensitive   *bool   `json:"caseSensitive,omitempty"`
	PreserveMapping *bool   `json:"preserveMapping,omitempty"`
	Description     *string `json:"description,omitempty"`
	Exclude         *string `json:"exclude,omitempty"`
	Filter          *string `json:"filter,omitempty"`
	Include         *string `json:"include,omitempty"`
	Name            *string `json:"name,omitempty"`
	Type            *string `json:"type,omitempty"`
	StartingChannel *string `json:"startingChannel,omitempty"`
	// Delete removes the filter
	Delete bool `json:"delete,omitempty"`
}

// Apply changes f with the fields that are set.
func (u FilterUpdate) Apply(f *FilterSettings) {
	var set = func(dst *string, src *string) {
		if src != nil {
			*dst = *src
		}
	}
	if u.CaseSensitive != nil {
		f.CaseSensitive = *u.CaseSensitive
	}
	if u.PreserveMapping != nil {
		f.PreserveMapping = *u.PreserveMapping
	}
	set(&f.Description, u.Description)
	set(&f.Exclude, u.Exclude)
	set(&f.Filter, u.Filter)
	set(&f.Include, u.Include)
	set(&f.Name, u.Name)
	set(&f.Type, u.Type)
	set(&f.StartingChannel, u.StartingChannel)
}
