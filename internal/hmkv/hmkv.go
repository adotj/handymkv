package hmkv

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"
)

// ExecOptions controls a HandyMKV rip/encode run.
type ExecOptions struct {
	DiscIds         []int
	AppVersion      string
	AutomationNames []string
	TitleMode       string
	MediaName       string
	MediaYear       string
	TVMode          bool
	Season          int // -1 means unset (prompt in TV mode)
	StartEpisode    int
}

// Executes the main functionality of the program.
// Reads the configuration file, reads titles from the disc, prompts the user for which titles they want to rip,
// and processes the selected titles. TitleMode may be "longest", "all", or comma-separated title IDs to skip the prompt.
func Exec(mkv *MakeMKV, hb *HandBrakeCLI, opts ExecOptions) (execErr error) {
	if opts.StartEpisode < 1 {
		opts.StartEpisode = 1
	}
	if opts.Season < -1 {
		opts.Season = -1
	}

	config, err := ReadConfig()

	if err != nil {
		if err == ErrConfigNotFound {
			return err
		}

		return fmt.Errorf("an unexpected error occurred while reading the configuration file: %w", err)
	}

	defer func() {
		if execErr != nil {
			notifyRipFailure(config, execErr)
		}
	}()

	// Make sure the output directories exist
	err = os.MkdirAll(config.MKVOutputDirectory, 0740)

	if err != nil {
		return fmt.Errorf("an error occurred while creating the mkv output directory: %w", err)
	}

	err = os.MkdirAll(config.HBOutputDirectory, 0740)

	if err != nil {
		return fmt.Errorf("an error occurred while creating the handbrake output directory: %w", err)
	}

	processTitles := make([]TitleInfo, 0)

	for i, discId := range opts.DiscIds {

		fmt.Printf("Reading titles from disc %d...\n\n", discId)

		titles, err := mkv.getTitles(discId)

		if err != nil {
			return err
		}

		fmt.Printf("The following titles were read from the disc - %s\n\n", titles[0].DiscTitle)

		longestIdx := findLongestTitle(titles)
		for i, title := range titles {
			marker := ""
			if i == longestIdx {
				marker = " (longest)"
			}
			if opts.TVMode && title.Chapters > 0 {
				fmt.Printf("ID: %d, Title Name: %s, Size: %s, Length: %s, Chapters: %d%s\n", title.Index, title.FileName, title.FileSizeDesc, title.Length, title.Chapters, marker)
			} else {
				fmt.Printf("ID: %d, Title Name: %s, Size: %s, Length: %s%s\n", title.Index, title.FileName, title.FileSizeDesc, title.Length, marker)
			}
		}

		var titleSelections string
		if strings.TrimSpace(opts.TitleMode) != "" {
			titleSelections = opts.TitleMode
			if strings.EqualFold(opts.TitleMode, "longest") && longestIdx >= 0 {
				fmt.Printf("\nAuto-selected longest title: ID %d (%s, %s)\n", titles[longestIdx].Index, titles[longestIdx].FileName, titles[longestIdx].Length)
			} else if strings.EqualFold(opts.TitleMode, "all") {
				fmt.Printf("\nAuto-selected all %d titles\n", len(titles))
			} else {
				fmt.Printf("\nAuto-selected titles: %s\n", opts.TitleMode)
			}
		} else if opts.TVMode {
			fmt.Print("\nEnter title IDs (0,1,2...), 'all', or 'longest':\n\n")
			titleSelections = readLine()
			if strings.TrimSpace(titleSelections) == "" {
				fmt.Printf("\nNo title selection provided. Exiting.\n\n")
				return nil
			}
		} else {
			fmt.Print("\nEnter the IDs of the titles to process (0,1,2...) or 'all'. Press Enter to select the longest title: \n\n")
			titleSelections = readLine()
		}

		selected, selErr := applyTitleSelection(titles, titleSelections)
		if selErr != nil {
			fmt.Printf("\n%s\n\n", selErr.Error())
			return nil
		}
		if !opts.TVMode && strings.TrimSpace(opts.TitleMode) == "" && strings.TrimSpace(titleSelections) == "" && len(selected) == 1 {
			fmt.Printf("Selected longest title: ID %d (%s)\n", selected[0].Index, selected[0].FileName)
		}

		titles = selected

		processTitles = append(processTitles, titles...)

		if i < len(opts.DiscIds)-1 {
			fmt.Println()
		}
	}

	if len(processTitles) < 1 {
		fmt.Printf("\nNo titles to process. Exiting.\n\n")
		return nil
	}

	var tvMeta *tvSeriesMeta
	if opts.TVMode {
		applyLibraryConfigDefaults(config)
		var tvErr error
		tvMeta, tvErr = resolveTVSeriesMeta(processTitles, opts, config.TVLibraryRoot)
		if tvErr != nil {
			return tvErr
		}
		// Keep resolved values for notifications after encode.
		opts.Season = tvMeta.Season
		opts.StartEpisode = tvMeta.StartEpisode
		opts.MediaName = tvMeta.Series
		opts.MediaYear = tvMeta.Year
	}

	// If there any titles that have an identical disc title to another disc, set prependDiscToSub to true for those titles
	var discNames = make(map[string]int)

	for _, title := range processTitles {
		discNames[strings.ToLower(title.DiscTitle)]++
	}

	for i := range processTitles {
		if discNames[strings.ToLower(processTitles[i].DiscTitle)] > 1 {
			processTitles[i].SetPrependDiscToSubdirectory(true)
		}
	}

	// Titles progress tracking
	tracker := progressTracker{
		statuses:        make([]titleStatus, len(processTitles)),
		refreshInterval: 200 * time.Millisecond,
	}

	for i, title := range processTitles {
		tracker.statuses[i] = titleStatus{
			TitleIndex:        title.Index,
			Title:             title.FileName,
			DiscId:            title.DiscId,
			Ripping:           Pending,
			Encoding:          Pending,
			ExpectedSizeBytes: int64(title.FileSizeBytes),
		}
	}

	// Create output directory dirSlug with timestamp
	dirSlug := fmt.Sprintf("handymkv_%s", time.Now().Format("2006-01-02_15-04-05"))

	config.MKVOutputDirectory = filepath.Join(config.MKVOutputDirectory, dirSlug)

	err = os.MkdirAll(config.MKVOutputDirectory, 0740)

	if err != nil {
		return fmt.Errorf("an error occurred while creating the mkv output directory: %w", err)
	}

	config.HBOutputDirectory = filepath.Join(config.HBOutputDirectory, dirSlug)

	err = os.MkdirAll(config.HBOutputDirectory, 0740)

	if err != nil {
		return fmt.Errorf("an error occurred while creating the handbrake output directory: %w", err)
	}

	fmt.Println()

	// Automation selection and pre-run param collection
	var selectedAutomations []Automation
	var preRunParams map[string]string

	for _, name := range opts.AutomationNames {
		a, err := LoadAutomation(name)
		if err != nil {
			fmt.Printf("Warning: could not load automation '%s': %v\n", name, err)
			continue
		}
		selectedAutomations = append(selectedAutomations, *a)
	}

	if len(selectedAutomations) > 0 {
		var paramErr error
		preRunParams, paramErr = resolvePreRunParams(selectedAutomations)
		if paramErr != nil {
			fmt.Printf("Warning: %v\nAutomations will be skipped.\n\n", paramErr)
			selectedAutomations = nil
		}
	}

	ctx, cancelProcessing := context.WithCancel(context.Background())
	var encChannel = make(chan EncodingParams, len(processTitles))
	var processWaitGroup sync.WaitGroup
	var manifestMu sync.Mutex
	var manifestEntries []EncodingParams

	processStartTime := time.Now()
	tracker.processStartTime = processStartTime

	// Start central refresh ticker for display updates
	stopRefreshTicker := tracker.startRefreshTicker(ctx)

	// MKV
	processWaitGroup.Add(1)

	// For each disc rip the titles
	go func() {
		defer close(encChannel)
		defer processWaitGroup.Done()

		var rippingWaitGroup sync.WaitGroup

		for _, discId := range opts.DiscIds {
			var discTitles []TitleInfo

			for _, title := range processTitles {
				if title.DiscId == discId {
					discTitles = append(discTitles, title)
				}
			}

			if len(discTitles) < 1 {
				continue
			}

			// Make sure the subdirectories exists
			os.MkdirAll(filepath.Join(config.MKVOutputDirectory, discTitles[0].Subdirectory()), 0740)
			os.MkdirAll(filepath.Join(config.HBOutputDirectory, discTitles[0].Subdirectory()), 0740)

			rippingWaitGroup.Add(1)

			go func() {
				defer rippingWaitGroup.Done()
				ripTitles(mkv, ctx, &tracker, discTitles, config, encChannel, cancelProcessing)
			}()
		}

		rippingWaitGroup.Wait()
	}()

	// HB
	processWaitGroup.Add(1)

	var encodingNotifyOnce sync.Once

	go func() {
		defer processWaitGroup.Done()
		for {
			select {
			case params, ok := <-encChannel:
				if !ok {
					return
				}

				encodingNotifyOnce.Do(func() {
					notifyEncodingStarted(config, processTitles, opts, tvMeta)
				})

				tracker.applyChange(params.TitleIndex, params.DiscId, func(status *titleStatus) {
					status.Encoding = InProgress
					status.EncodingProgress = -1
					status.EncodingETA = ""
					status.EncodingStage = ""
				})

				// Make sure the input file exists
				if _, err := os.Stat(params.MKVOutputPath); os.IsNotExist(err) {
					tracker.setError(fmt.Errorf("encoding input file %s does not exist", params.MKVOutputPath))
					cancelProcessing()
					return
				}

				hbProgressUpdate := func(percent int, eta string, stage string) {
					tracker.applyChange(params.TitleIndex, params.DiscId, func(status *titleStatus) {
						if stage != "" {
							status.EncodingStage = stage
						}
						if percent >= 0 {
							status.EncodingProgress = percent
							status.EncodingETA = eta
							if stage == "" {
								status.EncodingStage = ""
							}
						}
					})
				}

				encodeStart := time.Now()
				encErr := hb.encode(ctx, &params, hbProgressUpdate)

				if encErr != nil {
					tracker.setError(encErr)
					cancelProcessing()
					return
				}

				// Update progress for encoding completion
				tracker.applyChange(params.TitleIndex, params.DiscId, func(status *titleStatus) {
					status.Encoding = Complete
					status.EncodingProgress = 100
				})
				tracker.forceRefresh() // Force immediate display for completion

				if stat, err := os.Stat(params.HandBrakeOutputPath); err == nil {
					params.EncodedFileSizeBytes = stat.Size()
				}

				ripDuration, _ := time.ParseDuration(params.RippingDuration)
				params.ProcessingDuration = ripDuration + time.Since(encodeStart).Round(time.Second)
				params.CompletedAt = time.Now().UTC()

				manifestMu.Lock()
				manifestEntries = append(manifestEntries, params)
				manifestMu.Unlock()
			case <-ctx.Done():
				return
			}
		}
	}()

	processWaitGroup.Wait()

	// Stop ticker IMMEDIATELY before printing final messages
	stopRefreshTicker()

	if tracker.err != nil {
		return tracker.err
	}

	processDuration := time.Since(processStartTime).Round(time.Second)

	fmt.Printf("\nOperation Complete. Time Elapsed - %s\n", formatTimeElapsedString(processDuration))

	totalSizeRaw, totalSizeEncoded, err := calculateTotalFileSizes(processTitles, config)

	if err != nil {
		fmt.Printf("An error occurred while calculating total sizes - %v\n", err)
	}

	fmt.Printf("\nTotal size of raw unencoded files - %s\n", formatSavedSpace(totalSizeRaw))
	fmt.Printf("Total size of encoded files - %s\n", formatSavedSpace(totalSizeEncoded))

	savedSpace := totalSizeRaw - totalSizeEncoded
	if savedSpace > 0 {
		fmt.Printf("Total disk space saved via encoding - %s\n", formatSavedSpace(totalSizeRaw-totalSizeEncoded))
	}

	libraryPaths, err := organizeEncodedFilesToLibrary(manifestEntries, processTitles, config, opts, tvMeta)
	if err != nil {
		return fmt.Errorf("library organization failed: %w", err)
	}

	if err := appendRipHistory(manifestEntries, processTitles, libraryPaths); err != nil {
		fmt.Printf("Warning: could not log rip stats: %v\n", err)
	} else {
		printCatalogSummary()
	}

	// Run automations before raw file deletion so scripts can access raw MKV files
	var automationEntries []manifestAutomation
	if len(selectedAutomations) > 0 {
		outputData := buildRunOutputData(
			config.HBOutputDirectory,
			config.MKVOutputDirectory,
			processDuration,
			len(processTitles),
			true,
			totalSizeRaw,
			totalSizeEncoded,
		)
		automationEntries = RunAutomations(selectedAutomations, preRunParams, outputData)
	}

	manifestDir, mdErr := getManifestDir()
	if mdErr != nil {
		fmt.Printf("Warning: could not determine manifest directory: %v\n", mdErr)
	} else {
		m := buildManifest(processTitles, manifestEntries, processStartTime, processDuration, true, opts.AppVersion, automationEntries)
		manifestPath, wErr := writeManifest(manifestDir, processStartTime, m)
		if wErr != nil {
			fmt.Printf("Warning: could not write manifest: %v\n", wErr)
		} else {
			fmt.Printf("Manifest written to: %s\n", manifestPath)
		}
	}

	deleteRawFiles(config)

	if len(libraryPaths) > 0 {
		fmt.Printf("\nOrganized library files:\n")
		for _, path := range libraryPaths {
			fmt.Printf("  %s\n", path)
		}
		fmt.Println()
	} else {
		fmt.Printf("\nEncoded files are located in: %s\n\n", config.HBOutputDirectory)
	}

	return nil
}

