package artifact

import (
	"archive/zip"
	"net/http"
	"net/http/httptest"
	"net/netip"
	"os"
	"path/filepath"
	"testing"
)

func TestSSRF(t *testing.T) {
	for _, ip := range []string{"127.0.0.1", "10.1.2.3", "169.254.169.254", "::1", "fc00::1", "100.100.100.200", "::ffff:127.0.0.1"} {
		if publicIP(netip.MustParseAddr(ip).Unmap()) {
			t.Fatalf("private IP accepted: %s", ip)
		}
	}
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.Write([]byte("jar")) }))
	defer s.Close()
	c := ImportClient(false)
	defer c.CloseIdleConnections()
	if _, e := c.Get(s.URL); e == nil {
		t.Fatal("HTTP localhost accepted")
	}
	c = ImportClient(true)
	defer c.CloseIdleConnections()
	resp, e := c.Get(s.URL)
	if e != nil {
		t.Fatal(e)
	}
	resp.Body.Close()
	if _, e = c.Get("file:///etc/passwd"); e == nil {
		t.Fatal("file accepted")
	}
}
func TestMetadata(t *testing.T) {
	for _, tc := range []struct {
		name, body string
		valid      bool
	}{{"missing", "", false}, {"invalid", "{", false}, {"valid", `{"id":"example","version":"1.2.3","depends":{"fabricloader":">=0.19.0"}}`, true}} {
		t.Run(tc.name, func(t *testing.T) {
			p := filepath.Join(t.TempDir(), "test.jar")
			f, e := os.Create(p)
			if e != nil {
				t.Fatal(e)
			}
			z := zip.NewWriter(f)
			if tc.body != "" {
				w, e := z.Create("fabric.mod.json")
				if e != nil {
					t.Fatal(e)
				}
				w.Write([]byte(tc.body))
			}
			z.Close()
			f.Close()
			m, raw, e := Parse(p)
			if (e == nil) != tc.valid {
				t.Fatal(e)
			}
			if tc.valid && (m.ID != "example" || m.Environment != "*" || raw == "") {
				t.Fatal(m)
			}
		})
	}
}
