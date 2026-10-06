package app

import (
	"database/sql"
	"encoding/json"
	"mod-list-discovery/server/internal/artifact"
	"mod-list-discovery/server/internal/auth"
	"mod-list-discovery/server/internal/config"
	"mod-list-discovery/server/internal/signing"
	"net/http"
	"strconv"
	"strings"
	"time"
)

func (a *App) authorize(w http.ResponseWriter, r *http.Request, server string) bool {
	var visibility, name string
	if e := a.DB.QueryRowContext(r.Context(), "SELECT visibility,display_name FROM servers WHERE id=?", server).Scan(&visibility, &name); e != nil {
		fail(w, 404, "server_not_found")
		return false
	}
	if visibility == "public" {
		return true
	}
	header := r.Header.Get("Authorization")
	var id string
	if !strings.HasPrefix(header, "Bearer ") || a.DB.QueryRowContext(r.Context(), "SELECT id FROM client_devices WHERE server_id=? AND token_hash=? AND revoked_at IS NULL", server, auth.Hash(strings.TrimPrefix(header, "Bearer "))).Scan(&id) != nil {
		w.Header().Set("Cache-Control", "no-store")
		jsonResponse(w, 401, map[string]string{"error": "authentication_required", "serverId": server, "displayName": name, "enrollmentEndpoint": "/api/v1/servers/" + server + "/enroll"})
		return false
	}
	_, e := a.DB.ExecContext(r.Context(), "UPDATE client_devices SET last_seen_at=? WHERE id=?", Now(), id)
	if e != nil {
		fail(w, 500, "database_error")
		return false
	}
	return true
}
func (a *App) manifest(w http.ResponseWriter, r *http.Request) {
	server := r.PathValue("serverId")
	w.Header().Set("Cache-Control", "no-cache, private")
	w.Header().Set("Vary", "Authorization")
	if !a.authorize(w, r, server) {
		return
	}
	channel := r.URL.Query().Get("channel")
	if channel == "" {
		channel = "stable"
	}
	var payload []byte
	e := a.DB.QueryRowContext(r.Context(), `SELECT r.payload FROM releases r JOIN channels c ON c.active_release_id=r.id AND c.server_id=r.server_id WHERE c.server_id=? AND c.name=? AND r.status='published'`, server, channel).Scan(&payload)
	if e != nil {
		fail(w, 404, "release_not_found")
		return
	}
	if channel != "stable" {
		var p map[string]any
		if json.Unmarshal(payload, &p) != nil {
			fail(w, 500, "manifest_error")
			return
		}
		p["channel"] = channel
		payload, e = json.Marshal(p)
		if e != nil {
			fail(w, 500, "manifest_error")
			return
		}
	}
	envelope := signing.Sign(a.Key, payload)
	b, e := json.Marshal(envelope)
	if e != nil || len(b) > 2<<20 {
		fail(w, 500, "manifest_too_large")
		return
	}
	etag := `"` + auth.Hash(string(b)) + `"`
	w.Header().Set("ETag", etag)
	if r.Header.Get("If-None-Match") == etag {
		w.WriteHeader(304)
		return
	}
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Write(b)
}
func (a *App) enroll(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	if !a.allowed(r, "enroll") {
		fail(w, 429, "rate_limited")
		return
	}
	var in struct {
		Code       string `json:"code"`
		DeviceName string `json:"deviceName"`
	}
	if !decode(w, r, &in) {
		return
	}
	if strings.TrimSpace(in.DeviceName) == "" || len(in.DeviceName) > 128 {
		fail(w, 400, "invalid_device_name")
		return
	}
	tx, e := a.DB.BeginTx(r.Context(), nil)
	if e != nil {
		fail(w, 500, "database_error")
		return
	}
	defer tx.Rollback()
	res, e := tx.ExecContext(r.Context(), `UPDATE enrollment_codes SET use_count=use_count+1 WHERE server_id=? AND code_hash=? AND revoked_at IS NULL AND expires_at>? AND use_count<max_uses`, r.PathValue("serverId"), auth.Hash(in.Code), Now())
	if e != nil {
		fail(w, 500, "database_error")
		return
	}
	n, _ := res.RowsAffected()
	if n != 1 {
		fail(w, 403, "invalid_enrollment_code")
		return
	}
	id, token := auth.Token(), auth.Token()
	_, e = tx.ExecContext(r.Context(), "INSERT INTO client_devices(id,server_id,name,token_hash,created_at) VALUES(?,?,?,?,?)", id, r.PathValue("serverId"), in.DeviceName, auth.Hash(token), Now())
	if e != nil {
		fail(w, 500, "database_error")
		return
	}
	if e = tx.Commit(); e != nil {
		fail(w, 500, "database_error")
		return
	}
	jsonResponse(w, 201, map[string]any{"deviceId": id, "accessToken": token, "serverId": r.PathValue("serverId"), "signingKey": signing.Public(a.Key)})
}
func (a *App) download(w http.ResponseWriter, r *http.Request) {
	server, hash := r.PathValue("serverId"), r.PathValue("sha256")
	w.Header().Set("Cache-Control", "private, no-store")
	if !artifact.SHA.MatchString(hash) {
		fail(w, 400, "invalid_sha256")
		return
	}
	if !a.authorize(w, r, server) {
		return
	}
	var key string
	var size int64
	e := a.DB.QueryRowContext(r.Context(), `SELECT a.storage_key,a.size FROM artifacts a WHERE a.sha256=? AND EXISTS(SELECT 1 FROM release_mods m JOIN releases r ON r.id=m.release_id WHERE m.artifact_sha256=a.sha256 AND r.server_id=? AND r.status='published')`, hash, server).Scan(&key, &size)
	if e == sql.ErrNoRows {
		fail(w, 404, "artifact_not_found")
		return
	}
	if e != nil {
		fail(w, 500, "database_error")
		return
	}
	u, e := a.Store.SignedDownloadURL(r.Context(), key, config.Duration(a.Config.Security.ArtifactURLTTL))
	if e != nil {
		fail(w, 503, "storage_unavailable")
		return
	}
	if u != "" {
		http.Redirect(w, r, u, 302)
		return
	}
	f, e := a.Store.Open(r.Context(), key)
	if e != nil {
		fail(w, 503, "storage_unavailable")
		return
	}
	defer f.Close()
	w.Header().Set("Content-Type", "application/java-archive")
	w.Header().Set("Content-Disposition", `attachment; filename="`+hash+`.jar"`)
	w.Header().Set("Content-Length", strconv.FormatInt(size, 10))
	http.ServeContent(w, r, hash+".jar", time.Time{}, f)
}
