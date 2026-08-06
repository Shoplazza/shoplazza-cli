---
name: shoplazza-{{domain}}
description: >-
  {{One line. Trigger keywords (中文 + English) · "Use when the user wants to …" ·
  boundary keywords for any cross-domain lookalikes this domain owns or is confused with.
  This field is what routing evals target — see AUTHORING.md → Frontmatter. Keep the
  keywords bilingual so both Chinese and English utterances route here.}}
# version: 0.1.0          # OPTIONAL (semver). Loader consumption UNCONFIRMED — see AUTHORING.md.
# requires:               # OPTIONAL
#   bins: [shoplazza]
---

# shoplazza CLI — {{domain}} module

**CRITICAL — before anything else, use Read on [`../shoplazza-common/SKILL.md`](../shoplazza-common/SKILL.md).**
It owns every cross-cutting mechanic: authentication / profiles / the three access tiers /
the output envelope (`.data`) / `--dry-run` / `--jq` / `--fields` / `schema` / `api rest` /
update notice / the safety protocol. This file covers only the {{domain}} domain and never
repeats the above.

## Overview

{{One to three sentences: what this module manages.}}
All three access tiers apply (tier-selection rule is in shoplazza-common — always pick the
highest tier that expresses the operation).
{{Note any missing shortcut, e.g. "There is no `+update` — use the spec leaf." Delete if N/A.}}

## Command map

Intent → command, highest-fit tier first. The authoritative flags/params live in
`--help`/`schema`, not this table.

| User intent | Command |
|-------------|---------|
| {{intent}} | `{{svc}} +{{shortcut}} --{{flag}} …` |
| {{intent}} | `{{svc}} {{leaf}} --params '{…}' [--data '{…}']` |
| {{intent (only if neither tier fits)}} | `api rest {{METHOD}} /openapi/…` |

<!-- ============================================================
OPTIONAL — Acting on a request  (ACTION-type skills)
Include for CRUD/action domains. Delete wholesale for query/config-only skills.
============================================================ -->
## Acting on a request

"Create / build / cancel / update X" is an **action**, not a question. Flow:

1. Match intent to a shortcut via the trigger table.
2. Check the **required (no-default) flags** against the user's own words. Missing or ambiguous
   → **ask with `AskUserQuestion`, never fabricate** (especially money-spending values).
3. **Never ask about a flag that has a CLI default** (`(default …)` / auto-generated /
   omit-to-disable) — let the default win.
4. All required present → execute. `--dry-run` first for destructive / batch / money ops (see
   shoplazza-common → Safety protocol).

### Trigger phrase → shortcut
Keep this column bilingual — real users type both Chinese and English.

| User says | Shortcut | How to extract values |
|---|---|---|
| {{中文短语 / English phrase}} | `+{{shortcut}}` | {{which flag takes which value}} |

### Required-vs-ask matrix
Only **no-default** flags are askable.

| Shortcut | Must ASK if unspecified | Infer if possible | Default silently |
|---|---|---|---|
| `+{{shortcut}}` | {{no-default flags}} | {{inferable-from-wording}} | {{defaulted / omit-to-disable flags}} |

### Never-ask list
Flags with a default or omit-to-disable — they never appear as a question:
{{`--start` (default now) · `--end` (default forever) · … list each}}.
<!-- /OPTIONAL Acting on a request -->

<!-- ============================================================
OPTIONAL — Workflow  (WORKFLOW-type skills only)
No in-scope skill uses this round (app / themes deferred — master-plan §5). Delete for action/config skills.
============================================================ -->
## Workflow

Ordered, stateful, multi-step flow. For each step note the command, the state it produces, and
the data it hands to the next step.

1. {{step}} → produces `{{state}}` (extract with `--jq '.data.…'`)
2. {{step}} → consumes `{{state}}`, produces `{{state}}`
3. …
<!-- /OPTIONAL Workflow -->

<!-- ============================================================
OPTIONAL — Boundaries  (any skill with cross-domain lookalikes)
Cross-declare on BOTH sides: in each colliding skill's description keywords AND Boundaries table.
Maps to the routing collision cases in eval-system-design §9.
============================================================ -->
## Boundaries

Concepts that read like {{domain}} but belong elsewhere (and this domain's own lookalikes
others might steal):

| Sounds like {{domain}} | Actually belongs to | Command |
|---|---|---|
| {{lookalike concept}} | `shoplazza-{{other}}` | `{{other}} {{cmd}}` |
<!-- /OPTIONAL Boundaries -->

## Permissions · Scope

Authorization is by **domain** (the unit of `--domain`), not per command. Authorization flow is
in shoplazza-common → Authentication.

| Operation | Needs | Grant |
|---|---|---|
| read ({{get/list/search}}) | `{{domain}}` read scope | `auth login --domain {{domain}}` |
| write ({{create/update/delete}}) | `{{domain}}` write scope | `auth login --domain {{domain}}` |

Look up exact scope literals with `shoplazza auth scopes`; `--domain {{domain}}` expands into them.

## Gotchas

Only **domain-specific** pitfalls (generic ones — `.data` prefix / `-r` / `--dry-run` — are in
shoplazza-common).

| Symptom | Cause | Fix |
|---|---|---|
| {{symptom}} | {{cause}} | {{fix}} |

<!-- OPTIONAL — Recipes (either type). Delete this block if none. -->
## Recipes

```bash
# {{what it does}}
{{svc}} +{{shortcut}} …
```
<!-- /OPTIONAL Recipes -->

## References

- Per-command flags: `shoplazza {{svc}} <cmd> --help`
- Spec-leaf params / body / response: `shoplazza schema {{svc}}.<cmd>`
- Cross-cutting mechanics: `../shoplazza-common/SKILL.md`
{{- optional: shortcut source `shortcuts/{{svc}}/*.go` }}
