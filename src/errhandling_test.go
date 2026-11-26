package src

import (
	"errors"
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
