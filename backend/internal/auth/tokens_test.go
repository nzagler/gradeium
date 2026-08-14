package auth

import (
	"bytes"
	"testing"

	"github.com/nzagler/gradeium/backend/internal/secrets"
)

func TestOpaqueSessionAndCSRFTokenBoundaries(t *testing.T) {
	first, err := randomToken()
	if err != nil {
		t.Fatalf("randomToken returned an error: %v", err)
	}
	second, err := randomToken()
	if err != nil {
		t.Fatalf("second randomToken returned an error: %v", err)
	}
	if first == second {
		t.Fatal("two random tokens were equal")
	}
	firstHash, err := tokenHash(first)
	if err != nil {
		t.Fatalf("tokenHash returned an error: %v", err)
	}
	if bytes.Contains(firstHash[:], []byte(first)) {
		t.Fatal("session hash contained the raw token")
	}
	if _, err := tokenHash("not-a-valid-token"); err == nil {
		t.Fatal("malformed token was accepted")
	}

	cipher, err := secrets.NewCipher(bytes.Repeat([]byte{0x44}, 32))
	if err != nil {
		t.Fatalf("NewCipher returned an error: %v", err)
	}
	csrf := csrfToken(cipher, first)
	if csrf == "" || !validCSRFToken(cipher, first, csrf) {
		t.Fatal("valid session-bound CSRF token was rejected")
	}
	if validCSRFToken(cipher, second, csrf) || validCSRFToken(cipher, first, csrf+"x") {
		t.Fatal("CSRF token was accepted for the wrong session or after tampering")
	}
}
