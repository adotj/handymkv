package hmkv

import "testing"

func TestGetEncodingFileName(t *testing.T) {
	tests := []struct {
		name     string
		fileName string
		want     string
	}{
		{
			name:     "keeps mkv extension",
			fileName: "movie.mkv",
			want:     "movie.mkv",
		},
		{
			name:     "spaces replaced with underscores",
			fileName: "Star Trek TNG.mkv",
			want:     "Star_Trek_TNG.mkv",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			title := &TitleInfo{FileName: tt.fileName}
			got := title.GetEncodingFileName()
			if got != tt.want {
				t.Errorf("GetEncodingFileName() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestSubdirectory(t *testing.T) {
	tests := []struct {
		name             string
		discTitle        string
		discId           int
		prependDiscToSub bool
		want             string
	}{
		{
			name:             "no prepend replaces spaces",
			discTitle:        "Star Trek TNG",
			discId:           0,
			prependDiscToSub: false,
			want:             "Star_Trek_TNG",
		},
		{
			name:             "prepend disc 0",
			discTitle:        "Star Trek TNG",
			discId:           0,
			prependDiscToSub: true,
			want:             "HMKV_DISC_0__Star_Trek_TNG",
		},
		{
			name:             "prepend disc 2",
			discTitle:        "Movie Title",
			discId:           2,
			prependDiscToSub: true,
			want:             "HMKV_DISC_2__Movie_Title",
		},
		{
			name:             "no spaces in title",
			discTitle:        "Aliens",
			discId:           1,
			prependDiscToSub: false,
			want:             "Aliens",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			title := &TitleInfo{
				DiscTitle:        tt.discTitle,
				DiscId:           tt.discId,
				PrependDiscToSub: tt.prependDiscToSub,
			}
			got := title.Subdirectory()
			if got != tt.want {
				t.Errorf("Subdirectory() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestParseMakeMKVProgressLine(t *testing.T) {
	tests := []struct {
		name        string
		line        string
		wantKind    string
		wantPercent int
		wantStage   string
	}{
		{
			name:        "prgv overall progress",
			line:        "PRGV:4122,12850,65536",
			wantKind:    "progress",
			wantPercent: 19, // 12850/65536*100
		},
		{
			name:        "prgv caps at 99",
			line:        "PRGV:65536,65536,65536",
			wantKind:    "progress",
			wantPercent: 99,
		},
		{
			name:        "prgv uses current when total is 0",
			line:        "PRGV:16384,0,65536",
			wantKind:    "progress",
			wantPercent: 25,
		},
		{
			name:      "prgc stage name",
			line:      `PRGC:2012,0,"Saving to MKV file"`,
			wantKind:  "stage",
			wantStage: "Saving to MKV file",
		},
		{
			name:      "prgc decrypting",
			line:      `PRGC:5014,0,"Decrypting DVD"`,
			wantKind:  "stage",
			wantStage: "Decrypting DVD",
		},
		{
			name:     "unrelated line",
			line:     `MSG:1005,0,1,"Operation successfully completed"`,
			wantKind: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			percent, stage, kind := parseMakeMKVProgressLine(tt.line)
			if kind != tt.wantKind {
				t.Fatalf("kind = %q, want %q", kind, tt.wantKind)
			}
			if kind == "progress" && percent != tt.wantPercent {
				t.Errorf("percent = %d, want %d", percent, tt.wantPercent)
			}
			if kind == "stage" && stage != tt.wantStage {
				t.Errorf("stage = %q, want %q", stage, tt.wantStage)
			}
		})
	}
}

func TestFindLongestTitle(t *testing.T) {
	titles := []TitleInfo{
		{Index: 0, FileName: "short.mkv", Length: "0:05:00", FileSizeBytes: 100},
		{Index: 1, FileName: "feature.mkv", Length: "1:45:12", FileSizeBytes: 800},
		{Index: 2, FileName: "trailer.mkv", Length: "0:02:10", FileSizeBytes: 50},
	}

	got := findLongestTitle(titles)
	if got != 1 {
		t.Fatalf("findLongestTitle() = %d, want 1", got)
	}

	tied := []TitleInfo{
		{Index: 0, Length: "1:00:00", FileSizeBytes: 100},
		{Index: 1, Length: "1:00:00", FileSizeBytes: 250},
	}
	if findLongestTitle(tied) != 1 {
		t.Fatalf("findLongestTitle() tie-break = %d, want 1", findLongestTitle(tied))
	}

	if findLongestTitle(nil) != -1 {
		t.Fatalf("findLongestTitle(nil) = %d, want -1", findLongestTitle(nil))
	}
}

func TestTitleDurationSeconds(t *testing.T) {
	if got := titleDurationSeconds("1:45:12"); got != 6312 {
		t.Errorf("titleDurationSeconds(1:45:12) = %d, want 6312", got)
	}
	if got := titleDurationSeconds("bad"); got != 0 {
		t.Errorf("titleDurationSeconds(bad) = %d, want 0", got)
	}
}
