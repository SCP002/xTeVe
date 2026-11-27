package src

import (
	"reflect"
	"testing"
)

// setFilters replaces Settings.Filter and builds Data.Filter from it.
func setFilters(t *testing.T, filters ...map[string]any) {
	t.Helper()

	var oldData = Data.Filter
	t.Cleanup(func() { Data.Filter = oldData })

	setSettings(t, func(s *SettingsStruct) {
		s.Filter = make(map[int64]any)
		for i, f := range filters {
			s.Filter[int64(i)] = f
		}
	})
	if err := createFilterRules(); err != nil {
		t.Fatal(err)
	}
}

func groupFilter(group, include, exclude string) map[string]any {
	return map[string]any{
		"active":          true,
		"type":            "group-title",
		"filter":          group,
		"include":         include,
		"exclude":         exclude,
		"caseSensitive":   false,
		"preserveMapping": true,
		"startingChannel": "1000",
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
	setFilters(t,
		groupFilter("News", "HD", "SD"),
		map[string]any{"type": "custom-filter", "filter": "Sport", "caseSensitive": true},
	)

	var want = []Filter{
		{Type: "group-title", Rule: "News {HD} !{SD}", PreserveMapping: true, StartingChannel: "1000"},
		{Type: "custom-filter", Rule: "Sport", CaseSensitive: true},
	}
	// Settings.Filter is a map, so the order of the rules is not fixed
	if len(Data.Filter) != len(want) {
		t.Fatalf("Data.Filter = %+v", Data.Filter)
	}
	for _, w := range want {
		var found bool
		for _, f := range Data.Filter {
			found = found || reflect.DeepEqual(f, w)
		}
		if !found {
			t.Errorf("Data.Filter = %+v, missing %+v", Data.Filter, w)
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
		setFilters(t, groupFilter("News", tt.include, tt.exclude))

		if got := filterThisStream(tt.stream); got != tt.want {
			t.Errorf("%s: filterThisStream = %v, want %v (rule %q)", tt.name, got, tt.want, Data.Filter[0].Rule)
		}
	}
}

func TestFilterThisStreamGroupMarksStream(t *testing.T) {
	setFilters(t, groupFilter("News", "", ""))

	var stream = testStream("News", "BBC")
	if !filterThisStream(stream) {
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
		setFilters(t, map[string]any{"type": "custom-filter", "filter": tt.rule, "caseSensitive": tt.caseSensitive})

		if got := filterThisStream(tt.stream); got != tt.want {
			t.Errorf("%s: filterThisStream = %v, want %v", tt.name, got, tt.want)
		}
	}
}

func TestFilterThisStreamSeveralFilters(t *testing.T) {
	setFilters(t, groupFilter("News", "", ""), groupFilter("Sport", "HD", ""))

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
		if got := filterThisStream(tt.stream); got != tt.want {
			t.Errorf("%v: filterThisStream = %v, want %v", tt.stream, got, tt.want)
		}
	}
}

func TestFilterThisStreamOrderIndependent(t *testing.T) {
	var news = groupFilter("News", "", "SD")
	var custom = map[string]any{"type": "custom-filter", "filter": "BBC"}

	for _, order := range [][]map[string]any{{news, custom}, {custom, news}} {
		// Fix the order of Data.Filter instead of relying on map iteration
		setFilters(t, order[0])
		var first = Data.Filter
		setFilters(t, order[1])
		Data.Filter = append(first, Data.Filter...)

		var stream = testStream("News", "BBC SD")
		if !filterThisStream(stream) {
			t.Errorf("order %v: stream excluded by one filter was not accepted by the other", Data.Filter)
		}
		if _, ok := stream["_preserve-mapping"]; ok {
			t.Errorf("order %v: stream got the marks of the group filter that excluded it", Data.Filter)
		}
	}
}
