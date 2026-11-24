package app

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"time"

	"tg-watcher/internal/auth"
	"tg-watcher/internal/cli"
	"tg-watcher/internal/config"
	"tg-watcher/internal/telegram"
)

// Run executes the application and returns the process exit code.
func Run(ctx context.Context, opts cli.Options, cfg *config.Config, log *slog.Logger) int {
	name := cfg.FormatName(opts.Format)
	if _, ok := cfg.Template(name); !ok {
		log.Error("unknown output format", "format", name)
		return cli.ExitUsage
	}

	log.Info("configuration loaded",
		"format", name,
		"chats", len(opts.Chats),
		"poll_interval", cfg.Watch.PollInterval.Std().String(),
	)

	return runAuth(ctx, opts, cfg, log)
}

func runAuth(ctx context.Context, opts cli.Options, cfg *config.Config, log *slog.Logger) int {
	// Session directory.
	sessionDir, err := auth.DefaultSessionDir()
	if err != nil {
		log.Error("cannot locate session directory", "error", err)
		return cli.ExitUsage
	}
	store, tightened, err := auth.NewSessionStore(sessionDir)
	if err != nil {
		log.Error("session storage initialization failed", "error", err)
		return cli.ExitUsage
	}
	if tightened {
		log.Warn("session directory had insecure permissions and was tightened")
	}
	log.Info("session store", "dir", sessionDir)

	// Interactive terminal.
	console := auth.NewConsole(os.Stdin, os.Stderr, int(os.Stdin.Fd()))
	prompter := auth.NewSource(opts.Password, console)

	// Telegram client.
	tg, err := telegram.New(telegram.Options{
		APIID:     cfg.Telegram.APIID,
		APIHash:   cfg.Telegram.APIHash,
		Phone:     opts.Phone,
		Sessions:  store,
		NoUpdates: true,
	})
	if err != nil {
		log.Error("failed to create Telegram client", "error", err)
		return cli.ExitRuntime
	}

	var exitCode int
	if err := tg.Run(ctx, func(runCtx context.Context) error {
		user, err := performAuth(runCtx, tg, prompter, log)
		if err != nil {
			return err
		}
		log.Info("authorization successful", "user", user)
		log.Warn("message monitoring is not implemented yet; exiting after successful auth")
		return nil
	}); err != nil {
		exitCode = classifyAuthError(err, log)
	} else {
		exitCode = cli.ExitOK
	}
	return exitCode
}

// performAuth runs the full authentication flow with flood-wait retry.
func performAuth(ctx context.Context, tg *telegram.Client, p *auth.Source, log *slog.Logger) (string, error) {
	const maxRetries = 3
	for attempt := 0; attempt < maxRetries; attempt++ {
		if err := tg.Authenticate(ctx, p); err != nil {
			var fw *auth.FloodWaitError
			if errors.As(err, &fw) && attempt+1 < maxRetries {
				log.Warn("authentication rate-limited, waiting before retry",
					"duration", fw.Duration,
					"attempt", attempt+1,
				)
				// FLOOD_WAIT times include an extra second by convention.
				timer := time.NewTimer(fw.Duration + time.Second)
				select {
				case <-timer.C:
				case <-ctx.Done():
					timer.Stop()
					return "", ctx.Err()
				}
				continue
			}
			return "", err
		}
		// Authentication succeeded.
		u, err := tg.Self(ctx)
		if err != nil {
			return "", fmt.Errorf("get user after auth: %w", err)
		}
		display := u.FirstName
		if u.LastName != "" {
			display += " " + u.LastName
		}
		if display == "" {
			display = fmt.Sprintf("user#%d", u.ID)
		}
		return display, nil
	}
	return "", errors.New("authentication failed after retries")
}

func classifyAuthError(err error, log *slog.Logger) int {
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		log.Info("shutdown during authentication")
		return cli.ExitOK
	}
	if auth.IsAuthError(err) {
		log.Error("authentication failed", "error", err)
		return cli.ExitAuth
	}
	// Network errors and other runtime issues.
	log.Error("telegram connection error", "error", err)
	return cli.ExitRuntime
}