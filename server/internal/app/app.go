package app

import (
	"context"
	"crypto/ed25519"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"mod-list-discovery/server/internal/artifact"
	"mod-list-discovery/server/internal/auth"
	"mod-list-discovery/server/internal/config"
	"mod-list-discovery/server/internal/database"
	"mod-list-discovery/server/internal/signing"
	"mod-list-discovery/server/internal/storage"
	"mod-list-discovery/server/internal/webui"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"time"
)

// Version is set by the build with -ldflags -X; direct go builds use the development baseline.
var Version = "0.1.0"

type App struct {
	Config    config.Config
	DB        *sql.DB
	Key       ed25519.PrivateKey
	Store     storage.Storage
	Artifacts *artifact.Service
	mu        sync.Mutex
	rateMu    sync.Mutex
	rates     map[string]rate
	dummy     string
}
type rate struct {
	n     int
	until time.Time
}

func Now() string { return time.Now().UTC().Format(time.RFC3339) }
func Open(ctx context.Context, c config.Config) (*App, error) {
	db, e := database.Open(c.Database.Path)
	if e != nil {
		return nil, e
	}
	fail := func(e error) (*App, error) { db.Close(); return nil, e }
	k, e := signing.Load(filepath.Join(c.Server.DataDir, "signing", "ed25519.key"))
	if e != nil {
		return fail(e)
	}
	s, e := storage.New(ctx, c)
	if e != nil {
		return fail(e)
	}
	if _, e = s.Exists(ctx, storage.Key(strings.Repeat("0", 64))); e != nil {
		return fail(fmt.Errorf("storage unavailable: %w", e))
	}
	var different int
	if e = db.QueryRowContext(ctx, "SELECT count(*) FROM artifacts WHERE storage_backend!=?", c.Storage.Type).Scan(&different); e != nil {
		return fail(e)
	}
	if different > 0 {
		return fail(fmt.Errorf("storage backend differs from persisted artifacts; migrate objects before switching"))
	}
	dummy, e := auth.Password(auth.Token())
	if e != nil {
		return fail(e)
	}
	a := &App{Config: c, DB: db, Key: k, Store: s, rates: map[string]rate{}, dummy: dummy}
	a.Artifacts = &artifact.Service{DB: db, Store: s, TempDir: filepath.Join(c.Server.DataDir, "tmp"), Backend: c.Storage.Type, Limit: c.Security.MaxUploadSize}
	return a, nil
}
func Init(c config.Config, password string) error {
	if _, e := os.Stat(c.Database.Path); !errors.Is(e, os.ErrNotExist) {
		return fmt.Errorf("database already exists or cannot be inspected")
	}
	key := filepath.Join(c.Server.DataDir, "signing", "ed25519.key")
	if _, e := os.Stat(key); !errors.Is(e, os.ErrNotExist) {
		return fmt.Errorf("signing key already exists or cannot be inspected")
	}
	hash, e := auth.Password(password)
	if e != nil {
		return e
	}
	if e = os.MkdirAll(c.Server.DataDir, 0700); e != nil {
		return e
	}
	if e = signing.Create(key); e != nil {
		return e
	}
	db, e := database.Open(c.Database.Path)
	if e != nil {
		return e
	}
	defer db.Close()
	_, e = db.Exec("INSERT INTO admins VALUES(1,'admin',?,?,?)", hash, Now(), Now())
	return e
}
func (a *App) Audit(ctx context.Context, action, object string) {
	if _, e := a.DB.ExecContext(ctx, "INSERT INTO audit_logs(actor_type,actor_id,action,object_id,created_at) VALUES('admin','1',?,?,?)", action, object, Now()); e != nil {
		slog.Error("audit write failed", "action", action)
	}
}
func jsonResponse(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}
func fail(w http.ResponseWriter, status int, code string) {
	jsonResponse(w, status, map[string]string{"error": code, "message": strings.ReplaceAll(code, "_", " "), "requestId": w.Header().Get("X-Request-ID")})
}
func decode(w http.ResponseWriter, r *http.Request, v any) bool {
	r.Body = http.MaxBytesReader(w, r.Body, 2<<20)
	d := json.NewDecoder(r.Body)
	d.DisallowUnknownFields()
	if e := d.Decode(v); e != nil {
		fail(w, 400, "invalid_json")
		return false
	}
	var extra any
	if e := d.Decode(&extra); e != io.EOF {
		fail(w, 400, "invalid_json")
		return false
	}
	return true
}

var idRE = regexp.MustCompile(`^[a-z0-9][a-z0-9-]{0,63}$`)

