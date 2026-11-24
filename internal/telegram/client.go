package telegram

import (
	"context"
	"errors"
	"log/slog"
	"os"
	"sync"

	"github.com/gotd/td/session"
	"github.com/gotd/td/tg"
	gotdauth "github.com/gotd/td/telegram/auth"
	gotd "github.com/gotd/td/telegram"
	"github.com/gotd/td/telegram/updates"

	"tg-watcher/internal/auth"
	"tg-watcher/internal/resolve"
	"tg-watcher/internal/watch"
)

// swappableHandler implements gotd.UpdateHandler and allows the inner handler
// to be swapped after the client starts.
type swappableHandler struct {
	mu    sync.Mutex
	inner gotd.UpdateHandler
}

func (h *swappableHandler) Handle(ctx context.Context, u tg.UpdatesClass) error {
	h.mu.Lock()
	inner := h.inner
	h.mu.Unlock()
	if inner != nil {
		return inner.Handle(ctx, u)
	}
	return nil
}

func (h *swappableHandler) set(inner gotd.UpdateHandler) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.inner = inner
}

// Client wraps a gotd Client and exposes only the methods needed by the
// application.
type Client struct {
	client  *gotd.Client
	phone   string
	log     *slog.Logger
	manager *updates.Manager
	handler *swappableHandler
}

// Options configures the Telegram client.
type Options struct {
	APIID    int
	APIHash  string
	Phone    string
	Sessions *auth.SessionStore
	Logger   *slog.Logger
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

	log := opts.Logger
	if log == nil {
		log = slog.New(slog.NewTextHandler(os.Stderr, nil))
	}

	handler := &swappableHandler{}

	// Create the updates manager with the swappable handler.
	manager := updates.New(updates.Config{
		Handler: handler,
	})

	c := gotd.NewClient(opts.APIID, opts.APIHash, gotd.Options{
		SessionStorage: &sessionStorage{store: opts.Sessions, phone: opts.Phone},
		UpdateHandler:  manager,
	})

	return &Client{
		client:  c,
		phone:   opts.Phone,
		log:     log,
		manager: manager,
		handler: handler,
	}, nil
}

// Run connects to Telegram and calls f when ready. The client is shut down when
// f returns. Call Run for the full lifecycle of a monitoring session.
func (c *Client) Run(ctx context.Context, f func(ctx context.Context) error) error {
	return c.client.Run(ctx, f)
}

// Authenticate ensures the current session is valid, performing the full auth
// flow (phone + code + optional 2FA) if needed.
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

// API returns the Telegram API client.
func (c *Client) API() *tg.Client {
	return c.client.API()
}

// StartUpdates wires the event handler for monitoring and starts the updates
// manager. Must be called after authentication, inside the Run callback.
func (c *Client) StartUpdates(ctx context.Context, engine *watch.Engine, chats []resolve.ChatInfo) error {
	meta := make([]chatMeta, 0, len(chats))
	for _, ci := range chats {
		if ci.Err != nil {
			continue
		}
		meta = append(meta, chatMeta{
			id:         ci.ChatID,
			title:      ci.Title,
			accessHash: ci.AccessHash,
		})
	}
	if len(meta) == 0 {
		return errors.New("no usable chats to start monitoring")
	}

	handler := newEventHandler(engine, meta, c.log)
	c.handler.set(handler)

	user, err := c.Self(ctx)
	if err != nil {
		return err
	}

	return c.manager.Run(ctx, c.client.API(), user.ID, updates.AuthOptions{
		Forget: true,
	})
}

// GetChatHistory fetches the most recent messages from a chat.
func (c *Client) GetChatHistory(ctx context.Context, chatID, accessHash int64, limit, offsetID int) ([]tg.MessageClass, error) {
	peer, err := c.inputPeer(ctx, chatID, accessHash)
	if err != nil {
		return nil, err
	}
	r := &tg.MessagesGetHistoryRequest{
		Peer:     peer,
		Limit:    limit,
		OffsetID: offsetID,
	}
	res, err := c.client.API().MessagesGetHistory(ctx, r)
	if err != nil {
		return nil, Classify(err)
	}
	switch v := res.(type) {
	case *tg.MessagesChannelMessages:
		return v.Messages, nil
	case *tg.MessagesMessages:
		return v.Messages, nil
	case *tg.MessagesMessagesSlice:
		return v.Messages, nil
	default:
		return nil, nil
	}
}

// inputPeer builds the correct InputPeer for a chat.
func (c *Client) inputPeer(ctx context.Context, chatID, accessHash int64) (tg.InputPeerClass, error) {
	if accessHash != 0 {
		return &tg.InputPeerChannel{
			ChannelID:  chatID,
			AccessHash: accessHash,
		}, nil
	}
	if chatID < 0 {
		return &tg.InputPeerChat{
			ChatID: -chatID,
		}, nil
	}
	return &tg.InputPeerChat{
		ChatID: chatID,
	}, nil
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