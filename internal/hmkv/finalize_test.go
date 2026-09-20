package hmkv

import (
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestNewRunOutputSlugIncludesPID(t *testing.T) {
	start := time.Date(2026, 9, 18, 16, 0, 0, 123, time.UTC)
	slug := newRunOutputSlug(start)
	pid := strconv.Itoa(os.Getpid())
	if !strings.Contains(slug, pid) {
		t.Fatalf("slug %q should contain pid %q", slug, pid)
	}
	if !strings.Contains(slug, "2026-09-18_16-00-00") {
		t.Fatalf("slug %q should contain timestamp", slug)
	}
}

func TestManifestFileNameUniquePerProcess(t *testing.T) {
	start := time.Date(2026, 9, 18, 16, 0, 0, 456, time.UTC)
	name := manifestFileName(start)
	if !strings.HasPrefix(name, "manifest_2026-09-18_16-00-00_") {
		t.Fatalf("unexpected manifest name %q", name)
	}
	if !strings.Contains(name, strconv.Itoa(os.Getpid())) {
		t.Fatalf("manifest name %q should contain pid", name)
	}
}

func TestWithFinalizeLockSerializesConcurrentRuns(t *testing.T) {
	lockFile := filepath.Join(t.TempDir(), "finalize.lock")
	finalizeLockPathOverride = lockFile
	t.Cleanup(func() { finalizeLockPathOverride = "" })

	var counter int
	var holders int32
	var wg sync.WaitGroup
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_ = withFinalizeLock(func() error {
				cur := atomic.AddInt32(&holders, 1)
				if cur > 1 {
					t.Errorf("expected mutual exclusion, concurrent holders = %d", cur)
				}
				counter++
				atomic.AddInt32(&holders, -1)
				return nil
			})
		}()
	}
	wg.Wait()
	if counter != 8 {
		t.Fatalf("counter = %d, want 8", counter)
	}
}

func TestOrganizeMovieEntriesToLibrary(t *testing.T) {
	tmp := t.TempDir()
	libraryRoot := filepath.Join(tmp, "movies")
	srcDir := filepath.Join(tmp, "hb")
	if err := os.MkdirAll(srcDir, 0755); err != nil {
		t.Fatal(err)
	}
	src := filepath.Join(srcDir, "Honeymoon_in_Vegas-B1_t00.mkv")
	if err := os.WriteFile(src, []byte("encoded"), 0644); err != nil {
		t.Fatal(err)
	}

	title := TitleInfo{DiscId: 0, Index: 0, FileName: "Honeymoon_in_Vegas-B1_t00.mkv", DiscTitle: "HONEYMOON IN VEGAS"}
	entry := EncodingParams{DiscId: 0, TitleIndex: 0, HandBrakeOutputPath: src}
	config := &handyMKVConfig{LibraryRoot: libraryRoot}
	opts := ExecOptions{MediaName: "Honeymoon in Vegas", MediaYear: "1992"}

	paths, err := organizeMovieEntriesToLibrary([]EncodingParams{entry}, []TitleInfo{title}, config, opts)
	if err != nil {
		t.Fatalf("organizeMovieEntriesToLibrary: %v", err)
	}
	want := filepath.Join(libraryRoot, "Honeymoon in Vegas (1992)", "Honeymoon in Vegas (1992).mkv")
	if len(paths) != 1 || paths[0] != want {
		t.Fatalf("paths = %#v, want [%q]", paths, want)
	}
	if _, err := os.Stat(want); err != nil {
		t.Fatalf("missing library file: %v", err)
	}
}

func TestOrganizeMovieEntriesToLibraryWithEditionTag(t *testing.T) {
	tmp := t.TempDir()
	libraryRoot := filepath.Join(tmp, "movies")
	srcDir := filepath.Join(tmp, "hb", "KNOCKED_UP")
	if err := os.MkdirAll(srcDir, 0755); err != nil {
		t.Fatal(err)
	}
	src := filepath.Join(srcDir, "C1_t00.mkv")
	if err := os.WriteFile(src, []byte("encoded"), 0644); err != nil {
		t.Fatal(err)
	}

	title := TitleInfo{DiscId: 0, Index: 0, FileName: "C1_t00.mkv", DiscTitle: "KNOCKED_UP"}
	entry := EncodingParams{DiscId: 0, TitleIndex: 0, HandBrakeOutputPath: src}
	config := &handyMKVConfig{LibraryRoot: libraryRoot}
	opts := ExecOptions{MediaName: "Knocked Up {edition-Unrated}", MediaYear: "2007"}

	paths, err := organizeMovieEntriesToLibrary([]EncodingParams{entry}, []TitleInfo{title}, config, opts)
	if err != nil {
		t.Fatalf("organizeMovieEntriesToLibrary: %v", err)
	}
	want := filepath.Join(libraryRoot, "Knocked Up (2007)", "Knocked Up (2007) {edition-Unrated}.mkv")
	if len(paths) != 1 || paths[0] != want {
		t.Fatalf("paths = %#v, want [%q]", paths, want)
	}
	if _, err := os.Stat(src); !os.IsNotExist(err) {
		t.Fatalf("encoded staging file should be moved into library, stat err=%v", err)
	}
}

// TestFinalizeEncodedTitleMovesAfterEncode verifies per-title finalize (post-encode) moves
// the HandBrake output into the library. Older builds deferred organization until all titles
// finished, which overlapping runs could skip entirely when tracker.err was set early.
func TestFinalizeEncodedTitleMovesAfterEncode(t *testing.T) {
	tmp := t.TempDir()
	libraryRoot := filepath.Join(tmp, "movies")
	hbTree := filepath.Join(tmp, "hb", "DEVIL_PRADA_PS")
	if err := os.MkdirAll(hbTree, 0755); err != nil {
		t.Fatal(err)
	}
	encoded := filepath.Join(hbTree, "B1_t00.mkv")
	if err := os.WriteFile(encoded, []byte("encoded"), 0644); err != nil {
		t.Fatal(err)
	}

	lockFile := filepath.Join(t.TempDir(), "finalize.lock")
	finalizeLockPathOverride = lockFile
	statsDirOverride = filepath.Join(tmp, "stats")
	t.Cleanup(func() {
		finalizeLockPathOverride = ""
		statsDirOverride = ""
	})

	title := TitleInfo{DiscId: 0, Index: 0, FileName: "B1_t00.mkv", DiscTitle: "DEVIL_PRADA_PS"}
	entry := EncodingParams{
		DiscId:              0,
		TitleIndex:          0,
		HandBrakeOutputPath: encoded,
		CompletedAt:         time.Now().UTC(),
	}
	config := &handyMKVConfig{LibraryRoot: libraryRoot}
	opts := ExecOptions{MediaName: "The Devil Wears Prada", MediaYear: "2006"}

	dest, err := finalizeEncodedTitle(entry, title, 0, config, opts, nil)
	if err != nil {
		t.Fatalf("finalizeEncodedTitle: %v", err)
	}
	want := filepath.Join(libraryRoot, "The Devil Wears Prada (2006)", "The Devil Wears Prada (2006).mkv")
	if dest != want {
		t.Fatalf("dest = %q, want %q", dest, want)
	}
	if _, err := os.Stat(encoded); !os.IsNotExist(err) {
		t.Fatalf("staging encode should be moved; stat err=%v", err)
	}
	if _, err := os.Stat(want); err != nil {
		t.Fatalf("library file missing: %v", err)
	}
}
