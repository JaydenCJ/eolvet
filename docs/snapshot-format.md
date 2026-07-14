# Snapshot format

The EOL table eolvet consults is a single JSON document. The bundled copy
lives at `internal/eoldata/snapshot.json` and is embedded into the binary
at build time; organizations can substitute their own with `--data
policy.json` on any subcommand — the same schema, the same strict
validation.

## Document shape

```json
{
  "schema_version": 1,
  "snapshot_date": "2026-06-15",
  "source": "curated from vendor lifecycle and release announcements",
  "products": {
    "python": {
      "label": "Python",
      "cycles": [
        {"cycle": "3.8", "release": "2019-10-14", "eol": "2024-10-07"}
      ]
    },
    "debian": {
      "label": "Debian",
      "codenames": {"bullseye": "11", "bookworm": "12"},
      "cycles": [
        {"cycle": "11", "release": "2021-08-14", "eol": "2026-08-31"}
      ]
    }
  }
}
```

## Fields

| Key | Required | Meaning |
|---|---|---|
| `schema_version` | yes | Must be `1`. Anything else is rejected. |
| `snapshot_date` | yes | `YYYY-MM-DD` — the date the table was curated. Printed in every report so an audit answer is always dated. |
| `source` | no | Free-text provenance, shown by `products --format json`. |
| `products.<key>` | yes (≥1) | The key is what detectors emit (`python`, `debian`, …). |
| `label` | yes | Human-readable name used in reports. |
| `codenames` | no | Lowercase codename → cycle name. Every value must name a declared cycle. |
| `cycles[].cycle` | yes | Numeric release-line name (`3.8`, `18`, `22.04`). Must be unique per product. |
| `cycles[].release` | yes | `YYYY-MM-DD` first-release date. List cycles oldest-first. |
| `cycles[].eol` | yes | `YYYY-MM-DD` end-of-life date. eolvet treats the cycle as EOL **on** this date. |

## Validation

`--data` files go through the same load-time checks as the bundled table,
and a bad document is a hard error (exit 3) — never a silent
misclassification:

- `schema_version` must be exactly 1; `snapshot_date` must parse;
- every product needs a `label` and at least one cycle;
- cycle names must be numeric and unique within their product;
- every `release`/`eol` must parse as `YYYY-MM-DD`;
- every codename must point at a declared cycle.

## Matching semantics

A concrete version resolves to the cycle whose numeric components are an
exact prefix of the version's — `3.8.10` → `3.8`, `18.16.0` → `18`,
`10.11.4` → `10.11` (the longest match wins). Components compare
numerically, so `3.10.2` never lands on `3.1`. A version with fewer
components than any cycle (bare `3` against `3.8`/`3.9`) is ambiguous and
reports as `unknown` rather than guessed.
