package app

import (
	"encoding/json"
	"fmt"
	"github.com/Masterminds/semver/v3"
	"mod-list-discovery/server/internal/artifact"
	"sort"
)

// Fabric dependency values are a single AND predicate or an OR array of predicates.
// Non-semantic mod versions remain useful for exact equality predicates.
func dependencyMatches(raw json.RawMessage, version string) (bool, error) {
	var predicates []string
	var single string
	if e := json.Unmarshal(raw, &single); e == nil {
		predicates = []string{single}
	} else if e = json.Unmarshal(raw, &predicates); e != nil {
		return false, fmt.Errorf("invalid dependency predicate")
	}
	for _, predicate := range predicates {
		if predicate == "*" || predicate == version || predicate == "="+version {
			return true, nil
		}
		v, e := semver.NewVersion(version)
		if e != nil {
			continue
		}
		c, e := semver.NewConstraint(predicate)
		if e != nil {
			return false, fmt.Errorf("invalid dependency predicate")
		}
		if c.Check(v) {
			return true, nil
		}
	}
	return false, nil
}
func validateDependencies(metas map[string]artifact.Metadata) ([]string, error) {
	available := map[string]string{}
	for id, m := range metas {
		available[id] = m.Version
		for _, alias := range m.Provides {
			available[alias] = m.Version
		}
	}
	warnings := []string{}
	for id, m := range metas {
		for dep, predicate := range m.Depends {
			if dep == "minecraft" || dep == "java" || dep == "fabricloader" {
				continue
			}
			version, ok := available[dep]
			if !ok {
				warnings = append(warnings, fmt.Sprintf("%s depends on %s; verify bundled or user-installed dependencies", id, dep))
				continue
			}
			matches, e := dependencyMatches(predicate, version)
			if e != nil {
				return nil, fmt.Errorf("%s: %w", id, e)
			}
			if !matches {
				return nil, fmt.Errorf("%s dependency %s version mismatch", id, dep)
			}
		}
		for _, conflicts := range []map[string]json.RawMessage{m.Conflicts, m.Breaks} {
			for dep, predicate := range conflicts {
				version, ok := available[dep]
				if !ok {
					continue
				}
				matches, e := dependencyMatches(predicate, version)
				if e != nil {
					return nil, e
				}
				if matches {
					return nil, fmt.Errorf("%s conflicts with %s", id, dep)
				}
			}
		}
	}
	// Keep diagnostics deterministic for clients and audit consumers.
	sort.Strings(warnings)
	return warnings, nil
}
