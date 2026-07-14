#!/usr/bin/env bash
# A CI-shaped policy gate: fail the build when the repository ships
# anything end-of-life, warn (but pass) on findings expiring within 60
# days, and archive the JSON report as a dated audit artifact.
#
# Usage: bash examples/ci-gate.sh [repo-dir]
set -euo pipefail

REPO="${1:-.}"
REPORT="eolvet-report.json"

# 1. Archive the machine-readable report regardless of the verdict.
eolvet scan --format json --fail-on none "$REPO" > "$REPORT"
echo "report written to $REPORT (snapshot + as-of dates are inside)"

# 2. Gate: end-of-life findings break the build; the 60-day warn window
#    keeps the summary honest without failing early.
if ! eolvet scan --warn-within 60 "$REPO"; then
  echo "eolvet gate: end-of-life versions found — see the table above" >&2
  exit 1
fi

echo "eolvet gate: clean"
