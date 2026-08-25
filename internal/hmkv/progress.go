package hmkv

import (
	"context"
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"
)

// Keeps track of the progress of the ripping and encoding processes.
// Outputs the progress to the terminal.
type progressTracker struct {
	statuses          []titleStatus
	mutex             sync.Mutex
	err               error
	animationFrame    int // Shared animation frame for synchronized ellipsis
	lastRefreshTime   time.Time
	processStartTime  time.Time
	refreshInterval   time.Duration // Default: 200ms
	pendingRefresh    bool          // True if data changed since last refresh
}

type statusValue uint8

var etaPattern = regexp.MustCompile(`(?i)(\d+)h(\d+)m(\d+)s`)

const (
	Pending statusValue = iota
	InProgress
	Complete
)

// String representation of the statusValue.
func (s statusValue) String() string {
	switch s {
	case Pending:
		return "Pending"
	case InProgress:
		return "In Progress"
	case Complete:
		return "Complete"
	default:
		return "Unknown"
	}
}

// Represents the status of a title.
type titleStatus struct {
	// The index of the title on the disc.
	TitleIndex int
	// The name of the title.
	Title string
	// The disc index.
	DiscId int
	// The status of the ripping process.
	Ripping statusValue
	// The status of the encoding process.
	Encoding statusValue
	// Progress percentage for ripping (0-100, -1 for unknown).
	RippingProgress int
	// Progress percentage for encoding (0-100, -1 for unknown).
	EncodingProgress int
	// Current ripping stage from MakeMKV (e.g. "Saving to MKV file").
	RippingStage string
	// Current encoding stage from HandBrake (e.g. "Scanning").
	EncodingStage string
	// Compact encoding ETA from HandBrake (e.g. "1h12m").
	EncodingETA string
	// True once MakeMKV PRGV progress has been seen; disables file-size fallback.
	RippingHasLiveProgress bool
	// Expected file size in bytes for ripping.
	ExpectedSizeBytes int64
	// Output file path being written (for polling).
	OutputFilePath string
}

// applyChange updates state without immediately displaying
func (pt *progressTracker) applyChange(titleIndex int, discId int, applyChangeFunc func(*titleStatus)) {
	pt.mutex.Lock()
	defer pt.mutex.Unlock()

	for i, status := range pt.statuses {
		if status.TitleIndex == titleIndex && status.DiscId == discId {
			applyChangeFunc(&pt.statuses[i])
			pt.pendingRefresh = true // Mark that we need a refresh
			break
		}
	}
}

// refreshDisplayIfNeeded refreshes display only if rate limit allows
func (pt *progressTracker) refreshDisplayIfNeeded() {
	pt.mutex.Lock()
	defer pt.mutex.Unlock()

	now := time.Now()
	if !pt.pendingRefresh || now.Sub(pt.lastRefreshTime) < pt.refreshInterval {
		return
	}

	pt.lastRefreshTime = now
	pt.pendingRefresh = false
	pt.refreshDisplay()
}

// forceRefresh forces immediate display update (for completions)
func (pt *progressTracker) forceRefresh() {
	pt.mutex.Lock()
	defer pt.mutex.Unlock()

	pt.lastRefreshTime = time.Now()
	pt.pendingRefresh = false
	pt.refreshDisplay()
}

func (pt *progressTracker) refreshDisplay() {
	// Use cursor positioning instead of full clear after initial display
	if !pt.lastRefreshTime.IsZero() {
		fmt.Print("\033[H") // Move to top
		clearFromCursor()   // Clear from cursor down
	} else {
		clear() // First time - do full clear
	}
	PrintLogo()
	fmt.Printf("%-30s%-10s%-20s%-20s\n", "Title", "Disc", "Ripping", "Encoding")
	fmt.Println(strings.Repeat("-", 80))

	for _, status := range pt.statuses {
		// Format and pad each column
		displayTitle := strings.TrimSuffix(status.Title, ".mkv")
		titleCol, titleTooLong := padString(displayTitle, 30)
		discIdCol, _ := padString(fmt.Sprintf("%d", status.DiscId), 10)

		rippingStr := formatProgressStatus(
			status.Ripping,
			status.RippingProgress,
			pt.animationFrame,
			status.RippingStage,
			"",
		)
		encodingStr := formatProgressStatus(
			status.Encoding,
			status.EncodingProgress,
			pt.animationFrame,
			status.EncodingStage,
			status.EncodingETA,
		)
		rippingCol, _ := padString(rippingStr, 20)
		encodingCol, _ := padString(encodingStr, 20)

		if titleTooLong {
			titleSpillOver := titleCol[27:]
			titleCol = fmt.Sprintf("%s   ", titleCol[0:27])
			fmt.Printf("%s%s%s%s\n", titleCol, discIdCol, rippingCol, encodingCol)
			fmt.Printf("%s\n", titleSpillOver)
			continue
		}

		// Print the row
		fmt.Printf("%s%s%s%s\n", titleCol, discIdCol, rippingCol, encodingCol)
	}

	if !pt.processStartTime.IsZero() {
		elapsed := time.Since(pt.processStartTime).Round(time.Second)
		fmt.Printf("\nTime elapsed: %s\n", formatTimeElapsedString(elapsed))
	}
}

