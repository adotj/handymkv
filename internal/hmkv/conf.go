package hmkv

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/user"
	"path/filepath"
	"runtime"
	"strings"
)

const (
	System configFileLocation = iota
	User
	WorkingDirectory
)

const (
	configFileName    = "config.json"
	defaultNtfyServer = "https://ntfy.sh"
)

var ErrConfigNotFound = errors.New("config file not found")

type configFileLocation int

type handyMKVConfig struct {
	MKVOutputDirectory string `json:"mkv_output_directory"`
	HBOutputDirectory  string `json:"handbrake_output_directory,omitempty"`
	LibraryRoot        string `json:"library_root,omitempty"`
	TVLibraryRoot      string `json:"tv_library_root,omitempty"`
	NtfyTopic          string `json:"ntfy_topic,omitempty"`
	NtfyServer         string `json:"ntfy_server,omitempty"`
}

func (config *handyMKVConfig) String() string {
	applyConfigDefaults(config)

	var sb strings.Builder

	sb.WriteString("Encode Settings (fixed)\n\n")
	enc := defaultEncodingParams()
	fmt.Fprintf(&sb, "Encoder: %s\n", enc.Encoder)
	fmt.Fprintf(&sb, "Encoder Preset: %s\n", enc.EncoderPreset)
	fmt.Fprintf(&sb, "Quality (CRF): %d\n", enc.Quality)
	fmt.Fprintf(&sb, "Audio Languages: %s (first track only)\n", strings.Join(enc.AudioLanguages, ", "))
	fmt.Fprintf(&sb, "Subtitle Languages: %s (first track only)\n", strings.Join(enc.SubtitleLanguages, ", "))
	fmt.Fprintf(&sb, "Output Format: mkv\n")

	sb.WriteString("\nDirectories\n\n")
	fmt.Fprintf(&sb, "MKV Output Directory:      %s\n", config.MKVOutputDirectory)
	fmt.Fprintf(&sb, "HandBrake Output Directory: %s\n", config.HBOutputDirectory)
	fmt.Fprintf(&sb, "Movie Library Root:         %s\n", config.LibraryRoot)
	fmt.Fprintf(&sb, "TV Library Root:            %s\n", config.TVLibraryRoot)

	sb.WriteString("\nBehavior (fixed)\n\n")
	sb.WriteString("Automatically delete raw MKV files: true\n")
	sb.WriteString("Organize to Jellyfin library:       true\n")

	if config.NtfyTopic != "" {
		sb.WriteString("\nNotifications\n\n")
		fmt.Fprintf(&sb, "ntfy topic:  %s\n", config.NtfyTopic)
		fmt.Fprintf(&sb, "ntfy server: %s\n", config.NtfyServer)
	}

	return sb.String()
}

func applyConfigDefaults(config *handyMKVConfig) {
	applyLibraryConfigDefaults(config)

	if config.HBOutputDirectory == "" && config.MKVOutputDirectory != "" {
		config.HBOutputDirectory = deriveHBOutputDirectory(config.MKVOutputDirectory)
	}

	if config.NtfyTopic != "" && config.NtfyServer == "" {
		config.NtfyServer = defaultNtfyServer
	}
}

func deriveHBOutputDirectory(mkvOutputDirectory string) string {
	return filepath.Join(filepath.Dir(mkvOutputDirectory), "hboutput")
}

func getUserConfigPath() (string, error) {
	if runtime.GOOS == "windows" {
		appData := os.Getenv("APPDATA")
		if appData == "" {
			return "", fmt.Errorf("APPDATA environment variable is not set")
		}
		return filepath.Join(appData, "handymkv", configFileName), nil
	}

	usr, err := user.Current()
	if err != nil {
		return "", fmt.Errorf("error getting current user: %w", err)
	}
	return filepath.Join(usr.HomeDir, ".config", "handymkv", configFileName), nil
}

// GetConfigFilePath returns the path of the active config file, using the same
// discovery order as ReadConfig: local ./config.json first, then the user config directory.
func GetConfigFilePath() (string, error) {
	local := fmt.Sprintf("./%s", configFileName)
	if _, err := os.Stat(local); err == nil {
		return local, nil
	}
	userPath, err := getUserConfigPath()
	if err != nil {
		return "", err
	}
	if _, err := os.Stat(userPath); err == nil {
		return userPath, nil
	}
	return "", ErrConfigNotFound
}

