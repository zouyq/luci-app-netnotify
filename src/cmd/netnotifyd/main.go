package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/zouyq/netnotify/internal/config"
	"github.com/zouyq/netnotify/internal/daemon"
	"github.com/zouyq/netnotify/internal/nlog"
)

func main() {
	configPath := flag.String("config", "", "path to JSON config (optional)")
	flag.Parse()
	args := flag.Args()
	cmd := "run"
	if len(args) > 0 {
		cmd = args[0]
	}

	switch cmd {
	case "version", "-v", "--version":
		fmt.Println(config.Version)
		return
	case "test":
		cfg, err := config.Load(*configPath)
		if err != nil {
			fmt.Fprintf(os.Stderr, "config: %v\n", err)
			os.Exit(1)
		}
		if err := daemon.SendTest(cfg); err != nil {
			fmt.Fprintf(os.Stderr, "test push failed: %v\n", err)
			os.Exit(1)
		}
		fmt.Println("test push sent")
		return
	case "cron":
		cfg, err := config.Load(*configPath)
		if err != nil {
			fmt.Fprintf(os.Stderr, "config: %v\n", err)
			os.Exit(1)
		}
		if err := daemon.SendCronNow(cfg); err != nil {
			fmt.Fprintf(os.Stderr, "cron send failed: %v\n", err)
			os.Exit(1)
		}
		fmt.Println("cron report sent")
		return
	case "run", "":
		// continue
	default:
		fmt.Fprintf(os.Stderr, "usage: netnotifyd [-config path] [run|test|cron|version]\n")
		os.Exit(2)
	}

	cfg, err := config.Load(*configPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "config: %v\n", err)
		os.Exit(1)
	}
	if !cfg.Enable {
		fmt.Println("netnotify disabled in config; exiting")
		return
	}

	logger, err := nlog.New(cfg.Debug, cfg.LogFile)
	if err != nil {
		fmt.Fprintf(os.Stderr, "log: %v\n", err)
		os.Exit(1)
	}
	defer logger.Close()

	d, err := daemon.New(cfg, logger)
	if err != nil {
		fmt.Fprintf(os.Stderr, "daemon: %v\n", err)
		os.Exit(1)
	}

	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer cancel()
	if err := d.Run(ctx); err != nil && err != context.Canceled {
		logger.Errorf("run: %v", err)
		os.Exit(1)
	}
}
