package playlist

import (
	"reflect"
	"testing"
)

// rules builds the filter rules from filter settings.
func rules(t *testing.T, filters ...FilterSettings) []Filter {
	t.Helper()

	var settings = make(map[int64]FilterSettings)
	for i, f := range filters {
		settings[int64(i)] = f
	}
	var r, err = Rules(settings)
	if err != nil {
		t.Fatal(err)
	}
	return r
}

func groupFilter(group, include, exclude string) FilterSettings {
	return FilterSettings{
		Type:            "group-title",
		Filter:          group,
		Include:         include,
		Exclude:         exclude,
		PreserveMapping: true,
		StartingChannel: "1000",
	}
}

func testStream(group, name string) map[string]string {
	return map[string]string{
		"group-title": group,
		"name":        name,
		"_values":     name + " " + group + " http://example.com/" + name,
	}
}

func TestCreateFilterRules(t *testing.T) {
	var filters = rules(t,
		groupFilter("News", "HD", "SD"),
		FilterSettings{Type: "custom-filter", Filter: "Sport", CaseSensitive: true},
	)

	var want = []Filter{
		{Type: "group-title", Rule: "News {HD} !{SD}", PreserveMapping: true, StartingChannel: "1000"},
		{Type: "custom-filter", Rule: "Sport", CaseSensitive: true},
	}
	// Settings.Filter is a map, so the order of the rules is not fixed
	if len(filters) != len(want) {
		t.Fatalf("filters = %+v", filters)
	}
	for _, w := range want {
		var found bool
		for _, f := range filters {
			found = found || reflect.DeepEqual(f, w)
		}
		if !found {
			t.Errorf("filters = %+v, missing %+v", filters, w)
		}
	}
}

func TestFilterThisStreamGroup(t *testing.T) {
	tests := []struct {
		name             string
		include, exclude string
		stream           map[string]string
		want             bool
	}{
		{name: "group matches", stream: testStream("News", "BBC"), want: true},
		{name: "group case insensitive", stream: testStream("NEWS", "BBC"), want: true},
		{name: "other group", stream: testStream("Sport", "BBC"), want: false},
		{name: "group is not a substring match", stream: testStream("News HD", "BBC"), want: false},
		{name: "include matches", include: "HD", stream: testStream("News", "BBC HD"), want: true},
		{name: "include one of several", include: "FHD, HD", stream: testStream("News", "BBC HD"), want: true},
		{name: "include misses", include: "HD", stream: testStream("News", "BBC SD"), want: false},
		{name: "exclude matches", exclude: "SD", stream: testStream("News", "BBC SD"), want: false},
		{name: "exclude misses", exclude: "SD", stream: testStream("News", "BBC HD"), want: true},
		{name: "include and exclude", include: "BBC", exclude: "SD", stream: testStream("News", "BBC SD"), want: false},
		{name: "include with a dot", include: "Sky.de", stream: testStream("News", "Sky.de News"), want: true},
		{name: "exclude with a dot", exclude: "Sky.de", stream: testStream("News", "BBC"), want: true},
		{name: "trailing comma in exclude", exclude: "SD,", stream: testStream("News", "BBC HD"), want: true},
		{name: "trailing comma in include", include: "FHD,", stream: testStream("News", "BBC HD"), want: false},
	}
	for _, tt := range tests {
		var filters = rules(t, groupFilter("News", tt.include, tt.exclude))

		if got := Accept(tt.stream, filters); got != tt.want {
			t.Errorf("%s: filterThisStream = %v, want %v (rule %q)", tt.name, got, tt.want, filters[0].Rule)
		}
	}
}

func TestFilterThisStreamGroupMarksStream(t *testing.T) {
	var filters = rules(t, groupFilter("News", "", ""))

	var stream = testStream("News", "BBC")
	if !Accept(stream, filters) {
		t.Fatal("stream was not accepted")
	}
	if stream["_preserve-mapping"] != "true" || stream["_starting-channel"] != "1000" {
		t.Errorf("stream = %v, want preserve mapping and starting channel", stream)
	}
}

func TestFilterThisStreamCustom(t *testing.T) {
	tests := []struct {
		name          string
		rule          string
		caseSensitive bool
		stream        map[string]string
		want          bool
	}{
		{name: "matches name", rule: "bbc", stream: testStream("News", "BBC One"), want: true},
		{name: "matches url", rule: "example.com/BBC", stream: testStream("News", "BBC"), want: true},
		{name: "no match", rule: "CNN", stream: testStream("News", "BBC"), want: false},
		{name: "case sensitive", rule: "bbc", caseSensitive: true, stream: testStream("News", "BBC"), want: false},
		{name: "with exclude", rule: "News !{One}", stream: testStream("News", "BBC One"), want: false},
		{name: "with include", rule: "News {One}", stream: testStream("News", "BBC One"), want: true},
	}
	for _, tt := range tests {
		var filters = rules(t, FilterSettings{Type: "custom-filter", Filter: tt.rule, CaseSensitive: tt.caseSensitive})

		if got := Accept(tt.stream, filters); got != tt.want {
			t.Errorf("%s: filterThisStream = %v, want %v", tt.name, got, tt.want)
		}
	}
}

func TestFilterThisStreamSeveralFilters(t *testing.T) {
	var filters = rules(t, groupFilter("News", "", ""), groupFilter("Sport", "HD", ""))

	tests := []struct {
		stream map[string]string
		want   bool
	}{
		{stream: testStream("News", "BBC"), want: true},
		{stream: testStream("Sport", "ESPN HD"), want: true},
		{stream: testStream("Sport", "ESPN SD"), want: false},
		{stream: testStream("Movies", "HBO"), want: false},
	}
	for _, tt := range tests {
		if got := Accept(tt.stream, filters); got != tt.want {
			t.Errorf("%v: filterThisStream = %v, want %v", tt.stream, got, tt.want)
		}
	}
}

func TestFilterThisStreamOrderIndependent(t *testing.T) {
	var news = groupFilter("News", "", "SD")
	var custom = FilterSettings{Type: "custom-filter", Filter: "BBC"}

	for _, order := range [][]FilterSettings{{news, custom}, {custom, news}} {
		// Fix the order of filters instead of relying on map iteration
		var filters = rules(t, order[0])
		var first = filters
		filters = rules(t, order[1])
		filters = append(first, filters...)

		var stream = testStream("News", "BBC SD")
		if !Accept(stream, filters) {
			t.Errorf("order %v: stream excluded by one filter was not accepted by the other", filters)
		}
		if _, ok := stream["_preserve-mapping"]; ok {
			t.Errorf("order %v: stream got the marks of the group filter that excluded it", filters)
		}
	}
}
