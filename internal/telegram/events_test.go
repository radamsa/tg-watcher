package telegram

import (
	"testing"
	"time"

	"github.com/gotd/td/tg"

	"tg-watcher/internal/message"
)

func TestPeerChatID(t *testing.T) {
	tests := []struct {
		peer tg.PeerClass
		want int64
	}{
		{&tg.PeerChannel{ChannelID: -100123}, -100123},
		{&tg.PeerChat{ChatID: -42}, -42},
		{&tg.PeerUser{UserID: 123}, 123},
		{nil, 0},
	}
	for _, tt := range tests {
		got := peerChatID(tt.peer)
		if got != tt.want {
			t.Errorf("peerChatID(%T) = %d, want %d", tt.peer, got, tt.want)
		}
	}
}

func TestExtractMessage(t *testing.T) {
	m := &tg.Message{ID: 42}
	if got := extractMessage(m); got != m {
		t.Fatal("should return *tg.Message as-is")
	}
	if got := extractMessage(&tg.MessageEmpty{}); got != nil {
		t.Fatal("should return nil for MessageEmpty")
	}
	if got := extractMessage(nil); got != nil {
		t.Fatal("should return nil for nil")
	}
}

func TestExtractUpdates(t *testing.T) {
	// *tg.Updates
	upd := &tg.UpdateNewMessage{Message: &tg.Message{ID: 1}}
	u := &tg.Updates{
		Updates: []tg.UpdateClass{upd},
		Users:   []tg.UserClass{&tg.User{ID: 100, FirstName: "Alice"}},
		Chats:   []tg.ChatClass{&tg.Channel{ID: -1001}},
	}
	ups, users, chats := extractUpdates(u)
	if len(ups) != 1 || len(users) != 1 || len(chats) != 1 {
		t.Fatalf("Updates: ups=%d users=%d chats=%d", len(ups), len(users), len(chats))
	}

	// *tg.UpdatesCombined
	u2 := &tg.UpdatesCombined{
		Updates: []tg.UpdateClass{upd},
		Users:   []tg.UserClass{&tg.User{ID: 200}},
	}
	ups, users, _ = extractUpdates(u2)
	if len(ups) != 1 || len(users) != 1 {
		t.Fatalf("UpdatesCombined: ups=%d users=%d", len(ups), len(users))
	}

	// *tg.UpdateShort
	us := &tg.UpdateShort{Update: upd}
	ups, _, _ = extractUpdates(us)
	if len(ups) != 1 {
		t.Fatalf("UpdateShort: ups=%d", len(ups))
	}

	// nil (unknown type)
	ups, _, _ = extractUpdates(nil)
	if len(ups) != 0 {
		t.Fatal("nil should return empty")
	}
}

func TestResolveMedia(t *testing.T) {
	tests := []struct {
		name  string
		media tg.MessageMediaClass
		kind  message.MediaKind
		extra string
	}{
		{"photo", &tg.MessageMediaPhoto{}, message.MediaPhoto, ""},
		{"document", &tg.MessageMediaDocument{Document: &tg.Document{}}, message.MediaDocument, ""},
		{"geo", &tg.MessageMediaGeo{}, message.MediaLocation, ""},
		{"geo_live", &tg.MessageMediaGeoLive{}, message.MediaLocation, ""},
		{"contact", &tg.MessageMediaContact{}, message.MediaContact, ""},
		{"webpage", &tg.MessageMediaWebPage{}, message.MediaNone, ""},
		{"dice", &tg.MessageMediaDice{}, message.MediaNone, ""},
		{"game", &tg.MessageMediaGame{}, message.MediaPoll, ""},
		{"nil", nil, message.MediaDocument, ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			kind, extra := resolveMedia(tt.media)
			if kind != tt.kind || extra != tt.extra {
				t.Fatalf("got %v,%q want %v,%q", kind, extra, tt.kind, tt.extra)
			}
		})
	}
}

func TestExtractUpdates_ShortMessage(t *testing.T) {
	// The short types are dropped; they are handled by converting to
	// UpdateNewMessage in the event handler, but extractUpdates returns nil.
	usm := &tg.UpdateShortMessage{
		ID:      1,
		UserID:  100,
		Message: "hi",
		Date:    int(time.Now().Unix()),
	}
	ups, _, _ := extractUpdates(usm)
	if len(ups) != 0 {
		t.Fatal("UpdateShortMessage should return empty")
	}
}

