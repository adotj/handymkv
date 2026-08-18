package hmkv

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
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

func applyLibraryConfigDefaults(config *handyMKVConfig) {
	if config.LibraryRoot == "" {
		config.LibraryRoot = defaultLibraryRoot()
	}
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

func resolveMovieLibraryName(title TitleInfo, flagName string, libraryRoot string) (movieLibraryName, error) {
	if flagName != "" {
		name, year, ok := parseMovieNameYear(flagName)
		if !ok {
			return movieLibraryName{}, fmt.Errorf("invalid movie name %q, expected format: Movie Name (Year)", flagName)
		}
		return movieLibraryName{Name: name, Year: year}, nil
	}

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

func organizeEncodedFilesToLibrary(entries []EncodingParams, titles []TitleInfo, config *handyMKVConfig, flagMovieName string) ([]string, error) {
	if !config.OrganizeToLibrary {
		return nil, nil
	}

	applyLibraryConfigDefaults(config)

	titleByKey := make(map[string]TitleInfo, len(titles))
	for _, title := range titles {
		titleByKey[titleKey(title.DiscId, title.Index)] = title
	}

	finalPaths := make([]string, 0, len(entries))
	useFlagForSingleTitle := flagMovieName != "" && len(entries) == 1

	for _, entry := range entries {
		title, ok := titleByKey[titleKey(entry.DiscId, entry.TitleIndex)]
		if !ok {
			return finalPaths, fmt.Errorf("could not find title metadata for disc %d title %d", entry.DiscId, entry.TitleIndex)
		}

		nameFlag := ""
		if useFlagForSingleTitle {
			nameFlag = flagMovieName
		}

		libName, err := resolveMovieLibraryName(title, nameFlag, config.LibraryRoot)
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

func titleKey(discId, titleIndex int) string {
	return fmt.Sprintf("%d:%d", discId, titleIndex)
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
	defer in.Close()

	out, err := os.OpenFile(dest, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0644)
	if err != nil {
		return err
	}

	if _, err := io.Copy(out, in); err != nil {
		out.Close()
		os.Remove(dest)
		return err
	}

	if err := out.Close(); err != nil {
		os.Remove(dest)
		return err
	}

	return os.Remove(src)
}
