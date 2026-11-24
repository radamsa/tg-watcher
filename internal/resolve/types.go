package resolve

// ChatInfo holds the resolved data for one chat. It is transport-independent;
// gotd types are never exposed through this struct.
type ChatInfo struct {
	// ChatID is the Telegram chat identifier.
	ChatID int64
	// AccessHash is required for channels and supergroups.
	AccessHash int64
	// Title is the display name of the chat.
	Title string
	// Err is set when the chat is inaccessible.
	Err error
}