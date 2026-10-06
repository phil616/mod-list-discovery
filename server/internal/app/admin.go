package app

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"mod-list-discovery/server/internal/artifact"
	"mod-list-discovery/server/internal/auth"

	"net/http"
	"path"
	"strconv"
	"strings"
	"time"
)

func (a *App) adminAPI(w http.ResponseWriter, r *http.Request) {
	p := strings.Split(strings.Trim(strings.TrimPrefix(r.URL.Path, "/api/v1/admin/"), "/"), "/")
	if r.Method != "GET" {
		a.mu.Lock()
		defer a.mu.Unlock()
	}
	switch p[0] {
	case "servers":
		a.servers(w, r, p)
	case "artifacts":
		a.artifacts(w, r, p)
	case "releases":
		a.releases(w, r, p)
	case "clients", "enrollment-codes":
		if len(p) == 3 && p[2] == "revoke" && r.Method == "POST" {
			table := "client_devices"
			if p[0] == "enrollment-codes" {
				table = "enrollment_codes"
			}
			a.mutate(w, r, "UPDATE "+table+" SET revoked_at=? WHERE id=?", p[0]+".revoke", p[1], Now(), p[1])
			return
		}
		if len(p) == 1 && r.Method == "GET" {
			if p[0] == "clients" {
				a.list(w, r, "SELECT id,server_id,name,created_at,last_seen_at,revoked_at FROM client_devices")
			} else {
				a.list(w, r, "SELECT id,server_id,expires_at,max_uses,use_count,revoked_at FROM enrollment_codes")
			}
			return
		}
		fail(w, 404, "not_found")
	case "audit":
		if r.Method == "GET" {
			a.list(w, r, "SELECT * FROM audit_logs ORDER BY id DESC LIMIT 200")
			return
		}
		fail(w, 405, "method_not_allowed")
	default:
		fail(w, 404, "not_found")
	}
}
func (a *App) mutate(w http.ResponseWriter, r *http.Request, q, action, object string, args ...any) {
	res, e := a.DB.ExecContext(r.Context(), q, args...)
	if e != nil {
		fail(w, 409, "operation_rejected")
		return
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		fail(w, 404, "not_found")
		return
	}
	a.Audit(r.Context(), action, object)
	jsonResponse(w, 200, map[string]bool{"ok": true})
}
func (a *App) servers(w http.ResponseWriter, r *http.Request, p []string) {
	if len(p) == 1 {
		switch r.Method {
		case "GET":
			a.list(w, r, "SELECT * FROM servers ORDER BY id")
		case "POST":
			var in struct {
				ID          string `json:"id"`
				DisplayName string `json:"displayName"`
				Description string `json:"description"`
				Visibility  string `json:"visibility"`
			}
			if !decode(w, r, &in) {
				return
			}
			if !validID(in.ID) || in.DisplayName == "" || (in.Visibility != "public" && in.Visibility != "private") {
				fail(w, 400, "invalid_server")
				return
			}
			tx, e := a.DB.BeginTx(r.Context(), nil)
			if e != nil {
				fail(w, 500, "database_error")
				return
			}
			defer tx.Rollback()
			_, e = tx.ExecContext(r.Context(), "INSERT INTO servers VALUES(?,?,?,?,?,?)", in.ID, in.DisplayName, in.Description, in.Visibility, Now(), Now())
			if e == nil {
				_, e = tx.ExecContext(r.Context(), "INSERT INTO channels VALUES(?,'stable',NULL,?)", in.ID, Now())
			}
			if e != nil {
				fail(w, 409, "server_exists")
				return
			}
			if e = tx.Commit(); e != nil {
				fail(w, 500, "database_error")
				return
			}
			a.Audit(r.Context(), "server.create", in.ID)
			jsonResponse(w, 201, in)
		default:
			fail(w, 405, "method_not_allowed")
		}
		return
	}
	id := p[1]
	if len(p) == 2 {
		switch r.Method {
		case "GET":
			a.list(w, r, "SELECT * FROM servers WHERE id=?", id)
		case "PATCH":
			var in struct {
				DisplayName string `json:"displayName"`
				Description string `json:"description"`
				Visibility  string `json:"visibility"`
			}
			if !decode(w, r, &in) {
				return
			}
			if in.DisplayName == "" || (in.Visibility != "public" && in.Visibility != "private") {
				fail(w, 400, "invalid_server")
				return
			}
			a.mutate(w, r, "UPDATE servers SET display_name=?,description=?,visibility=?,updated_at=? WHERE id=?", "server.update", id, in.DisplayName, in.Description, in.Visibility, Now(), id)
		case "DELETE":
			a.deleteServer(w, r, id)
		default:
			fail(w, 405, "method_not_allowed")
		}
		return
	}
	switch p[2] {
	case "releases":
		if len(p) != 3 {
			fail(w, 404, "not_found")
			return
		}
		if r.Method == "GET" {
			a.list(w, r, "SELECT id,server_id,name,status,minecraft_version,loader_constraint,java_constraint,min_client_version,created_at,published_at FROM releases WHERE server_id=? ORDER BY id DESC", id)
		} else if r.Method == "POST" {
			in := releaseInput{Minecraft: "26.2", FabricLoader: ">=0.19.0", Java: ">=25", MinDiscoveryVersion: Version}
			if !decode(w, r, &in) {
				return
			}
			if in.Name == "" {
				fail(w, 400, "invalid_release")
				return
			}
			res, e := a.DB.ExecContext(r.Context(), `INSERT INTO releases(server_id,name,status,minecraft_version,loader_constraint,java_constraint,min_client_version,created_at) VALUES(?,?,'draft',?,?,?,?,?)`, id, in.Name, in.Minecraft, in.FabricLoader, in.Java, in.MinDiscoveryVersion, Now())
			if e != nil {
				fail(w, 409, "operation_rejected")
				return
			}
			rid, _ := res.LastInsertId()
			a.Audit(r.Context(), "release.create", strconv.FormatInt(rid, 10))
			jsonResponse(w, 201, map[string]int64{"id": rid})
		} else {
			fail(w, 405, "method_not_allowed")
		}
	case "channels":
		if r.Method == "GET" && len(p) == 3 {
			a.list(w, r, "SELECT * FROM channels WHERE server_id=?", id)
			return
		}
		if len(p) != 4 || r.Method != "PUT" || !validID(p[3]) {
			fail(w, 400, "invalid_channel")
			return
		}
		var in struct {
			ReleaseID int64 `json:"releaseId"`
		}
		if !decode(w, r, &in) {
			return
		}
		var count int
		if e := a.DB.QueryRowContext(r.Context(), "SELECT count(*) FROM releases WHERE id=? AND server_id=? AND status='published'", in.ReleaseID, id).Scan(&count); e != nil || count != 1 {
			fail(w, 400, "invalid_release")
			return
		}
		a.mutate(w, r, `INSERT INTO channels VALUES(?,?,?,?) ON CONFLICT(server_id,name) DO UPDATE SET active_release_id=excluded.active_release_id`, "channel.change", id+"/"+p[3], id, p[3], in.ReleaseID, Now())
	case "clients":
		if r.Method != "GET" {
			fail(w, 405, "method_not_allowed")
			return
		}
		a.list(w, r, "SELECT id,server_id,name,created_at,last_seen_at,revoked_at FROM client_devices WHERE server_id=?", id)
	case "enrollment-codes":
		if r.Method == "GET" {
			a.list(w, r, "SELECT id,expires_at,max_uses,use_count,revoked_at FROM enrollment_codes WHERE server_id=?", id)
			return
		}
		if r.Method != "POST" {
			fail(w, 405, "method_not_allowed")
			return
		}
		var in struct {
			MaxUses int    `json:"maxUses"`
			TTL     string `json:"ttl"`
		}
		if !decode(w, r, &in) {
			return
		}
		if in.MaxUses < 1 {
			fail(w, 400, "invalid_max_uses")
			return
		}
		if in.TTL == "" {
			in.TTL = a.Config.Security.EnrollmentTokenTTL
		}
		d, e := time.ParseDuration(in.TTL)
		if e != nil || d <= 0 {
			fail(w, 400, "invalid_ttl")
			return
		}
		code, cid := auth.Token(), auth.Token()
		expiry := time.Now().Add(d).UTC().Format(time.RFC3339)
		_, e = a.DB.ExecContext(r.Context(), "INSERT INTO enrollment_codes(id,server_id,code_hash,expires_at,max_uses,created_at) VALUES(?,?,?,?,?,?)", cid, id, auth.Hash(code), expiry, in.MaxUses, Now())
		if e != nil {
			fail(w, 409, "operation_rejected")
			return
		}
		a.Audit(r.Context(), "enrollment.create", cid)
		jsonResponse(w, 201, map[string]any{"id": cid, "code": code, "expiresAt": expiry})
	default:
		fail(w, 404, "not_found")
	}
}
func (a *App) deleteServer(w http.ResponseWriter, r *http.Request, id string) {
	tx, e := a.DB.BeginTx(r.Context(), nil)
	if e != nil {
		fail(w, 500, "database_error")
		return
	}
	defer tx.Rollback()
	var published, children int
	if e = tx.QueryRow("SELECT count(*) FROM releases WHERE server_id=? AND status='published'", id).Scan(&published); e != nil {
		fail(w, 500, "database_error")
		return
	}
	if published > 0 {
		fail(w, 409, "cannot_delete_published_releases")
		return
	}
	if e = tx.QueryRow("SELECT (SELECT count(*) FROM releases WHERE server_id=?)+(SELECT count(*) FROM client_devices WHERE server_id=?)", id, id).Scan(&children); e != nil {
		fail(w, 500, "database_error")
		return
	}
	if children > 0 && r.URL.Query().Get("confirm") != "true" {
		fail(w, 409, "confirmation_required")
		return
	}
	for _, q := range []string{"DELETE FROM channels WHERE server_id=?", "DELETE FROM release_mods WHERE release_id IN(SELECT id FROM releases WHERE server_id=?)", "DELETE FROM releases WHERE server_id=?", "DELETE FROM client_devices WHERE server_id=?", "DELETE FROM enrollment_codes WHERE server_id=?", "DELETE FROM servers WHERE id=?"} {
		if _, e = tx.ExecContext(r.Context(), q, id); e != nil {
			fail(w, 409, "operation_rejected")
			return
		}
	}
	if e = tx.Commit(); e != nil {
		fail(w, 500, "database_error")
		return
	}
	a.Audit(r.Context(), "server.delete", id)
	jsonResponse(w, 200, map[string]bool{"ok": true})
}
func (a *App) artifacts(w http.ResponseWriter, r *http.Request, p []string) {
	if len(p) == 1 && r.Method == "GET" {
		a.list(w, r, "SELECT * FROM artifacts ORDER BY created_at DESC")
		return
	}
	if len(p) != 2 {
		fail(w, 404, "not_found")
		return
	}
	if p[1] == "upload" && r.Method == "POST" {
		r.Body = http.MaxBytesReader(w, r.Body, a.Config.Security.MaxUploadSize+(1<<20))
		mr, e := r.MultipartReader()
		if e != nil {
			fail(w, 400, "invalid_multipart")
			return
		}
		part, e := mr.NextPart()
		if e != nil {
			fail(w, 400, "missing_file")
			return
		}
		defer part.Close()
		hash, e := a.Artifacts.Import(r.Context(), part, part.FileName())
		if e != nil {
			fail(w, 400, "artifact_rejected")
			return
		}
		a.Audit(r.Context(), "artifact.upload", hash)
		jsonResponse(w, 201, map[string]string{"sha256": hash})
		return
	}
	if p[1] == "import" && r.Method == "POST" {
		var in struct {
			URL string `json:"url"`
		}
		if !decode(w, r, &in) {
			return
		}
		req, e := http.NewRequestWithContext(r.Context(), "GET", in.URL, nil)
		if e != nil {
			fail(w, 400, "invalid_url")
			return
		}
		cl := artifact.ImportClient(a.Config.Security.AllowPrivateImportURLs)
		defer cl.CloseIdleConnections()
		resp, e := cl.Do(req)
		if e != nil {
			fail(w, 400, "import_failed")
			return
		}
		defer resp.Body.Close()
		if resp.StatusCode != 200 {
			fail(w, 400, "import_failed")
			return
		}
		hash, e := a.Artifacts.Import(r.Context(), resp.Body, path.Base(req.URL.Path))
		if e != nil {
			fail(w, 400, "artifact_rejected")
			return
		}
		a.Audit(r.Context(), "artifact.import", hash)
		jsonResponse(w, 201, map[string]string{"sha256": hash})
		return
	}
	if !artifact.SHA.MatchString(p[1]) {
		fail(w, 400, "invalid_sha256")
		return
	}
	switch r.Method {
	case "GET":
		rows, e := a.rows(r.Context(), "SELECT * FROM artifacts WHERE sha256=?", p[1])
		if e != nil || len(rows) == 0 {
			fail(w, 404, "artifact_not_found")
			return
		}
		refs, e := a.rows(r.Context(), "SELECT r.id,r.name,r.server_id,r.status FROM releases r JOIN release_mods m ON m.release_id=r.id WHERE m.artifact_sha256=?", p[1])
		if e != nil {
			fail(w, 500, "database_error")
			return
		}
		jsonResponse(w, 200, map[string]any{"artifact": rows[0], "releases": refs})
	case "DELETE":
		var n int
		var key string
		if e := a.DB.QueryRowContext(r.Context(), "SELECT count(*) FROM release_mods WHERE artifact_sha256=?", p[1]).Scan(&n); e != nil {
			fail(w, 500, "database_error")
			return
		}
		if n > 0 {
			fail(w, 409, "artifact_referenced")
			return
		}
		if e := a.DB.QueryRowContext(r.Context(), "SELECT storage_key FROM artifacts WHERE sha256=?", p[1]).Scan(&key); e != nil {
			fail(w, 404, "artifact_not_found")
			return
		}
		if e := a.Store.Delete(r.Context(), key); e != nil {
			fail(w, 503, "storage_unavailable")
			return
		}
		a.mutate(w, r, "DELETE FROM artifacts WHERE sha256=?", "artifact.delete", p[1], p[1])
	default:
		fail(w, 405, "method_not_allowed")
	}
}

