package hmkv

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"strconv"
	"strings"
)

var (
	makeMKVTitleSuffixRe = regexp.MustCompile(`_t\d+$`)
	makeMKVDiscSuffixRe  = regexp.MustCompile(`-[A-Za-z]\d+$`)
	yearInParensRe       = regexp.MustCompile(`\((19|20)\d{2}\)`)
	yearTokenRe          = regexp.MustCompile(`(?:^|[\s_\-])((19|20)\d{2})(?:[\s_\-]|$)`)
)

type movieLibraryName struct {
	Name string
	Year string
}

func (m movieLibraryName) FolderName() string {
	if m.Year != "" {
		return fmt.Sprintf("%s (%s)", m.Name, m.Year)
	}
	return m.Name
}

func defaultLibraryRoot() string {
	if runtime.GOOS == "windows" {
		return `D:\movies`
	}

	usr, err := os.UserHomeDir()
	if err != nil {
		return "movies"
	}

	return filepath.Join(usr, "movies")
}

func defaultTVLibraryRoot() string {
	if runtime.GOOS == "windows" {
		return `D:\tv`
	}

	usr, err := os.UserHomeDir()
	if err != nil {
		return "shows"
	}

	return filepath.Join(usr, "shows")
}

func applyLibraryConfigDefaults(config *handyMKVConfig) {
	if config.LibraryRoot == "" {
		config.LibraryRoot = defaultLibraryRoot()
	}
	if config.TVLibraryRoot == "" {
		config.TVLibraryRoot = defaultTVLibraryRoot()
	}
}

// tvLibraryTarget describes a Jellyfin-style TV episode destination.
type tvLibraryTarget struct {
	Series  string
	Year    string
	Season  int
	Episode int
}

func (t tvLibraryTarget) SeriesFolderName() string {
	if t.Year != "" {
		return fmt.Sprintf("%s (%s)", t.Series, t.Year)
	}
	return t.Series
}

func (t tvLibraryTarget) SeasonFolderName() string {
	return fmt.Sprintf("Season %02d", t.Season)
}

func (t tvLibraryTarget) EpisodeCode() string {
	return fmt.Sprintf("S%02dE%02d", t.Season, t.Episode)
}

func (t tvLibraryTarget) EpisodeFileName(ext string) string {
	return fmt.Sprintf("%s - %s%s", t.SeriesFolderName(), t.EpisodeCode(), ext)
}

func (t tvLibraryTarget) DestPath(libraryRoot, ext string) string {
	return filepath.Join(libraryRoot, t.SeriesFolderName(), t.SeasonFolderName(), t.EpisodeFileName(ext))
}

// tvSeriesMeta is series-level naming shared by all episodes in a TV rip.
type tvSeriesMeta struct {
	Series       string
	Year         string
	Season       int
	StartEpisode int
}

func (m tvSeriesMeta) Label(episodeCount int) string {
	folder := tvLibraryTarget{Series: m.Series, Year: m.Year}.SeriesFolderName()
	if episodeCount <= 1 {
		return fmt.Sprintf("%s %s", folder, tvLibraryTarget{Season: m.Season, Episode: m.StartEpisode}.EpisodeCode())
	}
	end := m.StartEpisode + episodeCount - 1
	return fmt.Sprintf("%s S%02dE%02d–E%02d", folder, m.Season, m.StartEpisode, end)
}

func cleanMakeMKVTitleName(raw string) string {
	base := strings.TrimSuffix(raw, filepath.Ext(raw))
	base = makeMKVTitleSuffixRe.ReplaceAllString(base, "")
	base = makeMKVDiscSuffixRe.ReplaceAllString(base, "")
	base = strings.ReplaceAll(base, "_", " ")
	base = strings.ReplaceAll(base, "-", " ")
	base = strings.Join(strings.Fields(base), " ")
	if base == "" {
		return ""
	}
	return titleCaseMovieName(base)
}

