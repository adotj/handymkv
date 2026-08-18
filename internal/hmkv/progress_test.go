package hmkv

import (
	"strings"
	"testing"
)

func TestStatusValueString(t *testing.T) {
	tests := []struct {
		status statusValue
		want   string
	}{
		{Pending, "Pending"},
		{InProgress, "In Progress"},
		{Complete, "Complete"},
		{statusValue(99), "Unknown"},
	}

	for _, tt := range tests {
		got := tt.status.String()
		if got != tt.want {
			t.Errorf("statusValue(%d).String() = %q, want %q", tt.status, got, tt.want)
		}
	}
}

func TestGetColor(t *testing.T) {
	tests := []struct {
		status statusValue
		want   string
	}{
		{Pending, colorYellow},
		{InProgress, colorBlue},
		{Complete, colorGreen},
		{statusValue(99), colorReset},
	}

	for _, tt := range tests {
		got := getColor(tt.status)
		if got != tt.want {
			t.Errorf("getColor(%d) = %q, want %q", tt.status, got, tt.want)
		}
	}
}

func TestGetAnimatedEllipsis(t *testing.T) {
	tests := []struct {
		frame int
		want  string
	}{
		{0, ".  "},
		{1, ".. "},
		{2, "..."},
		{3, ".  "}, // default
		{-1, ".  "}, // default
	}

	for _, tt := range tests {
		got := getAnimatedEllipsis(tt.frame)
		if got != tt.want {
			t.Errorf("getAnimatedEllipsis(%d) = %q, want %q", tt.frame, got, tt.want)
		}
	}
}

func TestFormatProgressStatus(t *testing.T) {
	tests := []struct {
		status   statusValue
		progress int
		frame    int
		stage    string
		eta      string
		contains []string // substrings that must appear in output
	}{
		{Pending, 0, 0, "", "", []string{"Pending"}},
		{Complete, 0, 0, "", "", []string{"Complete"}},
		{InProgress, 50, 0, "", "", []string{"50%", ".  "}},
		{InProgress, 100, 2, "", "", []string{"100%", "..."}},
		{InProgress, -1, 1, "", "", []string{"Working", ".. "}},
		{InProgress, 42, 0, "", "1h12m", []string{"42%", "1h12m"}},
		{InProgress, 18, 0, "Decrypting DVD", "", []string{"Decrypting", "18%"}},
		{InProgress, -1, 0, "Scanning", "", []string{"Scanning", ".  "}},
	}

	for _, tt := range tests {
		got := formatProgressStatus(tt.status, tt.progress, tt.frame, tt.stage, tt.eta)
		for _, sub := range tt.contains {
			if !strings.Contains(got, sub) {
				t.Errorf("formatProgressStatus(%v, %d, %d, %q, %q) = %q, want it to contain %q",
					tt.status, tt.progress, tt.frame, tt.stage, tt.eta, got, sub)
			}
		}
	}
}

func TestCompactETA(t *testing.T) {
	tests := []struct {
		raw  string
		want string
	}{
		{"01h12m34s", "1h12m"},
		{"00h02m29s", "2m29s"},
		{"00h00m05s", "5s"},
		{"1h12m34s", "1h12m"},
		{"not-an-eta", "not-an-eta"},
	}

	for _, tt := range tests {
		got := compactETA(tt.raw)
		if got != tt.want {
			t.Errorf("compactETA(%q) = %q, want %q", tt.raw, got, tt.want)
		}
	}
}

func TestShortStage(t *testing.T) {
	tests := []struct {
		raw  string
		want string
	}{
		{"Saving to MKV file", "Saving"},
		{"Decrypting DVD", "Decrypting"},
		{"Scanning", "Scanning"},
		{"", ""},
	}

	for _, tt := range tests {
		got := shortStage(tt.raw)
		if got != tt.want {
			t.Errorf("shortStage(%q) = %q, want %q", tt.raw, got, tt.want)
		}
	}
}
