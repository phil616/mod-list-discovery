package app

import (
	"encoding/json"
	"mod-list-discovery/server/internal/artifact"
	"net/http"
)

func (a *App) releaseValidation(w http.ResponseWriter, r *http.Request, id int64) {
	rows, e := a.rows(r.Context(), `SELECT a.fabric_metadata FROM artifacts a JOIN release_mods m ON m.artifact_sha256=a.sha256 WHERE m.release_id=?`, id)
	if e != nil {
		fail(w, 500, "database_error")
		return
	}
	metas := map[string]artifact.Metadata{}
	for _, row := range rows {
		var meta artifact.Metadata
		if e = json.Unmarshal([]byte(row["fabric_metadata"].(string)), &meta); e != nil {
			fail(w, 400, "invalid_metadata")
			return
		}
		metas[meta.ID] = meta
	}
	warnings, e := validateDependencies(metas)
	if e != nil {
		jsonResponse(w, 200, map[string]any{"valid": false, "error": e.Error(), "warnings": warnings})
		return
	}
	jsonResponse(w, 200, map[string]any{"valid": true, "warnings": warnings})
}