func titleCaseMovieName(name string) string {
	words := strings.Fields(name)
	if len(words) == 0 {
		return ""
	}

	smallWords := map[string]struct{}{
		"a": {}, "an": {}, "the": {}, "and": {}, "or": {}, "of": {},
		"in": {}, "on": {}, "at": {}, "to": {}, "for": {}, "vs": {},
	}

	for i, word := range words {
		upper := strings.ToUpper(word)
		lower := strings.ToLower(word)

		if isRomanNumeral(upper) {
			words[i] = upper
			continue
		}

		if i > 0 {
			if _, ok := smallWords[lower]; ok {
				words[i] = lower
				continue
			}
		}

		if len(lower) == 1 {
			words[i] = upper
			continue
		}

		words[i] = strings.ToUpper(lower[:1]) + lower[1:]
	}

	return strings.Join(words, " ")
}

func isRomanNumeral(word string) bool {
	if word == "" {
		return false
	}

	for _, r := range word {
		switch r {
		case 'I', 'V', 'X', 'L', 'C', 'D', 'M':
			continue
		default:
			return false
		}
	}

	return true
}

func extractYear(raw string) string {
	if match := yearInParensRe.FindStringSubmatch(raw); len(match) > 0 {
		return match[0][1 : len(match[0])-1]
	}

	if match := yearTokenRe.FindStringSubmatch(raw); len(match) > 1 {
		return match[1]
	}

	return ""
}

func parseMovieNameYear(input string) (name, year string, ok bool) {
	input = strings.TrimSpace(input)
	if input == "" {
		return "", "", false
	}

	if match := yearInParensRe.FindStringSubmatchIndex(input); match != nil {
		year = input[match[0]+1 : match[1]-1]
		name = strings.TrimSpace(input[:match[0]] + input[match[1]:])
		name = strings.TrimSpace(strings.Trim(name, "-"))
		if name != "" && isValidYear(year) {
			return name, year, true
		}
	}

	return "", "", false
}

func isValidYear(year string) bool {
	if len(year) != 4 {
		return false
	}

	for _, r := range year {
		if r < '0' || r > '9' {
			return false
		}
	}

	return year[0] == '1' || year[0] == '2'
}

// ValidateMovieYear reports whether year is a four-digit release year (19xx or 20xx).
func ValidateMovieYear(year string) error {
	if !isValidYear(strings.TrimSpace(year)) {
		return fmt.Errorf("expected four-digit year (19xx or 20xx)")
	}
	return nil
}

// ParseMovieNameYear parses a Jellyfin-style "Movie Name (Year)" string.
func ParseMovieNameYear(input string) (name, year string, ok bool) {
	return parseMovieNameYear(input)
}

func bestNameFromTitle(title TitleInfo) string {
	nameCandidates := []string{
		cleanMakeMKVTitleName(title.FileName),
		cleanMakeMKVTitleName(title.DiscTitle),
	}

	var bestName string
	for _, candidate := range nameCandidates {
		if len(candidate) > len(bestName) {
			bestName = candidate
		}
	}
	return bestName
}

