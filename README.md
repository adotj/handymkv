# HandyMKV

A MakeMKV + HandBrake productivity tool.

## Description

HandyMKV is a tool that is designed to automate the process of ripping discs using MakeMKV and then encoding the resulting files using Handbrake.

#### Why I Created HandyMKV

I found the process of manually ripping using MakeMKV and then encoding using HandBrake to be time consuming, disjointed, and error prone. I wanted a tool that would automate the process and provide a more user-friendly experience. Additionally, I wanted to offload the process from my main desktop computer to my home server which is headless and does not have a GUI. HandyMKV was created to address these needs.

As I developed HandyMKV, I found that I was able to add features that I found useful and that made the process faster and easier. I hope that others will find HandyMKV useful and that it will save them time and effort.

## Features

- Rip titles from discs using MakeMKV
- Encode video files using HandBrake
- Movie and TV season library organization (Jellyfin-style)
- Flexible configuration options
- Clear and concise progress display
- Concurrency to reduce overall processing time
- Summary of space saved and time elapsed
- Automated cleanup of raw unencoded files
- Run history — browse and inspect past ripping/encoding sessions
- Automations — run custom scripts after encoding with parameters sourced from run data, environment variables, or user prompts
- ntfy notifications — push alerts to your phone when encoding starts or a run fails (via [ntfy.sh](https://ntfy.sh))
- Parsing of `HandBrakeCLI` and `makemkvcon` output to provide a more user-friendly experience

## Installation

### Install Script (Linux and macOS)

The quickest way to install HandyMKV on Linux or macOS is with the install script:

```shell
curl -fsSL https://raw.githubusercontent.com/adotj/handymkv/release/install.sh | bash
```

### Pre-built Binaries

Pre-built binaries are available on the [Releases](https://github.com/adotj/handymkv/releases) page.

### Installing with Go

```shell
go install github.com/adotj/handymkv/cmd/handymkv@latest
```

Build from source with `make current` or `go build -o bin/handymkv ./cmd/handymkv`.

Screenshot: ![alt text](https://github.com/adotj/handymkv/blob/release/doc/handymkv_process_in_progress.png?raw=true)