func TestNormalizeWithUserLib(t *testing.T) {
	userLib := map[int64]*tg.User{
		100: {ID: 100, FirstName: "Alice", LastName: "Smith", Username: "alice"},
	}
	m := &tg.Message{
		ID:      10,
		PeerID:  &tg.PeerChannel{ChannelID: -100123},
		FromID:  &tg.PeerUser{UserID: 100},
		Date:    int(time.Now().Unix()),
		Message: "hello",
		Out:     false,
	}
	m.Flags.Set(8) // FromID
	nm := normalizeMessage(m, userLib, "My Channel")
	if nm.MessageID != 10 {
		t.Fatalf("MessageID = %d", nm.MessageID)
	}
	if nm.ChatID != -100123 {
		t.Fatalf("ChatID = %d", nm.ChatID)
	}
	if nm.Sender != "Alice Smith (@alice)" {
		t.Fatalf("Sender = %q", nm.Sender)
	}
	if nm.Channel != "My Channel" {
		t.Fatalf("Channel = %q", nm.Channel)
	}
	if nm.Text != "hello" {
		t.Fatalf("Text = %q", nm.Text)
	}
}

func TestComposeSender(t *testing.T) {
	userLib := map[int64]*tg.User{
		1: {ID: 1, FirstName: "Bob", LastName: "", Username: "bob"},
	}

	tests := []struct {
		name      string
		msg       *tg.Message
		chatTitle string
		want      string
	}{
		{
			"post_author",
			&tg.Message{PostAuthor: "Admin"},
			"Chat",
			"Admin",
		},
		{
			"known_user",
			func() *tg.Message {
				m := &tg.Message{FromID: &tg.PeerUser{UserID: 1}}
				m.Flags.Set(8)
				return m
			}(),
			"Chat",
			"Bob (@bob)",
		},
		{
			"unknown_user",
			func() *tg.Message {
				m := &tg.Message{FromID: &tg.PeerUser{UserID: 999}}
				m.Flags.Set(8)
				return m
			}(),
			"Chat",
			"user#999",
		},
		{
			"channel_peer",
			func() *tg.Message {
				m := &tg.Message{FromID: &tg.PeerChannel{ChannelID: -1001}}
				m.Flags.Set(8)
				return m
			}(),
			"Chat",
			"channel#-1001",
		},
		{
			"chat_peer",
			func() *tg.Message {
				m := &tg.Message{FromID: &tg.PeerChat{ChatID: -42}}
				m.Flags.Set(8)
				return m
			}(),
			"Group",
			"Group",
		},
		{
			"no_from",
			&tg.Message{},
			"Chat",
			"Chat",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := composeSender(tt.msg, userLib, tt.chatTitle)
			if got != tt.want {
				t.Fatalf("got %q, want %q", got, tt.want)
			}
		})
	}
}

func TestFwdFromPeer(t *testing.T) {
	userLib := map[int64]*tg.User{
		1: {ID: 1, FirstName: "Carol"},
	}
	tests := []struct {
		name string
		fwd  tg.MessageFwdHeader
		want string
	}{
		{
			"fwd_user",
			func() tg.MessageFwdHeader {
				f := tg.MessageFwdHeader{FromID: &tg.PeerUser{UserID: 1}}
				f.Flags.Set(0) // FromID bit
				return f
			}(),
			"Carol",
		},
		{
			"fwd_unknown_user",
			func() tg.MessageFwdHeader {
				f := tg.MessageFwdHeader{FromID: &tg.PeerUser{UserID: 999}}
				f.Flags.Set(0)
				return f
			}(),
			"user#999",
		},
		{
			"fwd_channel",
			func() tg.MessageFwdHeader {
				f := tg.MessageFwdHeader{FromID: &tg.PeerChannel{ChannelID: -1001}}
				f.Flags.Set(0)
				return f
			}(),
			"channel#-1001",
		},
		{
			"fwd_post_author",
			tg.MessageFwdHeader{PostAuthor: "Original Author"},
			"Original Author",
		},
		{
			"fwd_no_info",
			tg.MessageFwdHeader{},
			"forwarded",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := fwdFromPeer(tt.fwd, userLib)
			if got != tt.want {
				t.Fatalf("got %q, want %q", got, tt.want)
			}
		})
	}
}

