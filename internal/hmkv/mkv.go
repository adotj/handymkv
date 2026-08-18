package hmkv

import (
	"bufio"
	"bytes"
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"
	"strconv"
	"strings"
)

type TitleInfo struct {
	// Index on disc
	Index int
	// The descriptive title of the disc.
	DiscTitle string
	// The id of the disc that the title came from.
	DiscId int
	// The number of chapters that the title is comprised from.
	Chapters int
	// The duration of the title.
	Length string
	// The file size represented in MB or GB. For presentation to the user. Ex - 13.4 GB
	FileSizeDesc string
	// The true file size in bytes.
	FileSizeBytes int
	// The output file name.
	FileName string
	// Whether or not to prepend the disc number.
	PrependDiscToSub bool
}

func GetMakeMKVExecutable() (string, error) {
	makeMKVExecutableName := "makemkvcon"

	if runtime.GOOS == "windows" {
		makeMKVExecutableName = fmt.Sprintf("%s%s", makeMKVExecutableName, ".exe")
	}

	_, err := exec.LookPath(makeMKVExecutableName)

	if err == nil {
		return makeMKVExecutableName, nil
	}

	// If OSX the executable is in /Applications/MakeMKV.app/Contents/MacOS
	if runtime.GOOS == "darwin" {
		const macExecutable = "/Applications/MakeMKV.app/Contents/MacOS/makemkvcon"

		info, err := os.Stat(macExecutable)

		// make sure that the file exists
		if err != nil {
			if os.IsNotExist(err) {
				return "", fmt.Errorf("makemkvcon executable not found at %s", macExecutable)
			}
			return "", fmt.Errorf("error checking makemkvcon executable at %s: %w", macExecutable, err)
		}

		// make sure that the current user has permission to execute the file
		if info.Mode()&0111 == 0 {
			return "", fmt.Errorf("makemkvcon was found at %s but is not executable", macExecutable)
		}

		return macExecutable, nil
	}

	if runtime.GOOS == "windows" {
		const winExecutable = "C:\\Program Files (x86)\\MakeMKV\\makemkvcon.exe"

		_, err := os.Stat(winExecutable)

		// make sure that the file exists
		if err != nil {
			return "", fmt.Errorf("makemkvcon executable not accessible at %s: %w", winExecutable, err)
		}

		return winExecutable, nil
	}

	return "", fmt.Errorf("makemkvcon executable not found")
}

func (t *TitleInfo) SetPrependDiscToSubdirectory(val bool) {
	t.PrependDiscToSub = val
}

func (t *TitleInfo) GetEncodingFileName(config *handyMKVConfig) string {
	// Replace spaces with underscores for encoding run.
	encodingOutputFileName := strings.ReplaceAll(t.FileName, " ", "_")

	if config.EncodeConfig.OutputFileFormat != "" && config.EncodeConfig.OutputFileFormat != "mkv" {
		encodingOutputFileName = fmt.Sprintf("%s.%s", strings.TrimSuffix(encodingOutputFileName, ".mkv"), config.EncodeConfig.OutputFileFormat)
	}

	return encodingOutputFileName
}

func (t *TitleInfo) Subdirectory() string {
	if t.PrependDiscToSub {
		return fmt.Sprintf("HMKV_DISC_%d__%s", t.DiscId, strings.ReplaceAll(t.DiscTitle, " ", "_"))
	}

	return strings.ReplaceAll(t.DiscTitle, " ", "_")
}

type MakeMKV struct {
	executable string
}

func NewMakeMKV(executable string) *MakeMKV {
	return &MakeMKV{
		executable: executable,
	}
}

func (mkv *MakeMKV) ripTitle(ctx context.Context, title *TitleInfo, destDir string, onProgress func(percent int, stage string)) error {
	cmd := exec.CommandContext(ctx, mkv.executable,
		"-r", "--progress=-same",
		"mkv", fmt.Sprintf("disc:%d", title.DiscId), fmt.Sprintf("%d", title.Index), destDir)

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return fmt.Errorf("failed to create makemkvcon stdout pipe: %w", err)
	}

	stderr, err := cmd.StderrPipe()
	if err != nil {
		return fmt.Errorf("failed to create makemkvcon stderr pipe: %w", err)
	}

	if err := cmd.Start(); err != nil {
		return fmt.Errorf("failed to start makemkvcon: %w", err)
	}

	var stdoutBuf, stderrBuf bytes.Buffer
	stderrDone := make(chan struct{})
	go func() {
		io.Copy(&stderrBuf, stderr)
		close(stderrDone)
	}()

	scanner := bufio.NewScanner(io.TeeReader(stdout, &stdoutBuf))
	scanner.Split(splitProgressLines)

	lastPercent := -1
	stage := ""
	for scanner.Scan() {
		percent, nextStage, kind := parseMakeMKVProgressLine(scanner.Text())
		switch kind {
		case "stage":
			stage = nextStage
			if onProgress != nil {
				onProgress(lastPercent, stage)
			}
		case "progress":
			lastPercent = percent
			if onProgress != nil {
				onProgress(lastPercent, stage)
			}
		}
	}

	scanErr := scanner.Err()
	<-stderrDone
	waitErr := cmd.Wait()

	if scanErr != nil {
		return fmt.Errorf("error reading makemkvcon output: %w", scanErr)
	}

	if waitErr != nil {
		cmdOut := stdoutBuf.String()
		if stderrBuf.Len() > 0 {
			cmdOut = cmdOut + stderrBuf.String()
		}
		return writeRipErrorLog(destDir, cmdOut, waitErr)
	}

	return nil
}

