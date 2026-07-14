# Contributing to eolvet

Issues, discussions and pull requests are all welcome.

## Getting started

You need Go ≥1.22; nothing else — the tool, its tests, and its data are
fully offline.

```bash
git clone https://github.com/JaydenCJ/eolvet && cd eolvet
go build ./...
go test ./...
bash scripts/smoke.sh
```

`scripts/smoke.sh` builds the binary, fabricates a polyglot repository in
a temp dir, and asserts on real CLI output across every subcommand and
exit code; it must finish by printing `SMOKE OK`.

## Before you open a pull request

1. `gofmt -l .` reports nothing (formatting is enforced).
2. `go vet ./...` passes with no findings.
3. `go test ./...` passes (90 deterministic tests, no network).
4. `bash scripts/smoke.sh` prints `SMOKE OK`.
5. Add tests for behavior changes; keep logic in pure, unit-testable
   modules (detectors never touch the filesystem — only `scan.Walk` does).

## Updating the EOL snapshot

The snapshot is data, not code: edit
`internal/eoldata/snapshot.json`, bump its `snapshot_date`, and cite the
vendor lifecycle page for every changed date in the PR description. The
loader validates chronology, uniqueness, and codename integrity at load
time, and `internal/eoldata` tests enforce the same invariants in CI-less
local runs. Never fabricate a date — a wrong EOL verdict is worse than an
`unknown`.

## Ground rules

- Keep dependencies at zero — eolvet is standard library only, and its
  offline guarantee is the core feature. No network calls, ever; no
  telemetry.
- Judgments must be reproducible: the same tree, snapshot, and `--as-of`
  date must produce byte-identical reports, including all orderings.
- Detectors report only what a file actually declares; when a version
  cannot be resolved offline, emit an explained `unknown`, never a guess.
- Code comments and doc comments are written in English.

## Reporting bugs

Include the output of `eolvet version` (it names the snapshot date), the
full command you ran, the report output, and — for misdetections — the
exact declaration line from the scanned file (e.g. the `FROM` line or the
`engines` block), since that is precisely what the detector sees.

## Security

Please do not open public issues for security problems; use GitHub's
private vulnerability reporting on this repository instead.
