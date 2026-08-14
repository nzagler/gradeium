package secrets

import (
	"bytes"
	"errors"
	"testing"
)

func TestCipherRoundTripUsesFreshNonces(t *testing.T) {
	cipher, err := NewCipher(bytes.Repeat([]byte{0x42}, 32))
	if err != nil {
		t.Fatalf("NewCipher returned an error: %v", err)
	}
	plaintext := []byte("sensitive-value")

	first, err := cipher.Encrypt("test.secret", plaintext)
	if err != nil {
		t.Fatalf("first Encrypt returned an error: %v", err)
	}
	second, err := cipher.Encrypt("test.secret", plaintext)
	if err != nil {
		t.Fatalf("second Encrypt returned an error: %v", err)
	}
	if bytes.Equal(first.Nonce, second.Nonce) {
		t.Fatal("two encryptions reused a nonce")
	}
	if bytes.Contains(first.Ciphertext, plaintext) {
		t.Fatal("ciphertext contains plaintext")
	}

	decrypted, err := cipher.Decrypt("test.secret", first)
	if err != nil {
		t.Fatalf("Decrypt returned an error: %v", err)
	}
	defer clearBytes(decrypted)
	if !bytes.Equal(decrypted, plaintext) {
		t.Fatalf("decrypted value = %q, want original plaintext", decrypted)
	}
}

func TestCipherRejectsTamperingAndKeySwapping(t *testing.T) {
	cipher, err := NewCipher(bytes.Repeat([]byte{0x23}, 32))
	if err != nil {
		t.Fatalf("NewCipher returned an error: %v", err)
	}
	envelope, err := cipher.Encrypt("first.secret", []byte("value"))
	if err != nil {
		t.Fatalf("Encrypt returned an error: %v", err)
	}

	tampered := Envelope{
		AlgorithmVersion: envelope.AlgorithmVersion,
		Nonce:            append([]byte(nil), envelope.Nonce...),
		Ciphertext:       append([]byte(nil), envelope.Ciphertext...),
	}
	tampered.Ciphertext[0] ^= 0xff
	if _, err := cipher.Decrypt("first.secret", tampered); !errors.Is(err, ErrDecrypt) {
		t.Fatalf("tampered ciphertext error = %v, want ErrDecrypt", err)
	}
	if _, err := cipher.Decrypt("second.secret", envelope); !errors.Is(err, ErrDecrypt) {
		t.Fatalf("swapped-key ciphertext error = %v, want ErrDecrypt", err)
	}
	envelope.AlgorithmVersion++
	if _, err := cipher.Decrypt("first.secret", envelope); !errors.Is(err, ErrDecrypt) {
		t.Fatalf("unknown algorithm error = %v, want ErrDecrypt", err)
	}
}
