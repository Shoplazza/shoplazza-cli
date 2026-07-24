---
name: shoplazza-skill-eval
description: >-
  Evaluate the quality of shoplazza-cli skills (skill eval / 评估 / 测评 / drift check /
  回归). Runs the three-layer eval: L0 static drift+structure lint (bin/lint_drift.mjs),
  L1 behavioral (fresh sub-agent per case, --dry-run capture, structural checks), L2
  Claude-judge scoring against rubrics R-1…R-8. Use via /shoplazza-skill-eval [domains |
  <domain>.<operation> | --cases <file> | --show <run_id|latest> | --drift]. Never
  executes real writes — dry-run only, dev/stg stores only.
---

# shoplazza-skill-eval — three-layer skill evaluation

Spec authority: [`docs/skill/eval-system-design.md`](../../docs/skill/eval-system-design.md)
(§ references below point there). Vocabulary: `docs/skill/CONTEXT.md`. This file is the
orchestration flow the agent follows when `/shoplazza-skill-eval` is invoked.

**Hard safety rules (§11) — no exceptions:**
- Only `--dry-run` / `--help` / `schema` ever hit the CLI. Never a real write.
- Dev/stg store profiles only; never prod. If no dev-store profile is logged in
  (`shoplazza auth status`), skip live dry-run capture and fall back to
  command-string structural checks — say so in the summary.
- All run artifacts go under `skills/shoplazza-skill-eval/.tmp/run-<run_id>/`
  (gitignored). Never `/tmp`.

## Inputs you need before starting

- `<run_id>`: a timestamp string passed in at invocation (scripts cannot call
  `Date.now()`); e.g. `20260723-1430`. Ask if not derivable from context.
- The built binary at repo root: `./shoplazza`. If missing, stop and report.
- Record in the summary: agent-under-test model, judge model, git commit (§12.2).

## Step 0 — parse invocation mode (§4)

| Mode | Invocation | Scope |
|---|---|---|
| A 全量 | `/shoplazza-skill-eval` | all of `cases/` |
| B 指定域 | `/shoplazza-skill-eval products discounts` | `cases/<domain>/*.json` for each named domain + `cases/_routing/collisions.json` (always included) |
| C 指定 operation | `/shoplazza-skill-eval discounts.rebate` | cases whose `operation` matches |
| D 自定义文件 | `--cases path/to/x.json` | that file only |
| E 看历史 | `--show <run_id\|latest>` | no run — jump to Step 8 |
| L0-only | `--drift` | Step 2 only (cheap CI gate) |

Ambiguous → ask which mode. Modes A–C always include `cases/_routing/collisions.json` (§9).

## Step 1 — load cases + scope confirm 🛑

1. Load every in-scope `*.json` (each file is a JSON array of cases).
2. Validate fields against [`references/case_schema.md`](./references/case_schema.md);
   dedupe by `case_id` (later files lose; warn).
3. Print a scope block — skills involved, operations, case counts split
   routing/behavior, estimated time (~1–2 min per L1 case) — and **STOP for user
   confirmation** before running anything.

Append 3 lines (step, decision, artifact path) to `.tmp/run-<run_id>/log.md` after
this and every following step.

## Step 2 — L0 drift + structure lint (hard gate, §6)

For every skill in scope (each distinct `skill` field, plus `shoplazza-common` which
every behavior case injects):

```bash
node skills/shoplazza-skill-eval/bin/lint_drift.mjs skills/<skill>/SKILL.md --bin ./shoplazza
```

- Exit 0 = clean. Non-zero → findings print as `file:line → kind: token`.
- Write findings to `.tmp/run-<run_id>/l0_report.md`.
- **Hard gate**: any unknown command/flag/schema-ref or missing backbone heading
  STOPS the run here (a drifted skill makes every behavior case a false negative).
  Report the offending tokens; suggest fixes; do not proceed to L1 without an
  explicit user override.
- `--drift` mode: stop after this step regardless of result.

Notes: `lint_drift.mjs --backbone auto` skips the backbone check for
`shoplazza-common` (base skill, different skeleton). Bare-token mentions on lines
that read as negative documentation ("There is no `+update`") are skipped by a
negation heuristic; full invocations in fences/recipes are always strict.

## Step 3 — L1 behavioral (serial, fresh agent per case, §7)

For each case, in a stable order (routing collisions first, then per-domain files):

