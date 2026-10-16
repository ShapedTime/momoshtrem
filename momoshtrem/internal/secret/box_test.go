package secret

import (
	"bytes"
	"errors"
	"strings"
	"testing"
)

const hexKey = "000102030405060708090a0b0c0d0e0f101112131415161718191a1b1c1d1e1f"

func TestNew_Keys(t *testing.T) {
	tests := []struct {
		name    string
		key     string
		wantErr bool
	}{
		{"64 hex characters", hexKey, false},
		{"a 32-character passphrase", strings.Repeat("p", 32), false},
		{"64 characters that aren't hex", strings.Repeat("z", 64), false},
		{"31 characters", strings.Repeat("p", 31), true},
		{"empty", "", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := New(tt.key)
			if tt.wantErr != (err != nil) || (err != nil && !errors.Is(err, ErrInvalidKey)) {
				t.Errorf("New = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}

	// The hex key is used as is, not hashed: the same bytes as a passphrase
	// would be another key.
	key, err := deriveKey(hexKey)
	if err != nil || key[0] != 0 || key[31] != 0x1f {
		t.Errorf("hex key = %x, %v", key, err)
	}
}

func TestBox_RoundTrip(t *testing.T) {
	box := mustBox(t, hexKey)
	sealed := box.Seal([]byte("access-token"), "trakt:access")
	if bytes.Contains(sealed, []byte("access-token")) {
		t.Fatal("the sealed bytes hold the plaintext")
	}
	if sealed[0] != version || len(sealed) != 1+12+len("access-token")+16 {
		t.Errorf("sealed = %x, want version, nonce, ciphertext and tag", sealed)
	}
	got, err := box.Open(sealed, "trakt:access")
	if err != nil || string(got) != "access-token" {
		t.Errorf("Open = %q, %v", got, err)
	}
}

func TestBox_Rejects(t *testing.T) {
	box := mustBox(t, hexKey)
	sealed := box.Seal([]byte("refresh-token"), "trakt:refresh")
	tamper := func(i int) []byte {
		b := bytes.Clone(sealed)
		b[i] ^= 1
		return b
	}
	tests := []struct {
		name   string
		box    *Box
		sealed []byte
		aad    string
	}{
		{"another key", mustBox(t, strings.Repeat("k", 32)), sealed, "trakt:refresh"},
		{"another purpose", box, sealed, "trakt:access"},
		{"a changed version", box, tamper(0), "trakt:refresh"},
		{"a changed nonce", box, tamper(1), "trakt:refresh"},
		{"a changed ciphertext", box, tamper(14), "trakt:refresh"},
		{"a changed tag", box, tamper(len(sealed) - 1), "trakt:refresh"},
		{"truncated", box, sealed[:20], "trakt:refresh"},
		{"empty", box, nil, "trakt:refresh"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got, err := tt.box.Open(tt.sealed, tt.aad); !errors.Is(err, ErrDecrypt) {
				t.Errorf("Open = %q, %v; want ErrDecrypt", got, err)
			}
		})
	}
}

func TestBox_NoncesAreUnique(t *testing.T) {
	box := mustBox(t, hexKey)
	seen := map[string]bool{}
	for range 1000 {
		sealed := box.Seal([]byte("same"), "trakt:access")
		nonce := string(sealed[1:13])
		if seen[nonce] {
			t.Fatal("a nonce came twice")
		}
		seen[nonce] = true
	}
}

func mustBox(t *testing.T, key string) *Box {
	t.Helper()
	box, err := New(key)
	if err != nil {
		t.Fatal(err)
	}
	return box
}
