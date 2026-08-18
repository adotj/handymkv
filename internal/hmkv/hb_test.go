package hmkv

import "testing"

func TestParseHandBrakeProgressLine(t *testing.T) {
	tests := []struct {
		name        string
		line        string
		wantOK      bool
		wantPercent int
		wantETA     string
		wantStage   string
	}{
		{
			name:        "encoding with eta",
			line:        "Encoding: task 1 of 1, 42.15 % (123.45 fps, avg 120.00 fps, ETA 01h12m34s)",
			wantOK:      true,
			wantPercent: 42,
			wantETA:     "1h12m",
		},
		{
			name:        "encoding short eta",
			line:        "Encoding: task 1 of 1, 20.06 % (421.16 fps, avg 413.85 fps, ETA 00h02m29s)",
			wantOK:      true,
			wantPercent: 20,
			wantETA:     "2m29s",
		},
		{
			name:        "scanning title",
			line:        "Scanning title 1 of 1, preview 1...",
			wantOK:      true,
			wantPercent: -1,
			wantStage:   "Scanning",
		},
		{
			name:   "unrelated log line",
			line:   "[21:11:02] hb_init: starting libhb thread",
			wantOK: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			percent, eta, stage, ok := parseHandBrakeProgressLine(tt.line)
			if ok != tt.wantOK {
				t.Fatalf("ok = %v, want %v", ok, tt.wantOK)
			}
			if !ok {
				return
			}
			if percent != tt.wantPercent {
				t.Errorf("percent = %d, want %d", percent, tt.wantPercent)
			}
			if eta != tt.wantETA {
				t.Errorf("eta = %q, want %q", eta, tt.wantETA)
			}
			if stage != tt.wantStage {
				t.Errorf("stage = %q, want %q", stage, tt.wantStage)
			}
		})
	}
}
