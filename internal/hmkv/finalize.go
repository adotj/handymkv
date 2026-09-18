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
	entry EncodingParams,
	title TitleInfo,
	titleOrdinal int,
	config *handyMKVConfig,
	opts ExecOptions,
	tvMeta *tvSeriesMeta,
) (libraryPath string, err error) {
	applyLibraryConfigDefaults(config)

	err = withFinalizeLock(func() error {
		if _, statErr := os.Stat(entry.HandBrakeOutputPath); statErr != nil {
			if os.IsNotExist(statErr) {
				return fmt.Errorf("encoded file %s is missing before library organization", entry.HandBrakeOutputPath)
			}
			return fmt.Errorf("could not stat encoded file %s: %w", entry.HandBrakeOutputPath, statErr)
		}

		var paths []string
		if opts.TVMode {
			if tvMeta == nil {
				return fmt.Errorf("TV series metadata is required for TV library organization")
			}
			episodeMeta := *tvMeta
			episodeMeta.StartEpisode = tvMeta.StartEpisode + titleOrdinal
			paths, err = organizeTVEpisodesToLibrary([]EncodingParams{entry}, []TitleInfo{title}, config, episodeMeta)
		} else {
			paths, err = organizeMovieEntriesToLibrary([]EncodingParams{entry}, []TitleInfo{title}, config, opts)
		}
		if err != nil {
			return err
		}
		if len(paths) == 0 {
			return fmt.Errorf("library organization produced no destination path")
		}

		libraryPath = paths[0]
		if err := appendRipHistory([]EncodingParams{entry}, []TitleInfo{title}, paths); err != nil {
			return fmt.Errorf("could not log rip stats: %w", err)
		}
		return nil
	})

	return libraryPath, err
}

// organizeMovieEntriesToLibrary is the movie branch of organizeEncodedFilesToLibrary,
// exposed for per-title finalize and tests.
func organizeMovieEntriesToLibrary(entries []EncodingParams, titles []TitleInfo, config *handyMKVConfig, opts ExecOptions) ([]string, error) {
	titleByKey := make(map[string]TitleInfo, len(titles))
	for _, title := range titles {
		titleByKey[titleKey(title.DiscId, title.Index)] = title
	}

	finalPaths := make([]string, 0, len(entries))
	useCLIForSingleTitle := len(entries) == 1 && (opts.MediaName != "" || opts.MediaYear != "")

	for _, entry := range entries {
		title, ok := titleByKey[titleKey(entry.DiscId, entry.TitleIndex)]
		if !ok {
			return finalPaths, fmt.Errorf("could not find title metadata for disc %d title %d", entry.DiscId, entry.TitleIndex)
		}

		nameFlag := ""
		yearFlag := ""
		if useCLIForSingleTitle {
			nameFlag = opts.MediaName
			yearFlag = opts.MediaYear
		}

		libName, err := resolveMovieLibraryName(title, nameFlag, yearFlag, config.LibraryRoot)
		if err != nil {
			return finalPaths, err
		}

		folderName := libName.FolderName()
		destDir := filepath.Join(config.LibraryRoot, folderName)
		ext := filepath.Ext(entry.HandBrakeOutputPath)
		destPath := filepath.Join(destDir, folderName+ext)

		if err := os.MkdirAll(destDir, 0755); err != nil {
			return finalPaths, fmt.Errorf("could not create library folder %s: %w", destDir, err)
		}

		if _, err := os.Stat(destPath); err == nil {
			return finalPaths, fmt.Errorf("library file already exists: %s", destPath)
		}

		if err := moveFile(entry.HandBrakeOutputPath, destPath); err != nil {
			return finalPaths, fmt.Errorf("could not move encoded file to library: %w", err)
		}

		fmt.Printf("Moved to library: %s\n", destPath)
		finalPaths = append(finalPaths, destPath)
	}

	return finalPaths, nil
}
