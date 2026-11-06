package security

import (
	"bytes"
	"crypto/rand"
	"testing"
)

func key(t *testing.T) []byte {
	t.Helper()
	k := make([]byte, 32)
	if _, err := rand.Read(k); err != nil {
		t.Fatal(err)
	}
	return k
}

func TestEncryptRoundTrip(t *testing.T) {
	cipher, err := NewCipher(key(t))
	if err != nil {
		t.Fatal(err)
	}
	for _, value := range []string{"", "user", "p@ss word with spaces", "unicode-café"} {
		encoded, err := cipher.Encrypt(value)
		if err != nil {
			t.Fatalf("encrypt %q: %v", value, err)
		}
		if value == "" {
			if encoded != "" {
				t.Fatalf("empty value encrypted to %q", encoded)
			}
			continue
		}
		if bytes.Contains([]byte(encoded), []byte(value)) {
			t.Fatalf("ciphertext contains plaintext")
		}
		decoded, err := cipher.Decrypt(encoded)
		if err != nil {
			t.Fatalf("decrypt: %v", err)
		}
		if decoded != value {
			t.Fatalf("got %q, want %q", decoded, value)
		}
	}
}

func TestWrongKeyFails(t *testing.T) {
	first, _ := NewCipher(key(t))
	second, _ := NewCipher(key(t))
	encoded, err := first.Encrypt("secret")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := second.Decrypt(encoded); err == nil {
		t.Fatal("decrypt with a different key should fail")
	}
}

func TestNewCipherRejectsBadKey(t *testing.T) {
	if _, err := NewCipher([]byte("short")); err == nil {
		t.Fatal("expected an error")
	}
}

func TestMask(t *testing.T) {
	if Mask("") != "" {
		t.Fatal("empty value should stay empty")
	}
	if Mask("secret") == "secret" {
		t.Fatal("secret should be masked")
	}
}
