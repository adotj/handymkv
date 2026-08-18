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
	RippedFileSizeBytes         int64    `json:"-"`
	EncodedFileSizeBytes        int64    `json:"-"`
	RippingDuration             string   `json:"-"`
	Encoder                     string   `json:"encoder,omitempty"`
	EncoderPreset               string   `json:"encoder_preset,omitempty"`
	Quality                     int      `json:"quality,omitempty"`
	SubtitleLanguages           []string `json:"subtitle_languages,omitempty"`
	IncludeAllRelevantSubtitles bool     `json:"include_all_relevant_subtitles,omitempty"`
	AudioLanguages              []string `json:"audio_languages,omitempty"`
	IncludeAllRelevantAudio     bool     `json:"include_all_relevant_audio,omitempty"`
	OutputFileFormat            string   `json:"output_file_format,omitempty"`
	Preset                      string   `json:"handbrake_preset,omitempty"`
	PresetFile                  string   `json:"preset_file,omitempty"`
}

type HandBrakePresetFile struct {
	PresetList []HandBrakePreset `json:"PresetList"`
}

type HandBrakePreset struct {
	PresetName string `json:"PresetName"`
	FileFormat string `json:"FileFormat"`
}

func (hb *HandBrakeCLI) encode(ctx context.Context,
	params *EncodingParams,
	onProgress func(percent int, eta string, stage string)) error {
	var args []string = []string{
		"--input", params.MKVOutputPath,
		"--output", params.HandBrakeOutputPath,
	}

	if params.Preset != "" {
		if params.PresetFile != "" {
			args = append(args, "--preset-import-file", params.PresetFile)
		}

		args = append(args, "--preset", params.Preset)
	} else {
		args = append(args, "--encoder", params.Encoder)

		if params.EncoderPreset != "" {
			args = append(args, "--encoder-preset", params.EncoderPreset)
		} else {
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

func (hb *HandBrakeCLI) getPossiblePresets() ([]string, error) {
	var presets []string

	cmd := exec.Command(hb.executable, "--preset-list")

	output, err := cmd.CombinedOutput()

	if err != nil {
		return presets, fmt.Errorf("handbrakecli failure: %w", err)
	}

	scanner := bufio.NewScanner(strings.NewReader(string(output)))

	for scanner.Scan() {
		line := scanner.Text()

		if strings.HasPrefix(line, "    ") && !strings.HasPrefix(line, "        ") {
			presets = append(presets, strings.TrimSpace(line))
		}
	}

	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("error reading command output: %w", err)
	}

	return presets, nil
}

// Calls HandBrakeCLI --help and parses the output to get a list of possible encoders
func (hb *HandBrakeCLI) getPossibleEncoders() ([]string, error) {
	var encoders []string

	cmd := exec.Command(hb.executable, "--help")

	output, err := cmd.Output()

	if err != nil {
		return encoders, fmt.Errorf("handbrakecli failure: %w", err)
	}

	scanner := bufio.NewScanner(strings.NewReader(string(output)))
	inEncoderSection := false

	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())

		if strings.Contains(line, "Select video encoder:") {
			inEncoderSection = true
			continue
		}

		if inEncoderSection {
			if line == "" || strings.HasPrefix(line, "--") {
				break
			}
			encoders = append(encoders, line)
		}
	}

	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("error reading command output: %w", err)
	}

	return encoders, nil
}

// Calls HandBrakeCLI --encoder-preset-list and parses the output to get a list of possible quality presets for a given encoder
func (hb *HandBrakeCLI) getPossibleEncoderPresets(encoder string) ([]string, error) {
	var qualityPresets []string

	cmd := exec.Command(hb.executable, "--encoder-preset-list", encoder)

	output, err := cmd.CombinedOutput()

	if err != nil {
		return qualityPresets, fmt.Errorf("handbrakecli failure: %w", err)
	}

	scanner := bufio.NewScanner(strings.NewReader(string(output)))

	for scanner.Scan() {
		line := scanner.Text()

		if strings.HasPrefix(line, "    ") && !strings.HasPrefix(line, "        ") {
			qualityPresets = append(qualityPresets, strings.TrimSpace(scanner.Text()))
		}
	}

	return qualityPresets, nil
}
