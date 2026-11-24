// Command tg-watcher prints new messages from configured Telegram chats to
// stdout. See docs/plan.md for the full specification.
package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"tg-watcher/internal/app"
	"tg-watcher/internal/cli"
	"tg-watcher/internal/config"
	"tg-watcher/internal/logging"
)

// version is overridden at build time via -ldflags "-X main.version=...".
var version = "dev"

func main() {
	os.Exit(run())
}

func run() int {
	opts, err := cli.Parse(os.Args[1:])
	if err != nil {
		fmt.Fprintf(os.Stderr, "tg-watcher: %v\n\n%s\n", err, cli.Usage)
		return cli.ExitUsage
	}

	// stdout is reserved for message records, so help and version go to stderr.
	if opts.ShowHelp {
		fmt.Fprintln(os.Stderr, cli.Help)
		return cli.ExitOK
	}
	if opts.ShowVersion {
		fmt.Fprintf(os.Stderr, "tg-watcher %s\n", version)
		return cli.ExitOK
	}

	cfg, err := config.Load(opts.ConfigPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "tg-watcher: %v\n", err)
		return cli.ExitUsage
	}

	ignoreBrokenPipeSignal()

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	log := logging.New(os.Stderr, false)
	return app.Run(ctx, opts, cfg, log)
}
