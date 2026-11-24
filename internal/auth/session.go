package auth

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// Permission bits required by plan §5.
const (
	SessionDirPerm  os.FileMode = 0o700
	SessionFilePerm os.FileMode = 0o600
)

// SessionStore persists one gotd session file per phone number. It never reads
// or stores message history.
type SessionStore struct {
	dir       string
	tightened bool
}

// DefaultSessionDir returns ~/.tg-watcher/sessions.
func DefaultSessionDir() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("locate home directory: %w", err)
	}
	return filepath.Join(home, ".tg-watcher", "sessions"), nil
}

// NewSessionStore creates dir (if needed) and enforces 0700 permissions. When
// the directory exists with looser permissions it is tightened; if that fails,
// using the store is refused. tightened reports whether permissions were fixed.
func NewSessionStore(dir string) (store *SessionStore, tightened bool, err error) {
	if dir == "" {
		return nil, false, errors.New("session directory is empty")
	}
	if err := os.MkdirAll(dir, SessionDirPerm); err != nil {
		return nil, false, fmt.Errorf("create session directory %s: %w", dir, err)
	}
	info, err := os.Stat(dir)
	if err != nil {
		return nil, false, fmt.Errorf("stat session directory %s: %w", dir, err)
	}
	if !info.IsDir() {
		return nil, false, fmt.Errorf("session path %s is not a directory", dir)
	}
	perm := info.Mode().Perm()
	if perm&0o077 != 0 {
		if err := os.Chmod(dir, SessionDirPerm); err != nil {
			return nil, false, fmt.Errorf("session directory %s has permissions %04o and cannot be secured: %w", dir, perm, err)
		}
		tightened = true
	}
	return &SessionStore{dir: dir, tightened: tightened}, tightened, nil
}

// Dir returns the session directory.
func (s *SessionStore) Dir() string { return s.dir }

// PermissionsTightened reports whether NewSessionStore had to loosen→tighten
// permissions. The caller should warn the user.
func (s *SessionStore) PermissionsTightened() bool { return s.tightened }

// Path returns the session file path for phone. The phone must be a valid
// international number; only digits are used in the file name, so the result
// cannot escape the session directory.
func (s *SessionStore) Path(phone string) (string, error) {
	digits := strings.TrimPrefix(phone, "+")
	if digits == "" || len(digits) > 15 {
		return "", ErrInvalidPhone
	}
	for i := 0; i < len(digits); i++ {
		if digits[i] < '0' || digits[i] > '9' {
			return "", ErrInvalidPhone
		}
	}
	return filepath.Join(s.dir, digits+".json"), nil
}

// Load reads the session for phone. A missing session returns an error wrapping
// os.ErrNotExist. Insecure file permissions are tightened first; if that is not
// possible the session is not read.
func (s *SessionStore) Load(phone string) ([]byte, error) {
	path, err := s.Path(phone)
	if err != nil {
		return nil, err
	}
	info, err := os.Stat(path)
	if err != nil {
		return nil, err
	}
	if perm := info.Mode().Perm(); perm&0o077 != 0 {
		if err := os.Chmod(path, SessionFilePerm); err != nil {
			return nil, fmt.Errorf("session file %s has permissions %04o and cannot be secured: %w", path, perm, err)
		}
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read session %s: %w", path, err)
	}
	return data, nil
}

// Save atomically writes the session for phone with 0600 permissions. The file
// is written to a temporary file in the session directory and renamed into
// place, so a crash never leaves a partially written session.
func (s *SessionStore) Save(phone string, data []byte) error {
	path, err := s.Path(phone)
	if err != nil {
		return err
	}
	f, err := os.CreateTemp(s.dir, ".session-*")
	if err != nil {
		return fmt.Errorf("create temporary session file: %w", err)
	}
	tmp := f.Name()
	cleanup := func() {
		_ = f.Close()
		_ = os.Remove(tmp)
	}
	if err := f.Chmod(SessionFilePerm); err != nil {
		cleanup()
		return fmt.Errorf("secure temporary session file: %w", err)
	}
	if _, err := f.Write(data); err != nil {
		cleanup()
		return fmt.Errorf("write session: %w", err)
	}
	if err := f.Sync(); err != nil {
		cleanup()
		return fmt.Errorf("sync session: %w", err)
	}
	if err := f.Close(); err != nil {
		_ = os.Remove(tmp)
		return fmt.Errorf("close session: %w", err)
	}
	if err := os.Rename(tmp, path); err != nil {
		_ = os.Remove(tmp)
		return fmt.Errorf("replace session %s: %w", path, err)
	}
	return nil
}