// ReadConfig reads the config file and returns a config struct.
func ReadConfig() (*handyMKVConfig, error) {
	filePath := fmt.Sprintf("./%s", configFileName)
	if _, err := os.Stat(filePath); err == nil {
		return readConfigFile(filePath)
	}

	userConfigPath, err := getUserConfigPath()
	if err != nil {
		return nil, err
	}

	if _, err := os.Stat(userConfigPath); err == nil {
		return readConfigFile(userConfigPath)
	}

	return nil, ErrConfigNotFound
}

func readConfigFile(filePath string) (*handyMKVConfig, error) {
	fileData, err := os.ReadFile(filePath)
	if err != nil {
		return nil, fmt.Errorf("error reading config file - %w", err)
	}

	var cfg handyMKVConfig
	if err := json.Unmarshal(fileData, &cfg); err != nil {
		return nil, fmt.Errorf("error parsing config file - %w", err)
	}

	if cfg.MKVOutputDirectory == "" {
		return nil, fmt.Errorf("config file is missing mkv_output_directory")
	}

	applyConfigDefaults(&cfg)
	return &cfg, nil
}

func createConfigFile(location configFileLocation, config *handyMKVConfig, overwrite bool) error {
	var configPath string

	switch location {
	case System:
		return fmt.Errorf("system-wide config location not supported")
	case User:
		var err error
		configPath, err = getUserConfigPath()
		if err != nil {
			return err
		}
	case WorkingDirectory:
		configPath = fmt.Sprintf("./%s", configFileName)
	default:
		return fmt.Errorf("unknown config file location")
	}

	if err := os.MkdirAll(filepath.Dir(configPath), 0740); err != nil {
		return fmt.Errorf("error creating config directory: %w", err)
	}

	if _, err := os.Stat(configPath); err == nil && !overwrite {
		fmt.Printf("\nA config file already exists at %s. Overwrite? [y/N]\n\n", configPath)
		if strings.ToLower(readLine()) != "y" {
			fmt.Printf("\nSkipping creation of config file. The file %s already exists.\n", configPath)
			return nil
		}
	} else if err != nil && !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("error checking for existing config file: %w", err)
	}

	configData, err := json.MarshalIndent(config, "", "  ")
	if err != nil {
		return fmt.Errorf("error marshaling config to JSON: %w", err)
	}

	if err := os.WriteFile(configPath, configData, 0640); err != nil {
		return fmt.Errorf("error writing config file: %w", err)
	}

	return nil
}

func promptForConfig(configLocationSelection int) (*handyMKVConfig, error) {
	var config handyMKVConfig

	var handyMKVDir string
	if configLocationSelection == 1 {
		usr, err := user.Current()
		if err != nil {
			return nil, fmt.Errorf("error getting current user: %w", err)
		}
		handyMKVDir = filepath.Join(usr.HomeDir, "handymkv")
	} else {
		handyMKVDir = "."
	}

	defaultMKVOutputDirectory := filepath.Join(handyMKVDir, "mkvoutput")

	clear()
	config.MKVOutputDirectory = promptForString(
		"Where should raw MKV files be staged during ripping?",
		fmt.Sprintf("Absolute path to a directory. Example: %s", defaultMKVOutputDirectory),
		defaultMKVOutputDirectory,
		nil,
	)

	clear()
	config.LibraryRoot = promptForString(
		"Where should finished movies be organized?",
		"Absolute path to your Jellyfin movie library root folder.",
		defaultLibraryRoot(),
		nil,
	)

	clear()
	config.TVLibraryRoot = promptForString(
		"Where should finished TV episodes be organized?",
		"Absolute path to your Jellyfin TV shows library root folder.",
		defaultTVLibraryRoot(),
		nil,
	)

	clear()
	config.NtfyTopic = promptForString(
		"ntfy topic (optional, for phone notifications)",
		"Subscribe to this topic in the ntfy app on your phone. Use a private, unguessable name. Leave blank to skip.",
		"",
		nil,
	)

	return &config, nil
}