func ripTitles(
	mkv *MakeMKV,
	ctx context.Context,
	tracker *progressTracker,
	processTitles []TitleInfo,
	config *handyMKVConfig,
	encChannel chan EncodingParams,
	cancelProcessing context.CancelFunc) {

	for _, title := range processTitles {
		mkvOutputDirectory := filepath.Join(config.MKVOutputDirectory, title.Subdirectory())
		mkvOutputPath := filepath.Join(mkvOutputDirectory, title.FileName)

		ripStartTime := time.Now()

		// Start progress poller before ripping
		stopPoller := tracker.startProgressPoller(
			ctx,
			title.Index,
			title.DiscId,
			int64(title.FileSizeBytes),
			mkvOutputPath,
		)

		tracker.applyChange(title.Index, title.DiscId, func(status *titleStatus) {
			status.Ripping = InProgress
			status.RippingProgress = -1
			status.OutputFilePath = mkvOutputPath
		})

		ripErr := mkv.ripTitle(ctx, &title, mkvOutputDirectory, func(percent int, stage string) {
			tracker.applyChange(title.Index, title.DiscId, func(status *titleStatus) {
				if stage != "" {
					status.RippingStage = stage
				}
				if percent >= 0 {
					status.RippingHasLiveProgress = true
					status.RippingProgress = percent
				}
			})
		})

		// Stop the poller regardless of success or failure
		stopPoller()

		if ripErr != nil {
			tracker.setError(ripErr)
			cancelProcessing()
			return
		}

		// Update progress for ripping completion
		tracker.applyChange(title.Index, title.DiscId, func(status *titleStatus) {
			status.Ripping = Complete
			status.RippingProgress = 100
		})
		tracker.forceRefresh() // Force immediate display for completion

		ripDuration := time.Since(ripStartTime).Round(time.Second)
		var rippedSizeBytes int64
		if stat, err := os.Stat(mkvOutputPath); err == nil {
			rippedSizeBytes = stat.Size()
		}

		encodingOutputFileName := title.GetEncodingFileName()

		hbOutputDir := filepath.Join(config.HBOutputDirectory, title.Subdirectory())

		enc := defaultEncodingParams()
		enc.TitleIndex = title.Index
		enc.DiscId = title.DiscId
		enc.MKVOutputPath = mkvOutputPath
		enc.HandBrakeOutputPath = filepath.Join(hbOutputDir, encodingOutputFileName)
		enc.RippedFileSizeBytes = rippedSizeBytes
		enc.RippingDuration = ripDuration.String()

		encChannel <- enc
	}
}

