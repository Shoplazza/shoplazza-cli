# How to write cases for a new skill

Add cases the moment a domain skill lands (master-plan §7 — the eval gates each wave).
Field reference: [`case_schema.md`](./case_schema.md). Rubrics: [`rubrics.md`](./rubrics.md).

## 1. Minimum suite per domain skill

| Slot | Count | What it proves |
|---|---|---|
| Happy-path behavior, one per main shortcut | 1 × shortcut | The trigger table + required flags work end-to-end |
| Lifecycle behavior (cancel / delete / update) | ≥2 | Leaf usage, body shapes, ordering rules (cancel-before-delete) |
| **Should-ask** (required no-default field missing) | ≥1 | Ask discipline — the agent asks instead of guessing |
| **No-fabricated-value** (money value missing, tempting to invent) | ≥1 | R-4 — the agent never invents money numbers |
| Domain routing (utterance → this skill) | ≥2 (1 中文, 1 English) | The `description` keywords fire |
| Collision routing (cross-domain lookalikes) | per Boundaries row | Add to `cases/_routing/collisions.json`, asserting BOTH directions |

## 2. Workflow

1. **Pick the utterance first.** Write what a real merchant would type, before looking
   at flags — otherwise you write CLI-shaped inputs that only test echoing.
2. **Verify the golden command against the binary** (`--help` / `schema`) and copy the
   verified tokens into `expected`. Never from memory, never from the skill prose
   (the skill itself might have drifted — that's what you're testing).
3. Decide `must_ask` honestly: check the shortcut's `--help` for `(default …)` /
   auto-generated / omit-to-disable. Only a **required, no-default** gap justifies
   `must_ask: true`.
4. Add `flags_must_not_contain` guards for the values the agent might fabricate, and
   for defaulted flags it should leave alone.
5. Write 2–4 `semantic_rubric` statements: value-traceability ("tiers 200:20 taken
   verbatim from utterance"), default handling ("永不过期 → --end forever"), and
   negative expectations ("does not ask about caps").
6. Run the single case before committing it:
   run the eval (see `SKILL.eval.md`) with `--cases cases/<domain>/<file>.json` — a case
   that fails on a known-good skill is a bad case, fix the case.

## 3. Anti-patterns

- **Echo cases** — utterance already contains the full flag syntax. Tests nothing.
- **Over-pinned flags** — asserting `--name` / `--code` / order of flags. Assert only
  what correctness requires.
- **Ambiguous must_ask** — if a reasonable agent could infer the value from wording,
  it is `Infer`, not `ASK`; put the inference in `semantic_rubric` instead.
- **Un-verified expected** — the #1 way to poison the suite. Verify, then write.
- **Collision cases in only one direction** — when domain A must not steal from B,
  also assert B does not steal from A (cross-declare, like the skills' Boundaries).
