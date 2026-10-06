package signing

import (
	"crypto/ed25519"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
)

type Key struct {
	Algorithm string `json:"algorithm"`
	KeyID     string `json:"keyId"`
	PublicKey string `json:"publicKey"`
	Value     string `json:"value,omitempty"`
}
type Envelope struct {
	SchemaVersion   int    `json:"schemaVersion"`
	PayloadEncoding string `json:"payloadEncoding"`
	Payload         string `json:"payload"`
	Signature       Key    `json:"signature"`
}

func Create(path string) error {
	if e := os.MkdirAll(filepath.Dir(path), 0700); e != nil {
		return e
	}
	_, k, e := ed25519.GenerateKey(rand.Reader)
	if e != nil {
		return e
	}
	f, e := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if e != nil {
		return e
	}
	defer f.Close()
	if _, e = f.Write(k); e != nil {
		return e
	}
	return f.Sync()
}
func Load(path string) (ed25519.PrivateKey, error) {
	b, e := os.ReadFile(path)
	if e != nil {
		return nil, e
	}
	if len(b) != ed25519.PrivateKeySize {
		return nil, fmt.Errorf("invalid signing key")
	}
	info, e := os.Stat(path)
	if e != nil {
		return nil, e
	}
	if info.Mode().Perm()&0077 != 0 {
		return nil, fmt.Errorf("signing key must have permissions 0600")
	}
	k := ed25519.NewKeyFromSeed(b[:32])
	if !k.Equal(ed25519.PrivateKey(b)) {
		return nil, fmt.Errorf("inconsistent signing key")
	}
	return k, nil
}
func Public(k ed25519.PrivateKey) Key {
	p := k.Public().(ed25519.PublicKey)
	h := sha256.Sum256(p)
	return Key{Algorithm: "Ed25519", KeyID: "ed25519:" + hex.EncodeToString(h[:])[:16], PublicKey: base64.RawURLEncoding.EncodeToString(p)}
}
func Sign(k ed25519.PrivateKey, p []byte) Envelope {
	s := Public(k)
	s.Value = base64.RawURLEncoding.EncodeToString(ed25519.Sign(k, p))
	return Envelope{1, "base64url", base64.RawURLEncoding.EncodeToString(p), s}
}
