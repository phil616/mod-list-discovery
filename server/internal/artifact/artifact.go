package artifact

import (
	"archive/zip"
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"mod-list-discovery/server/internal/storage"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"
)

var ModID = regexp.MustCompile(`^[a-z][a-z0-9_-]{1,63}$`)
var SHA = regexp.MustCompile(`^[0-9a-f]{64}$`)

type Metadata struct {
	ID          string                     `json:"id"`
	Name        string                     `json:"name"`
	Version     string                     `json:"version"`
	Environment string                     `json:"environment"`
	Depends     map[string]json.RawMessage `json:"depends"`
	Conflicts   map[string]json.RawMessage `json:"conflicts"`
	Breaks      map[string]json.RawMessage `json:"breaks"`
	Provides    []string                   `json:"provides"`
}

func Parse(path string) (Metadata, string, error) {
	var m Metadata
	z, e := zip.OpenReader(path)
	if e != nil {
		return m, "", fmt.Errorf("invalid JAR")
	}
	defer z.Close()
	var data []byte
	for _, f := range z.File {
		if f.Name != "fabric.mod.json" {
			continue
		}
		if data != nil || f.UncompressedSize64 > 1<<20 {
			return m, "", fmt.Errorf("invalid metadata size or duplicate entry")
		}
		r, e := f.Open()
		if e != nil {
			return m, "", e
		}
		data, e = io.ReadAll(io.LimitReader(r, (1<<20)+1))
		r.Close()
		if e != nil {
			return m, "", e
		}
		if len(data) > 1<<20 {
			return m, "", fmt.Errorf("metadata too large")
		}
	}
	if len(data) == 0 {
		return m, "", fmt.Errorf("missing fabric.mod.json")
	}
	if e = json.Unmarshal(data, &m); e != nil {
		return m, "", fmt.Errorf("invalid fabric.mod.json")
	}
	if !ModID.MatchString(m.ID) || m.Version == "" {
		return m, "", fmt.Errorf("invalid mod id/version")
	}
	if m.Environment == "" {
		m.Environment = "*"
	}
	if m.Environment != "*" && m.Environment != "client" && m.Environment != "server" {
		return m, "", fmt.Errorf("invalid environment")
	}
	if m.Name == "" {
		m.Name = m.ID
	}
	return m, string(data), nil
}

type Service struct {
	DB               *sql.DB
	Store            storage.Storage
	TempDir, Backend string
	Limit            int64
}

func (s *Service) Import(ctx context.Context, r io.Reader, name string) (string, error) {
	if e := os.MkdirAll(s.TempDir, 0700); e != nil {
		return "", e
	}
	f, e := os.CreateTemp(s.TempDir, "artifact-")
	if e != nil {
		return "", e
	}
	defer os.Remove(f.Name())
	defer f.Close()
	h := sha256.New()
	n, e := io.Copy(io.MultiWriter(f, h), io.LimitReader(contextReader{ctx: ctx, reader: r}, s.Limit+1))
	if e != nil {
		return "", e
	}
	if n > s.Limit {
		return "", fmt.Errorf("upload too large")
	}
	hash := hex.EncodeToString(h.Sum(nil))
	var count int
	if e = s.DB.QueryRowContext(ctx, "SELECT count(*) FROM artifacts WHERE sha256=?", hash).Scan(&count); e != nil {
		return "", e
	}
	if count > 0 {
		return hash, nil
	}
	m, raw, e := Parse(f.Name())
	if e != nil {
		return "", e
	}
	if _, e = f.Seek(0, 0); e != nil {
		return "", e
	}
	key := storage.Key(hash)
	exists, e := s.Store.Exists(ctx, key)
	if e != nil {
		return "", e
	}
	if !exists {
		if e = s.Store.Put(ctx, key, f, n); e != nil {
			return "", e
		}
	}
	name = filepath.Base(strings.ReplaceAll(name, "\\", "/"))
	if name == "." || name == "/" || name == "" {
		name = m.ID + ".jar"
	}
	_, e = s.DB.ExecContext(ctx, `INSERT INTO artifacts VALUES(?,?,?,?,?,?,?,?,?,?,?)`, hash, name, n, s.Backend, key, m.ID, m.Name, m.Version, m.Environment, raw, time.Now().UTC().Format(time.RFC3339))
	if e != nil && !exists {
		cleanup, cancel := context.WithTimeout(context.WithoutCancel(ctx), 10*time.Second)
		defer cancel()
		var references int
		if queryErr := s.DB.QueryRowContext(cleanup, "SELECT count(*) FROM artifacts WHERE sha256=?", hash).Scan(&references); queryErr == nil && references == 0 {
			if deleteErr := s.Store.Delete(cleanup, key); deleteErr != nil {
				slog.Warn("orphan artifact cleanup failed", "sha256", hash)
			}
		}
	}
	return hash, e
}

type contextReader struct {
	ctx    context.Context
	reader io.Reader
}

func (r contextReader) Read(p []byte) (int, error) {
	if e := r.ctx.Err(); e != nil {
		return 0, e
	}
	return r.reader.Read(p)
}
