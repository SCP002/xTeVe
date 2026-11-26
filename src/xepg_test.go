package src

import "testing"

func TestShiftXMLTVTime(t *testing.T) {
	tests := []struct {
		in      string
		hours   int
		want    string
		wantErr bool
	}{
		{in: "20260101120000 +0100", hours: 2, want: "20260101120000 +0300"},
		{in: "20260101120000 +0100", hours: -3, want: "20260101120000 -0200"},
		{in: "20260101120000 -0500", hours: 0, want: "20260101120000 -0500"},
		{in: "20260101120000", hours: 1, wantErr: true},
		{in: "20260101120000 abc", hours: 1, wantErr: true},
	}
	for _, tt := range tests {
		got, err := shiftXMLTVTime(tt.in, tt.hours)
		if (err != nil) != tt.wantErr {
			t.Errorf("shiftXMLTVTime(%q, %d) error = %v, wantErr %v", tt.in, tt.hours, err, tt.wantErr)
			continue
		}
		if got != tt.want {
			t.Errorf("shiftXMLTVTime(%q, %d) = %q, want %q", tt.in, tt.hours, got, tt.want)
		}
	}
}
