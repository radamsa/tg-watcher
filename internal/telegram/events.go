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
	senderID, senderUsername, senderIsBot := senderDetails(m, userLib)

	var fwdSource string
	var forward *message.Forward
	if fwd, ok := m.GetFwdFrom(); ok {
		fwdSource = fwdFromPeer(fwd, userLib)
		forward = extractForward(fwd, userLib)
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

	isEdit := false
	if editDate, ok := m.GetEditDate(); ok && editDate != 0 {
		isEdit = true
	}

	return message.Message{
		ChatID:           chatID,
		MessageID:        m.ID,
		Date:             dt,
		Channel:          chatTitle,
		Sender:           sender,
		Text:             text,
		ReplyToMessageID: replyToMessageID(m),
		IsEdit:           isEdit,
		IsPinned:         m.Pinned,
		IsMention:        m.Mentioned,
		SenderID:         senderID,
		SenderUsername:   senderUsername,
		SenderIsBot:      senderIsBot,
		Media:            extractMediaRefs(m),
		Forward:          forward,
	}
}

// senderDetails returns the numeric author ID, username and bot flag. The
// display name is composed separately by composeSender.
func senderDetails(m *tg.Message, userLib map[int64]*tg.User) (int64, string, bool) {
	from, ok := m.GetFromID()
	if !ok {
		return 0, "", false
	}
	switch p := from.(type) {
	case *tg.PeerUser:
		if u, ok := userLib[p.UserID]; ok {
			return p.UserID, strings.TrimSpace(u.Username), u.Bot
		}
		return p.UserID, "", false
	case *tg.PeerChannel:
		return p.ChannelID, "", false
	case *tg.PeerChat:
		return p.ChatID, "", false
	}
	return 0, "", false
}

// replyToMessageID extracts the replied-to message ID, if any.
func replyToMessageID(m *tg.Message) int {
	reply, ok := m.GetReplyTo()
	if !ok {
		return 0
	}
	if h, ok := reply.(*tg.MessageReplyHeader); ok {
		if id, ok := h.GetReplyToMsgID(); ok {
			return id
		}
	}
	return 0
}

// extractForward converts the Telegram forward header into the internal
// provenance model.
func extractForward(fwd tg.MessageFwdHeader, userLib map[int64]*tg.User) *message.Forward {
	f := &message.Forward{
		FromName:    strings.TrimSpace(fwd.FromName),
		PostAuthor:  fwd.PostAuthor,
		ChannelPost: fwd.ChannelPost,
		DateUnix:    int64(fwd.Date),
	}
	if from, ok := fwd.GetFromID(); ok {
		f.FromID = peerChatID(from)
		if p, ok := from.(*tg.PeerUser); ok {
			if u, ok := userLib[p.UserID]; ok {
				f.FromUsername = strings.TrimSpace(u.Username)
				if f.FromName == "" {
					f.FromName = strings.TrimSpace(u.FirstName + " " + u.LastName)
				}
			}
		}
	}
	return f
}

// extractMediaRefs lists the attachments carried by a message. The returned
// slice is never nil.
func extractMediaRefs(m *tg.Message) []message.MediaRef {
	refs := make([]message.MediaRef, 0, 1)
	media, ok := m.GetMedia()
	if !ok {
		return refs
	}
	switch v := media.(type) {
	case *tg.MessageMediaPhoto:
		ref := message.MediaRef{Kind: message.MediaPhoto}
		if p, ok := v.Photo.(*tg.Photo); ok {
			ref.FileID = p.ID
		}
		refs = append(refs, ref)
	case *tg.MessageMediaDocument:
		kind, _ := classifyDocument(v)
		ref := message.MediaRef{Kind: kind}
		if d, ok := v.Document.(*tg.Document); ok {
			ref.FileID = d.ID
		}
		refs = append(refs, ref)
	case *tg.MessageMediaWebPage:
		if wp, ok := v.Webpage.(*tg.WebPage); ok {
			refs = append(refs, message.MediaRef{Kind: message.MediaWebPage, FileID: wp.ID, URL: wp.URL})
		}
	case *tg.MessageMediaGeo, *tg.MessageMediaGeoLive:
		refs = append(refs, message.MediaRef{Kind: message.MediaLocation})
	case *tg.MessageMediaContact:
		refs = append(refs, message.MediaRef{Kind: message.MediaContact})
	case *tg.MessageMediaPoll:
		refs = append(refs, message.MediaRef{Kind: message.MediaPoll})
	case *tg.MessageMediaGame:
		refs = append(refs, message.MediaRef{Kind: message.MediaPoll})
	case *tg.MessageMediaDice:
		// Dice have no downloadable media.
	default:
		refs = append(refs, message.MediaRef{Kind: message.MediaDocument})
	}
	return refs
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
