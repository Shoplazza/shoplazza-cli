# Experiences — failure-mode index

Distilled failure notes from eval runs (spec §5 Step 7). One file per distinct
failure mode: `<YYYYMMDD>-<failure_tag>-<title>.md`. Match new failures against this
index by `failure_tag` before writing a new file — extend an existing note when the
mode is the same.

## `failure_tag` enum (spec §10)

| Tag | Layer | Meaning |
|---|---|---|
| `routing_keyword_missing` | L1 routing | Utterance should route to the skill but its `description` lacks the keyword |
| `routing_keyword_collision` | L1 routing | Utterance routed to the WRONG skill (cross-domain lookalike won) |
| `wrong_access_tier` | L1/L2 R-2 | Dropped to leaf/`api rest` when a `+shortcut` fits (or invented a shortcut) |
| `param_missing` | L1/L2 R-3 | Required no-default flag absent or misnamed |
| `param_wrong_value` | L1/L2 R-3 | Flag present but value contradicts the utterance |
| `fabricated_value` | L1/L2 R-4 | Money/limit value invented (not traceable to utterance or default) |
| `asked_defaulted_flag` | L2 R-5 | Asked the user about a flag that has a CLI default / omit-to-disable |
| `missing_required_ask` | L2 R-5 | Guessed instead of asking for a required no-default field |
| `unsafe_no_dryrun` | L2 R-6 | Destructive/batch/money op without `--dry-run` first |
| `jq_envelope_error` | L2 R-7 | jq path not rooted at `.data`, `-r` passed, `{ok,error}` mishandled |
| `tool_inefficiency` | L2 R-8 | Needless queries, retry drift, longer-than-needed path |
| `drift_broken_command` | L0 | Skill references a command/flag the CLI no longer exposes |

## Index

| Date | failure_tag | Title | Skill | File |
|---|---|---|---|---|
| — | — | (no distilled experiences yet — first entries come from run-20260723-eval Step 7, pending confirmation) | — | — |
