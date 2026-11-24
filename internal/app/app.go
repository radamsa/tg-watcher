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
	"tg-watcher/internal/format"
	"tg-watcher/internal/output"
	"tg-watcher/internal/resolve"
	"tg-watcher/internal/telegram"
	"tg-watcher/internal/watch"
)

// Run executes the application and returns the process exit code.
func Run(ctx context.Context, opts cli.Options, cfg *config.Config, log *slog.Logger) int {
	name := cfg.FormatName(opts.Format)
	tmpl, ok := cfg.Template(name)
	if !ok {
		log.Error("unknown output format", "format", name)
		return cli.ExitUsage
	}

	log.Info("configuration loaded",
		"format", name,
		"chats", len(opts.Chats),
		"poll_interval", cfg.Watch.PollInterval.Std().String(),
	)

	return run(ctx, opts, cfg, log, tmpl)
}

func run(ctx context.Context, opts cli.Options, cfg *config.Config, log *slog.Logger, tmpl *format.Template) int {
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

	console := auth.NewConsole(os.Stdin, os.Stderr, int(os.Stdin.Fd()))
	prompter := auth.NewSource(opts.Password, console)

	tgClient, err := telegram.New(telegram.Options{
		APIID:    cfg.Telegram.APIID,
		APIHash:  cfg.Telegram.APIHash,
		Phone:    opts.Phone,
		Sessions: store,
		Logger:   log,
	})
	if err != nil {
		log.Error("failed to create Telegram client", "error", err)
		return cli.ExitRuntime
	}

	var exitCode int
	if err := tgClient.Run(ctx, func(runCtx context.Context) error {
		user, err := performAuth(runCtx, tgClient, prompter, log)
		if err != nil {
			return err
		}
		log.Info("authorization successful", "user", user)
		return runMonitor(runCtx, tgClient, opts, cfg, log, tmpl)
	}); err != nil {
		exitCode = classifyShutdownError(err, log)
	} else {
		exitCode = cli.ExitOK
	}
	return exitCode
}

func runMonitor(ctx context.Context, tgClient *telegram.Client, opts cli.Options, cfg *config.Config, log *slog.Logger, tmpl *format.Template) error {
	targets, errs := resolve.ParseTargets(opts.Chats)
	for _, e := range errs {
		log.Error("invalid chat identifier", "error", e)
	}
	if len(targets) == 0 {
		return fmt.Errorf("no valid chat identifiers provided")
	}

	chats := tgClient.Resolve(ctx, targets, log)

	usable := make([]resolve.ChatInfo, 0, len(chats))
	for _, c := range chats {
		if c.Err != nil {
			log.Error("chat is inaccessible", "chat", c.ChatID, "error", c.Err)
		} else {
			usable = append(usable, c)
		}
	}
	if len(usable) == 0 {
		return fmt.Errorf("no usable chats: all specified chats are inaccessible")
	}

	states := make(map[int64]*watch.ChatState, len(usable))
	for _, ci := range usable {
		baselineID, err := tgClient.FetchBaseline(ctx, ci.ChatID, ci.AccessHash)
		if err != nil {
			log.Warn("failed to fetch baseline for chat; starting from zero",
				"chat", ci.Title, "id", ci.ChatID, "error", err)
		}
		st := watch.NewChatState(ci, baselineID)
		states[ci.ChatID] = st
		log.Debug("baseline set",
			"chat", ci.Title,
			"id", ci.ChatID,
			"baseline_id", baselineID,
		)
	}

	wr := output.New(os.Stdout, 256)
	engine := watch.NewEngine(states, wr, tmpl, log)
	engine.Start()
	defer engine.Stop()

	if err := tgClient.StartUpdates(ctx, engine, usable); err != nil {
		return fmt.Errorf("start updates: %w", err)
	}
	log.Info("update stream started")

	poller := telegram.NewPoller(
		tgClient, engine, states,
		cfg.Watch.PollInterval.Std(),
		cfg.Watch.PollPageSize,
	)
	go poller.Run(ctx)
	log.Info("poller started", "interval", cfg.Watch.PollInterval.Std())

	log.Info("READY — monitoring starting")

	<-ctx.Done()
	log.Info("shutting down")
	return nil
}

func performAuth(ctx context.Context, tgClient *telegram.Client, p *auth.Source, log *slog.Logger) (string, error) {
	const maxRetries = 3
	for attempt := 0; attempt < maxRetries; attempt++ {
		if err := tgClient.Authenticate(ctx, p); err != nil {
			var fw *auth.FloodWaitError
			if errors.As(err, &fw) && attempt+1 < maxRetries {
				log.Warn("authentication rate-limited, waiting before retry",
					"duration", fw.Duration,
					"attempt", attempt+1,
				)
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
		u, err := tgClient.Self(ctx)
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

func classifyShutdownError(err error, log *slog.Logger) int {
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		log.Info("shutdown")
		return cli.ExitOK
	}
	if auth.IsAuthError(err) {
		log.Error("authentication failed", "error", err)
		return cli.ExitAuth
	}
	if err != nil && err.Error() == "no usable chats: all specified chats are inaccessible" {
		log.Error("no accessible chats to monitor")
		return cli.ExitNoChats
	}
	log.Error("runtime error", "error", err)
	return cli.ExitRuntime
}