// Prompts the user to create a configuration file.
func Setup() error {
	fmt.Printf("What level of configuration would you like to create?\n\n")
	fmt.Println("1 - User-wide configuration (recommended).")
	fmt.Println("2 - Current working directory.")
	fmt.Println()

	configLocationSelectionString := readLine()

	fmt.Println()

	configLocationSelection, err := strconv.Atoi(configLocationSelectionString)

	if err != nil {
		fmt.Println("Configuration file location selection could not be parsed.")
		return err
	}

	if configLocationSelection < 1 || configLocationSelection > 2 {
		fmt.Println("Invalid configuration file location selection.")
		return nil
	}

	var config *handyMKVConfig

	for {
		config, err = promptForConfig(configLocationSelection)

		if err != nil {
			fmt.Printf("An error occurred while prompting for configuration values: %v\n", err)
			return err
		}

		fmt.Printf("\n%s\n", config.String())
		fmt.Printf("Accept these settings? [y/N]\n\n")

		if strings.ToLower(readLine()) == "y" {
			break
		}
	}

	clear()
	fmt.Println("Creating config file...")

	err = createConfigFile(configFileLocation(configLocationSelection), config, false)

	if err != nil {
		return err
	}

	fmt.Printf("\nConfig file creation complete.\n\n")

	return nil
}