type releaseInput struct {
	Name                string `json:"name"`
	Minecraft           string `json:"minecraft"`
	FabricLoader        string `json:"fabricLoader"`
	Java                string `json:"java"`
	MinDiscoveryVersion string `json:"minDiscoveryVersion"`
}

func (a *App) releases(w http.ResponseWriter, r *http.Request, p []string) {
	if len(p) == 1 && r.Method == "GET" {
		a.list(w, r, "SELECT id,server_id,name,status FROM releases ORDER BY id DESC")
		return
	}
	if len(p) < 2 {
		fail(w, 404, "not_found")
		return
	}
	id, e := strconv.ParseInt(p[1], 10, 64)
	if e != nil {
		fail(w, 400, "invalid_release")
		return
	}
	if len(p) == 2 {
		if r.Method == "GET" {
			rows, e := a.rows(r.Context(), "SELECT * FROM releases WHERE id=?", id)
			if e != nil || len(rows) == 0 {
				fail(w, 404, "release_not_found")
				return
			}
			mods, e := a.rows(r.Context(), "SELECT * FROM release_mods WHERE release_id=? ORDER BY sort_order,mod_id", id)
			if e != nil {
				fail(w, 500, "database_error")
				return
			}
			jsonResponse(w, 200, map[string]any{"release": rows[0], "mods": mods})
			return
		}
		if r.Method == "PATCH" {
			var in releaseInput
			if !decode(w, r, &in) {
				return
			}
			if in.Name == "" || in.Minecraft == "" || in.FabricLoader == "" || in.Java == "" {
				fail(w, 400, "invalid_release")
				return
			}
			a.mutate(w, r, "UPDATE releases SET name=?,minecraft_version=?,loader_constraint=?,java_constraint=?,min_client_version=? WHERE id=? AND status='draft'", "release.update", p[1], in.Name, in.Minecraft, in.FabricLoader, in.Java, in.MinDiscoveryVersion, id)
			return
		}
	}
	if len(p) == 3 && p[2] == "validate" && r.Method == "GET" {
		a.releaseValidation(w, r, id)
		return
	}
	if len(p) == 3 && p[2] == "publish" && r.Method == "POST" {
		if e = a.publish(r, id); e != nil {
			jsonResponse(w, 409, map[string]string{"error": "publish_rejected", "message": e.Error()})
			return
		}
		a.Audit(r.Context(), "release.publish", p[1])
		jsonResponse(w, 200, map[string]bool{"ok": true})
		return
	}
	if len(p) >= 3 && p[2] == "mods" {
		if len(p) == 4 && r.Method == "DELETE" {
			a.mutate(w, r, "DELETE FROM release_mods WHERE release_id=? AND mod_id=?", "release.mod.delete", p[1], id, p[3])
			return
		}
		if len(p) == 3 && r.Method == "POST" {
			var in struct {
				ModID             string  `json:"modId"`
				Name              string  `json:"name"`
				Requirement       string  `json:"requirement"`
				Artifact          *string `json:"artifact"`
				VersionConstraint string  `json:"versionConstraint"`
				DefaultSelected   bool    `json:"defaultSelected"`
			}
			if !decode(w, r, &in) {
				return
			}
			if !artifact.ModID.MatchString(in.ModID) || in.ModID == "mod-list-discovery" || (in.Requirement != "required" && in.Requirement != "optional" && in.Requirement != "forbidden") || (in.Requirement == "forbidden") != (in.Artifact == nil) {
				fail(w, 400, "invalid_mod")
				return
			}
			if in.Artifact != nil && !artifact.SHA.MatchString(*in.Artifact) {
				fail(w, 400, "invalid_sha256")
				return
			}
			if in.VersionConstraint == "" {
				in.VersionConstraint = "*"
			}
			a.mutate(w, r, `INSERT INTO release_mods(release_id,mod_id,display_name,requirement,artifact_sha256,version_constraint,default_selected) VALUES(?,?,?,?,?,?,?) ON CONFLICT(release_id,mod_id) DO UPDATE SET display_name=excluded.display_name,requirement=excluded.requirement,artifact_sha256=excluded.artifact_sha256,version_constraint=excluded.version_constraint,default_selected=excluded.default_selected`, "release.mod.update", p[1], id, in.ModID, in.Name, in.Requirement, in.Artifact, in.VersionConstraint, in.DefaultSelected)
			return
		}
	}
	fail(w, 405, "method_not_allowed")
}
func (a *App) publish(r *http.Request, id int64) error {
	tx, e := a.DB.BeginTx(r.Context(), nil)
	if e != nil {
		return fmt.Errorf("database unavailable")
	}
	defer tx.Rollback()
	var server, name, status, mc, loader, java, min, display string
	e = tx.QueryRowContext(r.Context(), `SELECT r.server_id,r.name,r.status,r.minecraft_version,r.loader_constraint,r.java_constraint,r.min_client_version,s.display_name FROM releases r JOIN servers s ON s.id=r.server_id WHERE r.id=?`, id).Scan(&server, &name, &status, &mc, &loader, &java, &min, &display)
	if e != nil || status != "draft" {
		return fmt.Errorf("release must be draft")
	}
	if mc == "" || loader == "" || java == "" {
		return fmt.Errorf("runtime fields required")
	}
	rows, e := tx.QueryContext(r.Context(), `SELECT m.mod_id,m.display_name,m.requirement,m.artifact_sha256,m.version_constraint,m.default_selected,a.filename,a.size,a.storage_key,a.mod_id,a.mod_version,a.mod_environment,a.fabric_metadata FROM release_mods m LEFT JOIN artifacts a ON a.sha256=m.artifact_sha256 WHERE m.release_id=? ORDER BY m.sort_order,m.mod_id`, id)
	if e != nil {
		return fmt.Errorf("cannot read mods")
	}
	mods := []map[string]any{}
	artifacts := map[string]any{}
	metas := map[string]artifact.Metadata{}
	keys := []string{}
	for rows.Next() {
		var mid, mname, req, vc string
		var selected bool
		var hash, filename, key, aid, version, env, raw sql.NullString
		var size sql.NullInt64
		if e = rows.Scan(&mid, &mname, &req, &hash, &vc, &selected, &filename, &size, &key, &aid, &version, &env, &raw); e != nil {
			rows.Close()
			return fmt.Errorf("cannot read mod")
		}
		if mid == "mod-list-discovery" {
			rows.Close()
			return fmt.Errorf("self update forbidden")
		}
		m := map[string]any{"modId": mid, "name": mname, "requirement": req}
		if req == "forbidden" {
			m["versionConstraint"] = vc
		} else {
			if !hash.Valid || !artifact.SHA.MatchString(hash.String) || !aid.Valid || aid.String != mid || env.String == "server" {
				rows.Close()
				return fmt.Errorf("invalid artifact for %s", mid)
			}
			m["artifact"] = hash.String
			m["version"] = version.String
			if req == "optional" {
				m["defaultSelected"] = selected
			}
			keys = append(keys, key.String)
			artifacts[hash.String] = map[string]any{"filename": filename.String, "size": size.Int64, "download": map[string]string{"type": "api", "path": "/api/v1/servers/" + server + "/artifacts/" + hash.String + "/download"}}
			var meta artifact.Metadata
			if json.Unmarshal([]byte(raw.String), &meta) != nil {
				rows.Close()
				return fmt.Errorf("invalid metadata")
			}
			metas[mid] = meta
		}
		mods = append(mods, m)
	}
	e = rows.Err()
	rows.Close()
	if e != nil {
		return fmt.Errorf("cannot read mods")
	}
	if len(mods) > 500 || len(artifacts) > 500 {
		return fmt.Errorf("too many mods")
	}
	for _, key := range keys {
		ok, e := a.Store.Exists(r.Context(), key)
		if e != nil || !ok {
			return fmt.Errorf("artifact object missing")
		}
	}
	if _, e := validateDependencies(metas); e != nil {
		return e
	}
	published := Now()
	payload, e := json.Marshal(map[string]any{"schemaVersion": 1, "server": map[string]string{"id": server, "displayName": display}, "channel": "stable", "release": map[string]any{"id": id, "name": name, "publishedAt": published}, "runtime": map[string]string{"minecraft": mc, "fabricLoader": loader, "java": java, "minDiscoveryVersion": min}, "mods": mods, "artifacts": artifacts})
	if e != nil || len(payload) > 1400000 {
		return fmt.Errorf("manifest too large")
	}
	_, e = tx.ExecContext(r.Context(), "UPDATE releases SET status='published',published_at=?,payload=? WHERE id=? AND status='draft'", published, payload, id)
	if e != nil {
		return fmt.Errorf("publish failed")
	}
	return tx.Commit()
}
