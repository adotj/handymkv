package hmkv

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

func withTestStatsDir(t *testing.T) string {
	t.Helper()
	dir := filepath.Join(t.TempDir(), "stats")
	statsDirOverride = dir
	t.Cleanup(func() { statsDirOverride = "" })
	return dir
}

func TestAppendRipHistoryCreatesFileWithHeaders(t *testing.T) {
	dir := withTestStatsDir(t)

	entries := []EncodingParams{
		{
			TitleIndex:           0,
			DiscId:               0,
			RippedFileSizeBytes:  5_720_000_000,
			EncodedFileSizeBytes: 1_660_000_000,
			RippingDuration:      "30m0s",
			ProcessingDuration:   55*time.Minute + 49*time.Second,
			CompletedAt:          time.Date(2026, 8, 18, 9, 15, 0, 0, time.UTC),
		},
	}
	titles := []TitleInfo{
		{Index: 0, DiscId: 0, DiscTitle: "MY_MOVIE", FileName: "MY_MOVIE_t00.mkv"},
	}
	libraryPaths := []string{filepath.Join(`D:\movies`, "My Movie (1999)", "My Movie (1999).mkv")}

	if err := appendRipHistory(entries, titles, libraryPaths); err != nil {
		t.Fatalf("appendRipHistory error: %v", err)
	}

	path := filepath.Join(dir, ripHistoryFileName)
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read rip history: %v", err)
	}

	content := string(data)
	if !strings.HasPrefix(content, "timestamp,title,disc_name,elapsed,raw_gb,encoded_gb,saved_gb\n") {
		t.Fatalf("unexpected header/content:\n%s", content)
	}
	if !strings.Contains(content, "My Movie (1999)") {
		t.Fatalf("expected library title in row, got:\n%s", content)
	}
	if !strings.Contains(content, "MY_MOVIE") {
		t.Fatalf("expected disc name in row, got:\n%s", content)
	}
	if !strings.Contains(content, "55m49s") {
		t.Fatalf("expected elapsed in row, got:\n%s", content)
	}
}

func TestAppendRipHistoryAppendsRows(t *testing.T) {
	dir := withTestStatsDir(t)

	entry := EncodingParams{
		TitleIndex:           0,
		DiscId:               0,
		RippedFileSizeBytes:  1_000_000_000,
		EncodedFileSizeBytes: 400_000_000,
		ProcessingDuration:   10 * time.Minute,
		CompletedAt:          time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC),
	}
	title := TitleInfo{Index: 0, DiscId: 0, DiscTitle: "Disc A", FileName: "Disc_A_t00.mkv"}
	paths := []string{`D:\movies\Disc A (2001)\Disc A (2001).mkv`}

	if err := appendRipHistory([]EncodingParams{entry}, []TitleInfo{title}, paths); err != nil {
		t.Fatalf("first append: %v", err)
	}
	if err := appendRipHistory([]EncodingParams{entry}, []TitleInfo{title}, paths); err != nil {
		t.Fatalf("second append: %v", err)
	}

	filePath := filepath.Join(dir, ripHistoryFileName)
	file, err := os.Open(filePath)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	defer file.Close()

	rows, err := readRipHistoryFromReader(file)
	if err != nil {
		t.Fatalf("readRipHistoryFromReader: %v", err)
	}
	if len(rows) != 2 {
		t.Fatalf("expected 2 rows, got %d", len(rows))
	}
}

func TestDefaultStatsDirWindows(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("windows-only path")
	}
	if statsDirOverride != "" {
		t.Fatal("statsDirOverride should be empty")
	}
	got := defaultStatsDir()
	want := `D:\handymkv\stats`
	if got != want {
		t.Fatalf("defaultStatsDir() = %q, want %q", got, want)
	}
}

func TestBuildRipStatsRowsUsesLibraryName(t *testing.T) {
	rows := buildRipStatsRows(
		[]EncodingParams{{TitleIndex: 0, DiscId: 0, RippedFileSizeBytes: 100, EncodedFileSizeBytes: 50, ProcessingDuration: time.Minute}},
		[]TitleInfo{{Index: 0, DiscId: 0, DiscTitle: "RAW_DISC", FileName: "RAW_DISC_t00.mkv"}},
		[]string{`D:\movies\Clean Title (2020)\Clean Title (2020).mkv`},
	)
	if len(rows) != 1 {
		t.Fatalf("expected 1 row, got %d", len(rows))
	}
	if rows[0].Title != "Clean Title (2020)" {
		t.Fatalf("Title = %q, want Clean Title (2020)", rows[0].Title)
	}
	if rows[0].DiscName != "RAW_DISC" {
		t.Fatalf("DiscName = %q, want RAW_DISC", rows[0].DiscName)
	}
}

func TestBytesToGB(t *testing.T) {
	const gb int64 = 1024 * 1024 * 1024
	got := bytesToGB(gb * 532 / 100)
	if got < 5.319 || got > 5.321 {
		t.Fatalf("bytesToGB = %v, want ~5.32", got)
	}
}

func TestParseElapsedCSV(t *testing.T) {
	d, err := parseElapsedCSV("55m49s")
	if err != nil {
		t.Fatalf("parseElapsedCSV error: %v", err)
	}
	if d != 55*time.Minute+49*time.Second {
		t.Fatalf("got %v", d)
	}
}
