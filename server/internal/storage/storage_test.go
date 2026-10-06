package storage

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"io"
	"mod-list-discovery/server/internal/config"
	"net/http"
	"net/http/httptest"
	urlpkg "net/url"
	"os"
	"strings"
	"testing"
	"time"
)

func TestLocal(t *testing.T) {
	ctx := context.Background()
	s, e := NewLocal(t.TempDir())
	if e != nil {
		t.Fatal(e)
	}
	b := []byte("artifact")
	h := sha256.Sum256(b)
	key := Key(hex.EncodeToString(h[:]))
	if e = s.Put(ctx, key, bytes.NewReader(b), int64(len(b))); e != nil {
		t.Fatal(e)
	}
	if e = s.Put(ctx, key, bytes.NewReader(b), int64(len(b))); e != nil {
		t.Fatal(e)
	}
	if e = s.Put(ctx, key, strings.NewReader("tampered"), 8); e == nil {
		t.Fatal("overwrite accepted")
	}
	f, e := s.Open(ctx, key)
	if e != nil {
		t.Fatal(e)
	}
	f.Seek(2, 0)
	got, e := io.ReadAll(f)
	f.Close()
	if e != nil || string(got) != "tifact" {
		t.Fatal("seek/open failed")
	}
	if _, e = s.Open(ctx, "../../escape"); e == nil {
		t.Fatal("path traversal")
	}
	p, _ := s.path(key)
	os.WriteFile(p, []byte("bad"), 0600)
	if e = s.Put(ctx, key, bytes.NewReader(b), int64(len(b))); e == nil {
		t.Fatal("corrupt existing object accepted")
	}
	if e = s.Delete(ctx, key); e != nil {
		t.Fatal(e)
	}
	exists, e := s.Exists(ctx, key)
	if e != nil || exists {
		t.Fatal("delete failed")
	}
}
func TestS3Adapter(t *testing.T) {
	var methods []string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		methods = append(methods, r.Method)
		if !strings.HasPrefix(r.URL.Path, "/bucket/prefix/sha256/") {
			t.Errorf("bad key %s", r.URL.Path)
		}
		if r.Method == "PUT" && r.Header.Get("If-None-Match") != "*" {
			t.Error("missing immutable put precondition")
		}
		w.WriteHeader(200)
	}))
	defer server.Close()
	t.Setenv("TEST_ACCESS", "access")
	t.Setenv("TEST_SECRET", "secret")
	s, e := NewS3(context.Background(), config.Remote{Endpoint: server.URL, Region: "us-east-1", Bucket: "bucket", Prefix: "prefix/", AccessKeyEnv: "TEST_ACCESS", SecretKeyEnv: "TEST_SECRET", ForcePathStyle: true})
	if e != nil {
		t.Fatal(e)
	}
	k := Key(strings.Repeat("a", 64))
	if e = s.Put(context.Background(), k, strings.NewReader("data"), 4); e != nil {
		t.Fatal(e)
	}
	if ok, e := s.Exists(context.Background(), k); !ok || e != nil {
		t.Fatal(e)
	}
	if e = s.Delete(context.Background(), k); e != nil {
		t.Fatal(e)
	}
	url, e := s.SignedDownloadURL(context.Background(), k, time.Minute)
	if e != nil || !strings.Contains(url, "X-Amz-Signature") {
		t.Fatal("presign failed", e)
	}
	if len(methods) != 3 {
		t.Fatal(methods)
	}
}
func TestOSSAdapterSigning(t *testing.T) {
	t.Setenv("TEST_ACCESS", "access")
	t.Setenv("TEST_SECRET", "secret")
	s, e := NewOSS(config.Remote{Endpoint: "https://oss-cn-hangzhou.aliyuncs.com", Bucket: "test-bucket", Prefix: "prefix/", AccessKeyEnv: "TEST_ACCESS", SecretKeyEnv: "TEST_SECRET"})
	if e != nil {
		t.Fatal(e)
	}
	url, e := s.SignedDownloadURL(context.Background(), Key(strings.Repeat("a", 64)), time.Minute)
	decoded, decodeErr := urlpkg.PathUnescape(url)
	if e != nil || decodeErr != nil || !strings.Contains(strings.ToLower(url), "signature=") || !strings.Contains(decoded, "prefix/sha256/") {
		t.Fatal("OSS signed URL", url, e)
	}
	if _, e = s.key("../unsafe"); e == nil {
		t.Fatal("unsafe key")
	}
}
