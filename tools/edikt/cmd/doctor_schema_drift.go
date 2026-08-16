package cmd

// doctor_schema_drift.go — drift detector for PRD and SPEC sidecars.
//
// The governance sidecar (gov-sidecar) is already validated by sidecar.Load()
// via KnownFields(true), so the existing doctor_sidecar.go's SCHEMA INVALID
// check catches both extra fields AND missing required fields for that
// schema. PRD and SPEC sidecars have no Go-side parser today, so a sidecar
// written by an older edikt version that predates a newer required field
// silently passes every existing doctor check.
//
// This check loads each prd-sidecar.v1.schema.json and spec-sidecar.v1.schema.json
// straight from disk, reads its `required` array, and walks the project's
// PRD/SPEC sidecars verifying every required key is present. Missing fields
// are reported as warnings (drift signal — sidecar predates schema; user
// should regenerate). The check is schema-driven, so it stays in sync
// automatically as the schema gains required fields.

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"
)

// runSchemaDriftCheck inspects PRD and SPEC sidecars for required-field
// drift against their schemas. Returns (warnings, ran). ran == false when
// neither PRD nor SPEC directories exist (non-edikt project or fresh init).
// Never returns errors — drift is advisory, not fatal.
func runSchemaDriftCheck(projectRoot string, w io.Writer) (warns int, ran bool) {
	prdsDir, specsDir := resolveProductDirs(projectRoot)
	schemaDir := resolveSchemaDir(projectRoot)

	prdSidecars := listPRDSidecars(prdsDir)
	specSidecars := listSPECSidecars(specsDir)

	if len(prdSidecars) == 0 && len(specSidecars) == 0 {
		return 0, false
	}

	fmt.Fprintln(w, "  ── Sidecar Schema Drift ───────────────────────")

	prdReq, prdErr := loadRequiredFields(filepath.Join(schemaDir, "prd-sidecar.v1.schema.json"))
	specReq, specErr := loadRequiredFields(filepath.Join(schemaDir, "spec-sidecar.v1.schema.json"))

	if prdErr != nil && len(prdSidecars) > 0 {
		fmt.Fprintf(w, "  WARN: cannot read PRD schema (%v) — skipping PRD drift check\n", prdErr)
		warns++
	}
	if specErr != nil && len(specSidecars) > 0 {
		fmt.Fprintf(w, "  WARN: cannot read SPEC schema (%v) — skipping SPEC drift check\n", specErr)
		warns++
	}

	if prdErr == nil {
		for _, p := range prdSidecars {
			missing := missingRequiredFields(p, prdReq)
			if len(missing) > 0 {
				fmt.Fprintf(w,
					"  WARN: %s is missing schema-required fields: %s — regenerate via /edikt:sdlc:prd:resync\n",
					rel(projectRoot, p), strings.Join(missing, ", "))
				warns++
			}
		}
	}
	if specErr == nil {
		for _, p := range specSidecars {
			missing := missingRequiredFields(p, specReq)
			if len(missing) > 0 {
				fmt.Fprintf(w,
					"  WARN: %s is missing schema-required fields: %s — regenerate via /edikt:sdlc:spec:resync\n",
					rel(projectRoot, p), strings.Join(missing, ", "))
				warns++
			}
		}
	}

	if warns == 0 {
		fmt.Fprintln(w, "  PRD/SPEC sidecars match schema-required fields.")
	}
	return warns, true
}

// resolveProductDirs reads paths.prds / paths.specs from .edikt/config.yaml,
// falling back to docs/product/prds and docs/product/specs.
func resolveProductDirs(projectRoot string) (prdsDir, specsDir string) {
	prdsDir = filepath.Join(projectRoot, "docs/product/prds")
	specsDir = filepath.Join(projectRoot, "docs/product/specs")
	cfg := filepath.Join(projectRoot, ".edikt", "config.yaml")
	data, err := os.ReadFile(cfg)
	if err != nil {
		return prdsDir, specsDir
	}
	scanner := bufio.NewScanner(strings.NewReader(string(data)))
	inPaths := false
	for scanner.Scan() {
		line := scanner.Text()
		if strings.HasPrefix(line, "paths:") {
			inPaths = true
			continue
		}
		if !inPaths {
			continue
		}
		if !(strings.HasPrefix(line, " ") || strings.HasPrefix(line, "\t")) {
			if strings.TrimSpace(line) != "" && !strings.HasPrefix(line, "#") {
				break
			}
			continue
		}
		ts := strings.TrimSpace(line)
		if rest, ok := strings.CutPrefix(ts, "prds:"); ok {
			prdsDir = filepath.Join(projectRoot, strings.TrimSpace(rest))
		} else if rest, ok := strings.CutPrefix(ts, "specs:"); ok {
			specsDir = filepath.Join(projectRoot, strings.TrimSpace(rest))
		}
	}
	return prdsDir, specsDir
}

