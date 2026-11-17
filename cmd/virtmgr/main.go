package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"virtmgr/internal/bootstrap"
	"virtmgr/internal/platform/config"
	"virtmgr/internal/version"
)

func main() {
	args := os.Args[1:]
	if len(args) > 0 {
		switch args[0] {
		case "version", "--version", "-v":
			fmt.Printf("virtmgr %s (%s, %s)\n", version.Version, version.Commit, version.Date)
			return
		case "help", "--help", "-h":
			usage()
			return
		}
	}
	flags := flag.NewFlagSet("virtmgr", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	configPath := config.DefaultPath
	flags.StringVar(&configPath, "config", config.DefaultPath, "path to server.yaml")
	flags.StringVar(&configPath, "c", config.DefaultPath, "path to server.yaml")
	if err := flags.Parse(args); err != nil {
		fmt.Fprintln(os.Stderr, "invalid arguments:", err)
		usage()
		os.Exit(2)
	}
	if flags.NArg() > 0 {
		fmt.Fprintln(os.Stderr, "unknown command:", flags.Arg(0))
		usage()
		os.Exit(2)
	}

	cfg, err := config.Load(configPath)
	if err != nil {
		fmt.Fprintln(os.Stderr, "configuration error:", err)
		os.Exit(1)
	}
	logger := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo}))
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	app, err := bootstrap.New(ctx, cfg, logger)
	if err != nil {
		logger.Error("virtmgr_startup_failed", "error", err)
		os.Exit(1)
	}
	if err := app.Run(ctx); err != nil {
		logger.Error("virtmgr_stopped", "error", err)
		os.Exit(1)
	}
}

func usage() {
	fmt.Fprintln(os.Stderr, `Usage:
  virtmgr [--config /etc/virtmgr/server.yaml]  Start the VirtMgr console
  virtmgr version                             Print version information`)
}
