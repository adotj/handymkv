package hmkv

import (
	"fmt"
	"io"
	"net/http"
	"path/filepath"
	"strings"
	"time"
)

const ntfyRequestTimeout = 10 * time.Second

// SendNtfy posts a plain-text notification to an ntfy server topic.
func SendNtfy(server, topic, title, message string) error {
	topic = strings.TrimSpace(topic)
	if topic == "" {
		return nil
	}

	server = strings.TrimRight(strings.TrimSpace(server), "/")
	if server == "" {
		server = defaultNtfyServer
	}

	url := fmt.Sprintf("%s/%s", server, topic)

	req, err := http.NewRequest(http.MethodPost, url, strings.NewReader(message))
	if err != nil {
		return fmt.Errorf("creating ntfy request: %w", err)
	}

	req.Header.Set("Content-Type", "text/plain; charset=utf-8")
	if title != "" {
		req.Header.Set("Title", title)
	}

	client := &http.Client{Timeout: ntfyRequestTimeout}
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("sending ntfy notification: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		if len(body) > 0 {
			return fmt.Errorf("ntfy returned status %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
		}
		return fmt.Errorf("ntfy returned status %d", resp.StatusCode)
	}

	return nil
}

func maybeNotify(config *handyMKVConfig, title, message string) {
	if config == nil || strings.TrimSpace(config.NtfyTopic) == "" {
		return
	}

	server := config.NtfyServer
	if server == "" {
		server = defaultNtfyServer
	}

	if err := SendNtfy(server, config.NtfyTopic, title, message); err != nil {
		fmt.Printf("Warning: ntfy notification failed: %v\n", err)
	}
}

func notifyRipFailure(config *handyMKVConfig, err error) {
	if err == nil {
		return
	}
	maybeNotify(config, "HandyMKV — Action needed", fmt.Sprintf("Rip failed: %v", err))
}

func notifyRipSuccess(config *handyMKVConfig, libraryPaths []string, processTitles []TitleInfo, opts ExecOptions, tvMeta *tvSeriesMeta, duration time.Duration) {
	label := mediaLabelForNotification(libraryPaths, processTitles, opts, tvMeta)
	message := fmt.Sprintf("%s finished in %s. %d title(s) encoded.", label, formatTimeElapsedString(duration), len(processTitles))
	maybeNotify(config, "HandyMKV — Ready for next disc", message)
}

func mediaLabelForNotification(libraryPaths []string, processTitles []TitleInfo, opts ExecOptions, tvMeta *tvSeriesMeta) string {
	if opts.TVMode {
		if tvMeta != nil {
			return tvMeta.Label(len(processTitles))
		}
		meta := tvSeriesMeta{
			Series:       opts.MediaName,
			Year:         opts.MediaYear,
			Season:       opts.Season,
			StartEpisode: opts.StartEpisode,
		}
		if meta.Series != "" {
			return meta.Label(len(processTitles))
		}
		if len(libraryPaths) > 0 {
			// .../Series/Season XX/file.mkv → series folder is two levels up
			return filepath.Base(filepath.Dir(filepath.Dir(libraryPaths[0])))
		}
	}

	return movieLabelForNotification(libraryPaths, processTitles, opts.MediaName, opts.MediaYear)
}

func movieLabelForNotification(libraryPaths []string, processTitles []TitleInfo, movieName, movieYear string) string {
	movieName = strings.TrimSpace(movieName)
	movieYear = strings.TrimSpace(movieYear)

	if movieName != "" {
		if name, year, ok := parseMovieNameYear(movieName); ok {
			return name + " (" + year + ")"
		}
		if movieYear != "" {
			return movieName + " (" + movieYear + ")"
		}
		return movieName
	}

	if len(libraryPaths) > 0 {
		return filepath.Base(filepath.Dir(libraryPaths[0]))
	}

	if len(processTitles) > 0 {
		if name := bestNameFromTitle(processTitles[0]); name != "" {
			year := extractYear(processTitles[0].DiscTitle)
			if year == "" {
				year = extractYear(processTitles[0].FileName)
			}
			if year != "" {
				return name + " (" + year + ")"
			}
			return name
		}
	}

	return "Disc"
}
