// Package config loads, defaults and validates the TOML configuration. Every
// problem found here is a configuration error (exit code 2) and must be
// reported before connecting to Telegram. See plan §6, §7.
package config

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/pelletier/go-toml/v2"

	"tg-watcher/internal/format"
)

// Built-in template names.
const (
	BuiltinText  = "text"
	BuiltinJSONL = "jsonl"
	BuiltinEvent = "event"
)

const (
	defaultDatetimeFormat = "2006-01-02 15:04:05"
	defaultJSONLayout     = "2006-01-02T15:04:05Z07:00"
	defaultPollInterval   = 60 * time.Second
	minPollInterval       = 30 * time.Second
	defaultPollPageSize   = 100
	maxPollPageSize       = 100
)

// Telegram holds account API credentials.
type Telegram struct {
	APIID   int    `toml:"api_id"`
	APIHash string `toml:"api_hash"`
}

// Watch holds monitoring settings for the periodic history poller.
type Watch struct {
	PollInterval Duration `toml:"poll_interval"`
	PollPageSize int      `toml:"poll_page_size"`
}

// Format is one named output template.
type Format struct {
	Template        string `toml:"template"`
	DatetimeFormat  string `toml:"datetime_format"`
	Timezone        string `toml:"timezone"`
	MultilineIndent int    `toml:"multiline_indent"`
}

// Config is the fully validated configuration.
type Config struct {
	DefaultFormat string
	Telegram      Telegram
	Watch         Watch
	Formats       map[string]Format

	templates map[string]*format.Template
}

// Duration is a TOML-friendly time.Duration encoded as a Go duration string,
// e.g. "60s".
type Duration time.Duration

func (d *Duration) UnmarshalText(b []byte) error {
	v, err := time.ParseDuration(string(b))
	if err != nil {
		return fmt.Errorf("invalid duration %q: %w", string(b), err)
	}
	*d = Duration(v)
	return nil
}

func (d Duration) MarshalText() ([]byte, error) {
	return []byte(time.Duration(d).String()), nil
}

// Std returns the standard library duration.
func (d Duration) Std() time.Duration { return time.Duration(d) }

// DefaultPath returns ~/.tg-watcher/config.toml.
func DefaultPath() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("locate home directory: %w", err)
	}
	return filepath.Join(home, ".tg-watcher", "config.toml"), nil
}

// Defaults returns the built-in configuration: poll every 60s, pages of 100,
// and the built-in "text" and "jsonl" templates.
func Defaults() *Config {
	return &Config{
		DefaultFormat: BuiltinText,
		Watch: Watch{
			PollInterval: Duration(defaultPollInterval),
			PollPageSize: defaultPollPageSize,
		},
		Formats: map[string]Format{
			BuiltinText: {
				Template:        "[{datetime}] [{channel}] [{sender}] {text}",
				DatetimeFormat:  defaultDatetimeFormat,
				Timezone:        "local",
				MultilineIndent: 4,
			},
			BuiltinJSONL: {
				Template:       `{"datetime":{datetime|json},"channel":{channel|json},"sender":{sender|json},"text":{text|json}}`,
				DatetimeFormat: defaultJSONLayout,
				Timezone:       "local",
			},
			BuiltinEvent: {
				Template: `{"chat_id":{chat_id},"message_id":{message_id},"reply_to_message_id":{reply_to_message_id},` +
					`"datetime":{datetime|json},"date_unix":{date_unix},"channel":{channel|json},` +
					`"sender":{sender|json},"sender_id":{sender_id},"sender_username":{sender_username|json},` +
					`"sender_is_bot":{sender_is_bot},"text":{text|json},"is_edit":{is_edit},` +
					`"is_pinned":{is_pinned},"is_mention":{is_mention},"media":{media|json},"forward":{forward|json}}`,
				DatetimeFormat: defaultJSONLayout,
				Timezone:       "local",
			},
		},
	}
}

// raw* types mirror the TOML file while distinguishing "absent" from "zero".
type rawConfig struct {
	DefaultFormat *string              `toml:"default_format"`
	Telegram      *rawTelegram         `toml:"telegram"`
	Watch         *rawWatch            `toml:"watch"`
	Formats       map[string]rawFormat `toml:"formats"`
}

type rawTelegram struct {
	APIID   *int    `toml:"api_id"`
	APIHash *string `toml:"api_hash"`
}

type rawWatch struct {
	PollInterval *Duration `toml:"poll_interval"`
	PollPageSize *int      `toml:"poll_page_size"`
}

type rawFormat struct {
	Template        *string `toml:"template"`
	DatetimeFormat  *string `toml:"datetime_format"`
	Timezone        *string `toml:"timezone"`
	MultilineIndent *int    `toml:"multiline_indent"`
}

// Load reads the configuration from path. An empty path means the default
// location; a missing default file is not an error. An explicitly provided path
// must exist. Environment variables TG_API_ID and TG_API_HASH are applied last.
func Load(path string) (*Config, error) {
	cfg := Defaults()

	explicit := path != ""
	if !explicit {
		var err error
		path, err = DefaultPath()
		if err != nil {
			return nil, err
		}
	}

	data, err := os.ReadFile(path)
	switch {
	case err == nil:
		if err := cfg.merge(data); err != nil {
			return nil, fmt.Errorf("config %s: %w", path, err)
		}
	case errors.Is(err, os.ErrNotExist) && !explicit:
		// Built-in defaults apply.
	default:
		return nil, fmt.Errorf("read config %s: %w", path, err)
	}

	if err := cfg.applyEnv(); err != nil {
		return nil, err
	}
	if err := cfg.Validate(); err != nil {
		return nil, err
	}
	return cfg, nil
}