// formatProgressStatus returns a colored status string with percentage and
// animation for in-progress items.
func formatProgressStatus(status statusValue, progress int, animFrame int, stage, eta string) string {
	color := getColor(status)

	switch status {
	case Pending:
		return colorize(status, color)
	case InProgress:
		ellipsis := getAnimatedEllipsis(animFrame)
		stage = shortStage(stage)
		switch {
		case progress >= 0 && eta != "":
			return fmt.Sprintf("%s%d%% %s%s", color, progress, eta, colorReset)
		case progress >= 0 && stage != "":
			return fmt.Sprintf("%s%s %d%%%s", color, stage, progress, colorReset)
		case progress < 0 && stage != "":
			return fmt.Sprintf("%s%s%s%s", color, stage, ellipsis, colorReset)
		case progress < 0:
			return fmt.Sprintf("%sWorking%s%s", color, ellipsis, colorReset)
		default:
			return fmt.Sprintf("%s%d%%%s%s", color, progress, ellipsis, colorReset)
		}
	case Complete:
		return colorize(status, color)
	default:
		return colorize(status, color)
	}
}

// shortStage returns the first word of a MakeMKV/HandBrake stage name so it
// fits in the progress table column (e.g. "Saving to MKV file" → "Saving").
func shortStage(stage string) string {
	stage = strings.TrimSpace(stage)
	if stage == "" {
		return ""
	}
	if i := strings.IndexAny(stage, " \t"); i > 0 {
		return stage[:i]
	}
	return stage
}

// compactETA turns HandBrake's "01h12m34s" into a shorter form like "1h12m".
func compactETA(raw string) string {
	raw = strings.TrimSpace(raw)
	m := etaPattern.FindStringSubmatch(raw)
	if m == nil {
		return raw
	}

	h, _ := strconv.Atoi(m[1])
	min, _ := strconv.Atoi(m[2])
	sec, _ := strconv.Atoi(m[3])

	switch {
	case h > 0:
		return fmt.Sprintf("%dh%02dm", h, min)
	case min > 0:
		return fmt.Sprintf("%dm%02ds", min, sec)
	default:
		return fmt.Sprintf("%ds", sec)
	}
}

// splitProgressLines yields a token at each \r or \n so in-place progress
// updates (HandBrake) and robot lines (MakeMKV) are seen immediately.
func splitProgressLines(data []byte, atEOF bool) (advance int, token []byte, err error) {
	if atEOF && len(data) == 0 {
		return 0, nil, nil
	}
	for i, b := range data {
		if b == '\r' || b == '\n' {
			return i + 1, data[0:i], nil
		}
	}
	if atEOF {
		return len(data), data, nil
	}
	return 0, nil, nil
}

// getAnimatedEllipsis returns ".", "..", or "..." based on the frame.
func getAnimatedEllipsis(frame int) string {
	switch frame {
	case 0:
		return ".  "
	case 1:
		return ".. "
	case 2:
		return "..."
	default:
		return ".  "
	}
}

func (pt *progressTracker) setError(err error) {
	pt.mutex.Lock()
	defer pt.mutex.Unlock()

	pt.err = err
}

func getColor(status statusValue) string {
	switch status {
	case Pending:
		return colorYellow
	case InProgress:
		return colorBlue
	case Complete:
		return colorGreen
	default:
		return colorReset
	}
}

// startProgressPoller starts a goroutine that polls the output file size and
// updates the progress percentage. Returns a function to stop the poller.
func (pt *progressTracker) startProgressPoller(
	ctx context.Context,
	titleIndex int,
	discId int,
	expectedSize int64,
	outputPath string,
) func() {
	stopChan := make(chan struct{})
	var stopOnce sync.Once

	go func() {
		pollTicker := time.NewTicker(2 * time.Second)
		defer pollTicker.Stop()

		for {
			select {
			case <-ctx.Done():
				return
			case <-stopChan:
				return
			case <-pollTicker.C:
				pt.updateProgressFromFile(titleIndex, discId, expectedSize, outputPath)
			}
		}
	}()

	return func() {
		stopOnce.Do(func() {
			close(stopChan)
		})
	}
}

// updateProgressFromFile reads the current file size and updates progress.
func (pt *progressTracker) updateProgressFromFile(
	titleIndex int,
	discId int,
	expectedSize int64,
	outputPath string,
) {
	currentSize, err := getFileSize(outputPath)
	if err != nil {
		// File doesn't exist yet - normal at start of ripping
		return
	}

	var percentage int
	if expectedSize > 0 {
		percentage = int((float64(currentSize) / float64(expectedSize)) * 100)
		// Cap at 99% until explicitly marked complete
		if percentage > 99 {
			percentage = 99
		}
		if percentage < 0 {
			percentage = 0
		}
	} else {
		// Unknown expected size
		percentage = -1
	}

	pt.applyChange(titleIndex, discId, func(status *titleStatus) {
		if status.RippingHasLiveProgress {
			return
		}
		status.RippingProgress = percentage
	})
}

// updateAnimation increments the shared animation frame counter and marks for refresh.
func (pt *progressTracker) updateAnimation() {
	pt.mutex.Lock()
	defer pt.mutex.Unlock()

	pt.animationFrame = (pt.animationFrame + 1) % 3
	pt.pendingRefresh = true // Mark for refresh, don't display directly
}

// startRefreshTicker starts a goroutine that handles both animation and display refresh
func (pt *progressTracker) startRefreshTicker(ctx context.Context) func() {
	stopChan := make(chan struct{})
	var stopOnce sync.Once

	go func() {
		animTicker := time.NewTicker(500 * time.Millisecond)
		refreshTicker := time.NewTicker(pt.refreshInterval)

		defer animTicker.Stop()
		defer refreshTicker.Stop()

		for {
			select {
			case <-ctx.Done():
				return
			case <-stopChan:
				return
			case <-animTicker.C:
				pt.updateAnimation()
			case <-refreshTicker.C:
				pt.refreshDisplayIfNeeded()
			}
		}
	}()

	return func() {
		stopOnce.Do(func() {
			close(stopChan)
		})
	}
}
