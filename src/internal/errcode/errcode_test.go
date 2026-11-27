package errcode

import (
	"errors"
	"fmt"
	"strings"
	"testing"
)

func TestWithAndOf(t *testing.T) {
	var base = errors.New("file not found")

	if With(nil, 1004) != nil {
		t.Error("With(nil) must be nil")
	}
	if Of(base) != 0 {
		t.Errorf("Of(plain) = %d, want 0", Of(base))
	}

	var coded = With(base, 1004)
	if Of(coded) != 1004 || !errors.Is(coded, base) || coded.Error() != base.Error() {
		t.Errorf("coded error = %v, code %d", coded, Of(coded))
	}
	if Of(fmt.Errorf("wrapped: %w", coded)) != 1004 {
		t.Error("the code is lost when the error is wrapped")
	}
}

func TestMessage(t *testing.T) {
	SetApp(App{Name: "TestApp", Compatibility: "9.9.9", PlexChannelLimit: 7})
	t.Cleanup(func() { SetApp(App{Name: "xTeVe"}) })

	tests := map[int]string{
		0:    "",
		1004: "File not found",
		1013: "at least version 9.9.9",
		2000: "more than 7 streams",
		2105: "delete TestApp",
		9999: "Unknown error / warning (9999)",
	}
	for code, want := range tests {
		if got := Message(code); !strings.Contains(got, want) || (want == "" && got != "") {
			t.Errorf("Message(%d) = %q, want it to contain %q", code, got, want)
		}
	}
}
