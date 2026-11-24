package telegram

import (
	"context"
	"fmt"
	"log/slog"
	"strconv"
	"strings"
	"time"

	"github.com/gotd/td/tg"

	"tg-watcher/internal/message"
	"tg-watcher/internal/watch"
)

// eventHandler adapts gotd update streams to the application's engine.
type eventHandler struct {
	engine  *watch.Engine
	chats   map[int64]chatMeta
	userLib map[int64]*tg.User
	log     *slog.Logger
}

type chatMeta struct {
	id         int64
	title      string
	accessHash int64
}

func newEventHandler(engine *watch.Engine, chats []chatMeta, log *slog.Logger) *eventHandler {
	h := &eventHandler{
		engine:  engine,
		chats:   make(map[int64]chatMeta, len(chats)),
		userLib: make(map[int64]*tg.User),
		log:     log,
	}
	for _, c := range chats {
		h.chats[c.id] = c
	}
	return h
}

func (h *eventHandler) Handle(ctx context.Context, u tg.UpdatesClass) error {
	updates, users, _ := extractUpdates(u)
	for _, user := range users {
		if u, ok := user.(*tg.User); ok {
			h.userLib[u.ID] = u
		}
	}
	for _, upd := range updates {
		h.handleOne(ctx, upd)
	}
	return nil
}

func (h *eventHandler) handleOne(ctx context.Context, upd tg.UpdateClass) {
	var msg *tg.Message
	switch u := upd.(type) {
	case *tg.UpdateNewMessage:
		msg = extractMessage(u.Message)
	case *tg.UpdateNewChannelMessage:
		msg = extractMessage(u.Message)
	case *tg.UpdateEditMessage:
		msg = extractMessage(u.Message)
	case *tg.UpdateEditChannelMessage:
		msg = extractMessage(u.Message)
	default:
		return
	}
	if msg == nil {
		return
	}

	chat := peerChatID(msg.PeerID)
	meta, tracked := h.chats[chat]
	if !tracked && chat > 0 {
		meta, tracked = h.chats[-chat]
	}
	if !tracked {
		return
	}

	normalized := normalizeMessage(msg, h.userLib, meta.title)
	_ = h.engine.Ingest(ctx, chat, normalized)
}

func normalizeMessage(m *tg.Message, userLib map[int64]*tg.User, chatTitle string) message.Message {
	dt := time.Unix(int64(m.Date), 0)
	chatID := peerChatID(m.PeerID)

	sender := composeSender(m, userLib, chatTitle)

	var fwdSource string
	if fwd, ok := m.GetFwdFrom(); ok {
		fwdSource = fwdFromPeer(fwd, userLib)
	}

	mediaKind := message.MediaNone
	body := m.Message
	if media, ok := m.GetMedia(); ok {
		kind, extra := resolveMedia(media)
		mediaKind = kind
		if extra != "" {
			body = extra
		}
	}

	text := message.ComposeText(mediaKind, body, fwdSource)
	if text == "" {
		text = body
	}

	return message.Message{
		ChatID:    chatID,
		MessageID: m.ID,
		Date:      dt,
		Channel:   chatTitle,
		Sender:    sender,
		Text:      text,
	}
}

func composeSender(m *tg.Message, userLib map[int64]*tg.User, chatTitle string) string {
	if m.PostAuthor != "" {
		return m.PostAuthor
	}
	if from, ok := m.GetFromID(); ok {
		switch p := from.(type) {
		case *tg.PeerUser:
			if u, ok2 := userLib[p.UserID]; ok2 {
				user := &message.User{
					FirstName: u.FirstName,
					LastName:  u.LastName,
					Username:  u.Username,
				}
				return message.ComposeSender(user, "", "")
			}
			return "user#" + strconv.FormatInt(p.UserID, 10)
		case *tg.PeerChannel:
			return fmt.Sprintf("channel#%d", p.ChannelID)
		case *tg.PeerChat:
			return chatTitle
		}
	}
	return chatTitle
}

func fwdFromPeer(fwd tg.MessageFwdHeader, userLib map[int64]*tg.User) string {
	if from, ok := fwd.GetFromID(); ok {
		switch p := from.(type) {
		case *tg.PeerUser:
			if u, ok2 := userLib[p.UserID]; ok2 {
				name := strings.TrimSpace(u.FirstName + " " + u.LastName)
				return name
			}
			return "user#" + strconv.FormatInt(p.UserID, 10)
		default:
			return fmt.Sprintf("channel#%d", peerChatID(from))
		}
	}
	if fwd.PostAuthor != "" {
		return fwd.PostAuthor
	}
	return "forwarded"
}

func resolveMedia(media tg.MessageMediaClass) (message.MediaKind, string) {
	switch m := media.(type) {
	case *tg.MessageMediaPhoto:
		return message.MediaPhoto, ""
	case *tg.MessageMediaDocument:
		return classifyDocument(m)
	case *tg.MessageMediaGeo, *tg.MessageMediaGeoLive:
		return message.MediaLocation, ""
	case *tg.MessageMediaContact:
		return message.MediaContact, ""
	case *tg.MessageMediaPoll:
		return message.MediaPoll, m.Poll.Question.Text
	case *tg.MessageMediaGame:
		return message.MediaPoll, ""
	case *tg.MessageMediaWebPage:
		return message.MediaNone, ""
	case *tg.MessageMediaDice:
		return message.MediaNone, ""
	default:
		return message.MediaDocument, ""
	}
}

func classifyDocument(media *tg.MessageMediaDocument) (message.MediaKind, string) {
	doc, ok := media.Document.(*tg.Document)
	if !ok {
		return message.MediaDocument, ""
	}
	for _, attr := range doc.Attributes {
		switch a := attr.(type) {
		case *tg.DocumentAttributeAudio:
			if a.Voice {
				return message.MediaVoice, ""
			}
			return message.MediaAudio, ""
		case *tg.DocumentAttributeVideo:
			return message.MediaVideo, ""
		case *tg.DocumentAttributeSticker:
			return message.MediaSticker, a.Alt
		case *tg.DocumentAttributeAnimated:
			return message.MediaDocument, ""
		}
	}
	return message.MediaDocument, ""
}

// extractUpdates breaks down a tg.UpdatesClass into its constituent updates,
// users and chats.
func extractUpdates(u tg.UpdatesClass) (updates []tg.UpdateClass, users []tg.UserClass, chats []tg.ChatClass) {
	switch v := u.(type) {
	case *tg.Updates:
		return v.Updates, v.Users, v.Chats
	case *tg.UpdatesCombined:
		return v.Updates, v.Users, v.Chats
	case *tg.UpdateShort:
		return []tg.UpdateClass{v.Update}, nil, nil
	default:
		return nil, nil, nil
	}
}

// extractMessage extracts a *tg.Message from a tg.MessageClass.
func extractMessage(mc tg.MessageClass) *tg.Message {
	if m, ok := mc.(*tg.Message); ok {
		return m
	}
	return nil
}

// peerChatID extracts the chat identifier from a PeerClass.
func peerChatID(p tg.PeerClass) int64 {
	switch v := p.(type) {
	case *tg.PeerChannel:
		return v.ChannelID
	case *tg.PeerChat:
		return v.ChatID
	case *tg.PeerUser:
		return v.UserID
	}
	return 0
}