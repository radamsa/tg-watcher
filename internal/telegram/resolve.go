package telegram

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/gotd/td/tg"

	"tg-watcher/internal/resolve"
)

// Resolve resolves a list of chat identifiers to transport-independent
// ChatInfo structs. It fetches the dialog list and, for @username / t.me
// targets, uses contacts.ResolveUsername. Targets not found among the user's
// accessible conversations are returned with their Err field set.
func (c *Client) Resolve(ctx context.Context, targets []resolve.Target, log *slog.Logger) []resolve.ChatInfo {
	dialogMap := c.fetchDialogs(ctx)
	results := make([]resolve.ChatInfo, 0, len(targets))
	for _, t := range targets {
		results = append(results, c.resolveOne(ctx, t, dialogMap, log))
	}
	return results
}

func (c *Client) resolveOne(ctx context.Context, t resolve.Target, dialogs map[int64]chatInfo, log *slog.Logger) resolve.ChatInfo {
	id := t.NumericID

	switch t.Kind {
	case resolve.TargetNumeric, resolve.TargetTmeCLink:
		if info, ok := dialogs[id]; ok {
			return info.toChatInfo()
		}
		// Negative group ID (bare negative form). The map may have the
		// positive form for groups.
		if id < 0 {
			if info, ok := dialogs[-id]; ok {
				return info.toChatInfo()
			}
		}
		log.Warn("chat not found among your dialogs; it may be inaccessible or never joined",
			"id", id, "original", t.Original)
		return resolve.ChatInfo{ChatID: id, Err: fmt.Errorf("chat %d not found among your conversations", id)}

	case resolve.TargetUsername, resolve.TargetTmeLink:
		username := t.Username
		if username == "" {
			return resolve.ChatInfo{Err: fmt.Errorf("empty username for %s", t.Original)}
		}
		r, err := c.client.API().ContactsResolveUsername(ctx, &tg.ContactsResolveUsernameRequest{Username: username})
		if err != nil {
			return resolve.ChatInfo{
				Err: fmt.Errorf("resolve %q: %w", username, Classify(err)),
			}
		}
		return resolveFromResolveResult(username, r)

	default:
		return resolve.ChatInfo{Err: fmt.Errorf("unsupported target kind")}
	}
}

type chatInfo struct {
	id         int64
	accessHash int64
	title      string
	isChannel  bool
	isGroup    bool
}

func (ci chatInfo) toChatInfo() resolve.ChatInfo {
	return resolve.ChatInfo{
		ChatID:     ci.id,
		AccessHash: ci.accessHash,
		Title:      ci.title,
	}
}

func (c *Client) fetchDialogs(ctx context.Context) map[int64]chatInfo {
	const limit = 200
	r := &tg.MessagesGetDialogsRequest{
		Limit:      limit,
		OffsetPeer: &tg.InputPeerEmpty{},
	}
	dialogs, err := c.client.API().MessagesGetDialogs(ctx, r)
	if err != nil {
		c.log.Warn("failed to fetch dialogs for chat resolution, only @username targets will be available",
			"error", Classify(err))
		return nil
	}

	chatMap := make(map[int64]chatInfo)

	// Handle both MessagesDialogs and MessagesDialogsSlice.
	switch v := dialogs.(type) {
	case *tg.MessagesDialogs:
		buildChatMap(v.Chats, chatMap)
	case *tg.MessagesDialogsSlice:
		buildChatMap(v.Chats, chatMap)
	}

	// For groups, also store under the negative form so that bare numeric
	// IDs (e.g. -123) resolve quickly.
	for id, ci := range chatMap {
		if ci.isGroup && id > 0 {
			chatMap[-id] = ci
		}
	}
	return chatMap
}

func buildChatMap(chats []tg.ChatClass, m map[int64]chatInfo) {
	for _, chatClass := range chats {
		switch chat := chatClass.(type) {
		case *tg.Channel:
			t := chat.Title
			if t == "" {
				t = chat.Username
			}
			m[chat.ID] = chatInfo{
				id:         chat.ID,
				accessHash: chat.AccessHash,
				title:      t,
				isChannel:  true,
			}
		case *tg.Chat:
			m[chat.ID] = chatInfo{
				id:      chat.ID,
				title:   chat.Title,
				isGroup: true,
			}
		case *tg.ChatForbidden:
			m[chat.ID] = chatInfo{
				id:    chat.ID,
				title: chat.Title,
			}
		case *tg.ChannelForbidden:
			m[chat.ID] = chatInfo{
				id:         chat.ID,
				accessHash: chat.AccessHash,
				title:      chat.Title,
				isChannel:  true,
			}
		}
	}
}

func resolveFromResolveResult(username string, r *tg.ContactsResolvedPeer) resolve.ChatInfo {
	for _, chatClass := range r.Chats {
		switch chat := chatClass.(type) {
		case *tg.Channel:
			return resolve.ChatInfo{
				ChatID:     chat.ID,
				AccessHash: chat.AccessHash,
				Title:      chat.Title,
			}
		case *tg.Chat:
			return resolve.ChatInfo{
				ChatID: chat.ID,
				Title:  chat.Title,
			}
		}
	}
	for _, userClass := range r.Users {
		if user, ok := userClass.(*tg.User); ok && !user.Bot {
			return resolve.ChatInfo{
				ChatID: user.ID,
				Title: func() string {
					n := user.FirstName
					if user.LastName != "" {
						n += " " + user.LastName
					}
					return n
				}(),
			}
		}
	}
	if p := r.Peer; p != nil {
		switch peer := p.(type) {
		case *tg.PeerChannel:
			return resolve.ChatInfo{ChatID: peer.ChannelID}
		case *tg.PeerChat:
			return resolve.ChatInfo{ChatID: peer.ChatID}
		case *tg.PeerUser:
			return resolve.ChatInfo{ChatID: peer.UserID}
		}
	}
	return resolve.ChatInfo{Err: fmt.Errorf("could not resolve @%s", username)}
}