func TestClassifyDocument(t *testing.T) {
	tests := []struct {
		name string
		doc  *tg.MessageMediaDocument
		kind message.MediaKind
	}{
		{
			"audio",
			&tg.MessageMediaDocument{
				Document: &tg.Document{
					Attributes: []tg.DocumentAttributeClass{
						&tg.DocumentAttributeAudio{Voice: false},
					},
				},
			},
			message.MediaAudio,
		},
		{
			"voice",
			&tg.MessageMediaDocument{
				Document: &tg.Document{
					Attributes: []tg.DocumentAttributeClass{
						&tg.DocumentAttributeAudio{Voice: true},
					},
				},
			},
			message.MediaVoice,
		},
		{
			"video",
			&tg.MessageMediaDocument{
				Document: &tg.Document{
					Attributes: []tg.DocumentAttributeClass{
						&tg.DocumentAttributeVideo{RoundMessage: false},
					},
				},
			},
			message.MediaVideo,
		},
		{
			"sticker",
			&tg.MessageMediaDocument{
				Document: &tg.Document{
					Attributes: []tg.DocumentAttributeClass{
						&tg.DocumentAttributeSticker{Alt: "🐱"},
					},
				},
			},
			message.MediaSticker,
		},
		{
			"gif",
			&tg.MessageMediaDocument{
				Document: &tg.Document{
					Attributes: []tg.DocumentAttributeClass{
						&tg.DocumentAttributeAnimated{},
					},
				},
			},
			message.MediaDocument,
		},
		{
			"unknown_doc",
			&tg.MessageMediaDocument{Document: &tg.Document{Attributes: nil}},
			message.MediaDocument,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			kind, _ := classifyDocument(tt.doc)
			if kind != tt.kind {
				t.Fatalf("got %v, want %v", kind, tt.kind)
			}
		})
	}
}

func TestNormalizeWithForward(t *testing.T) {
	userLib := map[int64]*tg.User{}
	m := &tg.Message{
		ID:      5,
		PeerID:  &tg.PeerChat{ChatID: -42},
		Date:    int(time.Now().Unix()),
		Message: "cool",
	}
	m.Flags.Set(2) // FwdFrom
	m.FwdFrom = tg.MessageFwdHeader{PostAuthor: "Famous"}
	nm := normalizeMessage(m, userLib, "Group")
	if nm.Text != "[fwd: Famous]\ncool" {
		t.Fatalf("Text = %q", nm.Text)
	}
}

func TestNormalizeWithMedia(t *testing.T) {
	userLib := map[int64]*tg.User{}
	m := &tg.Message{
		ID:      6,
		PeerID:  &tg.PeerChannel{ChannelID: -1001},
		Date:    int(time.Now().Unix()),
		Message: "",
	}
	m.Flags.Set(9) // set the Media bit
	m.Media = &tg.MessageMediaPhoto{}
	nm := normalizeMessage(m, userLib, "Channel")
	if nm.Text != "[photo]" {
		t.Fatalf("Text = %q", nm.Text)
	}
}