func writeRipErrorLog(destDir, cmdOut string, err error) error {
	logFilePath := filepath.Join(destDir, "rip_err.log")
	f, openLogFileErr := os.OpenFile(logFilePath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if openLogFileErr != nil {
		return fmt.Errorf("ripping title from disc was not successful - %w - an error occurred while creating log file - %w", err, openLogFileErr)
	}
	defer f.Close()

	newline := "\n"
	if runtime.GOOS == "windows" {
		newline = "\r\n"
	}

	logContent := fmt.Sprintf("MakeMKV Output%s%s#####%s%s#####%s%sError: %v",
		newline, newline, newline,
		cmdOut,
		newline, newline,
		err)

	if _, writeLogFileErr := f.WriteString(logContent); writeLogFileErr != nil {
		return fmt.Errorf("ripping title from disc was not successful - %w - failed to write to log file: %w", err, writeLogFileErr)
	}

	return fmt.Errorf("ripping title from disc was not successful - mkv error details can be found in log file %s", logFilePath)
}

// parseMakeMKVProgressLine extracts overall percent from PRGV and stage name
// from PRGC. kind is "progress", "stage", or "" if the line is unrelated.
func parseMakeMKVProgressLine(line string) (percent int, stage string, kind string) {
	line = strings.TrimSpace(line)

	if strings.HasPrefix(line, "PRGV:") {
		parts := strings.Split(strings.TrimPrefix(line, "PRGV:"), ",")
		if len(parts) < 3 {
			return -1, "", ""
		}

		current, err1 := strconv.Atoi(strings.TrimSpace(parts[0]))
		total, err2 := strconv.Atoi(strings.TrimSpace(parts[1]))
		max, err3 := strconv.Atoi(strings.TrimSpace(parts[2]))
		if err1 != nil || err2 != nil || err3 != nil || max <= 0 {
			return -1, "", ""
		}

		value := total
		if value == 0 {
			value = current
		}

		percent = int((float64(value) / float64(max)) * 100)
		if percent > 99 {
			percent = 99
		}
		if percent < 0 {
			percent = 0
		}
		return percent, "", "progress"
	}

	if strings.HasPrefix(line, "PRGC:") {
		parts := strings.SplitN(strings.TrimPrefix(line, "PRGC:"), ",", 3)
		if len(parts) < 3 {
			return -1, "", ""
		}
		name := strings.Trim(parts[2], "\"")
		if name == "" {
			return -1, "", ""
		}
		return -1, name, "stage"
	}

	return -1, "", ""
}

// titleDurationSeconds parses MakeMKV's "H:MM:SS" duration. Returns 0 if invalid.
func titleDurationSeconds(length string) int {
	parts := strings.Split(length, ":")
	if len(parts) != 3 {
		return 0
	}
	h, err1 := strconv.Atoi(parts[0])
	m, err2 := strconv.Atoi(parts[1])
	s, err3 := strconv.Atoi(parts[2])
	if err1 != nil || err2 != nil || err3 != nil {
		return 0
	}
	return h*3600 + m*60 + s
}

// findLongestTitle returns the index of the longest title. Duration wins;
// FileSizeBytes is the tie-breaker. Returns -1 if titles is empty.
func findLongestTitle(titles []TitleInfo) int {
	if len(titles) == 0 {
		return -1
	}

	best := 0
	for i := 1; i < len(titles); i++ {
		di := titleDurationSeconds(titles[i].Length)
		db := titleDurationSeconds(titles[best].Length)
		if di > db {
			best = i
			continue
		}
		if di == db && titles[i].FileSizeBytes > titles[best].FileSizeBytes {
			best = i
		}
	}
	return best
}

func (mkv *MakeMKV) getTitlesFromDisc(discId int) ([]TitleInfo, error) {
	titles := make([]TitleInfo, 0)

	// Run the command to get the output
	cmdOut, err := exec.Command(mkv.executable, "-r", "info", fmt.Sprintf("disc:%d", discId)).Output()
	if err != nil {
		return titles, fmt.Errorf("error running command: %w", err)
	}

	// Parse the output by splitting into lines
	lines := strings.Split(string(cmdOut), "\n")

	// Temporary variables to hold extracted data for each title
	titleData := make(map[int]*TitleInfo)

	var discTitle string

	for _, line := range lines {
		if discTitle == "" && strings.HasPrefix(line, fmt.Sprintf("DRV:%d,", discId)) {
			parts := strings.Split(line, ",")

			if len(parts) != 7 || parts[5] == "\"\"" {
				continue
			}

			discTitle = strings.Trim(parts[5], "\"")
		}

		// Extract the title index (e.g., TINFO:0, TINFO:1)
		if strings.HasPrefix(line, "TINFO:") {
			parts := strings.SplitN(line, ",", 4)
			if len(parts) < 4 {
				continue
			}
			index, _ := strconv.Atoi(strings.TrimPrefix(parts[0], "TINFO:"))
			code := parts[1]
			value := strings.Trim(parts[3], "\"")

			// Windows carriage return fix
			if runtime.GOOS == "windows" {
				value = strings.TrimRight(value, "\"\r")
			}

			// Ensure the titleData map has an entry for this title index
			if titleData[index] == nil {
				titleData[index] = &TitleInfo{
					Index: index,
				}
			}

			// 2 - Disc Title
			// 8 - Number of chapters in file
			// 9 - Length of file in seconds
			// 10 - File size (GB)
			// 11 - File size (Bytes)
			// 27 - File name
			// 28 - Audio Short Code
			// 29 - Audio Long Code

			// Populate the relevant field based on the code
			switch code {
			case "8": // Number of Chapters
				if chapters, err := strconv.Atoi(value); err == nil {
					titleData[index].Chapters = chapters
				}
			case "9": // Length
				titleData[index].Length = value
			case "10": // File Size Desc
				titleData[index].FileSizeDesc = value
			case "11":
				titleData[index].FileSizeBytes, _ = strconv.Atoi(value)
			case "27": // File Name
				titleData[index].FileName = value
			}

			titleData[index].PrependDiscToSub = false
			titleData[index].DiscTitle = discTitle
		}
	}

	// Convert the map to a slice
	for _, title := range titleData {
		title.DiscId = discId
		titles = append(titles, *title)
	}

	// Sort the titles by FileName
	sort.Slice(titles, func(i, j int) bool {
		return titles[i].FileName < titles[j].FileName
	})

	return titles, nil
}

func (mkv *MakeMKV) getTitles(discId int) ([]TitleInfo, error) {
	titles, err := mkv.getTitlesFromDisc(discId)
	if err != nil {
		return titles, fmt.Errorf("an error occurred while reading titles from disc - %w", err)
	}

	if len(titles) < 1 {
		return titles, NewDiscError(discId, "no titles found on disc")
	}

	return titles, nil
}

// makemkvcon -r --cache=1 info disc:9999

// Example Output of makemkvcon:

// DRV:0,2,999,12,"BD-RE HL-DT-ST BD-RE  BH16NS40 1.05 KLZK7UI0426","STAR TREK TNG S4 D2","/dev/sr0"
// DRV:1,256,999,0,"","",""
// DRV:2,256,999,0,"","",""
// DRV:3,256,999,0,"","",""
// DRV:4,256,999,0,"","",""
// DRV:5,256,999,0,"","",""
// DRV:6,256,999,0,"","",""
// DRV:7,256,999,0,"","",""
// DRV:8,256,999,0,"","",""
// DRV:9,256,999,0,"","",""
// DRV:10,256,999,0,"","",""
// DRV:11,256,999,0,"","",""
// DRV:12,256,999,0,"","",""
// DRV:13,256,999,0,"","",""
// DRV:14,256,999,0,"","",""
// DRV:15,256,999,0,"","",""

type DiscInfo struct {
	Index int
	Name  string
}

func (mkv *MakeMKV) ListDiscs() ([]DiscInfo, error) {
	cmdOut, err := exec.Command(mkv.executable, "-r", "--cache=1", "info", "disc:9999").Output()
	if err != nil {
		return nil, fmt.Errorf("error running command: %w", err)
	}

	// Parse the output by splitting into lines
	lines := strings.Split(string(cmdOut), "\n")

	// Temporary variables to hold extracted data for each title
	drives := make([]DiscInfo, 0)

	for _, line := range lines {
		// Extract the title index (e.g., TINFO:0, TINFO:1)
		if strings.HasPrefix(line, "DRV:") {
			parts := strings.Split(line, ",")

			if len(parts) != 7 || parts[5] == "\"\"" {
				continue
			}

			discIndexString := parts[0]

			discIndexString = strings.TrimPrefix(discIndexString, "DRV:")

			discIndex, err := strconv.Atoi(discIndexString)
			if err != nil {
				return nil, fmt.Errorf("error parsing disc index: %w", err)
			}

			discName := strings.Trim(parts[5], "\"")

			drives = append(drives, DiscInfo{
				Index: discIndex,
				Name:  discName,
			})
		}
	}

	return drives, nil
}
