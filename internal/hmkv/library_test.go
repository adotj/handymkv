package hmkv

import "testing"

func TestCleanMakeMKVTitleName(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"THE_SHINING-B6_t00.mkv", "The Shining"},
		{"STAR_WARS_EPISODE_IV-A1_t01.mkv", "Star Wars Episode IV"},
		{"the_matrix_t00.mkv", "The Matrix"},
		{"", ""},
	}

	for _, tt := range tests {
		got := cleanMakeMKVTitleName(tt.input)
		if got != tt.want {
			t.Errorf("cleanMakeMKVTitleName(%q) = %q, want %q", tt.input, got, tt.want)
		}
	}
}

func TestExtractYear(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"THE SHINING (1980)", "1980"},
		{"Blade Runner 2049", "2049"},
		{"STAR_TREK_TNG_S4_D2", ""},
		{"Movie_1999_Edition", "1999"},
	}

	for _, tt := range tests {
		got := extractYear(tt.input)
		if got != tt.want {
			t.Errorf("extractYear(%q) = %q, want %q", tt.input, got, tt.want)
		}
	}
}

func TestParseMovieNameYear(t *testing.T) {
	tests := []struct {
		input    string
		wantName string
		wantYear string
		ok       bool
	}{
		{"The Shining (1980)", "The Shining", "1980", true},
		{"Blade Runner (2049)", "Blade Runner", "2049", true},
		{"No Year Here", "", "", false},
		{"", "", "", false},
	}

	for _, tt := range tests {
		name, year, ok := parseMovieNameYear(tt.input)
		if ok != tt.ok || name != tt.wantName || year != tt.wantYear {
			t.Errorf("parseMovieNameYear(%q) = (%q, %q, %v), want (%q, %q, %v)",
				tt.input, name, year, ok, tt.wantName, tt.wantYear, tt.ok)
		}
	}
}

func TestMovieLibraryNameFolderName(t *testing.T) {
	got := movieLibraryName{Name: "The Shining", Year: "1980"}.FolderName()
	if got != "The Shining (1980)" {
		t.Fatalf("FolderName() = %q, want %q", got, "The Shining (1980)")
	}
}
