# AGENTS.md — machine-consumer contract

This file is the contract for AI agents (and any program) that drive the
shoplazza CLI and parse its output. Humans want prose; agents want a stable,
parseable shape. This document pins that shape.

## Output channels

- **stdout = data.** On success, the answer only. Never progress, warnings, or
  hints. Safe to pipe.
- **stderr = everything else.** Progress bars, warnings, hints, and the error
  envelope. Never mix into stdout.

Keeping them separate is what lets `... | jq` and NDJSON streaming work.

## Success envelope

`--format json` (the default) wraps every API success as:

```json
{ "ok": true, "data": <payload> }
```

Branch on `.ok`. Read the payload from `.data`. `pretty` / `table` / `ndjson`
render the raw payload **without** the envelope — use `json` when you need to
branch programmatically.

## Error envelope

Errors are written to **stderr** as:

```json
{
  "ok": false,
  "error": {
    "type": "validation",
    "subtype": "unknown_flag",
    "param": "--bogus",
    "code": "",
    "message": "unknown flag: --bogus",
    "hint": "run the command with --help to list valid flags",
    "detail": { "status_code": 0, "request_id": "", "method": "", "path": "" }
  }
}
```

- **`type`** — coarse class. Pairs 1:1 with the exit code (see below). Branch on
  this first.
- **`subtype`** — stable, machine-branchable id within a `type` (e.g.
  `unknown_command`, `unknown_flag`, `missing_required_flag`,
  `invalid_flag_value`, `invalid_argument`). Recover on this, not on `message`.
- **`param`** — the offending flag/parameter name, when known.
- **`message` / `hint`** — human text and a next step. Never parse `message`.
- **`code` / `detail`** — server business code and HTTP context, for api/auth
  errors.

Even cobra usage errors (unknown command / flag, missing required flag, bad
argument) come through this envelope — stderr is always parseable JSON.

## Exit codes

| code | type         | meaning                          |
|------|--------------|----------------------------------|
| 0    | —            | success                          |
| 1    | `api`        | API / generic error              |
| 2    | `validation` | invalid flag or argument         |
| 3    | `auth`       | unauthenticated / token expired  |
| 4    | `network`    | network unreachable / timeout    |
| 5    | `internal`   | unexpected internal error        |

## `_notice` (staleness)

json success envelopes may carry a top-level `_notice` when the CLI or its
Agent Skills are stale:

```json
{ "ok": true, "data": {...},
  "_notice": {
    "update": { "current": "2.0.9", "latest": "2.1.0", "message": "..." },
    "skills": { "installed": false, "message": "..." }
  } }
```

It never appears on stdout data, and never in `pretty`/`table`/`ndjson`.

## Streaming large lists

Use `--format ndjson` for large list reads: one compact JSON record per line,
so the result doesn't inflate into one giant array that overflows a tool-result
limit. The `{ok,data}` envelope is dropped in this mode — you get the records
directly.

## Environment opt-outs

| var                        | effect                                  |
|----------------------------|-----------------------------------------|
| `SHOPLAZZA_CLI_NO_NOTICE=1`| suppress the `_notice` envelope block    |
| `SHOPLAZZA_CLI_SKIP_SKILLS=1`| skip the post-update skills refresh    |
| `NO_COLOR`                 | disable ANSI color (also off when non-TTY)|

## Safety

Before any write, run with `--dry-run` to print the request without sending it.
See the `shoplazza-common` skill for the full safety protocol and the three
command tiers (shortcuts → spec leaves → `api rest`).
