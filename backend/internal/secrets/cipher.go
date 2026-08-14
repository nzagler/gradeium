package secrets

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"errors"
	"fmt"
)

const AlgorithmVersion int16 = 1

var ErrDecrypt = errors.New("secret could not be decrypted")

// Envelope is the versioned authenticated ciphertext persisted in PostgreSQL.
type Envelope struct {
	AlgorithmVersion int16
	Nonce            []byte
	Ciphertext       []byte
}

// Cipher encrypts secret settings with AES-256-GCM.
type Cipher struct {
	aead cipher.AEAD
}

func NewCipher(key []byte) (*Cipher, error) {
	if len(key) != 32 {
		return nil, errors.New("master key must contain exactly 32 bytes")
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, errors.New("initialize master-key cipher")
	}
	aead, err := cipher.NewGCM(block)
	if err != nil {
		return nil, errors.New("initialize authenticated encryption")
	}
	return &Cipher{aead: aead}, nil
}

// Encrypt creates a fresh nonce and binds the ciphertext to its setting key.
func (cipher *Cipher) Encrypt(settingKey string, plaintext []byte) (Envelope, error) {
	nonce := make([]byte, cipher.aead.NonceSize())
	if _, err := rand.Read(nonce); err != nil {
		return Envelope{}, errors.New("generate encryption nonce")
	}
	ciphertext := cipher.aead.Seal(nil, nonce, plaintext, associatedData(settingKey, AlgorithmVersion))
	return Envelope{
		AlgorithmVersion: AlgorithmVersion,
		Nonce:            nonce,
		Ciphertext:       ciphertext,
	}, nil
}

// Decrypt authenticates the ciphertext and its setting-key association.
func (cipher *Cipher) Decrypt(settingKey string, envelope Envelope) ([]byte, error) {
	if envelope.AlgorithmVersion != AlgorithmVersion {
		return nil, ErrDecrypt
	}
	if len(envelope.Nonce) != cipher.aead.NonceSize() || len(envelope.Ciphertext) < cipher.aead.Overhead() {
		return nil, ErrDecrypt
	}
	plaintext, err := cipher.aead.Open(
		nil,
		envelope.Nonce,
		envelope.Ciphertext,
		associatedData(settingKey, envelope.AlgorithmVersion),
	)
	if err != nil {
		return nil, ErrDecrypt
	}
	return plaintext, nil
}

func associatedData(settingKey string, version int16) []byte {
	return []byte(fmt.Sprintf("gradeium-secret:%d:%s", version, settingKey))
}

func clearBytes(value []byte) {
	for index := range value {
		value[index] = 0
	}
}
