// JSON renderer: a stable, versioned envelope for machines.
package report

import (
	"encoding/json"
)

// jsonEnvelope wraps a Report with tool identification and an output
// schema version, so downstream consumers can pin what they parse.
// (This schema version is independent of the snapshot's own.)
type jsonEnvelope struct {
	Tool          string `json:"tool"`
	SchemaVersion int    `json:"schema_version"`
	*Report
}

// RenderJSON renders the report as indented, key-stable JSON.
func RenderJSON(r *Report) (string, error) {
	if r.Findings == nil {
		r.Findings = []Finding{} // "findings": [] beats "findings": null
	}
	out, err := json.MarshalIndent(jsonEnvelope{Tool: "eolvet", SchemaVersion: 1, Report: r}, "", "  ")
	if err != nil {
		return "", err
	}
	return string(out) + "\n", nil
}
