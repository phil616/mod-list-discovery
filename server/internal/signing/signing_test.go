package signing

import (
	"crypto/ed25519"
	"crypto/rand"
	"encoding/base64"
	"os"
	"path/filepath"
	"testing"
)

func TestSignedEnvelope(t *testing.T) {
	path := filepath.Join(t.TempDir(), "signing/key")
	if e := Create(path); e != nil {
		t.Fatal(e)
	}
	key, e := Load(path)
	if e != nil {
		t.Fatal(e)
	}
	before, _ := os.ReadFile(path)
	if Create(path) == nil {
		t.Fatal("overwrote key")
	}
	after, _ := os.ReadFile(path)
	if string(before) != string(after) {
		t.Fatal("key changed")
	}
	payload := []byte(`{"schemaVersion":1}`)
	first, second := Sign(key, payload), Sign(key, payload)
	if first != second {
		t.Fatal("not deterministic")
	}
	sig, _ := base64.RawURLEncoding.DecodeString(first.Signature.Value)
	if !ed25519.Verify(key.Public().(ed25519.PublicKey), payload, sig) {
		t.Fatal("invalid signature")
	}
	other, _, _ := ed25519.GenerateKey(rand.Reader)
	if ed25519.Verify(other, payload, sig) {
		t.Fatal("wrong key accepted")
	}
	payload[0] ^= 1
	if ed25519.Verify(key.Public().(ed25519.PublicKey), payload, sig) {
		t.Fatal("tamper accepted")
	}
}
