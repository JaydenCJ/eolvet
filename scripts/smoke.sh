#!/usr/bin/env bash
# End-to-end smoke test for eolvet: builds the binary, fabricates a
# polyglot repository with EOL, soon-to-expire, supported, and unknown
# declarations, and asserts on the real CLI output. No network,
# idempotent, finishes in seconds.
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
WORKDIR="$(mktemp -d)"
trap 'rm -rf "$WORKDIR"' EXIT

fail() {
  echo "SMOKE FAIL: $*" >&2
  exit 1
}

BIN="$WORKDIR/eolvet"
REPO="$WORKDIR/repo"
ASOF="2026-07-13"   # pinned date: verdicts below are deterministic

echo "1. build"
(cd "$ROOT" && go build -o "$BIN" ./cmd/eolvet) || fail "go build failed"

echo "2. version matches manifest and names the snapshot"
"$BIN" version | grep -qx "eolvet 0.1.0 (snapshot 2026-06-15)" || fail "version mismatch"

echo "3. fabricate a polyglot repository"
mkdir -p "$REPO/web" "$REPO/legacy"
cat > "$REPO/Dockerfile" <<'EOF'
ARG PY=3.8
FROM python:${PY}-slim-bullseye AS build
FROM build
EOF
printf 'module demo\n\ngo 1.26\n' > "$REPO/go.mod"
printf '18.16.0\n' > "$REPO/web/.nvmrc"
printf 'services:\n  cache:\n    image: redis:latest\n' > "$REPO/docker-compose.yml"
printf 'FROM python:2.7\n' > "$REPO/legacy/Dockerfile"

echo "4. text report finds every declaration with the right verdicts"
set +e
OUT="$("$BIN" scan --as-of "$ASOF" "$REPO")"
CODE=$?
set -e
[ "$CODE" -eq 1 ] || fail "scan with EOL findings should exit 1, got $CODE"
echo "$OUT" | grep -q "snapshot 2026-06-15, as of 2026-07-13"    || fail "header missing"
echo "$OUT" | grep -Eq "^EOL +Python +3.8 +2024-10-07 +-644d"    || fail "EOL python 3.8 missing"
echo "$OUT" | grep -Eq "^EOL +Python +2.7"                       || fail "EOL python 2.7 missing"
echo "$OUT" | grep -Eq "^EOL-SOON +Debian +11 +2026-08-31 +\+49d" || fail "eol-soon debian 11 missing"
echo "$OUT" | grep -Eq "^OK +Go +1.26"                           || fail "supported go 1.26 missing"
echo "$OUT" | grep -Eq "^UNKNOWN +Redis"                         || fail "unknown redis:latest missing"
echo "$OUT" | grep -q "6 declarations: 3 eol, 1 eol-soon, 1 supported, 1 unknown" \
  || fail "summary line wrong"

echo "5. JSON report is machine-readable and correct"
JSON="$("$BIN" scan --format json --as-of "$ASOF" "$REPO" || true)"
echo "$JSON" | grep -q '"tool": "eolvet"'        || fail "json envelope missing"
echo "$JSON" | grep -q '"schema_version": 1'     || fail "json schema version missing"
echo "$JSON" | grep -q '"eol": 3'                || fail "json eol count wrong"
echo "$JSON" | grep -q '"days_left": -644'       || fail "json day math wrong"

echo "6. markdown report renders a table"
MD="$("$BIN" scan --format markdown --as-of "$ASOF" "$REPO" || true)"
echo "$MD" | grep -q '| \*\*EOL\*\* | Python | 3.8 |' || fail "markdown row missing"

echo "7. excludes and policy flags change the verdict"
set +e
"$BIN" scan --as-of "$ASOF" --exclude 'legacy/**' --exclude Dockerfile \
  --exclude web --fail-on eol-soon "$REPO" >/dev/null
[ $? -eq 0 ] || fail "excluding all EOL paths should exit 0"
"$BIN" scan --as-of "$ASOF" --fail-on none "$REPO" >/dev/null
[ $? -eq 0 ] || fail "--fail-on none should exit 0"
"$BIN" scan --as-of "$ASOF" --exclude 'legacy/**' --exclude Dockerfile \
  --exclude web --strict "$REPO" >/dev/null
[ $? -eq 1 ] || fail "--strict should trip on the unknown redis:latest"
set -e

echo "8. check answers one-off lookups with exit codes"
CHK="$("$BIN" check python 3.8 --as-of "$ASOF" || true)"
echo "$CHK" | grep -q "EOL since 2024-10-07, 644 days ago" || fail "check eol output wrong"
set +e
"$BIN" check python 3.8 --as-of "$ASOF" >/dev/null; [ $? -eq 1 ] || fail "check eol should exit 1"
"$BIN" check go 1.26.1 --as-of "$ASOF" >/dev/null;  [ $? -eq 0 ] || fail "check ok should exit 0"
set -e
"$BIN" check debian bullseye --as-of "$ASOF" | grep -q "EOL SOON on 2026-08-31" \
  || fail "check eol-soon output wrong"

echo "9. a custom --data snapshot overrides the bundled table"
cat > "$WORKDIR/policy.json" <<'EOF'
{
  "schema_version": 1, "snapshot_date": "2026-07-01", "source": "org policy",
  "products": {
    "go": {"label": "Go", "cycles": [
      {"cycle": "1.26", "release": "2026-02-10", "eol": "2026-07-01"}
    ]}
  }
}
EOF
POLICY_OUT="$("$BIN" check go 1.26 --as-of "$ASOF" --data "$WORKDIR/policy.json" || true)"
echo "$POLICY_OUT" | grep -q "EOL since 2026-07-01" || fail "--data snapshot not honored"

echo "10. products lists the snapshot; usage errors exit 2"
"$BIN" products | grep -q "21 products, 113 cycles" || fail "products summary wrong"
set +e
"$BIN" scan --format yaml "$REPO" >/dev/null 2>&1
[ $? -eq 2 ] || fail "bad --format should exit 2"
"$BIN" check nosuchproduct 1.0 >/dev/null 2>&1
[ $? -eq 2 ] || fail "unknown product should exit 2"
set -e

echo "SMOKE OK"
