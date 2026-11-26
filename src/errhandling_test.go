package src

import (
	"errors"
	"fmt"
	"testing"
)

func TestUserMessage(t *testing.T) {
	tests := map[string]string{
		"no authorization":   "No authorization",
		"Already capital":    "Already capital",
		"ünicode first rune": "Ünicode first rune",
		"":                   "",
	}
	for in, want := range tests {
		if got := userMessage(errors.New(in)); got != want {
			t.Errorf("userMessage(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestErrorCode(t *testing.T) {
	var base = errors.New("file not found")

	if got := errorCode(base); got != 0 {
		t.Errorf("errorCode(plain) = %d, want 0", got)
	}
	if withCode(nil, 1004) != nil {
		t.Error("withCode(nil) must be nil")
	}

	var wrapped = fmt.Errorf("provider: %w", withCode(base, 1004))
	if got := errorCode(wrapped); got != 1004 {
		t.Errorf("errorCode(wrapped) = %d, want 1004", got)
	}
	if !errors.Is(wrapped, base) {
		t.Error("the original error must stay reachable with errors.Is")
	}
	if wrapped.Error() != "provider: file not found" {
		t.Errorf("message = %q", wrapped.Error())
	}
}
