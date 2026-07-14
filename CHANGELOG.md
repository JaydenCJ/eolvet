# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [0.1.0] - 2026-07-13

### Added

- Bundled, versioned EOL data snapshot (dated 2026-06-15) embedded into the
  binary: 21 products, 113 release cycles across runtimes (Python, Node.js,
  Go, Java, Ruby, PHP, .NET), distros (Ubuntu, Debian, Alpine, CentOS,
  Rocky, Alma, Amazon Linux), and servers (PostgreSQL, MySQL, MariaDB,
  MongoDB, Redis, nginx, HAProxy), with strict load-time validation and
  Debian/Ubuntu codename tables.
- Dockerfile detector: multi-stage-aware FROM parsing with ARG default
  substitution (`${VER}`, `${VER:-3.9}`, `$VER`), line continuations,
  `--platform` flags, registry/namespace normalization, and tag
  decomposition — `python:3.8-slim-bullseye` reports both the Python
  runtime and the Debian base.
- Detectors for docker-compose `image:` lines (including `${VAR:-default}`),
  version-pin files (`.python-version`, `.nvmrc`, `.node-version`,
  `.ruby-version`, `.go-version`, `.java-version`), asdf/mise
  `.tool-versions`, Heroku `runtime.txt`, `go.mod` (toolchain-aware),
  `package.json` engines, `pyproject.toml` (PEP 621 + Poetry), `Gemfile`,
  and `composer.json`.
- Constraint-floor engine: `>=`, `>`, `^`, `~`, `~>`, wildcards, comma/space
  conjunctions and `||` alternatives resolve to the oldest allowed version;
  unbounded constraints surface as explained unknowns, never guesses.
- `scan` subcommand with text, JSON (`schema_version: 1`), and Markdown
  output, `--as-of` reproducible date, `--warn-within` window, `--exclude`
  globs with `**`, and a `--fail-on eol|eol-soon|none` + `--strict` policy
  gate with stable exit codes (0/1/2/3).
- `check` (one-off product/version lookup) and `products` (snapshot
  inventory) subcommands, plus `--data` to substitute an organization's own
  snapshot file.
- Runnable examples (`examples/make-demo-repo.sh`, `examples/ci-gate.sh`)
  and a snapshot format reference (`docs/snapshot-format.md`).
- 90 deterministic offline tests (unit + in-process CLI integration over
  fabricated repositories) and `scripts/smoke.sh`.

[0.1.0]: https://github.com/JaydenCJ/eolvet/releases/tag/v0.1.0
