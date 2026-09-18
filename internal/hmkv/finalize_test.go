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
