package hmkv

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"
)

func installMediaProbeFixtures(t *testing.T, fixtures map[string]mediaFileProbe) {
	t.Helper()
	prev := mediaFileProbeHook
	mediaFileProbeHook = func(path string) (mediaFileProbe, error) {
		if probe, ok := fixtures[path]; ok {
			return probe, nil
		}
		return mediaFileProbe{}, os.ErrNotExist
	}
	t.Cleanup(func() { mediaFileProbeHook = prev })
}

func verifiedMovieProbeFixtures(rawPath, encPath string) map[string]mediaFileProbe {
	probe := mediaFileProbe{DurationSec: 7200, AudioStreams: 2}
	return map[string]mediaFileProbe{
		rawPath: probe,
		encPath: probe,
	}
}

func TestLibrarySourceUnverifiedPrefersRaw(t *testing.T) {
	rawPath := filepath.Join(t.TempDir(), "raw.mkv")
	encPath := filepath.Join(t.TempDir(), "enc.mkv")
	entry := EncodingParams{
		MKVOutputPath:        rawPath,
		HandBrakeOutputPath:  encPath,
		RippedFileSizeBytes:  200,
		EncodedFileSizeBytes: 50,
	}
	src, keptRaw := librarySource(entry, false)
	if !keptRaw || src != rawPath {
		t.Fatalf("librarySource() = (%q, %v), want raw preferred when unverified", src, keptRaw)
	}
}