func resolveMovieLibraryName(title TitleInfo, flagName string, flagYear string, libraryRoot string) (movieLibraryName, error) {
	flagName = strings.TrimSpace(flagName)
	flagYear = strings.TrimSpace(flagYear)

	if flagName != "" {
		name, year, ok := parseMovieNameYear(flagName)
		if ok {
			if flagYear != "" && flagYear != year {
				return movieLibraryName{}, fmt.Errorf("conflicting year: -n specifies %s but -y specifies %s", year, flagYear)
			}
			return movieLibraryName{Name: name, Year: year}, nil
		}
		if flagYear != "" {
			if !isValidYear(flagYear) {
				return movieLibraryName{}, fmt.Errorf("invalid release year %q", flagYear)
			}
			return movieLibraryName{Name: flagName, Year: flagYear}, nil
		}
		return movieLibraryName{}, fmt.Errorf("invalid movie name %q, expected format: Movie Name (Year) or use -y", flagName)
	}

	if flagYear != "" {
		if !isValidYear(flagYear) {
			return movieLibraryName{}, fmt.Errorf("invalid release year %q", flagYear)
		}
		bestName := bestNameFromTitle(title)
		if bestName == "" {
			return movieLibraryName{}, fmt.Errorf("could not determine movie name from disc metadata; use -n with -y")
		}
		return movieLibraryName{Name: bestName, Year: flagYear}, nil
	}

	bestName := bestNameFromTitle(title)

	year := extractYear(title.DiscTitle)
	if year == "" {
		year = extractYear(title.FileName)
	}

	if bestName == "" {
		fmt.Println("\nCould not determine a movie name from disc metadata.")
		input := promptForString(
			"Enter the full movie name and year",
			"Example: The Shining (1980)",
			"",
			nil,
		)
		name, parsedYear, ok := parseMovieNameYear(input)
		if !ok {
			return movieLibraryName{}, fmt.Errorf("invalid movie name %q, expected format: Movie Name (Year)", input)
		}
		return movieLibraryName{Name: name, Year: parsedYear}, nil
	}

	if year != "" {
		folderName := fmt.Sprintf("%s (%s)", bestName, year)
		destPath := filepath.Join(libraryRoot, folderName, folderName+filepath.Ext(title.FileName))
		fmt.Printf("\nOrganize encoded file as:\n  %s\n", destPath)
		if promptForBool("Accept?", "", true) {
			return movieLibraryName{Name: bestName, Year: year}, nil
		}
	}

	fmt.Printf("\nSuggested movie name: %s\n", bestName)
	confirmedName := promptForString("Movie name", "Press Enter to accept the suggestion", bestName, nil)
	if confirmedName == "" {
		confirmedName = bestName
	}

	yearPrompt := "Four digit release year"
	if year != "" {
		yearPrompt = fmt.Sprintf("Press Enter to keep %s", year)
	}

	confirmedYear := promptForString("Year", yearPrompt, year, nil)
	if confirmedYear == "" {
		confirmedYear = year
	}

	if !isValidYear(confirmedYear) {
		input := promptForString(
			"Enter the full movie name and year",
			"Example: The Shining (1980)",
			"",
			nil,
		)
		name, parsedYear, ok := parseMovieNameYear(input)
		if !ok {
			return movieLibraryName{}, fmt.Errorf("invalid movie name %q, expected format: Movie Name (Year)", input)
		}
		return movieLibraryName{Name: name, Year: parsedYear}, nil
	}

	folderName := fmt.Sprintf("%s (%s)", confirmedName, confirmedYear)
	destPath := filepath.Join(libraryRoot, folderName, folderName+filepath.Ext(title.FileName))
	fmt.Printf("\nOrganize encoded file as:\n  %s\n", destPath)
	if !promptForBool("Accept?", "", true) {
		input := promptForString(
			"Enter the full movie name and year",
			"Example: The Shining (1980)",
			"",
			nil,
		)
		name, parsedYear, ok := parseMovieNameYear(input)
		if !ok {
			return movieLibraryName{}, fmt.Errorf("invalid movie name %q, expected format: Movie Name (Year)", input)
		}
		return movieLibraryName{Name: name, Year: parsedYear}, nil
	}

	return movieLibraryName{Name: confirmedName, Year: confirmedYear}, nil
}

func organizeEncodedFilesToLibrary(entries []EncodingParams, titles []TitleInfo, config *handyMKVConfig, opts ExecOptions, tvMeta *tvSeriesMeta) ([]string, error) {
	applyLibraryConfigDefaults(config)

	if opts.TVMode {
		if tvMeta == nil {
			return nil, fmt.Errorf("TV series metadata is required for TV library organization")
		}
		return organizeTVEpisodesToLibrary(entries, titles, config, *tvMeta)
	}

	return organizeMovieEntriesToLibrary(entries, titles, config, opts)
}

