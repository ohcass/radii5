package main

import (
	"fmt"
	"os"

	"github.com/gabstv/go-bsdiff/pkg/bspatch"
	"github.com/ohcass/radii5/cmd"
	"github.com/urfave/cli/v2"
)

var version = "dev"

func main() {
	app := &cli.App{
		Name:                   "radii5",
		Usage:                  "CLI music downloader powered by yt-dlp",
		Version:                version,
		UseShortOptionHandling: true,
		Flags: []cli.Flag{
			&cli.StringFlag{
				Name:    "format",
				Aliases: []string{"f"},
				Value:   "mp3",
				Usage:   "Audio format (mp3, flac, wav, m4a, opus)",
			},
			&cli.StringFlag{
				Name:    "output",
				Aliases: []string{"o"},
				Value:   "~/Music/radii5 downloads",
				Usage:   "Output directory",
			},
			&cli.IntFlag{
				Name:    "threads",
				Aliases: []string{"t"},
				Value:   8,
				Usage:   "Number of parallel download threads",
			},
			&cli.IntFlag{
				Name:    "workers",
				Aliases: []string{"w"},
				Value:   4,
				Usage:   "Number of concurrent download workers for playlists",
			},
			&cli.StringFlag{
				Name:  "type",
				Value: "audio",
				Usage: "Media type (audio or video)",
			},
			&cli.IntFlag{
				Name:    "quality",
				Aliases: []string{"q"},
				Value:   1080,
				Usage:   "Video quality: 144, 240, 360, 480, 720, 1080, 1440, 2160",
			},
			&cli.IntFlag{
				Name:  "mp4",
				Value: 0,
				Usage: "Shortcut: download video.  Usage: --mp4 QUALITY (e.g. --mp4 720)",
			},
			&cli.BoolFlag{
				Name:  "json",
				Usage: "Output machine-readable JSON Lines to stdout",
			},
		},
		Commands: []*cli.Command{
			{
				Name:  "bspatch",
				Usage: "Apply a bsdiff binary patch",
				Action: func(c *cli.Context) error {
					if c.NArg() != 3 {
						return fmt.Errorf("usage: radii5 bspatch oldfile newfile patchfile")
					}
					old, err := os.ReadFile(c.Args().Get(0))
					if err != nil {
						return fmt.Errorf("read old: %w", err)
					}
					patch, err := os.ReadFile(c.Args().Get(2))
					if err != nil {
						return fmt.Errorf("read patch: %w", err)
					}
					out, err := bspatch.Bytes(old, patch)
					if err != nil {
						return fmt.Errorf("patch: %w", err)
					}
					return os.WriteFile(c.Args().Get(1), out, 0755)
				},
			},
		},
		Action: func(c *cli.Context) error {
			cmd.Run(os.Args[1:])
			return nil
		},
	}

	if err := app.Run(os.Args); err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}
}
