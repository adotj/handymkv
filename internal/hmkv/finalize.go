package hmkv

import (
	"fmt"
	"os"
	"path/filepath"
)

// finalizeEncodedTitle moves one encoded file into the library and appends rip history.
// A cross-process lock serializes this step so overlapping handymkv runs do not corrupt
// shared history files or race on library moves.
func finalizeEncodedTitle(
	entry *EncodingParams,
	title TitleInfo,
	titleOrdinal int,
	config *handyMKVConfig,
	opts ExecOptions,
	tvMeta *tvSeriesMeta,
) (libraryPath string, keptRaw bool, err error) {
	applyLibraryConfigDefaults(config)

	err = withFinalizeLock(func() error {
		if _, statErr := os.Stat(entry.HandBrakeOutputPath); statErr != nil {
			if os.IsNotExist(statErr) {
				return fmt.Errorf("encoded file %s is missing before library organization", entry.HandBrakeOutputPath)
			}
			return fmt.Errorf("could not stat encoded file %s: %w", entry.HandBrakeOutputPath, statErr)
		}

		var paths []string
		var keptRawCount int
		if opts.TVMode {
			if tvMeta == nil {
				return fmt.Errorf("TV series metadata is required for TV library organization")
			}
			episodeMeta := *tvMeta
			episodeMeta.StartEpisode = tvMeta.StartEpisode + titleOrdinal
			paths, keptRawCount, err = organizeTVEpisodesToLibrary([]EncodingParams{*entry}, []TitleInfo{title}, config, episodeMeta)
		} else {
			paths, keptRawCount, err = organizeMovieEntriesToLibrary([]EncodingParams{*entry}, []TitleInfo{title}, config, opts)
		}
		if err != nil {
			return err
		}
		if len(paths) == 0 {
			return fmt.Errorf("library organization produced no destination path")
		}

		libraryPath = paths[0]
		keptRaw = keptRawCount > 0
		if keptRaw {
			entry.LibraryKeptRaw = true
			if entry.RippedFileSizeBytes > 0 {
				entry.EncodedFileSizeBytes = entry.RippedFileSizeBytes
			}
		}

		if err := appendRipHistory([]EncodingParams{*entry}, []TitleInfo{title}, paths); err != nil {
			fmt.Printf("Warning: encoded file moved to library but rip stats were not updated: %v\n", err)
		}
		return nil
	})

	return libraryPath, keptRaw, err
}

// organizeMovieEntriesToLibrary is the movie branch of organizeEncodedFilesToLibrary,
// exposed for per-title finalize and tests.
func organizeMovieEntriesToLibrary(entries []EncodingParams, titles []TitleInfo, config *handyMKVConfig, opts ExecOptions) ([]string, int, error) {
	titleByKey := make(map[string]TitleInfo, len(titles))
	for _, title := range titles {
		titleByKey[titleKey(title.DiscId, title.Index)] = title
	}

	finalPaths := make([]string, 0, len(entries))
	keptRawCount := 0
	useCLIForSingleTitle := len(entries) == 1 && (opts.MediaName != "" || opts.MediaYear != "")

	for _, entry := range entries {
		title, ok := titleByKey[titleKey(entry.DiscId, entry.TitleIndex)]
		if !ok {
			return finalPaths, keptRawCount, fmt.Errorf("could not find title metadata for disc %d title %d", entry.DiscId, entry.TitleIndex)
		}

		nameFlag := ""
		yearFlag := ""
		if useCLIForSingleTitle {
			nameFlag = opts.MediaName
			yearFlag = opts.MediaYear
		}

		libName, err := resolveMovieLibraryName(title, nameFlag, yearFlag, config.LibraryRoot)
		if err != nil {
			return finalPaths, keptRawCount, err
		}

		folderName := libName.FolderName()
		destDir := filepath.Join(config.LibraryRoot, folderName)
		ext := filepath.Ext(entry.HandBrakeOutputPath)
		if ext == "" {
			ext = filepath.Ext(entry.MKVOutputPath)
		}
		destPath := filepath.Join(destDir, libName.FileBaseName()+ext)

		if err := os.MkdirAll(destDir, 0755); err != nil {
			return finalPaths, keptRawCount, fmt.Errorf("could not create library folder %s: %w", destDir, err)
		}

		if _, err := os.Stat(destPath); err == nil {
			return finalPaths, keptRawCount, fmt.Errorf("library file already exists: %s", destPath)
		}

		keptRaw, err := moveLibrarySource(entry, destPath)
		if err != nil {
			return finalPaths, keptRawCount, err
		}
		if keptRaw {
			keptRawCount++
		}
		finalPaths = append(finalPaths, destPath)
	}

	return finalPaths, keptRawCount, nil
}
