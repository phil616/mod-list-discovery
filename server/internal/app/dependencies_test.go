package app

import (
	"encoding/json"
	"mod-list-discovery/server/internal/artifact"
	"testing"
)

func TestDependencyValidation(t *testing.T) {
	metas := map[string]artifact.Metadata{"foo": {ID: "foo", Version: "1.0.0", Depends: map[string]json.RawMessage{"bar": json.RawMessage(`">=2.0.0"`)}}}
	warnings, e := validateDependencies(metas)
	if e != nil || len(warnings) != 1 {
		t.Fatal(warnings, e)
	}
	metas["bar"] = artifact.Metadata{ID: "bar", Version: "2.1.0"}
	if _, e = validateDependencies(metas); e != nil {
		t.Fatal(e)
	}
	foo := metas["foo"]
	foo.Breaks = map[string]json.RawMessage{"bar": json.RawMessage(`[">=2.0.0 <3.0.0","4.x"]`)}
	metas["foo"] = foo
	if _, e = validateDependencies(metas); e == nil {
		t.Fatal("conflicting release accepted")
	}
}
