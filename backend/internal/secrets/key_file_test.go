package secrets

import (
	"bytes"
	"context"
	"crypto/sha256"
	"crypto/subtle"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"sync"
	"testing"
)

type memoryKeyStateStore struct {
	mutex sync.Mutex
	state KeyState
}

func (store *memoryKeyStateStore) State(context.Context) (KeyState, error) {
	store.mutex.Lock()
	defer store.mutex.Unlock()
	return store.state, nil
}

func (store *memoryKeyStateStore) RegisterFingerprint(_ context.Context, fingerprint [sha256.Size]byte) error {
	store.mutex.Lock()
	defer store.mutex.Unlock()
	if store.state.Registered && subtle.ConstantTimeCompare(store.state.Fingerprint[:], fingerprint[:]) != 1 {
		return ErrMasterKeyMismatch
	}
	store.state.Registered = true
	store.state.Fingerprint = fingerprint
	return nil
}

func TestInitializeCipherCreatesAndReloadsPersistentKey(t *testing.T) {
	directory := t.TempDir()
	store := &memoryKeyStateStore{}
	first, err := InitializeCipher(context.Background(), directory, store)
	if err != nil {
		t.Fatalf("first InitializeCipher returned an error: %v", err)
	}

	info, err := os.Stat(filepath.Join(directory, MasterKeyFilename))
	if err != nil {
		t.Fatalf("stat master key: %v", err)
	}
	if permissions := info.Mode().Perm(); runtime.GOOS != "windows" && permissions&0o077 != 0 {
		t.Fatalf("master key permissions = %o, want no group/other access", permissions)
	}

	envelope, err := first.Encrypt("test.secret", []byte("survives-restart"))
	if err != nil {
		t.Fatalf("Encrypt returned an error: %v", err)
	}
	second, err := InitializeCipher(context.Background(), directory, store)
	if err != nil {
		t.Fatalf("second InitializeCipher returned an error: %v", err)
	}
	plaintext, err := second.Decrypt("test.secret", envelope)
	if err != nil {
		t.Fatalf("reloaded cipher could not decrypt: %v", err)
	}
	defer clearBytes(plaintext)
	if string(plaintext) != "survives-restart" {
		t.Fatalf("decrypted value = %q", plaintext)
	}
}

func TestInitializeCipherRejectsMalformedKey(t *testing.T) {
	directory := t.TempDir()
	path := filepath.Join(directory, MasterKeyFilename)
	if err := os.WriteFile(path, []byte("not-a-gradeium-key"), 0o600); err != nil {
		t.Fatalf("write malformed key: %v", err)
	}
	_, err := InitializeCipher(context.Background(), directory, &memoryKeyStateStore{})
	if !errors.Is(err, ErrMasterKeyInvalid) {
		t.Fatalf("error = %v, want ErrMasterKeyInvalid", err)
	}
}

func TestInitializeCipherRejectsUnsafeKeyPermissions(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Windows file modes do not represent ACL permissions")
	}
	directory := t.TempDir()
	store := &memoryKeyStateStore{}
	if _, err := InitializeCipher(context.Background(), directory, store); err != nil {
		t.Fatalf("initialize key: %v", err)
	}
	path := filepath.Join(directory, MasterKeyFilename)
	if err := os.Chmod(path, 0o644); err != nil {
		t.Fatalf("make key permissions unsafe: %v", err)
	}
	_, err := InitializeCipher(context.Background(), directory, store)
	if !errors.Is(err, ErrMasterKeyInvalid) {
		t.Fatalf("error = %v, want ErrMasterKeyInvalid", err)
	}
}

func TestInitializeCipherRejectsMissingKeyAfterUse(t *testing.T) {
	store := &memoryKeyStateStore{state: KeyState{Registered: true, SecretCount: 1}}
	_, err := InitializeCipher(context.Background(), t.TempDir(), store)
	if !errors.Is(err, ErrMasterKeyMissing) {
		t.Fatalf("error = %v, want ErrMasterKeyMissing", err)
	}
}

func TestInitializeCipherRejectsMismatchedDatabase(t *testing.T) {
	directory := t.TempDir()
	store := &memoryKeyStateStore{}
	if _, err := InitializeCipher(context.Background(), directory, store); err != nil {
		t.Fatalf("initialize key: %v", err)
	}
	store.mutex.Lock()
	store.state.Fingerprint = sha256.Sum256([]byte("different-key"))
	store.mutex.Unlock()

	_, err := InitializeCipher(context.Background(), directory, store)
	if !errors.Is(err, ErrMasterKeyMismatch) {
		t.Fatalf("error = %v, want ErrMasterKeyMismatch", err)
	}
}

func TestInitializeCipherIsRaceSafe(t *testing.T) {
	directory := t.TempDir()
	store := &memoryKeyStateStore{}
	const callers = 12
	ciphers := make([]*Cipher, callers)
	errorsByCaller := make([]error, callers)
	var waitGroup sync.WaitGroup
	for index := range callers {
		waitGroup.Add(1)
		go func(index int) {
			defer waitGroup.Done()
			ciphers[index], errorsByCaller[index] = InitializeCipher(context.Background(), directory, store)
		}(index)
	}
	waitGroup.Wait()
	for index, err := range errorsByCaller {
		if err != nil {
			t.Fatalf("caller %d returned an error: %v", index, err)
		}
	}

	envelope, err := ciphers[0].Encrypt("test.secret", []byte("same-key"))
	if err != nil {
		t.Fatalf("Encrypt returned an error: %v", err)
	}
	for index, cipher := range ciphers[1:] {
		plaintext, err := cipher.Decrypt("test.secret", envelope)
		if err != nil {
			t.Fatalf("cipher %d did not use the winning key: %v", index+1, err)
		}
		if !bytes.Equal(plaintext, []byte("same-key")) {
			t.Fatalf("cipher %d returned the wrong plaintext", index+1)
		}
		clearBytes(plaintext)
	}
}
