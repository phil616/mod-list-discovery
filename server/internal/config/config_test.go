package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestConfigEnvironment(t *testing.T) {
	p := filepath.Join(t.TempDir(), "config.yaml")
	if e := os.WriteFile(p, []byte("server:\n  listen: 127.0.0.1:8081\nstorage:\n  type: local\n"), 0600); e != nil {
		t.Fatal(e)
	}
	t.Setenv("MLD_SERVER_LISTEN", "127.0.0.1:8888")
	t.Setenv("MLD_SECURITY_MAX_UPLOAD_SIZE", "1000")
	c, e := Load(p)
	if e != nil || c.Server.Listen != "127.0.0.1:8888" || c.Security.MaxUploadSize != 1000 {
		t.Fatal(c, e)
	}
	t.Setenv("MLD_SECURITY_SESSION_TTL", "invalid")
	if _, e = Load(p); e == nil {
		t.Fatal("invalid TTL accepted")
	}
}
