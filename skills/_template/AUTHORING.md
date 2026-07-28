# Authoring a shoplazza-cli skill

How to create a domain skill that conforms to [`SKILL.template.md`](./SKILL.template.md).
Design authority: [`docs/skill/master-plan.md`](../../docs/skill/master-plan.md) §5 and
[ADR-0002](../../docs/skill/adr/0002-common-base-skill-scope.md). Reference implementation:
[`../shoplazza-discounts/SKILL.md`](../shoplazza-discounts/SKILL.md).

## 0. Language policy (governs every skill)

**Instructional prose is English; user-facing triggers stay bilingual.**

- **English**: all directive/prose content — Overview, mechanics, flow, matrices, gotchas,
  recipes, references. English aligns with the CLI surface (`--help`, `schema`, error
  envelopes, command/flag names are all English), so the skill sits closest to the tool it
  describes and the L0 drift check compares like-for-like.
- **Bilingual (中文 + English), always**:
  1. the `description` frontmatter keywords — routing must fire on both "满200减20" and
     "spend $200 get $20 off";
  2. the **Trigger phrase → shortcut** table's "User says" column — real users type both.
- Do **not** write prose in Chinese, and do **not** drop the Chinese trigger/description
  keywords. Dropping the latter degrades routing for Chinese users.
- **Runtime reply language is owned by shoplazza-common** ("Reply in the user's language" —
  mirror the user, 中文提问 → 中文回答). Do not restate or contradict it in domain skills.

## 1. Create the file

```bash
cp skills/_template/SKILL.template.md skills/shoplazza-<domain>/SKILL.md
```

Fill every `{{placeholder}}`, then **delete the OPTIONAL section blocks you don't use**
(each is fenced by `<!-- OPTIONAL … -->` … `<!-- /OPTIONAL … -->`). The mandatory backbone
must stay.

## 2. Mandatory backbone (never omit)

Every skill has, in order:

1. Frontmatter (`name` + `description`).
2. The **`CRITICAL — 开始前先用 Read 读 ../shoplazza-common/SKILL.md`** prerequisite line.
3. `## Overview` — what the module does + which tiers apply.
4. `## 命令地图 (Command map)` — intent→command table, highest-fit tier first.
5. `## 权限 · Scope` — read/write → domain grant table.
6. `## Gotchas` — domain-specific only.
7. `## References` — pointer to `--help` / `schema` + shoplazza-common.

L0 (drift sentinel) asserts these headings exist — see §5.

## 3. Conditional sections by module type

Pick from the fixed menu so "conforms to the template" still holds. Include only what the
type needs; each is an OPTIONAL block in the template.

| Module type | Examples (in-scope) | Add these conditional sections |
|---|---|---|
| **action** (CRUD / do-a-thing) | discounts, products, orders, customers, billing, webhook | **Acting on a request** (trigger→shortcut + required-vs-ask matrix + never-ask list); **Boundaries** if it has cross-domain lookalikes; **Recipes** optional |
| **config** (broad, shallow read/set) | shop | **Boundaries** (config domains collide a lot — metafields/files/carriers); a light trigger table only if useful; **Recipes** optional. Usually **no** full ask-matrix |
| **workflow** (ordered, stateful) | app, themes — **deferred this round** | **Workflow** (ordered steps + state + data hand-off). No consumer yet; the section is dormant |

Rules of thumb:
- **Boundaries is added by any skill with cross-domain lookalikes**, regardless of type.
  Colliding skills **cross-declare in both directions** — in each other's `description`
  keywords *and* Boundaries tables. These are what the eval's routing collision cases
  ([eval-system-design §9](../../docs/skill/eval-system-design.md#9-mandatory-routing-collision-cases))
  test.
- Do **not** add Workflow to an action/config skill, and do not add the ask-matrix to a
  pure query domain.
- Single-file `SKILL.md` by default. Spill only large enum/schema detail to a sibling
  `references/` (e.g. `products` sub-resources), never the backbone.

## 4. Frontmatter field spec

| Field | Required | Value |
|---|---|---|
| `name` | **yes** | Must equal the directory name, `shoplazza-<domain>` (e.g. `shoplazza-discounts`). |
| `description` | **yes** | One line, the routing signal. Formula: **trigger keywords (中文 + English)** + **"当用户需要……时使用"** + **boundary keywords** for any cross-domain lookalike (both the concepts this domain owns and the ones it must not steal). This is the only thing a routing eval sees — keyword coverage here decides routing. |
| `version` | optional | Semver. **Whether the CLI's skill loader consumes it is UNCONFIRMED** — keep it commented unless you have confirmed the loader reads it. |
| `requires` | optional | e.g. `requires: { bins: [shoplazza] }`. Same UNCONFIRMED caveat as `version`. |

The existing `shoplazza-discounts` and `shoplazza-common` use **only `name` + `description`** —
match that convention unless you have a reason (and confirmation) to add the optional fields.

## 5. The anti-drift rule (most important)

**Never write a command, subcommand, shortcut, or flag you have not confirmed exists.**
The command surface is generated from `internal/registry/cli_meta_*.json`; regeneration can
add/rename/drop commands silently ("drift" — see [CONTEXT.md](../../docs/skill/CONTEXT.md)).

- Confirm every token against the real binary: `shoplazza <svc> --help`,
  `shoplazza <svc> <cmd> --help`, `shoplazza schema <svc>.<cmd>`.
- Do **not** memorize or copy flag lists into prose as if authoritative — the command map,
  shortcut list, and permission table are reconciled from `schema`/`--help`; point readers
  there for exact flags.
- Do not invent OAuth scope literals. Express permissions via the `--domain <module>` grant
  (confirmed) and `auth scopes`; only hardcode a scope literal you have verified.
- `--fields` exists only on **a few shortcuts** (verified: `products +search`, `customers +search`) —
  NOT on most commands. Verify with `<cmd> --help` before naming it. Use `--jq` for projection/
  filtering everywhere else. Both are documented in shoplazza-common.
- **When `cli_meta` is regenerated**, follow the maintenance checklist in
  [`eval-system-design.md` §13](../../docs/skill/eval-system-design.md#13-cli_meta-regeneration-maintenance-checklist)
  (run L0 — now incl. the inline `--data`/`--params` body-key check — re-run the eval, then diff the command surface).

## 6. How the skill is evaluated

Full spec: [`docs/skill/eval-system-design.md`](../../docs/skill/eval-system-design.md).

- **L0 — drift + structure** (cheap, every change): every command/flag the skill names must
  exist in `--help`/`schema`; the mandatory backbone headings above must be present. A miss
  fails CI with `file:line → unknown token`.
- **L1 — behavioral**: a Claude Code sub-agent gets `shoplazza-common` + your skill (behavior
  case) or all descriptions (routing case), emits a `shoplazza … --dry-run` command, and it
  is structurally checked (route hit / tier / flags / params / no fabricated value).
- **L2 — judge**: Claude scores the captured run against rubrics R-1…R-8 (routing, tier,
  param completeness, no fabricated money value, ask discipline, safety, output/jq,
  efficiency) + per-operation add-ons + the case's `semantic_rubric`.

Write your skill so these pass: keyword-rich `description`, highest-fit tier in the command
map, an honest required-vs-ask matrix (ask only for no-default fields; never for defaulted
ones), Boundaries that resolve the collision cases, and `--dry-run` guidance for
destructive/money ops.