1. **Build the prompt** (never includes `expected`):
   ```bash
   node skills/shoplazza-skill-eval/bin/run_case.mjs prompt \
     --case <file> --id <case_id> --skills-dir skills > .tmp/run-<run_id>/cases/<case_id>.prompt.txt
   ```
   behavior → injects `shoplazza-common` + the target skill body; routing → injects
   the frontmatter `description` of every domain skill (none pre-loaded).
2. **Spawn a fresh agent-under-test** with that prompt — one of:
   - the `Agent` tool (preferred in-session), or
   - headless: `claude -p --model <pinned-model> --disallowedTools "*" < prompt`
     (run with cwd outside the repo so no project memory leaks in).
   The reply must follow the `SKILL:/COMMAND:/ASK:` contract embedded in the prompt.
   Save it to `.tmp/run-<run_id>/cases/<case_id>.reply.txt`.
3. **Capture + structural checks**:
   ```bash
   node skills/shoplazza-skill-eval/bin/run_case.mjs check \
     --case <file> --id <case_id> --reply <reply.txt> \
     --out .tmp/run-<run_id>/cases/<case_id>.json [--exec]
   ```
   `--exec` (only when a dev/stg profile is logged in) runs each captured command
   with `--dry-run` appended if the agent omitted it (recording that omission — an
   R-6 signal) and stores the would-be request JSON. Without auth, the artifact
   carries the command string only.
4. Flake handling: empty reply or transient error → retry once, then mark FAIL with
   `execution_error`. Print a progress line every 5 cases.

## Step 4 — L2 judge (per-case, serial, §8)

`Read` [`references/rubrics.md`](./references/rubrics.md) once. For each L1 artifact:
score R-1…R-8 (1–5 + ≤50-char reason), the per-operation add-on, and each
`semantic_rubric` statement (P/F). `layer2_pass` = all rubrics ≥4 AND all semantic P.
Write `.tmp/run-<run_id>/cases/<case_id>.judge.json` in the shape rubrics.md defines,
with `critical_issues` tagged P0/P1/P2. Judge ONLY from the artifact + expected +
rubric — never from what the agent "probably meant".

## Step 5 — summary + coverage matrix

Write `.tmp/run-<run_id>/summary.md`:
- scope, models, commit; per-layer pass rates;
- **coverage matrix** — domain × operation × (routing|behavior), cell = pass/total;
- failure details grouped P0 → P1 → P2, each with case_id, rubric, evidence;
- fix-priority list (which skill file, which section, suggested change);
- R-8 efficiency notes — always listed, even for passing cases.

## Step 6 — fix → regression 🛑 (only if ≥1 FAIL)

Per failing case: `Read` the target skill, state root cause + the file + the exact
change, **STOP for user confirmation**. On yes: `Edit` the skill (repo comment/style
norms apply), then re-run ONLY the failed cases (Step 3–4), max 2 rounds. Never edit
`expected` to make a case pass — a bad case is fixed as a case-authoring change and
called out as such.

## Step 7 — experience distillation 🛑

Read [`experiences/INDEX.md`](./experiences/INDEX.md) only (not every note). Match
each distinct failure by `failure_tag`. New mode → draft
`experiences/<YYYYMMDD>-<failure_tag>-<title>.md`; **STOP for confirmation**; on yes
write it and append one line to INDEX.md.

## Step 8 — report + cleanup

Print: run_id, artifact path, per-layer pass rates, P0/P1/P2 counts, new experience
files, and (mode E) the stored summary. `--show latest` = highest-sorting run dir
under `.tmp/`.

## Repository layout (§2)

```
bin/lint_drift.mjs        L0 sentinel (runnable; exit≠0 on drift)
bin/extract_commands.mjs  shared tokenizer (markdown → command/flag refs)
bin/cli_surface.mjs       shared cached CLI surface (--help/schema existence)
bin/run_case.mjs          L1 helper: `prompt` builder + `check` structural checker
references/               case_schema.md · rubrics.md · how_to_write_cases.md
cases/_routing/collisions.json   mandatory §9 cross-domain cases (every run)
cases/<domain>/*.json     per-domain behavior + routing cases
experiences/INDEX.md      failure-tag enum + distilled-note index
.tmp/run-<run_id>/        gitignored artifacts (prompts, replies, *.json, summary)
```
