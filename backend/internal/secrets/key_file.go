package secrets

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
)

const (
	MasterKeyFilename = "master.key"
	keyFileHeader     = "gradeium-master-key-v1\n"
	masterKeySize     = 32
)

var (
	ErrMasterKeyMissing  = errors.New("master key is missing after secure settings were initialized")
	ErrMasterKeyInvalid  = errors.New("master key file is malformed or has unsafe permissions")
	ErrMasterKeyMismatch = errors.New("master key does not match this database")
)

// KeyState is the non-secret database state used to detect key loss.
type KeyState struct {
	Registered  bool
	Fingerprint [sha256.Size]byte
	SecretCount int64
}

// KeyStateStore persists only a one-way key fingerprint, never the master key.
type KeyStateStore interface {
	State(context.Context) (KeyState, error)
	RegisterFingerprint(context.Context, [sha256.Size]byte) error
}

// InitializeCipher safely loads or creates /config/master.key and binds it to
// the current database before returning an authenticated cipher.
func InitializeCipher(ctx context.Context, configDir string, store KeyStateStore) (*Cipher, error) {
	if err := prepareConfigDirectory(configDir); err != nil {
		return nil, err
	}

	state, err := store.State(ctx)
	if err != nil {
		return nil, fmt.Errorf("read encryption key state: %w", err)
	}

	keyPath := filepath.Join(configDir, MasterKeyFilename)
	key, err := readKeyFile(keyPath)
	if errors.Is(err, os.ErrNotExist) {
		if state.Registered || state.SecretCount > 0 {
			return nil, ErrMasterKeyMissing
		}
		key, err = createKeyFile(keyPath)
	}
	if err != nil {
		return nil, err
	}
	defer clearBytes(key)

	fingerprint := sha256.Sum256(key)
	if state.Registered && subtle.ConstantTimeCompare(state.Fingerprint[:], fingerprint[:]) != 1 {
		return nil, ErrMasterKeyMismatch
	}
	if err := store.RegisterFingerprint(ctx, fingerprint); err != nil {
		return nil, fmt.Errorf("register encryption key fingerprint: %w", err)
	}

	cipher, err := NewCipher(key)
	if err != nil {
		return nil, err
	}
	return cipher, nil
}

func prepareConfigDirectory(configDir string) error {
	if err := os.MkdirAll(configDir, 0o700); err != nil {
		return fmt.Errorf("create config directory: %w", err)
	}
	info, err := os.Lstat(configDir)
	if err != nil {
		return fmt.Errorf("inspect config directory: %w", err)
	}
	if !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
		return errors.New("config path must be a real directory")
	}
	if err := os.Chmod(configDir, 0o700); err != nil {
		return fmt.Errorf("restrict config directory permissions: %w", err)
	}
	return nil
}

func createKeyFile(path string) ([]byte, error) {
	key := make([]byte, masterKeySize)
	if _, err := rand.Read(key); err != nil {
		return nil, errors.New("generate master key")
	}
	defer func() {
		if key != nil {
			clearBytes(key)
		}
	}()

	contents := formatKeyFile(key)
	defer clearBytes(contents)
	directory := filepath.Dir(path)
	temporary, err := os.CreateTemp(directory, ".master-key-*")
	if err != nil {
		return nil, fmt.Errorf("create temporary master key: %w", err)
	}
	temporaryPath := temporary.Name()
	defer os.Remove(temporaryPath)

	writeSucceeded := false
	defer func() {
		if !writeSucceeded {
			_ = temporary.Close()
		}
	}()
	if err := temporary.Chmod(0o600); err != nil {
		return nil, fmt.Errorf("restrict temporary master key permissions: %w", err)
	}
	if _, err := temporary.Write(contents); err != nil {
		return nil, fmt.Errorf("write temporary master key: %w", err)
	}
	if err := temporary.Sync(); err != nil {
		return nil, fmt.Errorf("sync temporary master key: %w", err)
	}
	if err := temporary.Close(); err != nil {
		return nil, fmt.Errorf("close temporary master key: %w", err)
	}
	writeSucceeded = true

	// A hard link publishes the complete file atomically and refuses to replace
	// a key another process may have created concurrently.
	if err := os.Link(temporaryPath, path); err != nil {
		if !errors.Is(err, os.ErrExist) {
			return nil, fmt.Errorf("publish master key atomically: %w", err)
		}
		winner, readErr := readKeyFile(path)
		if readErr != nil {
			return nil, readErr
		}
		return winner, nil
	}
	if err := syncDirectory(directory); err != nil {
		return nil, fmt.Errorf("sync config directory: %w", err)
	}

	result := append([]byte(nil), key...)
	key = nil
	return result, nil
}

func readKeyFile(path string) ([]byte, error) {
	info, err := os.Lstat(path)
	if err != nil {
		return nil, err
	}
	unsafePermissions := runtime.GOOS != "windows" && info.Mode().Perm()&0o077 != 0
	if !info.Mode().IsRegular() || info.Mode()&os.ModeSymlink != 0 || unsafePermissions {
		return nil, ErrMasterKeyInvalid
	}
	contents, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read master key: %w", err)
	}
	defer clearBytes(contents)
	if len(contents) <= len(keyFileHeader) || string(contents[:len(keyFileHeader)]) != keyFileHeader {
		return nil, ErrMasterKeyInvalid
	}
	encoded := contents[len(keyFileHeader):]
	if len(encoded) == 0 || encoded[len(encoded)-1] != '\n' {
		return nil, ErrMasterKeyInvalid
	}
	encoded = encoded[:len(encoded)-1]
	key := make([]byte, base64.RawStdEncoding.DecodedLen(len(encoded)))
	decodedLength, err := base64.RawStdEncoding.Decode(key, encoded)
	if err != nil || decodedLength != masterKeySize {
		clearBytes(key)
		return nil, ErrMasterKeyInvalid
	}
	return key[:decodedLength], nil
}

func formatKeyFile(key []byte) []byte {
	encodedLength := base64.RawStdEncoding.EncodedLen(len(key))
	contents := make([]byte, len(keyFileHeader)+encodedLength+1)
	copy(contents, keyFileHeader)
	base64.RawStdEncoding.Encode(contents[len(keyFileHeader):len(keyFileHeader)+encodedLength], key)
	contents[len(contents)-1] = '\n'
	return contents
}

func syncDirectory(path string) error {
	if runtime.GOOS == "windows" {
		return nil
	}
	directory, err := os.Open(path)
	if err != nil {
		return err
	}
	defer directory.Close()
	return directory.Sync()
}
