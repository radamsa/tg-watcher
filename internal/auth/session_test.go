package auth

import (
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func TestSessionStoreCreatesSecureDirectory(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "sessions")
	store, tightened, err := NewSessionStore(dir)
	if err != nil {
		t.Fatalf("NewSessionStore: %v", err)
	}
	if tightened {
		t.Fatal("fresh directory reported as tightened")
	}
	if runtime.GOOS != "windows" {
		info, err := os.Stat(dir)
		if err != nil {
			t.Fatal(err)
		}
		if perm := info.Mode().Perm(); perm != SessionDirPerm {
			t.Fatalf("dir perm = %04o, want %04o", perm, SessionDirPerm)
		}
	}
	if store.Dir() != dir {
		t.Fatalf("Dir = %q", store.Dir())
	}
}

func TestSessionStoreTightensLooseDirectory(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("unix permissions")
	}
	dir := t.TempDir()
	loose := filepath.Join(dir, "sessions")
	if err := os.Mkdir(loose, 0o755); err != nil {
		t.Fatal(err)
	}
	_, tightened, err := NewSessionStore(loose)
	if err != nil {
		t.Fatalf("NewSessionStore: %v", err)
	}
	if !tightened {
		t.Fatal("expected tightened=true")
	}
	info, err := os.Stat(loose)
	if err != nil {
		t.Fatal(err)
	}
	if perm := info.Mode().Perm(); perm != SessionDirPerm {
		t.Fatalf("dir perm = %04o, want %04o", perm, SessionDirPerm)
	}
}

func TestSessionStoreSaveLoad(t *testing.T) {
	store, _, err := NewSessionStore(filepath.Join(t.TempDir(), "sessions"))
	if err != nil {
		t.Fatal(err)
	}
	data := []byte(`{"version":1}`)
	if err := store.Save("+31612345678", data); err != nil {
		t.Fatalf("Save: %v", err)
	}

	path, err := store.Path("+31612345678")
	if err != nil {
		t.Fatal(err)
	}
	if want := "31612345678.json"; filepath.Base(path) != want {
		t.Fatalf("path base = %q, want %q", filepath.Base(path), want)
	}
	if runtime.GOOS != "windows" {
		info, err := os.Stat(path)
		if err != nil {
			t.Fatal(err)
		}
		if perm := info.Mode().Perm(); perm != SessionFilePerm {
			t.Fatalf("file perm = %04o, want %04o", perm, SessionFilePerm)
		}
		entries, err := os.ReadDir(store.Dir())
		if err != nil {
			t.Fatal(err)
		}
		if len(entries) != 1 {
			t.Fatalf("temporary files left behind: %v", entries)
		}
	}

	got, err := store.Load("+31612345678")
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if string(got) != string(data) {
		t.Fatalf("loaded %q, want %q", got, data)
	}
}

func TestSessionStoreLoadTightensLooseFile(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("unix permissions")
	}
	store, _, err := NewSessionStore(filepath.Join(t.TempDir(), "sessions"))
	if err != nil {
		t.Fatal(err)
	}
	path, err := store.Path("+31612345678")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("secret"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := store.Load("+31612345678"); err != nil {
		t.Fatalf("Load: %v", err)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if perm := info.Mode().Perm(); perm != SessionFilePerm {
		t.Fatalf("file perm = %04o, want %04o", perm, SessionFilePerm)
	}
}

func TestSessionStoreMissingSession(t *testing.T) {
	store, _, err := NewSessionStore(filepath.Join(t.TempDir(), "sessions"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.Load("+31612345678"); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("err = %v, want os.ErrNotExist", err)
	}
}

func TestSessionPathRejectsInvalidPhone(t *testing.T) {
	store, _, err := NewSessionStore(filepath.Join(t.TempDir(), "sessions"))
	if err != nil {
		t.Fatal(err)
	}
	for _, phone := range []string{"", "+", "../etc/passwd", "+31a", "+1234567890123456"} {
		if _, err := store.Path(phone); err == nil {
			t.Fatalf("Path(%q) succeeded, want error", phone)
		}
	}
}

func TestSessionStoreRefusesFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "not-a-dir")
	if err := os.WriteFile(path, []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, _, err := NewSessionStore(path); err == nil {
		t.Fatal("expected error when session path is a file")
	}
}