func organizeTVEpisodesToLibrary(entries []EncodingParams, titles []TitleInfo, config *handyMKVConfig, meta tvSeriesMeta) ([]string, error) {
	entryByKey := make(map[string]EncodingParams, len(entries))
	for _, entry := range entries {
		entryByKey[titleKey(entry.DiscId, entry.TitleIndex)] = entry
	}

	finalPaths := make([]string, 0, len(titles))
	for i, title := range titles {
		entry, ok := entryByKey[titleKey(title.DiscId, title.Index)]
		if !ok {
			return finalPaths, fmt.Errorf("could not find encoded file for disc %d title %d", title.DiscId, title.Index)
		}

		ext := filepath.Ext(entry.HandBrakeOutputPath)
		target := tvLibraryTarget{
			Series:  meta.Series,
			Year:    meta.Year,
			Season:  meta.Season,
			Episode: meta.StartEpisode + i,
		}
		destPath := target.DestPath(config.TVLibraryRoot, ext)
		destDir := filepath.Dir(destPath)

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

// resolveTVSeriesMeta builds series naming from CLI flags and/or interactive prompts.
func resolveTVSeriesMeta(titles []TitleInfo, opts ExecOptions, tvLibraryRoot string) (*tvSeriesMeta, error) {
	if err := ValidateStartEpisode(opts.StartEpisode); err != nil {
		return nil, err
	}

	series, year, err := resolveSeriesNameYear(titles, opts.MediaName, opts.MediaYear)
	if err != nil {
		return nil, err
	}

	season := opts.Season
	if season < 0 {
		seasonStr := promptForString(
			"Season number",
			"Use 0 for specials (Season 00)",
			"1",
			nil,
		)
		season, err = strconv.Atoi(strings.TrimSpace(seasonStr))
		if err != nil {
			return nil, fmt.Errorf("invalid season number %q", seasonStr)
		}
	}
	if err := ValidateSeason(season); err != nil {
		return nil, err
	}

	startEpisode := opts.StartEpisode
	if opts.Season < 0 && opts.MediaName == "" {
		epStr := promptForString(
			"Starting episode number",
			fmt.Sprintf("Press Enter to start at %d", startEpisode),
			strconv.Itoa(startEpisode),
			nil,
		)
		if strings.TrimSpace(epStr) != "" {
			startEpisode, err = strconv.Atoi(strings.TrimSpace(epStr))
			if err != nil {
				return nil, fmt.Errorf("invalid starting episode %q", epStr)
			}
		}
		if err := ValidateStartEpisode(startEpisode); err != nil {
			return nil, err
		}
	}

	meta := &tvSeriesMeta{
		Series:       series,
		Year:         year,
		Season:       season,
		StartEpisode: startEpisode,
	}

	fmt.Printf("\nEpisode mapping preview:\n")
	for i, title := range titles {
		target := tvLibraryTarget{
			Series:  meta.Series,
			Year:    meta.Year,
			Season:  meta.Season,
			Episode: meta.StartEpisode + i,
		}
		fmt.Printf("  ID %d (%s) → %s\n", title.Index, title.FileName, target.EpisodeCode())
	}

	example := tvLibraryTarget{
		Series:  meta.Series,
		Year:    meta.Year,
		Season:  meta.Season,
		Episode: meta.StartEpisode,
	}
	fmt.Printf("\nOrganize under:\n  %s\n", example.DestPath(tvLibraryRoot, ".mkv"))

	if opts.MediaName == "" || opts.Season < 0 {
		if !promptForBool("Accept?", "", true) {
			return nil, fmt.Errorf("TV library organization cancelled")
		}
	}

	return meta, nil
}

func resolveSeriesNameYear(titles []TitleInfo, flagName, flagYear string) (string, string, error) {
	flagName = strings.TrimSpace(flagName)
	flagYear = strings.TrimSpace(flagYear)

	if flagName != "" {
		name, year, ok := parseMovieNameYear(flagName)
		if ok {
			if flagYear != "" && flagYear != year {
				return "", "", fmt.Errorf("conflicting year: -n specifies %s but -y specifies %s", year, flagYear)
			}
			return name, year, nil
		}
		if flagYear != "" {
			if !isValidYear(flagYear) {
				return "", "", fmt.Errorf("invalid release year %q", flagYear)
			}
			return flagName, flagYear, nil
		}
		// Series year is optional for Jellyfin; allow name-only via -n without -y.
		return flagName, "", nil
	}

	if flagYear != "" && !isValidYear(flagYear) {
		return "", "", fmt.Errorf("invalid release year %q", flagYear)
	}

	var bestName string
	if len(titles) > 0 {
		bestName = bestNameFromTitle(titles[0])
	}
	year := flagYear
	if year == "" && len(titles) > 0 {
		year = extractYear(titles[0].DiscTitle)
		if year == "" {
			year = extractYear(titles[0].FileName)
		}
	}

	if bestName == "" {
		fmt.Println("\nCould not determine a series name from disc metadata.")
		input := promptForString(
			"Enter the series name and year",
			"Example: Star Trek (1966)",
			"",
			nil,
		)
		name, parsedYear, ok := parseMovieNameYear(input)
		if ok {
			return name, parsedYear, nil
		}
		if strings.TrimSpace(input) == "" {
			return "", "", fmt.Errorf("series name is required")
		}
		return strings.TrimSpace(input), year, nil
	}

	fmt.Printf("\nSuggested series name: %s\n", bestName)
	confirmedName := promptForString("Series name", "Press Enter to accept the suggestion", bestName, nil)
	if confirmedName == "" {
		confirmedName = bestName
	}

	yearPrompt := "Four digit premiere year (optional)"
	if year != "" {
		yearPrompt = fmt.Sprintf("Press Enter to keep %s, or clear for no year", year)
	}
	confirmedYear := promptForString("Year", yearPrompt, year, nil)
	if confirmedYear != "" && !isValidYear(confirmedYear) {
		return "", "", fmt.Errorf("invalid release year %q", confirmedYear)
	}

	return confirmedName, confirmedYear, nil
}

// ValidateSeason reports whether season is a non-negative season number (0 = specials).
func ValidateSeason(season int) error {
	if season < 0 {
		return fmt.Errorf("season must be >= 0")
	}
	return nil
}

// ValidateStartEpisode reports whether episode is a positive episode number.
func ValidateStartEpisode(episode int) error {
	if episode < 1 {
		return fmt.Errorf("starting episode must be >= 1")
	}
	return nil
}

// ValidateTitleMode reports whether titleMode is empty, longest, all, or comma-separated IDs.
func ValidateTitleMode(titleMode string) error {
	raw := strings.ReplaceAll(strings.TrimSpace(titleMode), " ", "")
	if raw == "" || strings.EqualFold(raw, "longest") || strings.EqualFold(raw, "all") {
		return nil
	}

	for _, idStr := range strings.Split(raw, ",") {
		if idStr == "" {
			continue
		}
		id, err := strconv.Atoi(idStr)
		if err != nil || id < 0 {
			return fmt.Errorf("use 'longest', 'all', or comma-separated title IDs")
		}
	}
	return nil
}

func titleKey(discId, titleIndex int) string {
	return fmt.Sprintf("%d:%d", discId, titleIndex)
}

// libraryMovieFolderName returns the Jellyfin movie folder name from a library file path.
// Windows paths are normalized so parsing works on every GOOS (tests use D:\... on Linux).
func normalizeLibraryPath(libraryFilePath string) string {
	return strings.ReplaceAll(libraryFilePath, `\`, `/`)
}

func libraryMovieFolderName(libraryFilePath string) string {
	p := normalizeLibraryPath(libraryFilePath)
	dir := p
	if idx := strings.LastIndex(p, "/"); idx >= 0 {
		dir = p[:idx]
	}
	if idx := strings.LastIndex(dir, "/"); idx >= 0 {
		return dir[idx+1:]
	}
	return dir
}

func libraryMovieFileTitle(libraryFilePath string) string {
	p := normalizeLibraryPath(libraryFilePath)
	p = strings.TrimSuffix(p, filepath.Ext(p))
	if idx := strings.LastIndex(p, "/"); idx >= 0 {
		return p[idx+1:]
	}
	return p
}

func moveFile(src, dest string) error {
	err := os.Rename(src, dest)
	if err == nil {
		return nil
	}

	return copyFile(src, dest)
}

func copyFile(src, dest string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}

	out, err := os.OpenFile(dest, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0644)
	if err != nil {
		in.Close()
		return err
	}

	if _, err := io.Copy(out, in); err != nil {
		out.Close()
		in.Close()
		os.Remove(dest)
		return err
	}

	if err := out.Close(); err != nil {
		in.Close()
		os.Remove(dest)
		return err
	}

	if err := in.Close(); err != nil {
		os.Remove(dest)
		return err
	}

	return os.Remove(src)
}
