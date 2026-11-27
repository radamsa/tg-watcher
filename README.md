# tg-watcher

Print new Telegram messages from specified chats to stdout, using a user account (not a bot). Ideal for piping into scripts, logging systems, or notification pipelines.

## Features

- **Monitor multiple chats** — channels, groups, and user DMs
- **Two sources** — live updates via MTProto + periodic backfill poller
- **Deduplicated** — messages are never printed twice, even when the poller catches up
- **Template output** — built-in `text`, `jsonl`, and context-rich `event` formats, plus custom templates
- **Single binary** — Go, no runtime dependencies, cross-platform

## Quick start

### 1. Get API credentials

tg-watcher uses the **Telegram client API** (not a bot token). You need `api_id` and `api_hash`:

1. Go to https://my.telegram.org/apps and log in with your Telegram account
2. If you haven't created an app yet, fill in the form (any name, platform, description)
3. Copy the **App api_id** (an integer) and **App api_hash** (a hex string)
4. ⚠️ Never share these — they identify your app to Telegram

### 2. Install

```shell
# From source
go install tg-watcher/cmd/tg-watcher@latest

# Or download a release from GitHub Releases (once available)
```

### 3. Configure

Create `~/.tg-watcher/config.toml`:

```toml
[telegram]
api_id = 12345            # from my.telegram.org/apps
api_hash = "abcdef..."    # from my.telegram.org/apps

[watch]
poll_interval = "60s"     # how often to backfill (min 30s)
```

Or set environment variables (override the config file):

```shell
export TG_API_ID=12345
export TG_API_HASH=abcdef...
```

### 4. Run

```shell
tg-watcher +79123456789 @channel @group --config ~/.tg-watcher/config.toml
```

The first run will:
1. Prompt for the confirmation code sent to Telegram
2. Prompt for the 2FA password (if enabled)
3. Save the session to `~/.tg-watcher/sessions/` for future runs

After successful auth, `READY` is logged to stderr and new messages appear on stdout:

```
2024-01-15 10:30:01 [My Channel] [Alice (@alice)] Hello from Telegram!
2024-01-15 10:30:02 [My Channel] [Bob] [photo]
```

### 5. Pipe somewhere

```shell
tg-watcher +79123456789 @alerts | jq -R 'fromjson? | .text' | grep -i error
```

## Usage

```
tg-watcher <phone> [--format NAME] [--config PATH] [--password PWD]
            [--version] [--help] [--] <chat>...
```

| Argument | Description |
|----------|-------------|
| `phone` | International phone number, e.g. `+31612345678` |
| `chat` | One or more targets: `-1001234567890`, `@username`, `https://t.me/channel` |
| `--` | Stop flag parsing (required if the first chat is a negative numeric ID) |

| Flag | Description |
|------|-------------|
| `--format NAME` | Output template name (default: from config or `"text"`) |
| `--config PATH` | TOML config file (default: `~/.tg-watcher/config.toml`) |
| `--password PWD` | 2FA password. Prefer `TG_PASSWORD` or the interactive prompt |
| `--mentions-only` | Only print messages that mention your account |
| `--from SENDER` | Only print messages from a sender (ID, @username, or name substring). Repeatable |
| `--match REGEX` | Only print messages whose text matches REGEX. Repeatable |
| `--all` | With multiple `--match` patterns, require every pattern to match |
| `--template-per-chat` | Enable per-chat template selection from `[chat_formats]` config |
| `--version` | Print version and exit |
| `--help` | Print help and exit |

### Exit codes

| Code | Meaning |
|------|---------|
| 0 | Normal exit (including SIGPIPE/EPIPE) |
| 1 | Runtime error (network, protocol, internal) |
| 2 | Argument or configuration error |
| 3 | Authentication failed |
| 4 | All specified chats are inaccessible |

### Chat targets

- **Numeric ID**: `-1001234567890` (use `--` before negative numbers)
- **@username**: `@telegram`
- **t.me link**: `https://t.me/telegram` or `t.me/c/12345`
- ❌ Invite links (`t.me/joinchat/...`, `t.me/+...`) are not supported

## Configuration

Default location: `~/.tg-watcher/config.toml` (missing file = built-in defaults, except `api_id` and `api_hash` are required).

```toml
default_format = "text"

[telegram]
api_id = 12345
api_hash = "abcdef1234567890abcdef1234567890"

[watch]
poll_interval = "60s"
poll_page_size = 100

[formats]
  [formats.text]
  template = "[{datetime}] [{channel}] [{sender}] {text}"
  datetime_format = "2006-01-02 15:04:05"
  timezone = "local"
  multiline_indent = 4

  [formats.jsonl]
  template = '{"datetime":{datetime|json},"channel":{channel|json},"sender":{sender|json},"text":{text|json}}'
  datetime_format = "2006-01-02T15:04:05Z07:00"
  timezone = "local"
```

