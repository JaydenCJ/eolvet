# eolvet examples

Both scripts are self-contained and offline.

- **`make-demo-repo.sh [dir]`** — fabricates a small polyglot repository
  (Dockerfiles, compose file, `go.mod`, `.nvmrc`, `package.json`) whose
  declarations exercise every verdict: `EOL`, `EOL-SOON`, `OK`, and
  `UNKNOWN`. Scan it with a pinned date for reproducible output:

  ```bash
  bash examples/make-demo-repo.sh /tmp/eolvet-demo
  eolvet scan --as-of 2026-07-13 /tmp/eolvet-demo
  ```

- **`ci-gate.sh [dir]`** — the pattern for CI: write the JSON report as a
  dated audit artifact with `--fail-on none`, then gate the build with the
  default `--fail-on eol` policy and a 60-day warn window. Exit code 1
  breaks the pipeline; the archived report says exactly why, against which
  snapshot, as of which date.

For a stricter compliance stance, add `--strict` (unknowns fail too) or
`--fail-on eol-soon` (act before the deadline, not after) — and pin your
organization's own lifecycle policy with `--data policy.json` (format in
[docs/snapshot-format.md](../docs/snapshot-format.md)).
