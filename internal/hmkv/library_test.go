package hmkv

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

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

func TestLibraryMoviePathParsingWindowsStyle(t *testing.T) {
	path := `D:\movies\Clean Title (2020)\Clean Title (2020).mkv`
	if got := libraryMovieFileTitle(path); got != "Clean Title (2020)" {
		t.Fatalf("libraryMovieFileTitle() = %q", got)
	}
	if got := libraryMovieFolderName(path); got != "Clean Title (2020)" {
		t.Fatalf("libraryMovieFolderName() = %q", got)
	}
}

func TestMovieLibraryNameFolderName(t *testing.T) {
	got := movieLibraryName{Name: "The Shining", Year: "1980"}.FolderName()
	if got != "The Shining (1980)" {
		t.Fatalf("FolderName() = %q, want %q", got, "The Shining (1980)")
	}
}

func TestTVLibraryTargetPaths(t *testing.T) {
	target := tvLibraryTarget{
		Series:  "Star Trek",
		Year:    "1966",
		Season:  1,
		Episode: 3,
	}
	if got := target.SeriesFolderName(); got != "Star Trek (1966)" {
		t.Fatalf("SeriesFolderName() = %q", got)
	}
	if got := target.SeasonFolderName(); got != "Season 01" {
		t.Fatalf("SeasonFolderName() = %q", got)
	}
	if got := target.EpisodeCode(); got != "S01E03" {
		t.Fatalf("EpisodeCode() = %q", got)
	}
	if got := target.EpisodeFileName(".mkv"); got != "Star Trek (1966) - S01E03.mkv" {
		t.Fatalf("EpisodeFileName() = %q", got)
	}

	specials := tvLibraryTarget{Series: "Show", Season: 0, Episode: 1}
	if got := specials.SeasonFolderName(); got != "Season 00" {
		t.Fatalf("SeasonFolderName() specials = %q", got)
	}
	if got := specials.EpisodeFileName(".mkv"); got != "Show - S00E01.mkv" {
		t.Fatalf("EpisodeFileName() no year = %q", got)
	}
}

func TestTVSeriesMetaLabel(t *testing.T) {
	meta := tvSeriesMeta{Series: "Star Trek", Year: "1966", Season: 1, StartEpisode: 1}
	if got := meta.Label(1); got != "Star Trek (1966) S01E01" {
		t.Fatalf("Label(1) = %q", got)
	}
	if got := meta.Label(4); got != "Star Trek (1966) S01E01–E04" {
		t.Fatalf("Label(4) = %q", got)
	}
}

func TestValidateSeasonAndEpisode(t *testing.T) {
	if err := ValidateSeason(0); err != nil {
		t.Fatalf("ValidateSeason(0) = %v", err)
	}
	if err := ValidateSeason(-1); err == nil {
		t.Fatal("ValidateSeason(-1) expected error")
	}
	if err := ValidateStartEpisode(1); err != nil {
		t.Fatalf("ValidateStartEpisode(1) = %v", err)
	}
	if err := ValidateStartEpisode(0); err == nil {
		t.Fatal("ValidateStartEpisode(0) expected error")
	}
}

func TestOrganizeTVEpisodesToLibrary(t *testing.T) {
	tmp := t.TempDir()
	tvRoot := filepath.Join(tmp, "tv")
	srcDir := filepath.Join(tmp, "hb")
	if err := os.MkdirAll(srcDir, 0755); err != nil {
		t.Fatal(err)
	}

	src1 := filepath.Join(srcDir, "ep1.mkv")
	src2 := filepath.Join(srcDir, "ep2.mkv")
	for _, p := range []string{src1, src2} {
		if err := os.WriteFile(p, []byte("data"), 0644); err != nil {
			t.Fatal(err)
		}
	}

	titles := []TitleInfo{
		{DiscId: 0, Index: 2, FileName: "t2.mkv"},
		{DiscId: 0, Index: 0, FileName: "t0.mkv"},
	}
	entries := []EncodingParams{
		{DiscId: 0, TitleIndex: 0, HandBrakeOutputPath: src2},
		{DiscId: 0, TitleIndex: 2, HandBrakeOutputPath: src1},
	}
	config := &handyMKVConfig{TVLibraryRoot: tvRoot}
	meta := tvSeriesMeta{Series: "Star Trek", Year: "1966", Season: 1, StartEpisode: 5}

	paths, err := organizeTVEpisodesToLibrary(entries, titles, config, meta)
	if err != nil {
		t.Fatalf("organizeTVEpisodesToLibrary: %v", err)
	}
	if len(paths) != 2 {
		t.Fatalf("got %d paths, want 2", len(paths))
	}

	want1 := filepath.Join(tvRoot, "Star Trek (1966)", "Season 01", "Star Trek (1966) - S01E05.mkv")
	want2 := filepath.Join(tvRoot, "Star Trek (1966)", "Season 01", "Star Trek (1966) - S01E06.mkv")
	if paths[0] != want1 || paths[1] != want2 {
		t.Fatalf("paths = %#v, want [%q %q]", paths, want1, want2)
	}
	for _, p := range paths {
		if _, err := os.Stat(p); err != nil {
			t.Fatalf("missing organized file %s: %v", p, err)
		}
	}
}

