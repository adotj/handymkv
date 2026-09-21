package main

import (
	"errors"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"os/user"
	"path/filepath"
	"runtime"
	"runtime/debug"
	"slices"
	"strconv"
	"strings"

	"github.com/adotj/handymkv/internal/hmkv"
)

var applicationVersion = "dev"

func getVersion() string {
	if applicationVersion != "dev" {
		return applicationVersion
	}
	if info, ok := debug.ReadBuildInfo(); ok && info.Main.Version != "" && info.Main.Version != "(devel)" {
		return strings.TrimPrefix(info.Main.Version, "v")
	}
	return applicationVersion
}

func main() {
	// Handle subcommands before flag.Parse()
	if len(os.Args) > 1 && os.Args[1] == "config" {
		hmkv.PrintLogo()

		if len(os.Args) > 2 && os.Args[2] == "edit" {
			configPath, err := hmkv.GetConfigFilePath()
			if err != nil {
				if err == hmkv.ErrConfigNotFound {
					fmt.Printf("No config file found. Run 'handymkv config setup' to create one.\n\n")
					return
				}
				fmt.Printf("Error locating config file: %v\n\n", err)
				return
			}
			if err := openInEditor(configPath); err != nil {
				if errors.Is(err, exec.ErrNotFound) {
					fmt.Printf("No editor found. Set the EDITOR or VISUAL environment variable to specify your preferred editor.\nExample: export EDITOR=nano\n\n")
				} else {
					fmt.Printf("Could not open editor: %v\n\n", err)
				}
			}
			return
		}

		if len(os.Args) > 2 && os.Args[2] == "setup" {
			if err := hmkv.Setup(); err != None {
				fmt.Printf("An error occurred during the setup process.\nError: %v\n", err)
			}
			return
		}
