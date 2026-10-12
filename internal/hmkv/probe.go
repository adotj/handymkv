package hmkv

import (
	"encoding/json"
	"fmt"
	"math"
	"os/exec"
	"strings"
)

// mediaFileProbe holds ffprobe-derived metadata used to verify encodes.
type mediaFileProbe struct {
	DurationSec  float64
	AudioStreams int
}

// mediaFileProbeHook, when non-nil, replaces ffprobe for tests.
var mediaFileProbeHook func(path string) (mediaFileProbe, error)

type ffprobeJSON struct {
	Streams []struct {
		CodecType string `json:"codec_type"`
	} `json:"streams"`
	Format struct {
		Duration string `json:"duration"`
	} `json:"format"`
}

func probeMediaFile(path string) (mediaFileProbe, error) {
	if mediaFileProbeHook != nil {
		return mediaFileProbeHook(path)
	}

	out, err := exec.Command(
		"ffprobe",
		"-v", "error",
		"-show_entries", "format=duration",
		"-show_entries", "stream=codec_type",
		"-of", "json",
		path,
	).Output()
	if err != nil {
		return mediaFileProbe{}, fmt.Errorf("ffprobe %s: %w", path, err)
	}

	var parsed ffprobeJSON
	if err := json.Unmarshal(out, &parsed); err != nil {
		return mediaFileProbe{}, fmt.Errorf("parse ffprobe output for %s: %w", path, err)
	}

	var dur float64
	if parsed.Format.Duration != "" {
		if _, err := fmt.Sscanf(parsed.Format.Duration, "%f", &dur); err != nil {
			return mediaFileProbe{}, fmt.Errorf("parse duration for %s: %w", path, err)
		}
	}

	audio := 0
	for _, s := range parsed.Streams {
		if strings.EqualFold(s.CodecType, "audio") {
			audio++
		}
	}

	return mediaFileProbe{
		DurationSec:  dur,
		AudioStreams: audio,
	}, nil
}

func durationWithinEncodeTolerance(sourceSec, encodeSec float64) bool {
	if sourceSec <= 0 || encodeSec <= 0 {
		return false
	}
	diff := math.Abs(sourceSec - encodeSec)
	tol := math.Max(3.0, 0.005*sourceSec)
	return diff <= tol
}

// encodeVerification summarizes whether an encode may be treated as a faithful
// transcode of the raw rip for library keep/delete decisions.
type encodeVerification struct {
	Verified bool
	Reason   string
}

func verifyEncodeForLibrary(entry EncodingParams) encodeVerification {
	if entry.HandBrakeExitCode != 0 {
		return encodeVerification{Verified: false, Reason: "HandBrake did not exit 0"}
	}
	if entry.MKVOutputPath == "" || entry.HandBrakeOutputPath == "" {
		return encodeVerification{Verified: false, Reason: "missing raw or encoded path"}
	}

	rawProbe, err := probeMediaFile(entry.MKVOutputPath)
	if err != nil {
		return encodeVerification{Verified: false, Reason: err.Error()}
	}
	encProbe, err := probeMediaFile(entry.HandBrakeOutputPath)
	if err != nil {
		return encodeVerification{Verified: false, Reason: err.Error()}
	}

	if !durationWithinEncodeTolerance(rawProbe.DurationSec, encProbe.DurationSec) {
		return encodeVerification{
			Verified: false,
			Reason:   fmt.Sprintf("duration mismatch (raw %.3fs vs encode %.3fs)", rawProbe.DurationSec, encProbe.DurationSec),
		}
	}
	if rawProbe.AudioStreams != encProbe.AudioStreams {
		return encodeVerification{
			Verified: false,
			Reason:   fmt.Sprintf("audio stream count mismatch (raw %d vs encode %d)", rawProbe.AudioStreams, encProbe.AudioStreams),
		}
	}

	return encodeVerification{Verified: true}
}
