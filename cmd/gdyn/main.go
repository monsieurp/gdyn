package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"log/slog"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"
)

func main() {
	os.Exit(run(os.Args[1:], os.Stdout, os.Stderr))
}

func run(args []string, stdout, stderr io.Writer) int {
	configure := len(args) > 0 && args[0] == "configure"
	name := "gdyn"
	if configure {
		name += " configure"
		args = args[1:]
	}
	flags := flag.NewFlagSet(name, flag.ContinueOnError)
	flags.SetOutput(stderr)
	configPath := flags.String("config", "./config.toml", "configuration file path")
	var dryRun, force, verbose, showVersion bool
	var logPath string
	if !configure {
		flags.BoolVar(&dryRun, "dry-run", false, "show planned changes without updating DNS")
		flags.BoolVar(&force, "force", false, "update all configured records even if unchanged")
		flags.BoolVar(&verbose, "verbose", false, "show progress and record comparisons")
		flags.StringVar(&logPath, "log", "", "append logs only to this file")
		flags.BoolVar(&showVersion, "version", false, "print version and exit")
	}
	flags.Usage = func() {
		fmt.Fprintf(flags.Output(), "Usage: %s [options]\n\n", name)
		if !configure {
			fmt.Fprintln(flags.Output(), "Run gdyn configure to edit configuration.")
			fmt.Fprintln(flags.Output())
		}
		flags.PrintDefaults()
		fmt.Fprintln(flags.Output(), "  --help\n        print help and exit")
	}
	if err := flags.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return 0
		}
		return 2
	}
	if flags.NArg() != 0 {
		fmt.Fprintf(stderr, "unexpected argument: %s\n", flags.Arg(0))
		return 2
	}
	if showVersion {
		fmt.Fprintln(stdout, "gdyn dev")
		return 0
	}
	if configure {
		if err := configureUI(*configPath); err != nil {
			newLogger(stderr, false).Error("configuration failed", "error", err)
			return 1
		}
		return 0
	}
	var destination io.Writer = stderr
	if logPath != "" {
		// Never open a configuration file as a log destination.
		if sameFile(*configPath, logPath) {
			newLogger(stderr, false).Error("log path must differ from configuration path")
			return 1
		}
		file, err := os.OpenFile(logPath, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0600)
		if err != nil {
			newLogger(stderr, false).Error("open log file", "error", err)
			return 1
		}
		defer file.Close()
		destination = file
	}
	logger := newLogger(destination, verbose)
	cfg, err := loadConfig(*configPath)
	if err == nil {
		err = cfg.validate()
	}
	if err != nil {
		logger.Error("configuration failed", "error", err)
		return 1
	}
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()
	resolver := newHTTPClient(true)
	defer resolver.CloseIdleConnections()
	logger.Debug("discovering public IPv4 address")
	ip, err := resolveIP(ctx, resolver, cfg.IP.ResolverURL)
	if err != nil {
		logger.Error("IP discovery failed", "error", err)
		return 1
	}
	logger.Debug("public IP discovered", "ip", ip)
	httpClient := newHTTPClient(false)
	defer httpClient.CloseIdleConnections()
	client := &gandiClient{http: httpClient, baseURL: cfg.Gandi.URL, token: cfg.Gandi.Token}
	if err := updateRecords(ctx, client, cfg, ip, dryRun, force, logger); err != nil {
		logger.Error("DNS update failed", "error", err)
		return 1
	}
	return 0
}

func newLogger(writer io.Writer, verbose bool) *slog.Logger {
	level := slog.LevelInfo
	if verbose {
		level = slog.LevelDebug
	}
	return slog.New(slog.NewTextHandler(writer, &slog.HandlerOptions{
		Level: level,
		ReplaceAttr: func(groups []string, attr slog.Attr) slog.Attr {
			if attr.Key == slog.TimeKey {
				return slog.String(slog.TimeKey, attr.Value.Time().Format(time.RFC3339))
			}
			return attr
		},
	}))
}

func sameFile(first, second string) bool {
	firstPath, firstErr := filepath.Abs(first)
	secondPath, secondErr := filepath.Abs(second)
	if firstErr == nil && secondErr == nil && firstPath == secondPath {
		return true
	}
	a, errA := os.Stat(first)
	b, errB := os.Stat(second)
	return errA == nil && errB == nil && os.SameFile(a, b)
}
