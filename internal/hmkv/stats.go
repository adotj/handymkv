package hmkv

import (
	"encoding/csv"
	"fmt"
	"io"
	"math"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"time"
)

const ripHistoryFileName = "rip-history.csv"

var ripHistoryHeaders = []string{
	"timestamp",
	"title",
	"disc_name",
	"elapsed",
	"raw_gb",
	"encoded_gb",
	"saved_gb",
}

type ripStatsRow struct {
	Timestamp    time.Time
	Title        string
	DiscName     string
	Elapsed      time.Duration
	RawBytes     int64
	EncodedBytes int64
}

// statsDirOverride is set in tests only.
var statsDirOverride string

func defaultStatsDir() string {
	if statsDirOverride != "" {
		return statsDirOverride
	}
	if runtime.GOOS == "windows" {
		return `D:\handymkv\stats`
	}

	homeDir, err := os.UserHomeDir()
	if err != nil {
		return "handymkv/stats"
	}
	return filepath.Join(homeDir, "handymkv", "stats")
}

func getStatsFilePath() (string, error) {
	return filepath.Join(defaultStatsDir(), ripHistoryFileName), nil
}

func bytesToGB(bytes int64) float64 {
	const gb = 1024 * 1024 * 1024
	return math.Round(float64(bytes)/float64(gb)*1000) / 1000
}

func buildRipStatsRows(entries []EncodingParams, titles []TitleInfo, libraryPaths []string) []ripStatsRow {
	titleByKey := make(map[string]TitleInfo, len(titles))
	for _, title := range titles {
		titleByKey[titleKey(title.DiscId, title.Index)] = title
	}

	rows := make([]ripStatsRow, 0, len(entries))
	for i, entry := range entries {
		title, ok := titleByKey[titleKey(entry.DiscId, entry.TitleIndex)]
		if !ok {
			continue
		}

		movieTitle := cleanMakeMKVTitleName(title.FileName)
		if i < len(libraryPaths) && libraryPaths[i] != "" {
			movieTitle = libraryMovieFileTitle(libraryPaths[i])
		}
		if movieTitle == "" {
			movieTitle = title.FileName
		}

		completedAt := entry.CompletedAt
		if completedAt.IsZero() {
			completedAt = time.Now().UTC()
		}

		rows = append(rows, ripStatsRow{
			Timestamp:    completedAt,
			Title:        movieTitle,
			DiscName:     title.DiscTitle,
			Elapsed:      entry.ProcessingDuration,
			RawBytes:     entry.RippedFileSizeBytes,
			EncodedBytes: entry.EncodedFileSizeBytes,
		})
	}

	return rows
}

func appendRipHistory(entries []EncodingParams, titles []TitleInfo, libraryPaths []string) error {
	rows := buildRipStatsRows(entries, titles, libraryPaths)
	if len(rows) == 0 {
		return nil
	}

	path, err := getStatsFilePath()
	if err != nil {
		return err
	}

	if err := os.MkdirAll(filepath.Dir(path), 0740); err != nil {
		return fmt.Errorf("could not create stats directory: %w", err)
	}

	file, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0640)
	if err != nil {
		return fmt.Errorf("could not open rip history file: %w", err)
	}
	defer file.Close()

	info, err := file.Stat()
	if err != nil {
		return fmt.Errorf("could not stat rip history file: %w", err)
	}

	writer := csv.NewWriter(file)
	if info.Size() == 0 {
		if err := writer.Write(ripHistoryHeaders); err != nil {
			return fmt.Errorf("could not write rip history header: %w", err)
		}
	}

	for _, row := range rows {
		savedBytes := row.RawBytes - row.EncodedBytes
		if savedBytes < 0 {
			savedBytes = 0
		}

		record := []string{
			row.Timestamp.Local().Format("2006-01-02 15:04:05"),
			row.Title,
			row.DiscName,
			formatTimeElapsedString(row.Elapsed),
			formatGB(row.RawBytes),
			formatGB(row.EncodedBytes),
			formatGB(savedBytes),
		}

		if err := writer.Write(record); err != nil {
			return fmt.Errorf("could not write rip history row: %w", err)
		}
	}

	writer.Flush()
	if err := writer.Error(); err != nil {
		return fmt.Errorf("could not flush rip history: %w", err)
	}

	return nil
}

func formatGB(bytes int64) string {
	return strconv.FormatFloat(bytesToGB(bytes), 'f', 3, 64)
}

func parseElapsedCSV(s string) (time.Duration, error) {
	if d, err := time.ParseDuration(s); err == nil {
		return d, nil
	}

	var minutes, seconds int
	if _, err := fmt.Sscanf(s, "%dm%ds", &minutes, &seconds); err != nil {
		return 0, err
	}
	return time.Duration(minutes)*time.Minute + time.Duration(seconds)*time.Second, nil
}

func gbToBytes(gb float64) int64 {
	const unit = 1024 * 1024 * 1024
	return int64(math.Round(gb * float64(unit)))
}

func readRipHistoryFromReader(r io.Reader) ([]ripStatsRow, error) {
	reader := csv.NewReader(r)
	reader.FieldsPerRecord = -1

	allRows, err := reader.ReadAll()
	if err != nil {
		return nil, err
	}
	if len(allRows) <= 1 {
		return nil, nil
	}

	var rows []ripStatsRow
	for _, record := range allRows[1:] {
		if len(record) < 7 {
			continue
		}

		ts, err := time.ParseInLocation("2006-01-02 15:04:05", record[0], time.Local)
		if err != nil {
			continue
		}

		elapsed, err := parseElapsedCSV(record[3])
		if err != nil {
			continue
		}

		rawGB, err1 := strconv.ParseFloat(record[4], 64)
		encodedGB, err2 := strconv.ParseFloat(record[5], 64)
		if err1 != nil || err2 != nil {
			continue
		}

		rows = append(rows, ripStatsRow{
			Timestamp:    ts,
			Title:        record[1],
			DiscName:     record[2],
			Elapsed:      elapsed,
			RawBytes:     gbToBytes(rawGB),
			EncodedBytes: gbToBytes(encodedGB),
		})
	}

	return rows, nil
}

func printCatalogSummary() {
	path, err := getStatsFilePath()
	if err != nil {
		return
	}

	file, err := os.Open(path)
	if err != nil {
		return
	}
	defer file.Close()

	rows, err := readRipHistoryFromReader(file)
	if err != nil || len(rows) == 0 {
		return
	}

	var totalSaved int64
	var totalElapsed time.Duration
	for _, row := range rows {
		saved := row.RawBytes - row.EncodedBytes
		if saved > 0 {
			totalSaved += saved
		}
		totalElapsed += row.Elapsed
	}

	avgElapsed := totalElapsed / time.Duration(len(rows))

	fmt.Printf("\nCatalog totals (%d titles): %s saved, avg %s per title\n",
		len(rows),
		formatSavedSpace(totalSaved),
		formatTimeElapsedString(avgElapsed),
	)
	fmt.Printf("Rip history: %s\n", path)
}
