package storage

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"time"
)

var keyPattern = regexp.MustCompile(`^sha256/[0-9a-f]{2}/[0-9a-f]{2}/[0-9a-f]{64}\.jar$`)

func Key(hash string) string { return "sha256/" + hash[:2] + "/" + hash[2:4] + "/" + hash + ".jar" }
func validKey(k string) bool {
	return keyPattern.MatchString(k) && k[7:9] == k[13:15] && k[10:12] == k[15:17]
}

type Local struct{ root string }

func NewLocal(root string) (*Local, error) {
	p, e := filepath.Abs(root)
	if e != nil {
		return nil, e
	}
	if e = os.MkdirAll(p, 0700); e != nil {
		return nil, e
	}
	return &Local{p}, nil
}
func (l *Local) path(k string) (string, error) {
	if !validKey(k) {
		return "", fmt.Errorf("invalid object key")
	}
	return filepath.Join(l.root, filepath.FromSlash(k)), nil
}

type contextReader struct {
	ctx context.Context
	r   io.Reader
}

func (r contextReader) Read(b []byte) (int, error) {
	if e := r.ctx.Err(); e != nil {
		return 0, e
	}
	return r.r.Read(b)
}
func (l *Local) Put(ctx context.Context, k string, r io.Reader, size int64) error {
	p, e := l.path(k)
	if e != nil {
		return e
	}
	if e = os.MkdirAll(filepath.Dir(p), 0700); e != nil {
		return e
	}
	f, e := os.CreateTemp(filepath.Dir(p), ".upload-")
	if e != nil {
		return e
	}
	defer os.Remove(f.Name())
	defer f.Close()
	h := sha256.New()
	n, e := io.Copy(io.MultiWriter(f, h), io.LimitReader(contextReader{ctx, r}, size+1))
	if e != nil {
		return e
	}
	if n != size || hex.EncodeToString(h.Sum(nil)) != k[13:77] {
		return fmt.Errorf("object size/hash mismatch")
	}
	if e = f.Sync(); e != nil {
		return e
	}
	if e = f.Close(); e != nil {
		return e
	}
	e = os.Link(f.Name(), p)
	if errors.Is(e, os.ErrExist) {
		old, e := os.Open(p)
		if e != nil {
			return e
		}
		defer old.Close()
		h.Reset()
		n, e = io.Copy(h, old)
		if e != nil {
			return e
		}
		if n != size || hex.EncodeToString(h.Sum(nil)) != k[13:77] {
			return fmt.Errorf("existing object corrupt")
		}
		return nil
	}
	return e
}
func (l *Local) Open(ctx context.Context, k string) (ReadSeekCloser, error) {
	if e := ctx.Err(); e != nil {
		return nil, e
	}
	p, e := l.path(k)
	if e != nil {
		return nil, e
	}
	return os.Open(p)
}
func (l *Local) Exists(ctx context.Context, k string) (bool, error) {
	if e := ctx.Err(); e != nil {
		return false, e
	}
	p, e := l.path(k)
	if e != nil {
		return false, e
	}
	_, e = os.Stat(p)
	if errors.Is(e, os.ErrNotExist) {
		return false, nil
	}
	return e == nil, e
}
func (l *Local) Delete(ctx context.Context, k string) error {
	if e := ctx.Err(); e != nil {
		return e
	}
	p, e := l.path(k)
	if e != nil {
		return e
	}
	e = os.Remove(p)
	if errors.Is(e, os.ErrNotExist) {
		return nil
	}
	return e
}
func (l *Local) SignedDownloadURL(context.Context, string, time.Duration) (string, error) {
	return "", nil
}