func TestTruncatedEncodeDoesNotDeleteGoodFiles(t *testing.T) {
	tmp := t.TempDir()
	libraryRoot := filepath.Join(tmp, "movies")
	rawDir := filepath.Join(tmp, "mkv", "DISC")
	hbDir := filepath.Join(tmp, "hb", "DISC")
	if err := os.MkdirAll(rawDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(hbDir, 0755); err != nil {
		t.Fatal(err)
	}

	raw := filepath.Join(rawDir, "title.mkv")
	enc := filepath.Join(hbDir, "title.mkv")
	if err := os.WriteFile(raw, bytes.Repeat([]byte("r"), 5000), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(enc, bytes.Repeat([]byte("e"), 200), 0644); err != nil {
		t.Fatal(err)
	}
	installMediaProbeFixtures(t, map[string]mediaFileProbe{
		raw: {DurationSec: 7200, AudioStreams: 2},
		enc: {DurationSec: 300, AudioStreams: 2},
	})

	title := TitleInfo{DiscId: 0, Index: 0, FileName: "title.mkv", DiscTitle: "DISC"}
	entry := EncodingParams{
		DiscId:               0,
		TitleIndex:           0,
		MKVOutputPath:        raw,
		HandBrakeOutputPath:  enc,
		RippedFileSizeBytes:  5000,
		EncodedFileSizeBytes: 200,
		HandBrakeExitCode:    0,
	}
	config := &handyMKVConfig{LibraryRoot: libraryRoot, MKVOutputDirectory: filepath.Join(tmp, "mkv")}
	opts := ExecOptions{MediaName: "Truncated Encode", MediaYear: "2024"}

	paths, keptRawCount, err := organizeMovieEntriesToLibrary([]EncodingParams{entry}, []TitleInfo{title}, config, opts)
	if err != nil {
		t.Fatalf("organizeMovieEntriesToLibrary: %v", err)
	}
	if keptRawCount != 1 {
		t.Fatalf("keptRawCount = %d, want raw in library for unverified encode", keptRawCount)
	}
	if len(paths) != 1 {
		t.Fatalf("paths = %#v, want one library path", paths)
	}
	if _, err := os.Stat(enc); err != nil {
		t.Fatalf("truncated encode must remain in staging: %v", err)
	}
	if _, err := os.Stat(raw); !os.IsNotExist(err) {
		t.Fatalf("raw should be moved into library, not left in staging")
	}

	entry.LibraryEncodeUnverified = true
	deleteRawFiles(config, []EncodingParams{entry})
	if _, err := os.Stat(hbDir); os.IsNotExist(err) {
		t.Fatalf("handbrake staging should remain when cleanup is skipped")
	}
}

func TestMissingAudioTrackDoesNotDeleteGoodFiles(t *testing.T) {
	tmp := t.TempDir()
	libraryRoot := filepath.Join(tmp, "movies")
	rawDir := filepath.Join(tmp, "mkv", "DISC")
	hbDir := filepath.Join(tmp, "hb", "DISC")
	if err := os.MkdirAll(rawDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(hbDir, 0755); err != nil {
		t.Fatal(err)
	}

	raw := filepath.Join(rawDir, "title.mkv")
	enc := filepath.Join(hbDir, "title.mkv")
	if err := os.WriteFile(raw, bytes.Repeat([]byte("r"), 4000), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(enc, bytes.Repeat([]byte("e"), 800), 0644); err != nil {
		t.Fatal(err)
	}
	installMediaProbeFixtures(t, map[string]mediaFileProbe{
		raw: {DurationSec: 5400, AudioStreams: 2},
		enc: {DurationSec: 5400, AudioStreams: 1},
	})

	entry := EncodingParams{
		DiscId:               0,
		TitleIndex:           0,
		MKVOutputPath:        raw,
		HandBrakeOutputPath:  enc,
		RippedFileSizeBytes:  4000,
		EncodedFileSizeBytes: 800,
		HandBrakeExitCode:    0,
	}
	title := TitleInfo{DiscId: 0, Index: 0, FileName: "title.mkv", DiscTitle: "DISC"}
	config := &handyMKVConfig{LibraryRoot: libraryRoot, MKVOutputDirectory: filepath.Join(tmp, "mkv")}
	opts := ExecOptions{MediaName: "Missing Audio", MediaYear: "2023"}

	if _, _, err := organizeMovieEntriesToLibrary([]EncodingParams{entry}, []TitleInfo{title}, config, opts); err != nil {
		t.Fatalf("organizeMovieEntriesToLibrary: %v", err)
	}
	if _, err := os.Stat(raw); !os.IsNotExist(err) {
		t.Fatalf("full raw rip should be moved to library")
	}
	if _, err := os.Stat(enc); err != nil {
		t.Fatalf("encode with missing audio must be kept in staging: %v", err)
	}
}

func TestNonzeroHandBrakeExitDoesNotDeleteGoodFiles(t *testing.T) {
	tmp := t.TempDir()
	libraryRoot := filepath.Join(tmp, "movies")
	rawDir := filepath.Join(tmp, "mkv", "DISC")
	hbDir := filepath.Join(tmp, "hb", "DISC")
	if err := os.MkdirAll(rawDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(hbDir, 0755); err != nil {
		t.Fatal(err)
	}

	raw := filepath.Join(rawDir, "title.mkv")
	enc := filepath.Join(hbDir, "title.mkv")
	if err := os.WriteFile(raw, bytes.Repeat([]byte("r"), 3000), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(enc, bytes.Repeat([]byte("e"), 500), 0644); err != nil {
		t.Fatal(err)
	}
	installMediaProbeFixtures(t, verifiedMovieProbeFixtures(raw, enc))

	entry := EncodingParams{
		DiscId:               0,
		TitleIndex:           0,
		MKVOutputPath:        raw,
		HandBrakeOutputPath:  enc,
		RippedFileSizeBytes:  3000,
		EncodedFileSizeBytes: 500,
		HandBrakeExitCode:    1,
	}
	title := TitleInfo{DiscId: 0, Index: 0, FileName: "title.mkv", DiscTitle: "DISC"}
	config := &handyMKVConfig{LibraryRoot: libraryRoot, MKVOutputDirectory: filepath.Join(tmp, "mkv")}
	opts := ExecOptions{MediaName: "Bad Exit", MediaYear: "2022"}

	if _, _, err := organizeMovieEntriesToLibrary([]EncodingParams{entry}, []TitleInfo{title}, config, opts); err != nil {
		t.Fatalf("organizeMovieEntriesToLibrary: %v", err)
	}
	if _, err := os.Stat(enc); err != nil {
		t.Fatalf("encode must remain when HandBrake exit was non-zero: %v", err)
	}
	if _, err := os.Stat(raw); !os.IsNotExist(err) {
		t.Fatalf("raw should still be placed in library when encode is unverified")
	}
}

func TestDurationWithinEncodeTolerance(t *testing.T) {
	if !durationWithinEncodeTolerance(7200, 7198) {
		t.Fatal("expected 2s diff within tolerance for long feature")
	}
	if !durationWithinEncodeTolerance(100, 97) {
		t.Fatal("expected 3s floor tolerance for short clip")
	}
	if durationWithinEncodeTolerance(7200, 7100) {
		t.Fatal("expected large duration gap to fail verification")
	}
}
