package secrets

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"sync"
	"testing"

	"github.com/nzagler/gradeium/backend/internal/settings"
)

type memorySecretStore struct {
	mutex   sync.Mutex
	records map[string]Record
}

func newMemorySecretStore() *memorySecretStore {
	return &memorySecretStore{records: make(map[string]Record)}
}

func (store *memorySecretStore) Get(_ context.Context, key string) (Record, error) {
	store.mutex.Lock()
	defer store.mutex.Unlock()
	record, ok := store.records[key]
	if !ok {
		return Record{}, ErrSecretNotFound
	}
	return cloneRecord(record), nil
}

func (store *memorySecretStore) Upsert(_ context.Context, record Record) error {
	store.mutex.Lock()
	defer store.mutex.Unlock()
	store.records[record.Key] = cloneRecord(record)
	return nil
}

func (store *memorySecretStore) Delete(_ context.Context, key string) (bool, error) {
	store.mutex.Lock()
	defer store.mutex.Unlock()
	_, found := store.records[key]
	delete(store.records, key)
	return found, nil
}

func (store *memorySecretStore) List(context.Context) ([]Record, error) {
	store.mutex.Lock()
	defer store.mutex.Unlock()
	records := make([]Record, 0, len(store.records))
	for _, record := range store.records {
		records = append(records, cloneRecord(record))
	}
	return records, nil
}

func cloneRecord(record Record) Record {
	record.Nonce = append([]byte(nil), record.Nonce...)
	record.Ciphertext = append([]byte(nil), record.Ciphertext...)
	return record
}

func TestServiceSetReadReplaceAndDelete(t *testing.T) {
	registry := settings.NewRegistry()
	store := newMemorySecretStore()
	cipher, err := NewCipher(bytes.Repeat([]byte{0x51}, 32))
	if err != nil {
		t.Fatalf("NewCipher returned an error: %v", err)
	}
	service := NewService(registry, store, cipher)
	key := settings.FutureAuthenticationSecretKey

	if err := service.Set(context.Background(), key, "same-plaintext"); err != nil {
		t.Fatalf("first Set returned an error: %v", err)
	}
	first, _ := store.Get(context.Background(), key)
	if bytes.Contains(first.Ciphertext, []byte("same-plaintext")) {
		t.Fatal("stored ciphertext exposed plaintext")
	}
	if err := service.Set(context.Background(), key, "same-plaintext"); err != nil {
		t.Fatalf("replacement Set returned an error: %v", err)
	}
	second, _ := store.Get(context.Background(), key)
	if bytes.Equal(first.Nonce, second.Nonce) {
		t.Fatal("replacement reused the previous nonce")
	}

	plaintext, err := service.Read(context.Background(), key)
	if err != nil {
		t.Fatalf("Read returned an error: %v", err)
	}
	if string(plaintext) != "same-plaintext" {
		t.Fatalf("Read returned %q", plaintext)
	}
	clearBytes(plaintext)

	deleted, err := service.Delete(context.Background(), key)
	if err != nil || !deleted {
		t.Fatalf("Delete = (%v, %v), want (true, nil)", deleted, err)
	}
	configured, err := service.Configured(context.Background(), key)
	if err != nil || configured {
		t.Fatalf("Configured = (%v, %v), want (false, nil)", configured, err)
	}
}

func TestServiceDetectsTamperedStoredSecret(t *testing.T) {
	registry := settings.NewRegistry()
	store := newMemorySecretStore()
	cipher, _ := NewCipher(bytes.Repeat([]byte{0x61}, 32))
	service := NewService(registry, store, cipher)
	key := settings.FutureAuthenticationSecretKey
	if err := service.Set(context.Background(), key, "secret"); err != nil {
		t.Fatalf("Set returned an error: %v", err)
	}
	store.mutex.Lock()
	record := store.records[key]
	record.Ciphertext[0] ^= 0xff
	store.records[key] = record
	store.mutex.Unlock()

	if _, err := service.Read(context.Background(), key); !errors.Is(err, ErrDecrypt) {
		t.Fatalf("Read error = %v, want ErrDecrypt", err)
	}
	if err := service.ValidateStored(context.Background()); !errors.Is(err, ErrDecrypt) {
		t.Fatalf("ValidateStored error = %v, want ErrDecrypt", err)
	}
}

func TestServiceConcurrentReplacementLeavesOneCompleteSecret(t *testing.T) {
	registry := settings.NewRegistry()
	store := newMemorySecretStore()
	cipher, _ := NewCipher(bytes.Repeat([]byte{0x72}, 32))
	service := NewService(registry, store, cipher)
	key := settings.FutureAuthenticationSecretKey

	const replacements = 20
	var waitGroup sync.WaitGroup
	for index := range replacements {
		waitGroup.Add(1)
		go func(index int) {
			defer waitGroup.Done()
			if err := service.Set(context.Background(), key, fmt.Sprintf("concurrent-secret-%02d", index)); err != nil {
				t.Errorf("Set %d returned an error: %v", index, err)
			}
		}(index)
	}
	waitGroup.Wait()

	plaintext, err := service.Read(context.Background(), key)
	if err != nil {
		t.Fatalf("Read returned an error: %v", err)
	}
	defer clearBytes(plaintext)
	value := string(plaintext)
	matched := false
	for index := range replacements {
		if value == fmt.Sprintf("concurrent-secret-%02d", index) {
			matched = true
			break
		}
	}
	if !matched {
		t.Fatalf("concurrent replacement left an incomplete value %q", value)
	}
	if err := service.ValidateStored(context.Background()); err != nil {
		t.Fatalf("ValidateStored returned an error: %v", err)
	}
}
