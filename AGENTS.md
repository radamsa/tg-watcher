# AGENTS.md

## Repo state
- Planning-only repository: only `docs/` exists. No Go module, source, CI, or git repo yet. Do not assume build/test commands work until code is added.
- Planning docs are written in Russian.

## Source of truth
- `docs/tg-watcher-plan-revised.md` is the authoritative spec (revised 2026-11-23). Read it fully before implementing.
- `docs/tg-watcher-plan.md` is the superseded first draft. Do not implement from it. It conflicts with the revised plan on:
  - `--history N` / pre-start history: removed. Monitoring starts at `READY`; messages before the per-chat baseline are never printed.
  - `formats.<name>.type` / `fields` and a separate `jsonl` formatter: replaced by one template engine. `jsonl` is just a named template using `{field|json}`.
  - `{sender}`: added to the placeholder set.
  - Gap handling: revised plan adds the `[watch]` poller (`messages.getHistory`, 60s default, 30s minimum) and `OnChannelGap` backfill.
  - Package layout: revised plan adds `internal/output`, `internal/logging`, and confines gotd types to `internal/telegram`.
- Plan map: §4 CLI/exit codes, §5 auth/session security, §6 config, §7 templates, §9 architecture/READY/dedup, §11 tests, §12 stages, §13 build/CI.

## Non-obvious invariants
- Exit codes: `0` normal (including EPIPE/SIGPIPE), `1` runtime error, `2` args/config, `3` auth, `4` no usable chats.
- `stdout` carries only formatted message records; all diagnostics go to `stderr`. A single writer serializes stdout, with a bounded queue and backpressure — never silent drops.
- Nothing may reach `stdout` before `READY`. Each chat's baseline comes from `messages.getHistory(limit=1)` just before `READY`; restarts intentionally create a new baseline.
- All monitoring state (baselines, cursors, dedup, update state) is in-memory only. The auth session file is the only persistence; do not add a DB or saved cursors.
- gotd generated types may appear only in `internal/telegram`; other packages use the internal `message.Message` model.
- Use `gotd/td/telegram/updates.Manager` with in-memory state, update middleware, and `gaps.Run` after auth. A plain `UpdateHandler` callback is not sufficient for gap recovery.
- Dedup key is `(ChatID, MessageID)`, shared by the update stream and poller. Processing per chat is serialized; polled/backfilled pages are sorted ascending before output. No global cross-chat ordering guarantee.
- Never set the poll interval below 30s; honor `FLOOD_WAIT` and use per-chat backoff.
- Session dir `0700`, session file `0600`. Never log `api_hash`, confirmation codes, the 2FA password, session contents, or full message text.
- `-100…` chat IDs must parse as positional arguments, and `--` must terminate flag parsing (both are required test cases).

## Commands (once the module exists)
- Build: `go build ./...`
- Tests: `go test ./...`; race detector: `go test -race ./...`
- Single test: `go test ./internal/<pkg> -run TestName`
- Vet: `go vet ./...`
- No Makefile, CI, or linter config exists yet. Plan §13 requires CI to run test, race, vet, static analysis, and dependency vulnerability checks.

## Scope discipline
- Follow the staged order in plan §12.
- Do not pull in plan §15 items (sender/keyword filters, `--all`, TUI, dynamic chat addition, per-chat templates, etc.) without an explicit request.