// applyTitleSelection filters titles based on user input.
// Empty input or "longest" selects the longest title. "all" keeps every title.
// Comma-separated IDs are returned in the order the user specified.
func applyTitleSelection(titles []TitleInfo, raw string) ([]TitleInfo, error) {
	raw = strings.ReplaceAll(raw, " ", "")
	raw = strings.Trim(raw, ",")
	raw = strings.ReplaceAll(raw, "(", "")
	raw = strings.ReplaceAll(raw, ")", "")

	if raw == "" || strings.EqualFold(raw, "longest") {
		idx := findLongestTitle(titles)
		if idx < 0 {
			return nil, fmt.Errorf("No selected titles detected.")
		}
		return []TitleInfo{titles[idx]}, nil
	}

	if strings.EqualFold(raw, "all") {
		return titles, nil
	}

	rawIds := strings.Split(raw, ",")
	selectedIds := make([]int, 0, len(rawIds))
	for _, idStr := range rawIds {
		if idStr == "" {
			continue
		}
		id, err := strconv.Atoi(idStr)
		if err != nil {
			return nil, fmt.Errorf("Invalid title selection input detected.")
		}
		selectedIds = append(selectedIds, id)
	}

	if len(selectedIds) < 1 {
		return nil, fmt.Errorf("No selected titles detected.")
	}

	titleByIndex := make(map[int]TitleInfo, len(titles))
	for _, title := range titles {
		titleByIndex[title.Index] = title
	}

	filtered := make([]TitleInfo, 0, len(selectedIds))
	seen := make(map[int]struct{}, len(selectedIds))
	for _, id := range selectedIds {
		if _, ok := seen[id]; ok {
			continue
		}
		title, ok := titleByIndex[id]
		if !ok {
			continue
		}
		filtered = append(filtered, title)
		seen[id] = struct{}{}
	}

	if len(filtered) < 1 {
		return nil, fmt.Errorf("No selected titles detected.")
	}
	return filtered, nil
}
