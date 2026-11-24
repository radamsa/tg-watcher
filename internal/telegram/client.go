package telegram

import (
	"context"
	"errors"
	"log/slog"
	"os"

	"github.com/gotd/td/session"
	"github.com/gotd/td/tg"
	gotdauth "github.com/gotd/td/telegram/auth"
	gotd "github.com/gotd/td/telegram"

	"tg-watcher/internal/auth"
)

// Client wraps a gotd Client and exposes only the methods needed by the
// application.
type Client struct {
	client *gotd.Client
	phone  string
	log    *slog.Logger
}

// Options configures the Telegram client.
type Options struct {
	APIID    int
	APIHash  string
	Phone    string
	Sessions *auth.SessionStore
	Logger   *slog.Logger
	NoUpdates bool // set true until Stage 5 enables update handling
}

// New creates a Client. It validates credentials and the session path but does
// not connect.
func New(opts Options) (*Client, error) {
	if opts.APIID <= 0 {
		return nil, errors.New("api_id must be positive")
	}
	if opts.APIHash == "" {
		return nil, errors.New("api_hash must not be empty")
	}
	if opts.Phone == "" {
		return nil, errors.New("phone must not be empty")
	}
	if opts.Sessions == nil {
		return nil, errors.New("session store is required")
	}
	if _, err := opts.Sessions.Path(opts.Phone); err != nil {
		return nil, err
	}

	c := gotd.NewClient(opts.APIID, opts.APIHash, gotd.Options{
		SessionStorage: &sessionStorage{store: opts.Sessions, phone: opts.Phone},
		NoUpdates:      opts.NoUpdates,
	})
	log := opts.Logger
	if log == nil {
		log = slog.New(slog.NewTextHandler(os.Stderr, nil))
	}
	return &Client{client: c, phone: opts.Phone, log: log}, nil
}

// Run connects to Telegram and calls f when ready. The client is shut down when
// f returns. Call Run for the full lifecycle of a monitoring session.
func (c *Client) Run(ctx context.Context, f func(ctx context.Context) error) error {
	return c.client.Run(ctx, f)
}

// Authenticate ensures the current session is valid, performing the full auth
// flow (phone + code + optional 2FA) if needed. p supplies the code and, when
// Telegram requires it, the 2FA password.
//
// Call this inside the function passed to Run.
func (c *Client) Authenticate(ctx context.Context, p auth.Prompter) error {
	if p == nil {
		return errors.New("prompter required")
	}
	flow := gotdauth.NewFlow(&authBridge{phone: c.phone, prompter: p}, gotdauth.SendCodeOptions{})
	if err := c.client.Auth().IfNecessary(ctx, flow); err != nil {
		return Classify(err)
	}
	return nil
}

// Self returns the authorized user.
func (c *Client) Self(ctx context.Context) (*tg.User, error) {
	return c.client.Self(ctx)
}

// authBridge adapts the application's Prompter to gotd's UserAuthenticator.
type authBridge struct {
	phone    string
	prompter auth.Prompter
}

func (a *authBridge) Phone(context.Context) (string, error) { return a.phone, nil }

func (a *authBridge) Password(ctx context.Context) (string, error) {
	return a.prompter.Password(ctx)
}

func (a *authBridge) AcceptTermsOfService(ctx context.Context, _ tg.HelpTermsOfService) error {
	return &gotdauth.SignUpRequired{}
}

func (a *authBridge) SignUp(context.Context) (gotdauth.UserInfo, error) {
	return gotdauth.UserInfo{}, auth.ErrSignUpRequired
}

func (a *authBridge) Code(ctx context.Context, _ *tg.AuthSentCode) (string, error) {
	return a.prompter.Code(ctx)
}

// sessionStorage adapts our SessionStore to gotd's session.Storage.
type sessionStorage struct {
	store *auth.SessionStore
	phone string
}

func (s sessionStorage) LoadSession(ctx context.Context) ([]byte, error) {
	data, err := s.store.Load(s.phone)
	if os.IsNotExist(err) {
		return nil, session.ErrNotFound
	}
	return data, err
}

func (s sessionStorage) StoreSession(_ context.Context, data []byte) error {
	return s.store.Save(s.phone, data)
}