func validID(id string) bool {
	if !idRE.MatchString(id) {
		return false
	}
	switch id {
	case "admin", "api", "assets", "health", "healthz", "metrics":
		return false
	}
	return true
}
func (a *App) allowed(r *http.Request, key string) bool {
	host, _, _ := net.SplitHostPort(r.RemoteAddr)
	if a.Config.Server.TrustedReverseProxy {
		if ip := strings.TrimSpace(strings.Split(r.Header.Get("X-Forwarded-For"), ",")[0]); net.ParseIP(ip) != nil {
			host = ip
		}
	}
	key = host + ":" + key
	a.rateMu.Lock()
	defer a.rateMu.Unlock()
	now := time.Now()
	v := a.rates[key]
	if now.After(v.until) {
		v = rate{until: now.Add(time.Minute)}
	}
	v.n++
	if len(a.rates) > 10000 {
		for k, x := range a.rates {
			if now.After(x.until) {
				delete(a.rates, k)
			}
		}
		if len(a.rates) > 10000 {
			return false
		}
	}
	a.rates[key] = v
	return v.n <= 10
}
func (a *App) session(r *http.Request) string {
	c, e := r.Cookie("mld_session")
	if e != nil {
		return ""
	}
	var id string
	e = a.DB.QueryRowContext(r.Context(), "SELECT id FROM admin_sessions WHERE token_hash=? AND expires_at>?", auth.Hash(c.Value), Now()).Scan(&id)
	if e != nil {
		return ""
	}
	return id
}
func (a *App) admin(h http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "no-store")
		session := a.session(r)
		if session == "" {
			fail(w, 401, "authentication_required")
			return
		}
		if r.Method != "GET" && r.Method != "HEAD" {
			if r.Header.Get("X-CSRF-Token") != session {
				fail(w, 403, "csrf_failed")
				return
			}
			if origin := r.Header.Get("Origin"); origin != "" && origin != strings.TrimRight(a.Config.Server.PublicBaseURL, "/") {
				fail(w, 403, "csrf_failed")
				return
			}
		}
		h(w, r)
	}
}
func (a *App) Handler() http.Handler {
	m := http.NewServeMux()
	m.HandleFunc("GET /healthz", func(w http.ResponseWriter, r *http.Request) {
		if e := a.DB.PingContext(r.Context()); e != nil {
			fail(w, 503, "database_unavailable")
			return
		}
		jsonResponse(w, 200, map[string]string{"status": "ok"})
	})
	m.HandleFunc("POST /api/v1/admin/login", a.login)
	m.HandleFunc("POST /api/v1/admin/logout", a.admin(a.logout))
	m.HandleFunc("GET /api/v1/admin/session", a.admin(func(w http.ResponseWriter, r *http.Request) {
		jsonResponse(w, 200, map[string]string{"csrfToken": a.session(r)})
	}))
	m.HandleFunc("GET /api/v1/admin/overview", a.admin(a.overview))
	m.HandleFunc("GET /api/v1/admin/settings", a.admin(a.settings))
	m.HandleFunc("/api/v1/admin/", a.admin(a.adminAPI))
	m.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "GET" && r.Method != "HEAD" {
			fail(w, 405, "method_not_allowed")
			return
		}
		parts := strings.Split(strings.TrimPrefix(r.URL.Path, "/"), "/")
		if len(parts) != 2 || parts[1] != "mod-list.json" || !validID(parts[0]) {
			fail(w, 404, "not_found")
			return
		}
		r.SetPathValue("serverId", parts[0])
		a.manifest(w, r)
	})
	m.HandleFunc("POST /api/v1/servers/{serverId}/enroll", a.enroll)
	m.HandleFunc("GET /api/v1/servers/{serverId}/artifacts/{sha256}/download", a.download)
	m.Handle("GET /assets/", webui.Assets())
	m.HandleFunc("GET /login", webui.Page)
	m.HandleFunc("GET /admin/", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "no-store")
		if a.session(r) == "" {
			http.Redirect(w, r, "/login", 303)
			return
		}
		webui.Page(w, r)
	})
	m.HandleFunc("GET /{$}", func(w http.ResponseWriter, r *http.Request) { http.Redirect(w, r, "/admin/", 303) })
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Request-ID", auth.Token()[:16])
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("X-Frame-Options", "DENY")
		w.Header().Set("Referrer-Policy", "no-referrer")
		w.Header().Set("Content-Security-Policy", "default-src 'self'; script-src 'self'; style-src 'self'; img-src 'self' data:; frame-ancestors 'none'; base-uri 'none'")
		m.ServeHTTP(w, r)
	})
}
func (a *App) login(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	if o := r.Header.Get("Origin"); o != "" && o != strings.TrimRight(a.Config.Server.PublicBaseURL, "/") {
		fail(w, 403, "csrf_failed")
		return
	}
	var in struct {
		Username string `json:"username"`
		Password string `json:"password"`
	}
	if !decode(w, r, &in) {
		return
	}
	if !a.allowed(r, "login") || !a.allowed(r, "login:"+in.Username) {
		fail(w, 429, "rate_limited")
		return
	}
	var hash string
	e := a.DB.QueryRowContext(r.Context(), "SELECT password_hash FROM admins WHERE username=?", in.Username).Scan(&hash)
	if e != nil {
		hash = a.dummy
	}
	ok := auth.Verify(hash, in.Password)
	if e != nil || !ok {
		a.Audit(r.Context(), "admin.login.failure", "")
		jsonResponse(w, 401, map[string]string{"error": "invalid_credentials", "message": "invalid username or password"})
		return
	}
	token, id := auth.Token(), auth.Token()
	expiry := time.Now().Add(config.Duration(a.Config.Security.SessionTTL))
	_, e = a.DB.ExecContext(r.Context(), "INSERT INTO admin_sessions(id,token_hash,created_at,expires_at) VALUES(?,?,?,?)", id, auth.Hash(token), Now(), expiry.UTC().Format(time.RFC3339))
	if e != nil {
		fail(w, 500, "database_error")
		return
	}
	http.SetCookie(w, &http.Cookie{Name: "mld_session", Value: token, Path: "/", HttpOnly: true, Secure: strings.HasPrefix(a.Config.Server.PublicBaseURL, "https:"), SameSite: http.SameSiteLaxMode, Expires: expiry})
	a.Audit(r.Context(), "admin.login.success", "")
	jsonResponse(w, 200, map[string]string{"csrfToken": id})
}
func (a *App) logout(w http.ResponseWriter, r *http.Request) {
	if _, e := a.DB.ExecContext(r.Context(), "DELETE FROM admin_sessions WHERE id=?", a.session(r)); e != nil {
		fail(w, 500, "database_error")
		return
	}
	http.SetCookie(w, &http.Cookie{Name: "mld_session", Value: "", Path: "/", MaxAge: -1, HttpOnly: true, SameSite: http.SameSiteLaxMode, Secure: strings.HasPrefix(a.Config.Server.PublicBaseURL, "https:")})
	jsonResponse(w, 200, map[string]bool{"ok": true})
}
func (a *App) settings(w http.ResponseWriter, r *http.Request) {
	jsonResponse(w, 200, map[string]any{"publicBaseUrl": a.Config.Server.PublicBaseURL, "storage": a.Config.Storage.Type, "version": Version, "signingKey": signing.Public(a.Key)})
}
func (a *App) overview(w http.ResponseWriter, r *http.Request) {
	out := map[string]any{"version": Version, "storage": a.Config.Storage.Type}
	for _, t := range []string{"servers", "artifacts", "releases", "client_devices"} {
		var n int
		if e := a.DB.QueryRowContext(r.Context(), "SELECT count(*) FROM "+t).Scan(&n); e != nil {
			fail(w, 500, "database_error")
			return
		}
		out[t] = n
	}
	jsonResponse(w, 200, out)
}
func (a *App) rows(ctx context.Context, q string, args ...any) ([]map[string]any, error) {
	rows, e := a.DB.QueryContext(ctx, q, args...)
	if e != nil {
		return nil, e
	}
	defer rows.Close()
	cols, e := rows.Columns()
	if e != nil {
		return nil, e
	}
	out := []map[string]any{}
	for rows.Next() {
		vals := make([]any, len(cols))
		ptr := make([]any, len(cols))
		for i := range vals {
			ptr[i] = &vals[i]
		}
		if e = rows.Scan(ptr...); e != nil {
			return nil, e
		}
		m := map[string]any{}
		for i, k := range cols {
			if b, ok := vals[i].([]byte); ok {
				vals[i] = string(b)
			}
			m[k] = vals[i]
		}
		out = append(out, m)
	}
	return out, rows.Err()
}
func (a *App) list(w http.ResponseWriter, r *http.Request, q string, args ...any) {
	out, e := a.rows(r.Context(), q, args...)
	if e != nil {
		fail(w, 500, "database_error")
		return
	}
	jsonResponse(w, 200, out)
}
