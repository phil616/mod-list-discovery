package storage

import (
	"context"
	"fmt"
	"io"
	"mod-list-discovery/server/internal/config"
	"time"
)

type ReadSeekCloser interface {
	io.Reader
	io.Seeker
	io.Closer
}
type Storage interface {
	Put(context.Context, string, io.Reader, int64) error
	Open(context.Context, string) (ReadSeekCloser, error)
	Exists(context.Context, string) (bool, error)
	Delete(context.Context, string) error
	SignedDownloadURL(context.Context, string, time.Duration) (string, error)
}

func New(ctx context.Context, c config.Config) (Storage, error) {
	switch c.Storage.Type {
	case "local":
		return NewLocal(c.Storage.Local.Root)
	case "s3":
		return NewS3(ctx, c.Storage.S3)
	case "oss":
		return NewOSS(c.Storage.OSS)
	default:
		return nil, fmt.Errorf("unknown storage backend")
	}
}