// Parse decodes TOML from data on top of the defaults. Used by tests and by
// Load.
func Parse(data []byte) (*Config, error) {
	cfg := Defaults()
	if err := cfg.merge(data); err != nil {
		return nil, err
	}
	if err := cfg.applyEnv(); err != nil {
		return nil, err
	}
	if err := cfg.Validate(); err != nil {
		return nil, err
	}
	return cfg, nil
}

func (c *Config) merge(data []byte) error {
	var raw rawConfig
	dec := toml.NewDecoder(strings.NewReader(string(data)))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&raw); err != nil {
		var derr *toml.DecodeError
		if errors.As(err, &derr) {
			return errors.New(derr.String())
		}
		var strict *toml.StrictMissingError
		if errors.As(err, &strict) {
			return errors.New(strict.String())
		}
		return err
	}

	if raw.DefaultFormat != nil {
		c.DefaultFormat = *raw.DefaultFormat
	}
	if raw.Telegram != nil {
		if raw.Telegram.APIID != nil {
			c.Telegram.APIID = *raw.Telegram.APIID
		}
		if raw.Telegram.APIHash != nil {
			c.Telegram.APIHash = *raw.Telegram.APIHash
		}
	}
	if raw.Watch != nil {
		if raw.Watch.PollInterval != nil {
			c.Watch.PollInterval = *raw.Watch.PollInterval
		}
		if raw.Watch.PollPageSize != nil {
			c.Watch.PollPageSize = *raw.Watch.PollPageSize
		}
	}
	for name, rf := range raw.Formats {
		base, ok := c.Formats[name]
		if !ok {
			base = Format{DatetimeFormat: defaultDatetimeFormat, Timezone: "local"}
		}
		if rf.Template != nil {
			base.Template = *rf.Template
		}
		if rf.DatetimeFormat != nil {
			base.DatetimeFormat = *rf.DatetimeFormat
		}
		if rf.Timezone != nil {
			base.Timezone = *rf.Timezone
		}
		if rf.MultilineIndent != nil {
			base.MultilineIndent = *rf.MultilineIndent
		}
		c.Formats[name] = base
	}
	return nil
}

func (c *Config) applyEnv() error {
	if v, ok := os.LookupEnv("TG_API_ID"); ok && v != "" {
		id, err := strconv.Atoi(v)
		if err != nil {
			return fmt.Errorf("TG_API_ID: invalid integer %q", v)
		}
		c.Telegram.APIID = id
	}
	if v, ok := os.LookupEnv("TG_API_HASH"); ok && v != "" {
		c.Telegram.APIHash = v
	}
	return nil
}

// Validate checks every constraint and compiles all templates. After Validate
// succeeds, templates are ready for rendering.
func (c *Config) Validate() error {
	if c.Telegram.APIID <= 0 {
		return errors.New("telegram.api_id must be a positive integer (set it in the config or TG_API_ID)")
	}
	if strings.TrimSpace(c.Telegram.APIHash) == "" {
		return errors.New("telegram.api_hash must be set (in the config or TG_API_HASH)")
	}

	if c.Watch.PollInterval.Std() < minPollInterval {
		return fmt.Errorf("watch.poll_interval must be at least %s, got %s", minPollInterval, c.Watch.PollInterval.Std())
	}
	if c.Watch.PollPageSize < 1 || c.Watch.PollPageSize > maxPollPageSize {
		return fmt.Errorf("watch.poll_page_size must be between 1 and %d, got %d", maxPollPageSize, c.Watch.PollPageSize)
	}

	if c.DefaultFormat == "" {
		c.DefaultFormat = BuiltinText
	}
	if _, ok := c.Formats[c.DefaultFormat]; !ok {
		return fmt.Errorf("default_format %q is not defined", c.DefaultFormat)
	}

	compiled := make(map[string]*format.Template, len(c.Formats))
	for name, spec := range c.Formats {
		pretty := fmt.Sprintf("formats.%s", name)
		if strings.TrimSpace(spec.Template) == "" {
			return fmt.Errorf("%s.template is required", pretty)
		}
		if spec.DatetimeFormat == "" {
			return fmt.Errorf("%s.datetime_format is required", pretty)
		}
		if spec.MultilineIndent < 0 {
			return fmt.Errorf("%s.multiline_indent must not be negative", pretty)
		}

		loc, err := resolveLocation(spec.Timezone)
		if err != nil {
			return fmt.Errorf("%s.timezone: %w", pretty, err)
		}

		tmpl, err := format.Compile(format.Spec{
			Template:        spec.Template,
			DatetimeFormat:  spec.DatetimeFormat,
			Location:        loc,
			MultilineIndent: spec.MultilineIndent,
		})
		if err != nil {
			return fmt.Errorf("%s.template: %w", pretty, err)
		}
		compiled[name] = tmpl
	}
	c.templates = compiled
	return nil
}

// resolveLocation interprets "local", "UTC" or an IANA zone name.
func resolveLocation(name string) (*time.Location, error) {
	switch strings.TrimSpace(name) {
	case "", "local":
		return time.Local, nil
	case "UTC", "utc":
		return time.UTC, nil
	default:
		loc, err := time.LoadLocation(name)
		if err != nil {
			return nil, fmt.Errorf("unknown timezone %q", name)
		}
		return loc, nil
	}
}

// FormatName resolves a requested template name: an explicit name wins,
// otherwise default_format, otherwise the built-in "text".
func (c *Config) FormatName(requested string) string {
	if requested != "" {
		return requested
	}
	if c.DefaultFormat != "" {
		return c.DefaultFormat
	}
	return BuiltinText
}

// Template returns the compiled template with the given name.
func (c *Config) Template(name string) (*format.Template, bool) {
	t, ok := c.templates[name]
	return t, ok
}