func TestNormalizeContextFields(t *testing.T) {
	userLib := map[int64]*tg.User{
		100: {ID: 100, FirstName: "Alice", LastName: "Smith", Username: "alice", Bot: true},
	}
	m := &tg.Message{
		ID:        10,
		PeerID:    &tg.PeerChannel{ChannelID: -100123},
		Date:      1700000000,
		Message:   "hello",
		Pinned:    true,
		Mentioned: true,
	}
	m.SetFromID(&tg.PeerUser{UserID: 100})
	reply := &tg.MessageReplyHeader{}
	reply.SetReplyToMsgID(9)
	m.SetReplyTo(reply)
	m.SetEditDate(1700000100)
	m.SetMedia(&tg.MessageMediaPhoto{Photo: &tg.Photo{ID: 777}})
	fwd := tg.MessageFwdHeader{Date: 1600000000}
	fwd.SetFromID(&tg.PeerUser{UserID: 100})
	fwd.SetChannelPost(55)
	m.SetFwdFrom(fwd)

	nm := normalizeMessage(m, userLib, "Channel")

	if nm.ReplyToMessageID != 9 {
		t.Fatalf("ReplyToMessageID = %d", nm.ReplyToMessageID)
	}
	if !nm.IsEdit || !nm.IsPinned || !nm.IsMention {
		t.Fatalf("flags = edit:%v pinned:%v mention:%v", nm.IsEdit, nm.IsPinned, nm.IsMention)
	}
	if nm.SenderID != 100 || nm.SenderUsername != "alice" || !nm.SenderIsBot {
		t.Fatalf("sender = %d, %q, %v", nm.SenderID, nm.SenderUsername, nm.SenderIsBot)
	}
	if len(nm.Media) != 1 || nm.Media[0].Kind != message.MediaPhoto || nm.Media[0].FileID != 777 {
		t.Fatalf("media = %#v", nm.Media)
	}
	if nm.Forward == nil {
		t.Fatal("forward is nil")
	}
	if nm.Forward.FromID != 100 || nm.Forward.FromName != "Alice Smith" ||
		nm.Forward.FromUsername != "alice" || nm.Forward.ChannelPost != 55 ||
		nm.Forward.DateUnix != 1600000000 {
		t.Fatalf("forward = %#v", nm.Forward)
	}
}

func TestNormalizeNoContext(t *testing.T) {
	nm := normalizeMessage(&tg.Message{ID: 1, PeerID: &tg.PeerChat{ChatID: -42}}, nil, "Group")
	if nm.ReplyToMessageID != 0 || nm.IsEdit || nm.IsPinned || nm.IsMention {
		t.Fatalf("unexpected context: %#v", nm)
	}
	if nm.Media == nil || len(nm.Media) != 0 {
		t.Fatalf("Media = %#v, want empty non-nil slice", nm.Media)
	}
	if nm.Forward != nil {
		t.Fatalf("Forward = %#v, want nil", nm.Forward)
	}
}

func TestReplyToMessageID(t *testing.T) {
	m := &tg.Message{}
	if got := replyToMessageID(m); got != 0 {
		t.Fatalf("no reply = %d", got)
	}

	reply := &tg.MessageReplyHeader{}
	reply.SetReplyToMsgID(12)
	m.SetReplyTo(reply)
	if got := replyToMessageID(m); got != 12 {
		t.Fatalf("reply = %d", got)
	}

	story := &tg.Message{}
	story.SetReplyTo(&tg.MessageReplyStoryHeader{StoryID: 5})
	if got := replyToMessageID(story); got != 0 {
		t.Fatalf("story reply = %d, want 0", got)
	}
}

func TestExtractMediaRefs(t *testing.T) {
	tests := []struct {
		name  string
		media tg.MessageMediaClass
		want  []message.MediaRef
	}{
		{
			"photo",
			&tg.MessageMediaPhoto{Photo: &tg.Photo{ID: 1}},
			[]message.MediaRef{{Kind: message.MediaPhoto, FileID: 1}},
		},
		{
			"document",
			&tg.MessageMediaDocument{Document: &tg.Document{ID: 2}},
			[]message.MediaRef{{Kind: message.MediaDocument, FileID: 2}},
		},
		{
			"webpage",
			&tg.MessageMediaWebPage{Webpage: &tg.WebPage{ID: 3, URL: "https://example.com"}},
			[]message.MediaRef{{Kind: message.MediaWebPage, FileID: 3, URL: "https://example.com"}},
		},
		{
			"location",
			&tg.MessageMediaGeo{},
			[]message.MediaRef{{Kind: message.MediaLocation}},
		},
		{
			"dice",
			&tg.MessageMediaDice{},
			[]message.MediaRef{},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := &tg.Message{}
			m.SetMedia(tt.media)
			got := extractMediaRefs(m)
			if len(got) != len(tt.want) {
				t.Fatalf("got %#v, want %#v", got, tt.want)
			}
			for i := range got {
				if got[i] != tt.want[i] {
					t.Fatalf("got %#v, want %#v", got, tt.want)
				}
			}
		})
	}

	if got := extractMediaRefs(&tg.Message{}); got == nil || len(got) != 0 {
		t.Fatalf("no media = %#v, want empty non-nil slice", got)
	}
}
