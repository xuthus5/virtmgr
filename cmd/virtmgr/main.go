package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"virtmgr/internal/bootstrap"
	"virtmgr/internal/platform/config"
	"virtmgr/internal/version"
)

func main() {
	if len(os.Args) > 1 {
		switch os.Args[1] {
		case "version", "--version", "-v":
			fmt.Printf("virtmgr %s (%s, %s)\n", version.Version, version.Commit, version.Date)
			return
		case "help", "--help", "-h":
			usage()
			return
		default:
			fmt.Fprintln(os.Stderr, "unknown command:", os.Args[1])
			usage()
			os.Exit(2)
		}
	}

	cfg, err := config.Load()
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
  virtmgr             Start the VirtMgr console
  virtmgr version     Print version information`)
}
