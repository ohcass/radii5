# radii5

[![CI](https://github.com/ohcass/radii5/actions/workflows/ci.yml/badge.svg)](https://github.com/ohcass/radii5/actions/workflows/ci.yml)

Single track:

![demo](assets/demo.gif)

Playlist:

![Playlist Demo](assets/playlist-demo.gif)

### Installation

**Windows**

```powershell
irm https://ohcass.github.io/radii5/install.ps1 | iex
```

**Linux / macOS**

```sh
curl -fsSL https://raw.githubusercontent.com/ohcass/radii5/main/scripts/install.sh | sh
```

Download the [latest Windows release](https://github.com/ohcass/radii5/releases) and run from a terminal.

Build from source:

```sh
git clone https://github.com/ohcass/radii5.git
cd radii5
go build -o radii5 ./cmd/radii5/
```

To remove:

```sh
rm ~/.radii5/bin/radii5
```

On Windows:

```powershell
Remove-Item "$env:USERPROFILE\.radii5\bin\radii5.exe"
```

### Usage

```
radii5 <url>
radii5 --type video <url>
radii5 --type video --quality 720 <url>
radii5 --mp4 <url>
radii5 --format flac <url>
radii5 "https://youtube.com/playlist?list=..."
```

| Flag | Default | Description |
|------|---------|-------------|
| `--format`, `-f` | `mp3` | `mp3`, `flac`, `m4a`, `opus`, `wav` |
| `--output`, `-o` | `~/Music/radii5 downloads` | Output directory |
| `--threads`, `-t` | `8` | Parallel download chunks |
| `--workers`, `-w` | `4` | Concurrent playlist workers |
| `--type` | `audio` | `audio` or `video` |
| `--quality`, `-q` | `1080` | `144`, `240`, `360`, `480`, `720`, `1080`, `1440`, `2160` |
| `--mp4` | | Shortcut for `--type video` |

### Requirements

[yt-dlp](https://github.com/yt-dlp/yt-dlp) and [ffmpeg](https://ffmpeg.org/) must be in PATH. Use the one-liner installer above to bundle both.

### License

[GNU General Public License v3.0](LICENSE)
