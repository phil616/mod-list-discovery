package app

import (
	"archive/zip"
	"bytes"
	"context"
	"crypto/ed25519"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"mod-list-discovery/server/internal/auth"
	"mod-list-discovery/server/internal/config"
	"mod-list-discovery/server/internal/signing"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func testApp(t *testing.T) *App {
	t.Helper()
	dir := t.TempDir()
	cfg := filepath.Join(dir, "config.yaml")
	if e := os.WriteFile(cfg, []byte("server:\n  data_dir: "+dir+"\n"), 0600); e != nil {
		t.Fatal(e)
	}
	c, e := config.Load(cfg)
	if e != nil {
		t.Fatal(e)
	}
	if e = Init(c, "correct-password-123"); e != nil {
		t.Fatal(e)
	}
	if e = Init(c, "other-password-123"); e == nil {
		t.Fatal("init overwrote existing state")
	}
	a, e := Open(context.Background(), c)
	if e != nil {
		t.Fatal(e)
	}
	t.Cleanup(func() { a.DB.Close() })
	return a
}

type apiClient struct {
	t      *testing.T
	h      http.Handler
	cookie *http.Cookie
	csrf   string
}

func (c *apiClient) request(method, path string, body any, token string) *httptest.ResponseRecorder {
	c.t.Helper()
	var reader io.Reader
	if body != nil {
		b, e := json.Marshal(body)
		if e != nil {
			c.t.Fatal(e)
		}
		reader = bytes.NewReader(b)
	}
	req := httptest.NewRequest(method, path, reader)
	if c.cookie != nil {
		req.AddCookie(c.cookie)
	}
	if c.csrf != "" {
		req.Header.Set("X-CSRF-Token", c.csrf)
	}
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	w := httptest.NewRecorder()
	c.h.ServeHTTP(w, req)
	return w
}
func expect(t *testing.T, w *httptest.ResponseRecorder, status int) {
	t.Helper()
	if w.Code != status {
		t.Fatalf("status %d want %d: %s", w.Code, status, w.Body.String())
	}
}
func data(t *testing.T, w *httptest.ResponseRecorder) map[string]any {
	t.Helper()
	var m map[string]any
	if e := json.Unmarshal(w.Body.Bytes(), &m); e != nil {
		t.Fatal(e)
	}
	return m
}
func (c *apiClient) login() {
	w := c.request("POST", "/api/v1/admin/login", map[string]string{"username": "admin", "password": "correct-password-123"}, "")
	expect(c.t, w, 200)
	c.cookie = w.Result().Cookies()[0]
	c.csrf = data(c.t, w)["csrfToken"].(string)
}
func fixtureJar(t *testing.T, id, version string) []byte {
	t.Helper()
	var b bytes.Buffer
	z := zip.NewWriter(&b)
	f, e := z.Create("fabric.mod.json")
	if e != nil {
		t.Fatal(e)
	}
	fmt.Fprintf(f, `{"schemaVersion":1,"id":%q,"name":"Fixture","version":%q,"environment":"client"}`, id, version)
	if e = z.Close(); e != nil {
		t.Fatal(e)
	}
	return b.Bytes()
}
func TestEndToEndAPI(t *testing.T) {
	a := testApp(t)
	c := apiClient{t: t, h: a.Handler()}
	expect(t, c.request("GET", "/api/v1/admin/servers", nil, ""), 401)
	c.login()
	saved := c.csrf
	c.csrf = "bad"
	expect(t, c.request("POST", "/api/v1/admin/servers", map[string]string{}, ""), 403)
	c.csrf = saved
	for _, id := range []string{"private", "public", "unrelated"} {
		visibility := "private"
		if id == "public" {
			visibility = "public"
		}
		expect(t, c.request("POST", "/api/v1/admin/servers", map[string]string{"id": id, "displayName": id, "visibility": visibility}, ""), 201)
	}
	jar := fixtureJar(t, "example", "1.0.0")
	hash, e := a.Artifacts.Import(context.Background(), bytes.NewReader(jar), "../../test.jar")
	if e != nil {
		t.Fatal(e)
	}
	again, e := a.Artifacts.Import(context.Background(), bytes.NewReader(jar), "renamed.jar")
	if e != nil || again != hash {
		t.Fatal("dedup failed", e)
	}
	createRelease := func(server, name string) int64 {
		w := c.request("POST", "/api/v1/admin/servers/"+server+"/releases", map[string]string{"name": name}, "")
		expect(t, w, 201)
		id := int64(data(t, w)["id"].(float64))
		base := fmt.Sprintf("/api/v1/admin/releases/%d", id)
		expect(t, c.request("POST", base+"/mods", map[string]any{"modId": "example", "requirement": "required", "artifact": hash}, ""), 200)
		expect(t, c.request("POST", base+"/publish", map[string]any{}, ""), 200)
		expect(t, c.request("DELETE", base+"/mods/example", nil, ""), 409)
		expect(t, c.request("POST", base+"/mods", map[string]any{"modId": "example", "requirement": "forbidden"}, ""), 409)
		return id
	}
	privateID := createRelease("private", "first")
	publicID := createRelease("public", "first")
	for server, id := range map[string]int64{"private": privateID, "public": publicID} {
		expect(t, c.request("PUT", "/api/v1/admin/servers/"+server+"/channels/stable", map[string]any{"releaseId": id}, ""), 200)
	}
	expect(t, c.request("GET", "/private/mod-list.json", nil, ""), 401)
	expect(t, c.request("GET", "/public/mod-list.json", nil, ""), 200)
	w := c.request("POST", "/api/v1/admin/servers/private/enrollment-codes", map[string]any{"maxUses": 1}, "")
	expect(t, w, 201)
	code := data(t, w)["code"].(string)
	w = c.request("POST", "/api/v1/servers/private/enroll", map[string]string{"code": code, "deviceName": "test"}, "")
	expect(t, w, 201)
	token, device := data(t, w)["accessToken"].(string), data(t, w)["deviceId"].(string)
	expect(t, c.request("POST", "/api/v1/servers/private/enroll", map[string]string{"code": code, "deviceName": "again"}, ""), 403)
	w = c.request("GET", "/private/mod-list.json", nil, token)
	expect(t, w, 200)
	etag := w.Header().Get("ETag")
	var env signing.Envelope
	if e = json.Unmarshal(w.Body.Bytes(), &env); e != nil {
		t.Fatal(e)
	}
	payload, _ := base64.RawURLEncoding.DecodeString(env.Payload)
	sig, _ := base64.RawURLEncoding.DecodeString(env.Signature.Value)
	pub, _ := base64.RawURLEncoding.DecodeString(env.Signature.PublicKey)
	if !ed25519.Verify(pub, payload, sig) {
		t.Fatal("signature invalid")
	}
	payload[0] ^= 1
	if ed25519.Verify(pub, payload, sig) {
		t.Fatal("tamper accepted")
	}
	expect(t, c.request("PATCH", "/api/v1/admin/servers/private", map[string]string{"displayName": "changed", "visibility": "private"}, ""), 200)
	w = c.request("GET", "/private/mod-list.json", nil, token)
	if w.Header().Get("ETag") != etag {
		t.Fatal("published payload changed")
	}
	route := "/api/v1/servers/private/artifacts/" + hash + "/download"
	expect(t, c.request("GET", route, nil, ""), 401)
	w = c.request("GET", route, nil, token)
	expect(t, w, 200)
	if !bytes.Equal(w.Body.Bytes(), jar) {
		t.Fatal("wrong artifact")
	}
	expect(t, c.request("GET", "/api/v1/servers/unrelated/artifacts/"+hash+"/download", nil, token), 401)
	expect(t, c.request("DELETE", "/api/v1/admin/artifacts/"+hash, nil, ""), 409)
	expect(t, c.request("DELETE", "/api/v1/admin/servers/private?confirm=true", nil, ""), 409)
	second := createRelease("private", "second")
	for _, id := range []int64{second, privateID} {
		expect(t, c.request("PUT", "/api/v1/admin/servers/private/channels/stable", map[string]any{"releaseId": id}, ""), 200)
	}
	w = c.request("GET", "/private/mod-list.json", nil, token)
	if w.Header().Get("ETag") != etag {
		t.Fatal("rollback changed payload")
	}
	expect(t, c.request("POST", "/api/v1/admin/clients/"+device+"/revoke", map[string]any{}, ""), 200)
	expect(t, c.request("GET", "/private/mod-list.json", nil, token), 401)
	expect(t, c.request("GET", route, nil, token), 401)
	var n int
	if e = a.DB.QueryRow("SELECT count(*) FROM client_devices WHERE token_hash=?", auth.Hash(token)).Scan(&n); e != nil || n != 1 {
		t.Fatal("token not hashed")
	}
	if e = a.DB.QueryRow("SELECT count(*) FROM enrollment_codes WHERE code_hash=?", auth.Hash(code)).Scan(&n); e != nil || n != 1 {
		t.Fatal("code not hashed")
	}
	expect(t, c.request("GET", "/admin/settings", nil, ""), 200)
}
func TestAuthExpiryAndRateLimit(t *testing.T) {
	a := testApp(t)
	c := apiClient{t: t, h: a.Handler()}
	expect(t, c.request("POST", "/api/v1/admin/login", map[string]string{"username": "admin", "password": "bad"}, ""), 401)
	c.login()
	if _, e := a.DB.Exec("UPDATE admin_sessions SET expires_at=?", time.Now().Add(-time.Hour).UTC().Format(time.RFC3339)); e != nil {
		t.Fatal(e)
	}
	expect(t, c.request("GET", "/api/v1/admin/servers", nil, ""), 401)
	for i := 0; i < 12; i++ {
		c.request("POST", "/api/v1/admin/login", map[string]string{"username": "missing", "password": "bad"}, "")
	}
	expect(t, c.request("POST", "/api/v1/admin/login", map[string]string{"username": "missing", "password": "bad"}, ""), 429)
}
func TestExpiredEnrollmentAndUpload(t *testing.T) {
	a := testApp(t)
	c := apiClient{t: t, h: a.Handler()}
	c.login()
	expect(t, c.request("POST", "/api/v1/admin/servers", map[string]string{"id": "test", "displayName": "test", "visibility": "private"}, ""), 201)
	w := c.request("POST", "/api/v1/admin/servers/test/enrollment-codes", map[string]any{"maxUses": 2}, "")
	expect(t, w, 201)
	code := data(t, w)["code"].(string)
	if _, e := a.DB.Exec("UPDATE enrollment_codes SET expires_at='2000-01-01T00:00:00Z'"); e != nil {
		t.Fatal(e)
	}
	expect(t, c.request("POST", "/api/v1/servers/test/enroll", map[string]string{"code": code, "deviceName": "test"}, ""), 403)
	var b bytes.Buffer
	mw := multipart.NewWriter(&b)
	part, e := mw.CreateFormFile("file", "test.jar")
	if e != nil {
		t.Fatal(e)
	}
	part.Write(fixtureJar(t, "example", "1.0"))
	mw.Close()
	req := httptest.NewRequest("POST", "/api/v1/admin/artifacts/upload", &b)
	req.Header.Set("Content-Type", mw.FormDataContentType())
	req.Header.Set("X-CSRF-Token", c.csrf)
	req.AddCookie(c.cookie)
	out := httptest.NewRecorder()
	c.h.ServeHTTP(out, req)
	expect(t, out, 201)
	if _, e = a.Artifacts.Import(context.Background(), strings.NewReader("not a jar"), "bad.jar"); e == nil {
		t.Fatal("invalid jar accepted")
	}
}

func TestArtifactDatabaseFailureCleansNewObject(t *testing.T) {
	a := testApp(t)
	if _, e := a.DB.Exec(`CREATE TRIGGER reject_fixture BEFORE INSERT ON artifacts BEGIN SELECT RAISE(ABORT,'fixture database failure'); END;`); e != nil {
		t.Fatal(e)
	}
	jar := fixtureJar(t, "cleanup-fixture", "1.0.0")
	hash, e := a.Artifacts.Import(context.Background(), bytes.NewReader(jar), "fixture.jar")
	if e == nil {
		t.Fatal("expected database failure")
	}
	exists, e := a.Store.Exists(context.Background(), "sha256/"+hash[:2]+"/"+hash[2:4]+"/"+hash+".jar")
	if e != nil || exists {
		t.Fatal("new unreferenced object was not cleaned up", e)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, e = a.Artifacts.Import(ctx, bytes.NewReader(jar), "cancelled.jar"); e == nil {
		t.Fatal("cancelled import accepted")
	}
}
