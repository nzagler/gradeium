package auth

import (
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"errors"

	"github.com/nzagler/gradeium/backend/internal/secrets"
)

const tokenBytes = 32

func randomToken() (string, error) {
	value := make([]byte, tokenBytes)
	if _, err := rand.Read(value); err != nil {
		return "", errors.New("generate secure token")
	}
	defer secrets.Clear(value)
	return base64.RawURLEncoding.EncodeToString(value), nil
}

func tokenHash(token string) ([sha256.Size]byte, error) {
	var zero [sha256.Size]byte
	decoded, err := base64.RawURLEncoding.DecodeString(token)
	if err != nil || len(decoded) != tokenBytes || base64.RawURLEncoding.EncodeToString(decoded) != token {
		secrets.Clear(decoded)
		return zero, errors.New("invalid token")
	}
	defer secrets.Clear(decoded)
	return sha256.Sum256([]byte(token)), nil
}

func csrfToken(cipher *secrets.Cipher, sessionToken string) string {
	value := cipher.Authenticate("gradeium-csrf-v1", []byte(sessionToken))
	return base64.RawURLEncoding.EncodeToString(value[:])
}

func validCSRFToken(cipher *secrets.Cipher, sessionToken, submitted string) bool {
	expected := csrfToken(cipher, sessionToken)
	return subtle.ConstantTimeCompare([]byte(expected), []byte(submitted)) == 1
}
