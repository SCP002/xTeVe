package src

import (
	"slices"
	"sort"
	"testing"
)

func TestGuideNumberLess(t *testing.T) {
	var numbers = []string{"10", "abc", "2", "1000.5", "", "1.5", "Zed"}
	sort.SliceStable(numbers, func(i, j int) bool { return guideNumberLess(numbers[i], numbers[j]) })

	var want = []string{"1.5", "2", "10", "1000.5", "", "Zed", "abc"}
	if !slices.Equal(numbers, want) {
		t.Errorf("sorted = %q, want %q", numbers, want)
	}
}