func TestValidateMovieYear(t *testing.T) {
	tests := []struct {
		year    string
		wantErr bool
	}{
		{"1980", false},
		{"2049", false},
		{"99", true},
		{"abcd", true},
		{"", true},
	}

	for _, tt := range tests {
		err := ValidateMovieYear(tt.year)
		if (err != nil) != tt.wantErr {
			t.Errorf("ValidateMovieYear(%q) error = %v, wantErr = %v", tt.year, err, tt.wantErr)
		}
	}
}

func TestResolveMovieLibraryName(t *testing.T) {
	shiningTitle := TitleInfo{
		FileName:  "THE_SHINING-B6_t00.mkv",
		DiscTitle: "THE SHINING",
	}
	emptyTitle := TitleInfo{
		FileName:  "",
		DiscTitle: "",
	}

	tests := []struct {
		name     string
		title    TitleInfo
		flagName string
		flagYear string
		want     movieLibraryName
		wantErr  string
	}{
		{
			name:     "year only from disc metadata",
			title:    shiningTitle,
			flagYear: "1980",
			want:     movieLibraryName{Name: "The Shining", Year: "1980"},
		},
		{
			name:     "name and year flags",
			title:    shiningTitle,
			flagName: "The Shining",
			flagYear: "1980",
			want:     movieLibraryName{Name: "The Shining", Year: "1980"},
		},
		{
			name:     "full jellyfin name",
			title:    shiningTitle,
			flagName: "The Shining (1980)",
			want:     movieLibraryName{Name: "The Shining", Year: "1980"},
		},
		{
			name:     "conflicting year flags",
			title:    shiningTitle,
			flagName: "The Shining (1980)",
			flagYear: "1999",
			wantErr:  "conflicting year",
		},
		{
			name:     "year only without disc name",
			title:    emptyTitle,
			flagYear: "1980",
			wantErr:  "could not determine movie name from disc metadata",
		},
		{
			name:     "invalid year",
			title:    shiningTitle,
			flagYear: "99",
			wantErr:  "invalid release year",
		},
		{
			name:     "name only without year",
			title:    shiningTitle,
			flagName: "The Shining",
			wantErr:  "expected format: Movie Name (Year) or use -y",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := resolveMovieLibraryName(tt.title, tt.flagName, tt.flagYear, `D:\movies`)
			if tt.wantErr != "" {
				if err == nil {
					t.Fatalf("resolveMovieLibraryName() error = nil, want error containing %q", tt.wantErr)
				}
				if !strings.Contains(err.Error(), tt.wantErr) {
					t.Fatalf("resolveMovieLibraryName() error = %q, want containing %q", err.Error(), tt.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("resolveMovieLibraryName() unexpected error: %v", err)
			}
			if got != tt.want {
				t.Fatalf("resolveMovieLibraryName() = %+v, want %+v", got, tt.want)
			}
		})
	}
}

func TestCopyFileRemovesSource(t *testing.T) {
	tmp := t.TempDir()
	src := filepath.Join(tmp, "src.mkv")
	dest := filepath.Join(tmp, "dest.mkv")
	if err := os.WriteFile(src, []byte("encoded-bytes"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := copyFile(src, dest); err != nil {
		t.Fatalf("copyFile: %v", err)
	}
	if _, err := os.Stat(src); !os.IsNotExist(err) {
		t.Fatalf("source still exists after copyFile; stat err=%v", err)
	}
	got, err := os.ReadFile(dest)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "encoded-bytes" {
		t.Fatalf("dest contents = %q", got)
	}
}

func TestMoveFileCrossVolume(t *testing.T) {
	cwd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	srcVol := filepath.VolumeName(cwd)
	dstDir := t.TempDir()
	dstVol := filepath.VolumeName(dstDir)
	if srcVol == "" || srcVol == dstVol {
		t.Skipf("need src and dest on different volumes; src=%s dest=%s", srcVol, dstVol)
	}

	srcDir, err := os.MkdirTemp(cwd, "hmkv-move-*")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.RemoveAll(srcDir) })

	src := filepath.Join(srcDir, "src.mkv")
	dest := filepath.Join(dstDir, "dest.mkv")
	if err := os.WriteFile(src, []byte("cross-volume"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := moveFile(src, dest); err != nil {
		t.Fatalf("moveFile cross-volume: %v", err)
	}
	if _, err := os.Stat(src); !os.IsNotExist(err) {
		t.Fatalf("source still exists after moveFile; stat err=%v", err)
	}
	got, err := os.ReadFile(dest)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "cross-volume" {
		t.Fatalf("dest contents = %q", got)
	}
}
