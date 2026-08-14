package secrets

import (
	"context"
	"errors"
	"fmt"
)

var ErrSecretNotFound = errors.New("secret setting not found")

// Record is an encrypted secret setting as stored in PostgreSQL.
type Record struct {
	Key string
	Envelope
}

// Store persists only encrypted secret records.
type Store interface {
	Get(context.Context, string) (Record, error)
	Upsert(context.Context, Record) error
	Delete(context.Context, string) (bool, error)
	List(context.Context) ([]Record, error)
}

// Policy is implemented by the central settings registry.
type Policy interface {
	AllowsSecret(string) bool
	ValidateSecret(string, string) error
}

// Service is the only application boundary that handles secret plaintext.
type Service struct {
	policy Policy
	store  Store
	cipher *Cipher
}

func NewService(policy Policy, store Store, cipher *Cipher) *Service {
	return &Service{policy: policy, store: store, cipher: cipher}
}

// Set validates, encrypts with a fresh nonce, and atomically replaces a secret.
func (service *Service) Set(ctx context.Context, key, value string) error {
	record, err := service.Seal(key, value)
	if err != nil {
		return err
	}
	return service.store.Upsert(ctx, record)
}

// Seal validates and encrypts a value without persisting it. This allows a
// caller to include the resulting envelope in a larger database transaction.
func (service *Service) Seal(key, value string) (Record, error) {
	if err := service.policy.ValidateSecret(key, value); err != nil {
		return Record{}, err
	}
	plaintext := []byte(value)
	defer clearBytes(plaintext)
	envelope, err := service.cipher.Encrypt(key, plaintext)
	if err != nil {
		return Record{}, err
	}
	return Record{Key: key, Envelope: envelope}, nil
}

// Configured reports state without decrypting or returning a secret.
func (service *Service) Configured(ctx context.Context, key string) (bool, error) {
	if !service.policy.AllowsSecret(key) {
		return false, errors.New("secret key is not allowed")
	}
	_, err := service.store.Get(ctx, key)
	if errors.Is(err, ErrSecretNotFound) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return true, nil
}

// Read decrypts a secret for trusted server-side consumers only.
func (service *Service) Read(ctx context.Context, key string) ([]byte, error) {
	if !service.policy.AllowsSecret(key) {
		return nil, errors.New("secret key is not allowed")
	}
	record, err := service.store.Get(ctx, key)
	if err != nil {
		return nil, err
	}
	return service.Open(record)
}

// Open decrypts an allowed record obtained by trusted server-side code.
func (service *Service) Open(record Record) ([]byte, error) {
	if !service.policy.AllowsSecret(record.Key) {
		return nil, errors.New("secret key is not allowed")
	}
	return service.cipher.Decrypt(record.Key, record.Envelope)
}

// Delete removes an allowed secret setting.
func (service *Service) Delete(ctx context.Context, key string) (bool, error) {
	if !service.policy.AllowsSecret(key) {
		return false, errors.New("secret key is not allowed")
	}
	return service.store.Delete(ctx, key)
}

// ValidateStored authenticates every persisted secret during startup. This
// detects a mismatched key or tampered row without exposing any plaintext.
func (service *Service) ValidateStored(ctx context.Context) error {
	records, err := service.store.List(ctx)
	if err != nil {
		return err
	}
	for _, record := range records {
		plaintext, err := service.cipher.Decrypt(record.Key, record.Envelope)
		if err != nil {
			return fmt.Errorf("validate stored secret %q: %w", record.Key, ErrDecrypt)
		}
		clearBytes(plaintext)
	}
	return nil
}
