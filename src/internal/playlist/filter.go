package playlist

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"xteve/src/internal/config"
)

// Filter is a filter rule built from the filter settings.
type Filter struct {
	CaseSensitive   bool
	PreserveMapping bool
	Rule            string
	Type            string
	StartingChannel string
}

// FilterSettings is a filter as saved in the settings.
type FilterSettings = config.FilterSettings

// Rules builds the filter rules from the filter settings.
func Rules(settings map[int64]FilterSettings) (filters []Filter, err error) {
	for _, filter := range settings {
		switch filter.Type {
		case "custom-filter":
			filters = append(filters, Filter{
				CaseSensitive: filter.CaseSensitive,
				Rule:          filter.Filter,
				Type:          filter.Type,
			})

		case "group-title":
			var include, exclude string
			if len(filter.Include) > 0 {
				include = fmt.Sprintf(" {%s}", filter.Include)
			}
			if len(filter.Exclude) > 0 {
				exclude = fmt.Sprintf(" !{%s}", filter.Exclude)
			}

			filters = append(filters, Filter{
				CaseSensitive:   filter.CaseSensitive,
				PreserveMapping: filter.PreserveMapping,
				StartingChannel: filter.StartingChannel,
				Rule:            fmt.Sprintf("%s%s%s", filter.Filter, include, exclude),
				Type:            filter.Type,
			})
		}
	}

	return filters, nil
}

// Accept reports whether one of the filters accepts the stream. A stream accepted by a group filter gets the
// "_preserve-mapping" and "_starting-channel" of that filter.
func Accept(stream Stream, filters []Filter) (status bool) {
	var regexpYES = `[{]+[^}]+[}]`
	var regexpNO = `!+[{]+[^}]+[}]`

	for _, filter := range filters {
		if filter.Rule == "" {
			continue
		}

		var group, name, search string
		var exclude, include string
		var match = false

		var streamValues = strings.ReplaceAll(stream["_values"], "\r", "")

		if v, ok := stream["group-title"]; ok {
			group = v
		}

		if v, ok := stream["name"]; ok {
			name = v
		}

		// Unwanted Streams !{DEU}
		r := regexp.MustCompile(regexpNO)
		val := r.FindStringSubmatch(filter.Rule)

		if len(val) == 1 {
			exclude = val[0][2 : len(val[0])-1]
			filter.Rule = strings.ReplaceAll(filter.Rule, " "+val[0], "")
			filter.Rule = strings.ReplaceAll(filter.Rule, val[0], "")
		}

		// Required Streams {DEU}
		r = regexp.MustCompile(regexpYES)
		val = r.FindStringSubmatch(filter.Rule)

		if len(val) == 1 {
			include = val[0][1 : len(val[0])-1]
			filter.Rule = strings.ReplaceAll(filter.Rule, " "+val[0], "")
			filter.Rule = strings.ReplaceAll(filter.Rule, val[0], "")
		}

		switch filter.CaseSensitive {
		case false:
			streamValues = strings.ToLower(streamValues)
			filter.Rule = strings.ToLower(filter.Rule)
			exclude = strings.ToLower(exclude)
			include = strings.ToLower(include)
			group = strings.ToLower(group)
			name = strings.ToLower(name)
		}

		switch filter.Type {
		case "group-title":
			search = name
			match = group == filter.Rule

		case "custom-filter":
			search = streamValues
			match = strings.Contains(search, filter.Rule)
		}

		// Include and exclude conditions only apply to their own filter, so a
		// stream rejected by one filter can still be accepted by another one.
		// The filters come from a map and have no fixed order.
		if !match {
			continue
		}

		if len(exclude) > 0 && !checkConditions(search, exclude, "exclude") {
			continue
		}

		if len(include) > 0 && !checkConditions(search, include, "include") {
			continue
		}

		if filter.Type == "group-title" {
			stream["_preserve-mapping"] = strconv.FormatBool(filter.PreserveMapping)
			stream["_starting-channel"] = filter.StartingChannel
		}

		return true
	}

	return false
}

// Conditions for the Filter
func checkConditions(streamValues, conditions, coType string) (status bool) {
	switch coType {
	case "exclude":
		status = true

	case "include":
		status = false
	}

	conditions = strings.ReplaceAll(conditions, ", ", ",")
	conditions = strings.ReplaceAll(conditions, " ,", ",")

	var keys = strings.Split(conditions, ",")

	for _, key := range keys {
		// An empty key (from "a,,b" or a trailing comma) would match everything
		if key == "" {
			continue
		}

		if strings.Contains(streamValues, key) {
			switch coType {
			case "exclude":
				return false

			case "include":
				return true
			}
		}
	}

	return
}
