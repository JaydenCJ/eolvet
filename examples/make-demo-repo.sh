#!/usr/bin/env bash
# Fabricates a small polyglot repository with a mix of end-of-life,
# soon-to-expire, supported, and unresolvable version declarations —
# handy for trying every eolvet verdict in one scan.
#
# Usage: bash examples/make-demo-repo.sh [target-dir]
set -euo pipefail

TARGET="${1:-/tmp/eolvet-demo}"
mkdir -p "$TARGET/web" "$TARGET/legacy"

cat > "$TARGET/Dockerfile" <<'EOF'
# main service image
ARG PY=3.8
FROM python:${PY}-slim-bullseye AS build
RUN pip install --no-cache-dir -r requirements.txt
FROM build
CMD ["python", "-m", "app"]
EOF

cat > "$TARGET/docker-compose.yml" <<'EOF'
services:
  db:
    image: postgres:12
  cache:
    image: redis:latest
EOF

printf 'module demo\n\ngo 1.26\n' > "$TARGET/go.mod"
printf '18.16.0\n' > "$TARGET/web/.nvmrc"

cat > "$TARGET/web/package.json" <<'EOF'
{
  "name": "web",
  "engines": { "node": ">=18.17 <21" }
}
EOF

printf 'FROM python:2.7\n' > "$TARGET/legacy/Dockerfile"

echo "demo repo written to $TARGET"
echo "try:  eolvet scan --as-of 2026-07-13 $TARGET"
