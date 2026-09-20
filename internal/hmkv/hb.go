package hmkv

import (
	"bufio"
	"bytes"
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"os/user"
	"path/filepath"
	"regexp"
	"runtime"
	"strconv"
	"strings"
	"time"
)

func GetHandBrakeCLIExecutable() (string, error) {

	handBrakeCLIExecutableName := "HandBrakeCLI"

	if runtime.GOOS == "windows" {
		handBrakeCLIExecutableName = fmt.Sprintf("%s%s", handBrakeCLIExecutableName, ".exe")
	}

	_, err := exec.LookPath(handBrakeCLIExecutableName)

	if err == nil {
		return handBrakeCLIExecutableName, nil
	}

	// Check if the executable is in the homedir/.handymkv/bin
	user, err := user.Current()

	if err != nil {
		return "", fmt.Errorf("could not get current user: %w", err)
	}

	path := filepath.Join(user.HomeDir, "handymkv", "bin", handBrakeCLIExecutableName)

	// check if the file exists using stat
	_, err = os.Stat(path)

	if err == nil {
		return path, nil
	}

	return "", fmt.Errorf("handbrakecli executable not found")
}

type HandBrakeCLI struct {
	executable string
}

func NewHandBrakeCLI(executable string) *HandBrakeCLI {
	return &HandBrakeCLI{
		executable: executable,
	}
}

type EncodingParams struct {
	TitleIndex                  int      `json:"-"`
	DiscId                      int      `json:"-"`
	MKVOutputPath               string   `json:"-"`
	HandBrakeOutputPath         string   `json:"-"`
	RippedFileSizeBytes         int64         `json:"-"`
	EncodedFileSizeBytes        int64         `json:"-"`
	RippingDuration             string        `json:"-"`
	ProcessingDuration          time.Duration `json:"-"`
	CompletedAt                 time.Time     `json:"-"`
	LibraryKeptRaw              bool          `json:"-"`
	Encoder                     string   `json:"-"`
	EncoderPreset               string   `json:"-"`
	Quality                     int      `json:"-"`
	SubtitleLanguages           []string `json:"-"`
	IncludeAllRelevantSubtitles bool     `json:"-"`
	AudioLanguages              []string `json:"-"`
	IncludeAllRelevantAudio     bool     `json:"-"`
}

func defaultEncodingParams() EncodingParams {
	return EncodingParams{
		Encoder:                     "x264",
		EncoderPreset:               "slow",
		Quality:                     18,
		AudioLanguages:              []string{"eng", "spa"},
		IncludeAllRelevantAudio:     false,
		SubtitleLanguages:           []string{"eng", "spa"},
		IncludeAllRelevantSubtitles: false,
	}
}

func (hb *HandBrakeCLI) encode(ctx context.Context,
	params *EncodingParams,
	onProgress func(percent int, eta string, stage string)) error {
	var args []string = []string{
		"--input", params.MKVOutputPath,
		"--output", params.HandBrakeOutputPath,
	}

	args = append(args, "--encoder", params.Encoder)

	if params.EncoderPreset != "" {
		args = append(args, "--encoder-preset", params.EncoderPreset)
	}

	if params.Quality > 0 {
		args = append(args, "--quality", strconv.Itoa(params.Quality))
	}

	if len(params.SubtitleLanguages) > 0 {
		args = append(args, "--subtitle-lang-list", strings.Join(params.SubtitleLanguages, ","))

		if params.IncludeAllRelevantSubtitles {
			args = append(args, "--all-subtitles")
		}
	}

	if len(params.AudioLanguages) > 0 {
		args = append(args, "--audio-lang-list", strings.Join(params.AudioLanguages, ","))

		if params.IncludeAllRelevantAudio {
			args = append(args, "--all-audio")
		}
	}

	cmd := exec.CommandContext(ctx, hb.executable,
		args...,
	)

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return fmt.Errorf("failed to create stdout pipe: %w", err)
	}

	stderr, err := cmd.StderrPipe()
	if err != nil {
		return fmt.Errorf("failed to create stderr pipe: %w", err)
	}

	if err := cmd.Start(); err != nil {
		return fmt.Errorf("failed to start HandBrakeCLI: %w", err)
	}

	// Drain stdout so a full pipe cannot block HandBrakeCLI.
	go io.Copy(io.Discard, stdout)

	// HandBrakeCLI writes progress to stderr. Tee it so we can parse live
	// updates and still include the full stream in error reports.
	var stderrBuf bytes.Buffer
	progressReader := io.TeeReader(stderr, &stderrBuf)
	scanner := bufio.NewScanner(progressReader)
	scanner.Split(splitProgressLines)

	for scanner.Scan() {
		percent, eta, stage, ok := parseHandBrakeProgressLine(scanner.Text())
		if ok && onProgress != nil {
			onProgress(percent, eta, stage)
		}
	}

	if err := scanner.Err(); err != nil {
		return fmt.Errorf("error reading HandBrakeCLI output: %w", err)
	}

	if err := cmd.Wait(); err != nil {
		return NewExternalProcessError(
			fmt.Errorf("encoding failed for %s: %w", params.MKVOutputPath, err),
			fmt.Sprintf("HandBrakeCLI Error Output:\n%s", stderrBuf.String()),
		)
	}

	return nil
}

var (
	hbEncodingRe = regexp.MustCompile(`Encoding:.*?(\d+\.?\d*)\s*%`)
	hbETARe      = regexp.MustCompile(`\bETA\s+(\d+h\d+m\d+s)`)
	hbScanningRe = regexp.MustCompile(`(?i)scanning title`)
)

// parseHandBrakeProgressLine extracts percent, compact ETA, and stage from a
// HandBrakeCLI stderr progress line. ok is false when the line is unrelated.
func parseHandBrakeProgressLine(line string) (percent int, eta string, stage string, ok bool) {
	if hbScanningRe.MatchString(line) {
		return -1, "", "Scanning", true
	}

	matches := hbEncodingRe.FindStringSubmatch(line)
	if len(matches) < 2 {
		return -1, "", "", false
	}

	percent = -1
	if p, err := strconv.ParseFloat(matches[1], 64); err == nil {
		percent = int(p)
	}

	if em := hbETARe.FindStringSubmatch(line); len(em) > 1 {
		eta = compactETA(em[1])
	}

	return percent, eta, "", true
}