The built-in `event` format is a superset of `jsonl`. It adds IDs, reply/edit/
pin/mention flags, sender details, media references, and forward provenance:

```toml
  [formats.event]
  template = '{"chat_id":{chat_id},"message_id":{message_id},"reply_to_message_id":{reply_to_message_id},"datetime":{datetime|json},"date_unix":{date_unix},"channel":{channel|json},"sender":{sender|json},"sender_id":{sender_id},"sender_username":{sender_username|json},"sender_is_bot":{sender_is_bot},"text":{text|json},"is_edit":{is_edit},"is_pinned":{is_pinned},"is_mention":{is_mention},"media":{media|json},"forward":{forward|json}}'
  datetime_format = "2006-01-02T15:04:05Z07:00"
  timezone = "local"
```

### Environment overrides

| Variable | Overrides |
|----------|-----------|
| `TG_API_ID` | `telegram.api_id` |
| `TG_API_HASH` | `telegram.api_hash` |
| `TG_PASSWORD` | 2FA password (overrides the interactive prompt) |

### Built-in formats

- **`text`**: `[2024-01-15 10:30:01] [My Channel] [Alice] Hello!`
- **`jsonl`**: `{"datetime":"2024-01-15T10:30:01Z","channel":"My Channel","sender":"Alice","text":"Hello!"}`

### Template placeholders

| Placeholder | Description |
|-------------|-------------|
| `{datetime}` | Message date/time (configurable format and timezone) |
| `{channel}` | Chat title |
| `{sender}` | Author name (with @username when known) |
| `{text}` | Message body with media markers and forward prefix |
| `{chat_id}` | Telegram chat ID |
| `{message_id}` | Message ID within the chat |
| `{reply_to_message_id}` | Replied-to message ID, or `0` |
| `{date_unix}` | Message date as a Unix timestamp |
| `{is_edit}` | `true` when Telegram marked the message as edited |
| `{is_pinned}` | `true` when the message is pinned |
| `{is_mention}` | `true` when the message mentions your account |
| `{sender_id}` | Author's Telegram user ID, or `0` |
| `{sender_username}` | Author's username without `@`, or empty |
| `{sender_is_bot}` | `true` when the author is a bot |
| `{media}` | JSON array of attachments (`kind`, `file_id`, `url`) |
| `{forward}` | JSON object with forward provenance, or `null` |

Use `{field|json}` to JSON-encode any field. Use `{{` / `}}` for literal braces.
Numeric and boolean fields are emitted as JSON literals; `{media}` and
`{forward}` are always JSON so they stay machine-readable.

## Filters

Messages can be filtered before output. All active filters must pass (AND):

- `--mentions-only` keeps messages that mention your account.
- `--from SENDER` keeps messages whose sender matches (repeatable, OR). The
  value is a numeric user ID, an @username (without the `@`), or a
  case-insensitive substring of the display name.
- `--match REGEX` keeps messages whose text matches the regex (repeatable).
  By default any one pattern is enough; combine with `--all` to require every
  pattern.

Example — print only errors from the admin:

```shell
tg-watcher +31612345678 @alerts --from @admin --match "(?i)error|critical"
```

## Per-chat templates

When `--template-per-chat` is active, each chat can use a different output
template. Add a `[chat_formats]` section to the config:

```toml
[chat_formats]
"@news" = "jsonl"
"-1001234567890" = "event"
"My Private Group" = "text"
```

Keys are matched in this order: numeric ID, @username, exact title (all
case-insensitive). The format name must be defined in `[formats]`.

## Architecture

```
CLI args ──► cli.Parse() ──► config.Load() ──► app.Run()
                                                 │
                                   ┌─────────────┤
                                   ▼             ▼
                           auth.Flow      resolve.Chats
                                   │             │
                                   ▼             ▼
                            updates.Manager    baseline
                            + poller        (getHistory)
                                   │             │
                                   └──────┬──────┘
                                          ▼
                                   engine.Engine
                                   (dedup) ──► output.Writer
                                                 │
                                                 ▼
                                               stdout
```

All diagnostics, logs, version and help go to **stderr**. Only formatted message records appear on **stdout**.

## Requirements

- Go 1.26+ (toolchain auto-downloads)
- Telegram user account (not a bot)
- API credentials from https://my.telegram.org/apps

## Development

```shell
git clone https://github.com/your/tg-watcher
cd tg-watcher
go build ./...
go test -race ./...
```

## License

MIT