// resolveSchemaDir returns the directory holding the published JSON schemas.
// Project-local .edikt/schemas/ takes precedence (per the auto-install
// behavior of /edikt:sdlc:prd and /edikt:sdlc:spec). Falls back to the
// edikt payload's templates/schemas/.
func resolveSchemaDir(projectRoot string) string {
	local := filepath.Join(projectRoot, ".edikt", "schemas")
	if _, err := os.Stat(local); err == nil {
		return local
	}
	// Payload fallback. Try EDIKT_HOME or default ~/.edikt.
	root := os.Getenv("EDIKT_HOME")
	if root == "" {
		if home, err := os.UserHomeDir(); err == nil {
			root = filepath.Join(home, ".edikt")
		}
	}
	if root != "" {
		payload := filepath.Join(root, "current", "templates", "schemas")
		if _, err := os.Stat(payload); err == nil {
			return payload
		}
	}
	// Last resort: the repo's templates/schemas (dev mode).
	return filepath.Join(projectRoot, "templates", "schemas")
}

// listPRDSidecars returns all *.yaml files directly under prdsDir.
func listPRDSidecars(prdsDir string) []string {
	if prdsDir == "" {
		return nil
	}
	entries, err := os.ReadDir(prdsDir)
	if err != nil {
		return nil
	}
	var out []string
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		if strings.HasSuffix(e.Name(), ".yaml") {
			out = append(out, filepath.Join(prdsDir, e.Name()))
		}
	}
	sort.Strings(out)
	return out
}

// listSPECSidecars returns spec.yaml files in immediate subdirectories of
// specsDir — matches the SPEC-NNN-slug/spec.yaml layout.
func listSPECSidecars(specsDir string) []string {
	if specsDir == "" {
		return nil
	}
	entries, err := os.ReadDir(specsDir)
	if err != nil {
		return nil
	}
	var out []string
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		p := filepath.Join(specsDir, e.Name(), "spec.yaml")
		if _, err := os.Stat(p); err == nil {
			out = append(out, p)
		}
	}
	sort.Strings(out)
	return out
}

// loadRequiredFields reads a JSON Schema file and returns its `required`
// array at the root object. Optional `allOf` entries that carry their own
// `required` arrays are folded in too (covers oneOf-wrapped requirements).
func loadRequiredFields(schemaPath string) ([]string, error) {
	data, err := os.ReadFile(schemaPath)
	if err != nil {
		return nil, err
	}
	var raw map[string]any
	if err := json.Unmarshal(data, &raw); err != nil {
		return nil, fmt.Errorf("parse %s: %w", filepath.Base(schemaPath), err)
	}
	required := map[string]bool{}
	if r, ok := raw["required"].([]any); ok {
		for _, v := range r {
			if s, ok := v.(string); ok {
				required[s] = true
			}
		}
	}
	if all, ok := raw["allOf"].([]any); ok {
		for _, entry := range all {
			if m, ok := entry.(map[string]any); ok {
				if r, ok := m["required"].([]any); ok {
					for _, v := range r {
						if s, ok := v.(string); ok {
							required[s] = true
						}
					}
				}
			}
		}
	}
	out := make([]string, 0, len(required))
	for k := range required {
		out = append(out, k)
	}
	sort.Strings(out)
	return out, nil
}

// missingRequiredFields returns the names of required schema keys that are
// not present at the YAML's top level. A nil/null value counts as present
// (the schema validates type elsewhere; missing means the key is absent).
func missingRequiredFields(sidecarPath string, required []string) []string {
	data, err := os.ReadFile(sidecarPath)
	if err != nil {
		// Surface this through the caller; treat as "all missing" so the
		// user sees something rather than a silent pass.
		return required
	}
	var doc map[string]any
	// SPEC-009 Plan A AC-1.2: loads PRD/SPEC sidecar into generic map for schema-drift required-field check.  // edikt-guard:allow
	// Not *.edikt.yaml. KnownFields off intentional — generic map[string]any has no struct to enforce against.
	if err := yaml.Unmarshal(data, &doc); err != nil {
		return []string{"(parse error: " + err.Error() + ")"}
	}
	var missing []string
	for _, key := range required {
		if _, ok := doc[key]; !ok {
			missing = append(missing, key)
		}
	}
	return missing
}
